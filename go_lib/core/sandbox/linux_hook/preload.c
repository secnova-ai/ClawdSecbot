#define _GNU_SOURCE

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <strings.h>
#include <stdarg.h>
#include <dlfcn.h>
#include <errno.h>
#include <fcntl.h>
#include <unistd.h>
#include <limits.h>
#include <sys/stat.h>
#include <sys/uio.h>
#include <sys/types.h>
#include <sys/socket.h>
#include <dirent.h>
#include <netinet/in.h>
#include <arpa/inet.h>
#include <netdb.h>
#include <ifaddrs.h>
#include <net/if.h>

// 沙箱策略结构 - 支持黑名单/白名单模式、域名拦截
typedef struct {
    int log_only;

    int file_policy_whitelist;
    char **blocked_paths;
    size_t blocked_paths_count;
    char **allowed_paths;
    size_t allowed_paths_count;

    int network_policy_whitelist;
    char **blocked_ips;
    size_t blocked_ips_count;
    char **allowed_ips;
    size_t allowed_ips_count;
    char **blocked_domains;
    size_t blocked_domains_count;
    char **allowed_domains;
    size_t allowed_domains_count;

    int command_policy_whitelist;
    char **blocked_cmds;
    size_t blocked_cmds_count;
    char **allowed_cmds;
    size_t allowed_cmds_count;
} sandbox_policy_t;

static sandbox_policy_t g_policy;
static int g_policy_loaded = 0;
static __thread int g_in_interceptor = 0;

// 白名单模式自动放行的自身路径
static char g_policy_file_path[PATH_MAX] = {0};
static char g_gateway_binary_path[PATH_MAX] = {0};
static char g_gateway_config_path[PATH_MAX] = {0};

// 本机网卡 IP 列表（网络白名单自动放行）
#define MAX_LOCAL_IPS 64
static char g_local_ips[MAX_LOCAL_IPS][INET6_ADDRSTRLEN];
static size_t g_local_ip_count = 0;

// 前置声明
static void normalize_policy_paths(char **paths, size_t count);
static int is_path_blocked(const char *path);
static void clean_path(char *buf, size_t buf_size, const char *path);
static void collect_local_ips(void);

// ---------------- 日志输出 ----------------

// 输出沙箱运行信息日志，支持 printf 格式化
static void log_info(const char *fmt, ...) {
    if (!fmt) return;

    char message[4096] = {0};
    va_list ap;
    va_start(ap, fmt);
    vsnprintf(message, sizeof(message), fmt, ap);
    va_end(ap);

    fprintf(stderr, "[ClawdSecbot] %s\n", message);
    fflush(stderr);
}

// 输出沙箱拦截事件日志(action=BLOCK/LOG_ONLY, type=FILE/NET/DNS/CMD)
static void log_event(const char *action, const char *type, const char *target) {
    log_info("ACTION=%s TYPE=%s TARGET=%s",
             action ? action : "UNKNOWN",
             type ? type : "",
             target ? target : "");
}

// ---------------- 工具函数 ----------------

// 释放字符串数组及其中每个元素的内存
static void free_string_array(char **arr, size_t count) {
    if (!arr) return;
    for (size_t i = 0; i < count; ++i) {
        free(arr[i]);
    }
    free(arr);
}

// 安全版 strndup，拷贝最多 n 个字节并追加 '\0'
static char *strndup_safe(const char *s, size_t n) {
    char *p = (char *)malloc(n + 1);
    if (!p) return NULL;
    memcpy(p, s, n);
    p[n] = '\0';
    return p;
}

// 在 JSON buffer 中查找键对应的值起始位置
// key 参数不含引号 (如 "blocked_paths"), 函数会检查 JSON 中的 "key": value 格式
static const char *find_key_start(const char *buf, const char *key) {
    if (!buf || !key) return NULL;

    size_t key_len = strlen(key);
    const char *p = buf;
    while ((p = strstr(p, key)) != NULL) {
        if (p > buf && p[-1] == '"' && p[key_len] == '"') {
            const char *colon = strchr(p + key_len + 1, ':');
            if (!colon) { p += key_len; continue; }
            colon++;
            while (*colon == ' ' || *colon == '\t' || *colon == '\n' || *colon == '\r') {
                colon++;
            }
            return colon;
        }
        p += key_len;
    }
    return NULL;
}

// 纯内存路径规范化: 处理 ./../重复斜杠/尾部斜杠, 不依赖 realpath()
static void clean_path(char *buf, size_t buf_size, const char *path) {
    if (!buf || buf_size == 0 || !path || path[0] == '\0') {
        if (buf && buf_size > 0) buf[0] = '\0';
        return;
    }

    char tmp[PATH_MAX] = {0};
    const char *src = path;
    int absolute = (src[0] == '/');

    const char *segments[PATH_MAX / 2];
    size_t seg_count = 0;

    while (*src) {
        while (*src == '/') src++;
        if (*src == '\0') break;

        const char *seg_start = src;
        while (*src && *src != '/') src++;
        size_t seg_len = (size_t)(src - seg_start);

        if (seg_len == 1 && seg_start[0] == '.') {
            continue;
        }
        if (seg_len == 2 && seg_start[0] == '.' && seg_start[1] == '.') {
            if (seg_count > 0 && !(segments[seg_count - 1] == seg_start)) {
                seg_count--;
            }
            continue;
        }
        segments[seg_count++] = seg_start;
    }

    char *dst = tmp;
    char *end = tmp + sizeof(tmp) - 1;

    if (absolute) {
        *dst++ = '/';
    }

    for (size_t i = 0; i < seg_count && dst < end; ++i) {
        if (i > 0 && dst < end) *dst++ = '/';
        const char *s = segments[i];
        while (*s && *s != '/' && dst < end) {
            *dst++ = *s++;
        }
    }
    *dst = '\0';

    if (tmp[0] == '\0') {
        snprintf(buf, buf_size, "%s", absolute ? "/" : ".");
    } else {
        snprintf(buf, buf_size, "%s", tmp);
    }
}

// 解析 JSON 中的布尔字段 (true/false)
static int parse_bool_field(const char *buf, const char *key, int *out) {
    const char *v = find_key_start(buf, key);
    if (!v || !out) return -1;
    if (strncmp(v, "true", 4) == 0) { *out = 1; return 0; }
    if (strncmp(v, "false", 5) == 0) { *out = 0; return 0; }
    return -1;
}

// 解析策略类型字段，返回 1 表示白名单，0 表示黑名单(默认)
static int parse_policy_type(const char *buf, const char *key) {
    const char *v = find_key_start(buf, key);
    if (!v || *v != '"') return 0;
    return (strncmp(v + 1, "whitelist", 9) == 0) ? 1 : 0;
}

// 解析 JSON 中的字符串数组字段 (如 "key": ["a", "b"])
static int parse_string_array(const char *buf, const char *key, char ***out_arr, size_t *out_len) {
    if (!out_arr || !out_len) return -1;
    *out_arr = NULL;
    *out_len = 0U;

    const char *v = find_key_start(buf, key);
    if (!v) return -1;

    const char *start = strchr(v, '[');
    if (!start) return -1;
    const char *end = strchr(start, ']');
    if (!end || end <= start) return -1;

    size_t count = 0;
    const char *p = start;
    while (p < end) {
        const char *q = strchr(p, '"');
        if (!q || q >= end) break;
        const char *r = strchr(q + 1, '"');
        if (!r || r >= end) break;
        count++;
        p = r + 1;
    }

    if (count == 0) return 0;

    char **arr = (char **)calloc(count, sizeof(char *));
    if (!arr) return -1;

    size_t idx = 0;
    p = start;
    while (p < end && idx < count) {
        const char *q = strchr(p, '"');
        if (!q || q >= end) break;
        const char *r = strchr(q + 1, '"');
        if (!r || r >= end) break;

        size_t len = (size_t)(r - (q + 1));
        arr[idx] = strndup_safe(q + 1, len);
        if (!arr[idx]) {
            free_string_array(arr, idx);
            return -1;
        }
        idx++;
        p = r + 1;
    }

    *out_arr = arr;
    *out_len = idx;
    return 0;
}

// 解析 JSON 中的单值字符串字段 (如 "key": "value"), 写入 out 缓冲区
static int parse_string_field(const char *buf, const char *key, char *out, size_t out_size) {
    if (!out || out_size == 0) return -1;
    out[0] = '\0';
    const char *v = find_key_start(buf, key);
    if (!v || *v != '"') return -1;
    const char *start = v + 1;
    const char *end = strchr(start, '"');
    if (!end) return -1;
    size_t len = (size_t)(end - start);
    if (len >= out_size) len = out_size - 1;
    memcpy(out, start, len);
    out[len] = '\0';
    return 0;
}

// ---------------- 策略加载 ----------------

// 从指定路径的 JSON 文件加载沙箱策略到全局结构体
static void load_policy_from_file(const char *path) {
    FILE *f = fopen(path, "r");
    if (!f) {
        log_info("policy file not found, sandbox disabled");
        return;
    }

    if (fseek(f, 0, SEEK_END) != 0) { fclose(f); return; }
    long size = ftell(f);
    if (size <= 0) { fclose(f); return; }
    if (fseek(f, 0, SEEK_SET) != 0) { fclose(f); return; }

    char *buf = (char *)malloc((size_t)size + 1);
    if (!buf) { fclose(f); return; }

    size_t n = fread(buf, 1, (size_t)size, f);
    fclose(f);
    buf[n] = '\0';

    memset(&g_policy, 0, sizeof(g_policy));

    int log_only = 0;
    if (parse_bool_field(buf, "log_only", &log_only) == 0) {
        g_policy.log_only = log_only;
    }

    g_policy.file_policy_whitelist = parse_policy_type(buf, "file_policy_type");
    g_policy.network_policy_whitelist = parse_policy_type(buf, "network_policy_type");
    g_policy.command_policy_whitelist = parse_policy_type(buf, "command_policy_type");

    parse_string_array(buf, "blocked_paths", &g_policy.blocked_paths, &g_policy.blocked_paths_count);
    parse_string_array(buf, "allowed_paths", &g_policy.allowed_paths, &g_policy.allowed_paths_count);
    parse_string_array(buf, "blocked_ips", &g_policy.blocked_ips, &g_policy.blocked_ips_count);
    parse_string_array(buf, "allowed_ips", &g_policy.allowed_ips, &g_policy.allowed_ips_count);
    parse_string_array(buf, "blocked_domains", &g_policy.blocked_domains, &g_policy.blocked_domains_count);
    parse_string_array(buf, "allowed_domains", &g_policy.allowed_domains, &g_policy.allowed_domains_count);
    parse_string_array(buf, "blocked_commands", &g_policy.blocked_cmds, &g_policy.blocked_cmds_count);
    parse_string_array(buf, "allowed_commands", &g_policy.allowed_cmds, &g_policy.allowed_cmds_count);

    parse_string_field(buf, "gateway_binary_path", g_gateway_binary_path, sizeof(g_gateway_binary_path));
    parse_string_field(buf, "gateway_config_path", g_gateway_config_path, sizeof(g_gateway_config_path));

    normalize_policy_paths(g_policy.blocked_paths, g_policy.blocked_paths_count);
    normalize_policy_paths(g_policy.allowed_paths, g_policy.allowed_paths_count);

    g_policy_loaded = 1;
    free(buf);

    log_info("policy loaded: file=%s(%zu/%zu) net=%s(%zu/%zu) domain=(%zu/%zu) cmd=%s(%zu/%zu) log_only=%d",
             g_policy.file_policy_whitelist ? "whitelist" : "blacklist",
             g_policy.blocked_paths_count, g_policy.allowed_paths_count,
             g_policy.network_policy_whitelist ? "whitelist" : "blacklist",
             g_policy.blocked_ips_count, g_policy.allowed_ips_count,
             g_policy.blocked_domains_count, g_policy.allowed_domains_count,
             g_policy.command_policy_whitelist ? "whitelist" : "blacklist",
             g_policy.blocked_cmds_count, g_policy.allowed_cmds_count,
             g_policy.log_only);
}

// ---------------- 策略匹配 ----------------

// 检查 path 是否以 prefix 开头
static int has_prefix(const char *path, const char *prefix) {
    if (!path || !prefix) return 0;
    size_t n = strlen(prefix);
    if (strlen(path) < n) return 0;
    return strncmp(path, prefix, n) == 0;
}

// 去除路径末尾多余的 '/'，保留根路径 "/"
static void trim_trailing_slash(char *path) {
    if (!path) return;
    size_t len = strlen(path);
    while (len > 1 && path[len - 1] == '/') {
        path[len - 1] = '\0';
        len--;
    }
}

// 去掉路径两端对称的引号，例如 "~/a.txt" -> ~/a.txt
static const char *strip_wrapping_quotes(const char *path, char *out, size_t out_size) {
    if (!path || !out || out_size == 0) return path;
    out[0] = '\0';

    size_t len = strlen(path);
    if (len >= 2) {
        char first = path[0];
        char last = path[len - 1];
        if ((first == '"' && last == '"') || (first == '\'' && last == '\'')) {
            size_t content_len = len - 2;
            if (content_len >= out_size) content_len = out_size - 1;
            memcpy(out, path + 1, content_len);
            out[content_len] = '\0';
            return out;
        }
    }

    snprintf(out, out_size, "%s", path);
    return out;
}

// 展开 HOME 目录前缀，支持 "~/" 和 "$HOME/" 两种写法
static const char *expand_home_prefix(const char *path, char *out, size_t out_size) {
    if (!path || !out || out_size == 0) return path;
    out[0] = '\0';

    const char *home = getenv("HOME");
    if (!home || home[0] == '\0') {
        snprintf(out, out_size, "%s", path);
        return out;
    }

    if (strncmp(path, "~/", 2) == 0) {
        snprintf(out, out_size, "%s/%s", home, path + 2);
        return out;
    }
    if (strncmp(path, "$HOME/", 6) == 0) {
        snprintf(out, out_size, "%s/%s", home, path + 6);
        return out;
    }

    snprintf(out, out_size, "%s", path);
    return out;
}

// 归一化单个策略路径：去引号 -> 展开 HOME -> clean_path -> realpath (若存在)
static void normalize_policy_path(char **path_ptr) {
    if (!path_ptr || !(*path_ptr)) return;

    char *raw = *path_ptr;
    char unquoted[PATH_MAX] = {0};
    char expanded[PATH_MAX] = {0};
    const char *without_quotes = strip_wrapping_quotes(raw, unquoted, sizeof(unquoted));
    const char *candidate = expand_home_prefix(without_quotes, expanded, sizeof(expanded));
    if (!candidate || candidate[0] == '\0') return;

    char cleaned[PATH_MAX] = {0};
    clean_path(cleaned, sizeof(cleaned), candidate);
    if (cleaned[0] == '\0') return;

    char resolved[PATH_MAX] = {0};
    const char *final_path = cleaned;
    if (realpath(cleaned, resolved) != NULL) {
        trim_trailing_slash(resolved);
        final_path = resolved;
    }

    char *norm = strdup(final_path);
    if (norm) {
        free(raw);
        *path_ptr = norm;
    }
}

// 批量归一化策略路径数组，避免配置路径与访问路径归一化规则不一致导致漏拦截
static void normalize_policy_paths(char **paths, size_t count) {
    if (!paths || count == 0) return;
    for (size_t i = 0; i < count; ++i) {
        normalize_policy_path(&paths[i]);
    }
}

// Build an absolute path from pathname and dirfd (for open/openat relative-path checks).
// Returns 0 on success; negative on failure.
static int build_absolute_path(const char *pathname, int dirfd, char *out, size_t out_size) {
    if (!pathname || !out || out_size == 0) return -1;
    out[0] = '\0';

    if (pathname[0] == '/') {
        snprintf(out, out_size, "%s", pathname);
        return 0;
    }

    char base[PATH_MAX] = {0};
    if (dirfd == AT_FDCWD) {
        if (!getcwd(base, sizeof(base))) return -1;
    } else {
        char procfd[64] = {0};
        snprintf(procfd, sizeof(procfd), "/proc/self/fd/%d", dirfd);
        ssize_t n = readlink(procfd, base, sizeof(base) - 1);
        if (n <= 0) return -1;
        base[n] = '\0';
    }

    if (snprintf(out, out_size, "%s/%s", base, pathname) >= (int)out_size) return -1;
    return 0;
}

// 标准化访问路径用于策略匹配: 去引号 -> 展开 HOME -> 转绝对路径 -> clean_path -> realpath
static const char *normalize_path_for_policy(const char *pathname, int dirfd, char *buf, size_t buf_size) {
    if (!pathname || !buf || buf_size == 0) return pathname;

    char unquoted[PATH_MAX] = {0};
    char expanded[PATH_MAX] = {0};
    const char *without_quotes = strip_wrapping_quotes(pathname, unquoted, sizeof(unquoted));
    const char *prepared = expand_home_prefix(without_quotes, expanded, sizeof(expanded));
    if (!prepared || prepared[0] == '\0') {
        prepared = pathname;
    }

    char absolute[PATH_MAX] = {0};
    const char *candidate = prepared;

    if (build_absolute_path(prepared, dirfd, absolute, sizeof(absolute)) == 0) {
        candidate = absolute;
    }

    char cleaned[PATH_MAX] = {0};
    clean_path(cleaned, sizeof(cleaned), candidate);

    if (realpath(cleaned, buf) != NULL) {
        trim_trailing_slash(buf);
        return buf;
    }

    snprintf(buf, buf_size, "%s", cleaned);
    trim_trailing_slash(buf);
    return buf;
}

// 路径边界匹配：
// prefix="/a/b" 仅匹配 "/a/b" 或 "/a/b/..."，不匹配 "/a/b2"
static int match_path_with_boundary(const char *path, const char *prefix) {
    if (!path || !prefix) return 0;
    size_t plen = strlen(prefix);
    if (plen == 0) return 0;
    if (strncmp(path, prefix, plen) != 0) return 0;

    if (path[plen] == '\0') return 1;
    if (prefix[plen - 1] == '/') return 1;
    return path[plen] == '/';
}

// Resolve an fd to absolute path via /proc/self/fd/<fd>.
// Returns 0 on success; negative on failure.
static int resolve_fd_path(int fd, char *out, size_t out_size) {
    if (!out || out_size == 0) return -1;
    out[0] = '\0';
    if (fd < 0) return -1;

    char procfd[64] = {0};
    snprintf(procfd, sizeof(procfd), "/proc/self/fd/%d", fd);
    ssize_t n = readlink(procfd, out, out_size - 1);
    if (n <= 0) return -1;
    out[n] = '\0';
    return 0;
}

// Unified file path policy enforcement.
// Returns 1 when blocked (and errno set unless log_only), 0 when allowed.
static int enforce_path_policy(const char *type, const char *path) {
    if (!path) return 0;
    if (!is_path_blocked(path)) return 0;

    log_event(g_policy.log_only ? "LOG_ONLY" : "BLOCK", type ? type : "FILE", path);
    if (!g_policy.log_only) {
        errno = EACCES;
        return 1;
    }
    return 0;
}

// 路径操作检查位图，顺序固定为:
// 目录打开 -> 创建 -> 读取 -> 重命名 -> 写入 -> 删除
#define PATH_OP_DIR_OPEN (1U << 0)
#define PATH_OP_CREATE   (1U << 1)
#define PATH_OP_READ     (1U << 2)
#define PATH_OP_RENAME   (1U << 3)
#define PATH_OP_WRITE    (1U << 4)
#define PATH_OP_DELETE   (1U << 5)

// 按统一顺序执行路径检查，确保跨平台语义一致。
static int enforce_path_policy_by_mask(const char *path, unsigned int op_mask) {
    if (!path) return 0;
    if (op_mask & PATH_OP_DIR_OPEN) {
        if (enforce_path_policy("PATH-DIR-OPEN", path)) return 1;
    }
    if (op_mask & PATH_OP_CREATE) {
        if (enforce_path_policy("PATH-CREATE", path)) return 1;
    }
    if (op_mask & PATH_OP_READ) {
        if (enforce_path_policy("PATH-READ", path)) return 1;
    }
    if (op_mask & PATH_OP_RENAME) {
        if (enforce_path_policy("PATH-RENAME", path)) return 1;
    }
    if (op_mask & PATH_OP_WRITE) {
        if (enforce_path_policy("PATH-WRITE", path)) return 1;
    }
    if (op_mask & PATH_OP_DELETE) {
        if (enforce_path_policy("PATH-DELETE", path)) return 1;
    }
    return 0;
}

// 将 open/openat flags 映射为路径操作位图。
static unsigned int build_open_op_mask(int flags) {
    unsigned int op_mask = 0;
    int acc_mode = flags & O_ACCMODE;

    if (flags & O_DIRECTORY) {
        op_mask |= PATH_OP_DIR_OPEN;
    }
    if ((flags & O_CREAT) || (flags & O_EXCL)
#ifdef O_TMPFILE
        || (flags & O_TMPFILE)
#endif
    ) {
        op_mask |= PATH_OP_CREATE;
    }
    if (acc_mode == O_RDONLY || acc_mode == O_RDWR) {
        op_mask |= PATH_OP_READ;
    }
    if (acc_mode == O_WRONLY || acc_mode == O_RDWR || (flags & O_TRUNC) || (flags & O_APPEND)) {
        op_mask |= PATH_OP_WRITE;
    }
    return op_mask;
}

// 将 stdio mode 映射为路径操作位图。
static unsigned int build_stdio_mode_op_mask(const char *mode, unsigned int fallback_mask) {
    if (!mode || mode[0] == '\0') {
        return fallback_mask;
    }
    unsigned int op_mask = 0;
    switch (mode[0]) {
        case 'r':
            op_mask |= PATH_OP_READ;
            break;
        case 'w':
        case 'a':
            op_mask |= PATH_OP_CREATE | PATH_OP_WRITE;
            break;
        default:
            op_mask |= fallback_mask;
            break;
    }
    if (strchr(mode, '+') != NULL) {
        op_mask |= PATH_OP_READ | PATH_OP_WRITE;
    }
    return op_mask;
}

// Unified fd-based policy enforcement for read/write style syscalls.
// Returns 1 when blocked (and errno set unless log_only), 0 when allowed.
// Skips stdin/stdout/stderr, sandbox log fd, and reentrant calls to prevent
// infinite recursion (log_event -> fprintf -> write -> enforce_fd_policy -> ...).
static int enforce_fd_policy(const char *type, int fd) {
    if (fd <= STDERR_FILENO || g_in_interceptor) return 0;

    g_in_interceptor = 1;

    char fd_path[PATH_MAX] = {0};
    if (resolve_fd_path(fd, fd_path, sizeof(fd_path)) != 0) {
        g_in_interceptor = 0;
        return 0;
    }

    char norm_path[PATH_MAX] = {0};
    const char *path_for_check = normalize_path_for_policy(fd_path, AT_FDCWD, norm_path, sizeof(norm_path));
    int result = enforce_path_policy(type, path_for_check);

    g_in_interceptor = 0;
    return result;
}

// 白名单模式下始终允许的系统路径前缀
static int is_system_path(const char *path) {
    static const char *prefixes[] = {
        "/lib/", "/lib64/", "/usr/lib/", "/usr/lib64/",
        "/proc/", "/dev/", "/sys/",
        "/etc/ld.so", "/etc/resolv.conf", "/etc/nsswitch.conf",
        "/etc/hosts", "/etc/ssl/", "/etc/ca-certificates/",
        "/tmp/", "/run/",
        NULL
    };
    for (int i = 0; prefixes[i]; ++i) {
        if (has_prefix(path, prefixes[i])) return 1;
    }
    return 0;
}

// 白名单模式下自动放行沙箱自身相关路径（策略文件、日志、网关二进制/配置所在目录）
static int is_self_path(const char *path) {
    if (!path) return 0;
    static const char *self_ptrs[] = {NULL, NULL, NULL};
    self_ptrs[0] = g_policy_file_path;
    self_ptrs[1] = g_gateway_binary_path;
    self_ptrs[2] = g_gateway_config_path;

    for (int i = 0; i < 3; ++i) {
        if (!self_ptrs[i] || self_ptrs[i][0] == '\0') continue;
        if (match_path_with_boundary(path, self_ptrs[i])) return 1;

        char dir[PATH_MAX] = {0};
        snprintf(dir, sizeof(dir), "%s", self_ptrs[i]);
        char *slash = strrchr(dir, '/');
        if (slash && slash != dir) {
            *slash = '\0';
            if (match_path_with_boundary(path, dir)) return 1;
        }
    }
    return 0;
}

// 判断文件路径是否被策略拦截 (支持黑名单/白名单模式)
static int is_path_blocked(const char *path) {
    if (!g_policy_loaded || !path) return 0;

    if (g_policy.file_policy_whitelist) {
        if (g_policy.allowed_paths_count == 0) return 0;
        if (is_system_path(path)) return 0;
        if (is_self_path(path)) return 0;
        for (size_t i = 0; i < g_policy.allowed_paths_count; ++i) {
            if (match_path_with_boundary(path, g_policy.allowed_paths[i])) return 0;
        }
        return 1;
    }

    for (size_t i = 0; i < g_policy.blocked_paths_count; ++i) {
        if (match_path_with_boundary(path, g_policy.blocked_paths[i])) return 1;
    }
    return 0;
}

// 枚举本机所有网卡 IP 地址（含 loopback），存入 g_local_ips
static void collect_local_ips(void) {
    g_local_ip_count = 0;
    struct ifaddrs *ifap = NULL;
    if (getifaddrs(&ifap) != 0) return;

    for (struct ifaddrs *ifa = ifap; ifa && g_local_ip_count < MAX_LOCAL_IPS; ifa = ifa->ifa_next) {
        if (!ifa->ifa_addr) continue;
        char addr[INET6_ADDRSTRLEN] = {0};
        if (ifa->ifa_addr->sa_family == AF_INET) {
            struct sockaddr_in *sin = (struct sockaddr_in *)ifa->ifa_addr;
            inet_ntop(AF_INET, &sin->sin_addr, addr, sizeof(addr));
        } else if (ifa->ifa_addr->sa_family == AF_INET6) {
            struct sockaddr_in6 *sin6 = (struct sockaddr_in6 *)ifa->ifa_addr;
            inet_ntop(AF_INET6, &sin6->sin6_addr, addr, sizeof(addr));
        } else {
            continue;
        }
        int dup = 0;
        for (size_t i = 0; i < g_local_ip_count; ++i) {
            if (strcmp(g_local_ips[i], addr) == 0) { dup = 1; break; }
        }
        if (!dup) {
            snprintf(g_local_ips[g_local_ip_count], INET6_ADDRSTRLEN, "%s", addr);
            g_local_ip_count++;
        }
    }
    freeifaddrs(ifap);
}

// 判断 IP 是否为本机地址
static int is_local_ip(const char *ip) {
    if (!ip) return 0;
    for (size_t i = 0; i < g_local_ip_count; ++i) {
        if (strcmp(ip, g_local_ips[i]) == 0) return 1;
    }
    return 0;
}

// IP 通配符匹配: '*' 可匹配任意长度字符（用于 10.0.*.* 等模式）
static int ip_pattern_matches(const char *ip, const char *pattern) {
    if (!ip || !pattern) return 0;

    while (*pattern) {
        if (*pattern == '*') {
            pattern++;
            if (*pattern == '\0') return 1;
            while (*ip) {
                if (ip_pattern_matches(ip, pattern)) return 1;
                ip++;
            }
            return ip_pattern_matches(ip, pattern);
        }
        if (*ip == '\0' || *ip != *pattern) return 0;
        ip++;
        pattern++;
    }
    return *ip == '\0';
}

// 判断 IP 地址是否被策略拦截 (支持黑名单/白名单模式)
static int is_ip_blocked(const char *ip) {
    if (!g_policy_loaded || !ip) return 0;

    if (g_policy.network_policy_whitelist) {
        if (g_policy.allowed_ips_count == 0 && g_policy.allowed_domains_count == 0) return 0;
        if (is_local_ip(ip)) return 0;
        for (size_t i = 0; i < g_policy.allowed_ips_count; ++i) {
            if (ip_pattern_matches(ip, g_policy.allowed_ips[i])) return 0;
        }
        return 1;
    }

    for (size_t i = 0; i < g_policy.blocked_ips_count; ++i) {
        if (ip_pattern_matches(ip, g_policy.blocked_ips[i])) return 1;
    }
    return 0;
}

// 域名匹配: 精确匹配或后缀匹配 (e.g. pattern=baidu.com 匹配 www.baidu.com)
static int domain_matches(const char *domain, const char *pattern) {
    if (!domain || !pattern) return 0;

    // 支持通配符子域名模式: *.example.com
    if (pattern[0] == '*' && pattern[1] == '.') {
        const char *suffix = pattern + 2;
        size_t dlen = strlen(domain);
        size_t slen = strlen(suffix);
        if (slen == 0 || dlen < slen) return 0;
        if (strcasecmp(domain + (dlen - slen), suffix) != 0) return 0;

        // 允许匹配根域名和子域名，避免 "*.example.com" 规则遗漏 "example.com"
        if (dlen == slen) return 1;
        return domain[dlen - slen - 1] == '.';
    }

    if (strcasecmp(domain, pattern) == 0) return 1;
    size_t dlen = strlen(domain);
    size_t plen = strlen(pattern);
    if (dlen > plen + 1) {
        const char *suffix = domain + (dlen - plen);
        if (suffix[-1] == '.' && strcasecmp(suffix, pattern) == 0) return 1;
    }
    return 0;
}

// 判断域名是否被策略拦截 (支持黑名单/白名单模式，含后缀匹配)
static int is_domain_blocked(const char *domain) {
    if (!g_policy_loaded || !domain) return 0;

    if (g_policy.network_policy_whitelist) {
        if (g_policy.allowed_domains_count == 0 && g_policy.allowed_ips_count == 0) return 0;
        for (size_t i = 0; i < g_policy.allowed_domains_count; ++i) {
            if (domain_matches(domain, g_policy.allowed_domains[i])) return 0;
        }
        return 1;
    }

    for (size_t i = 0; i < g_policy.blocked_domains_count; ++i) {
        if (domain_matches(domain, g_policy.blocked_domains[i])) return 1;
    }
    return 0;
}

// 判断命令是否被策略拦截 (支持黑名单/白名单模式，子串匹配)
static int is_cmd_boundary_char(char c) {
    return c == '\0' || c == ' ' || c == '\t' || c == '\n' || c == '\r' ||
           c == '/' || c == '\\' || c == '"' || c == '\'' || c == '=' ||
           c == ':' || c == ';' || c == '|' || c == '&';
}

// 命令匹配: 大小写不敏感 + 边界感知，减少误匹配
static int cmd_matches(const char *cmd, const char *pattern) {
    if (!cmd || !pattern) return 0;
    size_t plen = strlen(pattern);
    if (plen == 0) return 0;

    const char *cursor = cmd;
    while ((cursor = strcasestr(cursor, pattern)) != NULL) {
        char left = (cursor == cmd) ? '\0' : cursor[-1];
        char right = cursor[plen];
        if (is_cmd_boundary_char(left) && is_cmd_boundary_char(right)) {
            return 1;
        }
        cursor++;
    }
    return 0;
}

// 判断命令是否被策略拦截 (支持黑名单/白名单模式)
static int is_cmd_blocked(const char *cmd) {
    if (!g_policy_loaded || !cmd) return 0;

    if (g_policy.command_policy_whitelist) {
        if (g_policy.allowed_cmds_count == 0) return 0;
        for (size_t i = 0; i < g_policy.allowed_cmds_count; ++i) {
            if (g_policy.allowed_cmds[i] && cmd_matches(cmd, g_policy.allowed_cmds[i])) return 0;
        }
        return 1;
    }

    for (size_t i = 0; i < g_policy.blocked_cmds_count; ++i) {
        if (g_policy.blocked_cmds[i] && cmd_matches(cmd, g_policy.blocked_cmds[i])) return 1;
    }
    return 0;
}

// ---------------- 生命周期 ----------------

// 库加载时自动执行: 读取策略文件路径 -> 推导日志 -> 加载策略 -> 枚举本机 IP
__attribute__((constructor))
static void sandbox_init(void) {
    const char *policy_path = getenv("SANDBOX_POLICY_FILE");

    if (policy_path && policy_path[0] != '\0') {
        snprintf(g_policy_file_path, sizeof(g_policy_file_path), "%s", policy_path);
    }

    if (!policy_path || policy_path[0] == '\0') {
        log_info("SANDBOX_POLICY_FILE not set, sandbox disabled");
        return;
    }

    log_info("loading sandbox policy");
    load_policy_from_file(policy_path);

    if (g_policy_loaded) {
        log_info("sandbox policy loaded successfully");
        if (g_policy.network_policy_whitelist) {
            collect_local_ips();
            log_info("collected %zu local IPs for network whitelist auto-allow", g_local_ip_count);
            for (size_t i = 0; i < g_local_ip_count; ++i) {
                log_info("  local IP: %s", g_local_ips[i]);
            }
        }
    } else {
        log_info("sandbox policy load FAILED");
    }
}

// 库卸载时自动执行: 释放策略结构体内存
__attribute__((destructor))
static void sandbox_fini(void) {
    free_string_array(g_policy.blocked_paths, g_policy.blocked_paths_count);
    free_string_array(g_policy.allowed_paths, g_policy.allowed_paths_count);
    free_string_array(g_policy.blocked_ips, g_policy.blocked_ips_count);
    free_string_array(g_policy.allowed_ips, g_policy.allowed_ips_count);
    free_string_array(g_policy.blocked_domains, g_policy.blocked_domains_count);
    free_string_array(g_policy.allowed_domains, g_policy.allowed_domains_count);
    free_string_array(g_policy.blocked_cmds, g_policy.blocked_cmds_count);
    free_string_array(g_policy.allowed_cmds, g_policy.allowed_cmds_count);
}

// ---------------- 系统调用拦截 ----------------

static int (*real_open_fn)(const char *, int, ...) = NULL;
static int (*real_openat_fn)(int, const char *, int, ...) = NULL;
static int (*real_open64_fn)(const char *, int, ...) = NULL;
static int (*real_openat64_fn)(int, const char *, int, ...) = NULL;
static int (*real_creat_fn)(const char *, mode_t) = NULL;
static int (*real_creat64_fn)(const char *, mode_t) = NULL;
static FILE *(*real_fopen_fn)(const char *, const char *) = NULL;
static FILE *(*real_fopen64_fn)(const char *, const char *) = NULL;
static FILE *(*real_freopen_fn)(const char *, const char *, FILE *) = NULL;
static FILE *(*real_freopen64_fn)(const char *, const char *, FILE *) = NULL;
static DIR *(*real_opendir_fn)(const char *) = NULL;
static DIR *(*real_opendir64_fn)(const char *) = NULL;
static struct dirent *(*real_readdir_fn)(DIR *) = NULL;
static struct dirent64 *(*real_readdir64_fn)(DIR *) = NULL;
static int (*real_scandir_fn)(const char *, struct dirent ***,
                              int (*)(const struct dirent *),
                              int (*)(const struct dirent **, const struct dirent **)) = NULL;
static int (*real_scandir64_fn)(const char *, struct dirent64 ***,
                                int (*)(const struct dirent64 *),
                                int (*)(const struct dirent64 **, const struct dirent64 **)) = NULL;
static int (*real_stat_fn)(const char *, struct stat *) = NULL;
static int (*real_lstat_fn)(const char *, struct stat *) = NULL;
static int (*real_fstatat_fn)(int, const char *, struct stat *, int) = NULL;
static int (*real_stat64_fn)(const char *, struct stat64 *) = NULL;
static int (*real_lstat64_fn)(const char *, struct stat64 *) = NULL;
static int (*real_fstatat64_fn)(int, const char *, struct stat64 *, int) = NULL;
static ssize_t (*real_read_fn)(int, void *, size_t) = NULL;
static ssize_t (*real_write_fn)(int, const void *, size_t) = NULL;
static ssize_t (*real_pread_fn)(int, void *, size_t, off_t) = NULL;
static ssize_t (*real_pwrite_fn)(int, const void *, size_t, off_t) = NULL;
static ssize_t (*real_pread64_fn)(int, void *, size_t, off64_t) = NULL;
static ssize_t (*real_pwrite64_fn)(int, const void *, size_t, off64_t) = NULL;
static ssize_t (*real_readv_fn)(int, const struct iovec *, int) = NULL;
static ssize_t (*real_writev_fn)(int, const struct iovec *, int) = NULL;
static int (*real_mkdir_fn)(const char *, mode_t) = NULL;
static int (*real_mkdirat_fn)(int, const char *, mode_t) = NULL;
static int (*real_unlink_fn)(const char *) = NULL;
static int (*real_unlinkat_fn)(int, const char *, int) = NULL;
static int (*real_rmdir_fn)(const char *) = NULL;
static int (*real_rename_fn)(const char *, const char *) = NULL;
static int (*real_renameat_fn)(int, const char *, int, const char *) = NULL;
static int (*real_renameat2_fn)(int, const char *, int, const char *, unsigned int) = NULL;
static int (*real_connect_fn)(int, const struct sockaddr *, socklen_t) = NULL;
static ssize_t (*real_sendto_fn)(int, const void *, size_t, int,
                                 const struct sockaddr *, socklen_t) = NULL;
static ssize_t (*real_sendmsg_fn)(int, const struct msghdr *, int) = NULL;
static int (*real_system_fn)(const char *) = NULL;
static int (*real_execve_fn)(const char *, char *const [], char *const []) = NULL;
static int (*real_getaddrinfo_fn)(const char *, const char *,
                                  const struct addrinfo *, struct addrinfo **) = NULL;

// 从 sockaddr 提取 IP 并检查是否被网络策略拦截，被拦截返回 1
static int check_sockaddr_blocked(const struct sockaddr *addr, socklen_t addrlen,
                                  char *ip_out, size_t ip_out_size) {
    if (!addr || !g_policy_loaded || !ip_out) return 0;
    ip_out[0] = '\0';

    if (addr->sa_family == AF_INET && addrlen >= (socklen_t)sizeof(struct sockaddr_in)) {
        const struct sockaddr_in *in = (const struct sockaddr_in *)addr;
        inet_ntop(AF_INET, &in->sin_addr, ip_out, ip_out_size);
    } else if (addr->sa_family == AF_INET6 && addrlen >= (socklen_t)sizeof(struct sockaddr_in6)) {
        const struct sockaddr_in6 *in6 = (const struct sockaddr_in6 *)addr;
        inet_ntop(AF_INET6, &in6->sin6_addr, ip_out, ip_out_size);
    }

    if (ip_out[0] != '\0' && is_ip_blocked(ip_out)) {
        return 1;
    }
    return 0;
}

// 拦截 open() 系统调用，检查文件路径是否被策略禁止
int open(const char *pathname, int flags, ...) {
    if (!real_open_fn) {
        real_open_fn = (int (*)(const char *, int, ...))dlsym(RTLD_NEXT, "open");
    }

    char norm_path[PATH_MAX] = {0};
    const char *path_for_check = normalize_path_for_policy(pathname, AT_FDCWD, norm_path, sizeof(norm_path));
    if (pathname && enforce_path_policy_by_mask(path_for_check, build_open_op_mask(flags))) {
        return -1;
    }

    va_list ap;
    va_start(ap, flags);
    int ret;
    if (flags & O_CREAT) {
        int mode_int = va_arg(ap, int);
        mode_t mode = (mode_t)mode_int;
        ret = real_open_fn(pathname, flags, mode);
    } else {
        ret = real_open_fn(pathname, flags);
    }
    va_end(ap);
    return ret;
}

// 拦截 openat() 系统调用，覆盖现代 glibc 内部使用 openat 的场景
int openat(int dirfd, const char *pathname, int flags, ...) {
    if (!real_openat_fn) {
        real_openat_fn = (int (*)(int, const char *, int, ...))dlsym(RTLD_NEXT, "openat");
    }

    char norm_path[PATH_MAX] = {0};
    const char *path_for_check = normalize_path_for_policy(pathname, dirfd, norm_path, sizeof(norm_path));
    if (pathname && enforce_path_policy_by_mask(path_for_check, build_open_op_mask(flags))) {
        return -1;
    }

    va_list ap;
    va_start(ap, flags);
    int ret;
    if (flags & O_CREAT) {
        int mode_int = va_arg(ap, int);
        mode_t mode = (mode_t)mode_int;
        ret = real_openat_fn(dirfd, pathname, flags, mode);
    } else {
        ret = real_openat_fn(dirfd, pathname, flags);
    }
    va_end(ap);
    return ret;
}

int open64(const char *pathname, int flags, ...) {
    if (!real_open64_fn) {
        real_open64_fn = (int (*)(const char *, int, ...))dlsym(RTLD_NEXT, "open64");
    }

    char norm_path[PATH_MAX] = {0};
    const char *path_for_check = normalize_path_for_policy(pathname, AT_FDCWD, norm_path, sizeof(norm_path));
    if (pathname && enforce_path_policy_by_mask(path_for_check, build_open_op_mask(flags))) {
        return -1;
    }

    va_list ap;
    va_start(ap, flags);
    int ret;
    if (flags & O_CREAT) {
        int mode_int = va_arg(ap, int);
        mode_t mode = (mode_t)mode_int;
        ret = real_open64_fn(pathname, flags, mode);
    } else {
        ret = real_open64_fn(pathname, flags);
    }
    va_end(ap);
    return ret;
}

int openat64(int dirfd, const char *pathname, int flags, ...) {
    if (!real_openat64_fn) {
        real_openat64_fn = (int (*)(int, const char *, int, ...))dlsym(RTLD_NEXT, "openat64");
    }

    char norm_path[PATH_MAX] = {0};
    const char *path_for_check = normalize_path_for_policy(pathname, dirfd, norm_path, sizeof(norm_path));
    if (pathname && enforce_path_policy_by_mask(path_for_check, build_open_op_mask(flags))) {
        return -1;
    }

    va_list ap;
    va_start(ap, flags);
    int ret;
    if (flags & O_CREAT) {
        int mode_int = va_arg(ap, int);
        mode_t mode = (mode_t)mode_int;
        ret = real_openat64_fn(dirfd, pathname, flags, mode);
    } else {
        ret = real_openat64_fn(dirfd, pathname, flags);
    }
    va_end(ap);
    return ret;
}

int creat(const char *pathname, mode_t mode) {
    if (!real_creat_fn) {
        real_creat_fn = (int (*)(const char *, mode_t))dlsym(RTLD_NEXT, "creat");
    }

    char norm_path[PATH_MAX] = {0};
    const char *path_for_check = normalize_path_for_policy(pathname, AT_FDCWD, norm_path, sizeof(norm_path));
    if (pathname && enforce_path_policy_by_mask(path_for_check, PATH_OP_CREATE | PATH_OP_WRITE)) {
        return -1;
    }

    return real_creat_fn(pathname, mode);
}

int creat64(const char *pathname, mode_t mode) {
    if (!real_creat64_fn) {
        real_creat64_fn = (int (*)(const char *, mode_t))dlsym(RTLD_NEXT, "creat64");
    }

    char norm_path[PATH_MAX] = {0};
    const char *path_for_check = normalize_path_for_policy(pathname, AT_FDCWD, norm_path, sizeof(norm_path));
    if (pathname && enforce_path_policy_by_mask(path_for_check, PATH_OP_CREATE | PATH_OP_WRITE)) {
        return -1;
    }

    return real_creat64_fn(pathname, mode);
}

FILE *fopen(const char *pathname, const char *mode) {
    if (!real_fopen_fn) {
        real_fopen_fn = (FILE *(*)(const char *, const char *))dlsym(RTLD_NEXT, "fopen");
    }

    char norm_path[PATH_MAX] = {0};
    const char *path_for_check = normalize_path_for_policy(pathname, AT_FDCWD, norm_path, sizeof(norm_path));
    if (pathname && enforce_path_policy_by_mask(path_for_check, build_stdio_mode_op_mask(mode, PATH_OP_READ))) {
        return NULL;
    }

    return real_fopen_fn(pathname, mode);
}

FILE *fopen64(const char *pathname, const char *mode) {
    if (!real_fopen64_fn) {
        real_fopen64_fn = (FILE *(*)(const char *, const char *))dlsym(RTLD_NEXT, "fopen64");
    }

    char norm_path[PATH_MAX] = {0};
    const char *path_for_check = normalize_path_for_policy(pathname, AT_FDCWD, norm_path, sizeof(norm_path));
    if (pathname && enforce_path_policy_by_mask(path_for_check, build_stdio_mode_op_mask(mode, PATH_OP_READ))) {
        return NULL;
    }

    return real_fopen64_fn(pathname, mode);
}

FILE *freopen(const char *pathname, const char *mode, FILE *stream) {
    if (!real_freopen_fn) {
        real_freopen_fn = (FILE *(*)(const char *, const char *, FILE *))dlsym(RTLD_NEXT, "freopen");
    }

    if (pathname) {
        char norm_path[PATH_MAX] = {0};
        const char *path_for_check = normalize_path_for_policy(pathname, AT_FDCWD, norm_path, sizeof(norm_path));
        if (enforce_path_policy_by_mask(path_for_check, build_stdio_mode_op_mask(mode, PATH_OP_WRITE))) {
            return NULL;
        }
    }

    return real_freopen_fn(pathname, mode, stream);
}

FILE *freopen64(const char *pathname, const char *mode, FILE *stream) {
    if (!real_freopen64_fn) {
        real_freopen64_fn = (FILE *(*)(const char *, const char *, FILE *))dlsym(RTLD_NEXT, "freopen64");
    }

    if (pathname) {
        char norm_path[PATH_MAX] = {0};
        const char *path_for_check = normalize_path_for_policy(pathname, AT_FDCWD, norm_path, sizeof(norm_path));
        if (enforce_path_policy_by_mask(path_for_check, build_stdio_mode_op_mask(mode, PATH_OP_WRITE))) {
            return NULL;
        }
    }

    return real_freopen64_fn(pathname, mode, stream);
}

// 拦截 opendir()，阻止枚举被禁止目录
DIR *opendir(const char *name) {
    if (!real_opendir_fn) {
        real_opendir_fn = (DIR *(*)(const char *))dlsym(RTLD_NEXT, "opendir");
    }

    char norm_path[PATH_MAX] = {0};
    const char *path_for_check = normalize_path_for_policy(name, AT_FDCWD, norm_path, sizeof(norm_path));
    if (name && enforce_path_policy_by_mask(path_for_check, PATH_OP_DIR_OPEN | PATH_OP_READ)) {
        return NULL;
    }

    return real_opendir_fn(name);
}

// 拦截 opendir64()，覆盖 64 位目录接口
DIR *opendir64(const char *name) {
    if (!real_opendir64_fn) {
        real_opendir64_fn = (DIR *(*)(const char *))dlsym(RTLD_NEXT, "opendir64");
    }

    char norm_path[PATH_MAX] = {0};
    const char *path_for_check = normalize_path_for_policy(name, AT_FDCWD, norm_path, sizeof(norm_path));
    if (name && enforce_path_policy_by_mask(path_for_check, PATH_OP_DIR_OPEN | PATH_OP_READ)) {
        return NULL;
    }

    return real_opendir64_fn(name);
}

// 拦截 readdir()，防止通过已打开 fd 继续查看被禁止目录内容
struct dirent *readdir(DIR *dirp) {
    if (!real_readdir_fn) {
        real_readdir_fn = (struct dirent *(*)(DIR *))dlsym(RTLD_NEXT, "readdir");
    }

    if (dirp) {
        int fd = dirfd(dirp);
        if (fd >= 0 && enforce_fd_policy("PATH-READ", fd)) {
            return NULL;
        }
    }

    return real_readdir_fn(dirp);
}

// 拦截 readdir64()，覆盖 64 位目录读取接口
struct dirent64 *readdir64(DIR *dirp) {
    if (!real_readdir64_fn) {
        real_readdir64_fn = (struct dirent64 *(*)(DIR *))dlsym(RTLD_NEXT, "readdir64");
    }

    if (dirp) {
        int fd = dirfd(dirp);
        if (fd >= 0 && enforce_fd_policy("PATH-READ", fd)) {
            return NULL;
        }
    }

    return real_readdir64_fn(dirp);
}

// 拦截 scandir()，阻止目录扫描结果返回
int scandir(const char *dirp, struct dirent ***namelist,
            int (*filter)(const struct dirent *),
            int (*compar)(const struct dirent **, const struct dirent **)) {
    if (!real_scandir_fn) {
        real_scandir_fn = (int (*)(const char *, struct dirent ***,
                                   int (*)(const struct dirent *),
                                   int (*)(const struct dirent **, const struct dirent **)))
                          dlsym(RTLD_NEXT, "scandir");
    }

    char norm_path[PATH_MAX] = {0};
    const char *path_for_check = normalize_path_for_policy(dirp, AT_FDCWD, norm_path, sizeof(norm_path));
    if (dirp && enforce_path_policy_by_mask(path_for_check, PATH_OP_DIR_OPEN | PATH_OP_READ)) {
        return -1;
    }

    return real_scandir_fn(dirp, namelist, filter, compar);
}

// 拦截 scandir64()，覆盖 64 位目录扫描接口
int scandir64(const char *dirp, struct dirent64 ***namelist,
              int (*filter)(const struct dirent64 *),
              int (*compar)(const struct dirent64 **, const struct dirent64 **)) {
    if (!real_scandir64_fn) {
        real_scandir64_fn = (int (*)(const char *, struct dirent64 ***,
                                     int (*)(const struct dirent64 *),
                                     int (*)(const struct dirent64 **, const struct dirent64 **)))
                            dlsym(RTLD_NEXT, "scandir64");
    }

    char norm_path[PATH_MAX] = {0};
    const char *path_for_check = normalize_path_for_policy(dirp, AT_FDCWD, norm_path, sizeof(norm_path));
    if (dirp && enforce_path_policy_by_mask(path_for_check, PATH_OP_DIR_OPEN | PATH_OP_READ)) {
        return -1;
    }

    return real_scandir64_fn(dirp, namelist, filter, compar);
}

int stat(const char *pathname, struct stat *statbuf) {
    if (!real_stat_fn) {
        real_stat_fn = (int (*)(const char *, struct stat *))dlsym(RTLD_NEXT, "stat");
    }

    char norm_path[PATH_MAX] = {0};
    const char *path_for_check = normalize_path_for_policy(pathname, AT_FDCWD, norm_path, sizeof(norm_path));
    if (pathname && enforce_path_policy_by_mask(path_for_check, PATH_OP_READ)) {
        return -1;
    }

    return real_stat_fn(pathname, statbuf);
}

int lstat(const char *pathname, struct stat *statbuf) {
    if (!real_lstat_fn) {
        real_lstat_fn = (int (*)(const char *, struct stat *))dlsym(RTLD_NEXT, "lstat");
    }

    char norm_path[PATH_MAX] = {0};
    const char *path_for_check = normalize_path_for_policy(pathname, AT_FDCWD, norm_path, sizeof(norm_path));
    if (pathname && enforce_path_policy_by_mask(path_for_check, PATH_OP_READ)) {
        return -1;
    }

    return real_lstat_fn(pathname, statbuf);
}

int fstatat(int dirfd, const char *pathname, struct stat *statbuf, int flags) {
    if (!real_fstatat_fn) {
        real_fstatat_fn = (int (*)(int, const char *, struct stat *, int))dlsym(RTLD_NEXT, "fstatat");
    }

    char norm_path[PATH_MAX] = {0};
    const char *path_for_check = normalize_path_for_policy(pathname, dirfd, norm_path, sizeof(norm_path));
    if (pathname && enforce_path_policy_by_mask(path_for_check, PATH_OP_READ)) {
        return -1;
    }

    return real_fstatat_fn(dirfd, pathname, statbuf, flags);
}

int stat64(const char *pathname, struct stat64 *statbuf) {
    if (!real_stat64_fn) {
        real_stat64_fn = (int (*)(const char *, struct stat64 *))dlsym(RTLD_NEXT, "stat64");
    }

    char norm_path[PATH_MAX] = {0};
    const char *path_for_check = normalize_path_for_policy(pathname, AT_FDCWD, norm_path, sizeof(norm_path));
    if (pathname && enforce_path_policy_by_mask(path_for_check, PATH_OP_READ)) {
        return -1;
    }

    return real_stat64_fn(pathname, statbuf);
}

int lstat64(const char *pathname, struct stat64 *statbuf) {
    if (!real_lstat64_fn) {
        real_lstat64_fn = (int (*)(const char *, struct stat64 *))dlsym(RTLD_NEXT, "lstat64");
    }

    char norm_path[PATH_MAX] = {0};
    const char *path_for_check = normalize_path_for_policy(pathname, AT_FDCWD, norm_path, sizeof(norm_path));
    if (pathname && enforce_path_policy_by_mask(path_for_check, PATH_OP_READ)) {
        return -1;
    }

    return real_lstat64_fn(pathname, statbuf);
}

int fstatat64(int dirfd, const char *pathname, struct stat64 *statbuf, int flags) {
    if (!real_fstatat64_fn) {
        real_fstatat64_fn = (int (*)(int, const char *, struct stat64 *, int))dlsym(RTLD_NEXT, "fstatat64");
    }

    char norm_path[PATH_MAX] = {0};
    const char *path_for_check = normalize_path_for_policy(pathname, dirfd, norm_path, sizeof(norm_path));
    if (pathname && enforce_path_policy_by_mask(path_for_check, PATH_OP_READ)) {
        return -1;
    }

    return real_fstatat64_fn(dirfd, pathname, statbuf, flags);
}

ssize_t read(int fd, void *buf, size_t count) {
    if (!real_read_fn) {
        real_read_fn = (ssize_t (*)(int, void *, size_t))dlsym(RTLD_NEXT, "read");
    }

    if (enforce_fd_policy("PATH-READ", fd)) {
        return -1;
    }

    return real_read_fn(fd, buf, count);
}

ssize_t write(int fd, const void *buf, size_t count) {
    if (!real_write_fn) {
        real_write_fn = (ssize_t (*)(int, const void *, size_t))dlsym(RTLD_NEXT, "write");
    }

    if (enforce_fd_policy("PATH-WRITE", fd)) {
        return -1;
    }

    return real_write_fn(fd, buf, count);
}

ssize_t pread(int fd, void *buf, size_t count, off_t offset) {
    if (!real_pread_fn) {
        real_pread_fn = (ssize_t (*)(int, void *, size_t, off_t))dlsym(RTLD_NEXT, "pread");
    }

    if (enforce_fd_policy("PATH-READ", fd)) {
        return -1;
    }

    return real_pread_fn(fd, buf, count, offset);
}

ssize_t pwrite(int fd, const void *buf, size_t count, off_t offset) {
    if (!real_pwrite_fn) {
        real_pwrite_fn = (ssize_t (*)(int, const void *, size_t, off_t))dlsym(RTLD_NEXT, "pwrite");
    }

    if (enforce_fd_policy("PATH-WRITE", fd)) {
        return -1;
    }

    return real_pwrite_fn(fd, buf, count, offset);
}

ssize_t pread64(int fd, void *buf, size_t count, off64_t offset) {
    if (!real_pread64_fn) {
        real_pread64_fn = (ssize_t (*)(int, void *, size_t, off64_t))dlsym(RTLD_NEXT, "pread64");
    }

    if (enforce_fd_policy("PATH-READ", fd)) {
        return -1;
    }

    return real_pread64_fn(fd, buf, count, offset);
}

ssize_t pwrite64(int fd, const void *buf, size_t count, off64_t offset) {
    if (!real_pwrite64_fn) {
        real_pwrite64_fn = (ssize_t (*)(int, const void *, size_t, off64_t))dlsym(RTLD_NEXT, "pwrite64");
    }

    if (enforce_fd_policy("PATH-WRITE", fd)) {
        return -1;
    }

    return real_pwrite64_fn(fd, buf, count, offset);
}

ssize_t readv(int fd, const struct iovec *iov, int iovcnt) {
    if (!real_readv_fn) {
        real_readv_fn = (ssize_t (*)(int, const struct iovec *, int))dlsym(RTLD_NEXT, "readv");
    }

    if (enforce_fd_policy("PATH-READ", fd)) {
        return -1;
    }

    return real_readv_fn(fd, iov, iovcnt);
}

ssize_t writev(int fd, const struct iovec *iov, int iovcnt) {
    if (!real_writev_fn) {
        real_writev_fn = (ssize_t (*)(int, const struct iovec *, int))dlsym(RTLD_NEXT, "writev");
    }

    if (enforce_fd_policy("PATH-WRITE", fd)) {
        return -1;
    }

    return real_writev_fn(fd, iov, iovcnt);
}

int mkdir(const char *pathname, mode_t mode) {
    if (!real_mkdir_fn) {
        real_mkdir_fn = (int (*)(const char *, mode_t))dlsym(RTLD_NEXT, "mkdir");
    }

    char norm_path[PATH_MAX] = {0};
    const char *path_for_check = normalize_path_for_policy(pathname, AT_FDCWD, norm_path, sizeof(norm_path));
    if (pathname && enforce_path_policy_by_mask(path_for_check, PATH_OP_CREATE)) {
        return -1;
    }

    return real_mkdir_fn(pathname, mode);
}

int mkdirat(int dirfd, const char *pathname, mode_t mode) {
    if (!real_mkdirat_fn) {
        real_mkdirat_fn = (int (*)(int, const char *, mode_t))dlsym(RTLD_NEXT, "mkdirat");
    }

    char norm_path[PATH_MAX] = {0};
    const char *path_for_check = normalize_path_for_policy(pathname, dirfd, norm_path, sizeof(norm_path));
    if (pathname && enforce_path_policy_by_mask(path_for_check, PATH_OP_CREATE)) {
        return -1;
    }

    return real_mkdirat_fn(dirfd, pathname, mode);
}

int unlink(const char *pathname) {
    if (!real_unlink_fn) {
        real_unlink_fn = (int (*)(const char *))dlsym(RTLD_NEXT, "unlink");
    }

    char norm_path[PATH_MAX] = {0};
    const char *path_for_check = normalize_path_for_policy(pathname, AT_FDCWD, norm_path, sizeof(norm_path));
    if (pathname && enforce_path_policy_by_mask(path_for_check, PATH_OP_DELETE)) {
        return -1;
    }

    return real_unlink_fn(pathname);
}

int unlinkat(int dirfd, const char *pathname, int flags) {
    if (!real_unlinkat_fn) {
        real_unlinkat_fn = (int (*)(int, const char *, int))dlsym(RTLD_NEXT, "unlinkat");
    }

    char norm_path[PATH_MAX] = {0};
    const char *path_for_check = normalize_path_for_policy(pathname, dirfd, norm_path, sizeof(norm_path));
    if (pathname && enforce_path_policy_by_mask(path_for_check, PATH_OP_DELETE)) {
        return -1;
    }

    return real_unlinkat_fn(dirfd, pathname, flags);
}

int rmdir(const char *pathname) {
    if (!real_rmdir_fn) {
        real_rmdir_fn = (int (*)(const char *))dlsym(RTLD_NEXT, "rmdir");
    }

    char norm_path[PATH_MAX] = {0};
    const char *path_for_check = normalize_path_for_policy(pathname, AT_FDCWD, norm_path, sizeof(norm_path));
    if (pathname && enforce_path_policy_by_mask(path_for_check, PATH_OP_DELETE)) {
        return -1;
    }

    return real_rmdir_fn(pathname);
}

int rename(const char *oldpath, const char *newpath) {
    if (!real_rename_fn) {
        real_rename_fn = (int (*)(const char *, const char *))dlsym(RTLD_NEXT, "rename");
    }

    if (oldpath) {
        char old_norm[PATH_MAX] = {0};
        const char *old_for_check = normalize_path_for_policy(oldpath, AT_FDCWD, old_norm, sizeof(old_norm));
        if (enforce_path_policy_by_mask(old_for_check, PATH_OP_RENAME)) {
            return -1;
        }
    }
    if (newpath) {
        char new_norm[PATH_MAX] = {0};
        const char *new_for_check = normalize_path_for_policy(newpath, AT_FDCWD, new_norm, sizeof(new_norm));
        if (enforce_path_policy_by_mask(new_for_check, PATH_OP_RENAME)) {
            return -1;
        }
    }

    return real_rename_fn(oldpath, newpath);
}

int renameat(int olddirfd, const char *oldpath, int newdirfd, const char *newpath) {
    if (!real_renameat_fn) {
        real_renameat_fn = (int (*)(int, const char *, int, const char *))dlsym(RTLD_NEXT, "renameat");
    }

    if (oldpath) {
        char old_norm[PATH_MAX] = {0};
        const char *old_for_check = normalize_path_for_policy(oldpath, olddirfd, old_norm, sizeof(old_norm));
        if (enforce_path_policy_by_mask(old_for_check, PATH_OP_RENAME)) {
            return -1;
        }
    }
    if (newpath) {
        char new_norm[PATH_MAX] = {0};
        const char *new_for_check = normalize_path_for_policy(newpath, newdirfd, new_norm, sizeof(new_norm));
        if (enforce_path_policy_by_mask(new_for_check, PATH_OP_RENAME)) {
            return -1;
        }
    }

    return real_renameat_fn(olddirfd, oldpath, newdirfd, newpath);
}

int renameat2(int olddirfd, const char *oldpath, int newdirfd, const char *newpath, unsigned int flags) {
    if (!real_renameat2_fn) {
        real_renameat2_fn = (int (*)(int, const char *, int, const char *, unsigned int))dlsym(RTLD_NEXT, "renameat2");
    }

    if (!real_renameat2_fn) {
        errno = ENOSYS;
        return -1;
    }

    if (oldpath) {
        char old_norm[PATH_MAX] = {0};
        const char *old_for_check = normalize_path_for_policy(oldpath, olddirfd, old_norm, sizeof(old_norm));
        if (enforce_path_policy_by_mask(old_for_check, PATH_OP_RENAME)) {
            return -1;
        }
    }
    if (newpath) {
        char new_norm[PATH_MAX] = {0};
        const char *new_for_check = normalize_path_for_policy(newpath, newdirfd, new_norm, sizeof(new_norm));
        if (enforce_path_policy_by_mask(new_for_check, PATH_OP_RENAME)) {
            return -1;
        }
    }

    return real_renameat2_fn(olddirfd, oldpath, newdirfd, newpath, flags);
}

// 拦截 connect() 系统调用，提取目标 IP 地址并检查是否被策略禁止
int connect(int sockfd, const struct sockaddr *addr, socklen_t addrlen) {
    if (!real_connect_fn) {
        real_connect_fn = (int (*)(int, const struct sockaddr *, socklen_t))dlsym(RTLD_NEXT, "connect");
    }

    char ip[INET6_ADDRSTRLEN] = {0};
    if (check_sockaddr_blocked(addr, addrlen, ip, sizeof(ip))) {
        log_event(g_policy.log_only ? "LOG_ONLY" : "BLOCK", "NET", ip);
        if (!g_policy.log_only) {
            errno = ECONNREFUSED;
            return -1;
        }
    }

    return real_connect_fn(sockfd, addr, addrlen);
}

// 拦截 sendto() 系统调用，覆盖 ICMP ping / UDP 等使用 sendto 直接发包的场景
ssize_t sendto(int sockfd, const void *buf, size_t len, int flags,
               const struct sockaddr *dest_addr, socklen_t addrlen) {
    if (!real_sendto_fn) {
        real_sendto_fn = (ssize_t (*)(int, const void *, size_t, int,
                                      const struct sockaddr *, socklen_t))
                         dlsym(RTLD_NEXT, "sendto");
    }

    if (dest_addr) {
        char ip[INET6_ADDRSTRLEN] = {0};
        if (check_sockaddr_blocked(dest_addr, addrlen, ip, sizeof(ip))) {
            log_event(g_policy.log_only ? "LOG_ONLY" : "BLOCK", "NET-SENDTO", ip);
            if (!g_policy.log_only) {
                errno = ECONNREFUSED;
                return -1;
            }
        }
    }

    return real_sendto_fn(sockfd, buf, len, flags, dest_addr, addrlen);
}

// 拦截 sendmsg() 系统调用，覆盖通过 msghdr 指定目标地址发包的场景
ssize_t sendmsg(int sockfd, const struct msghdr *msg, int flags) {
    if (!real_sendmsg_fn) {
        real_sendmsg_fn = (ssize_t (*)(int, const struct msghdr *, int))
                          dlsym(RTLD_NEXT, "sendmsg");
    }

    if (msg && msg->msg_name && msg->msg_namelen > 0) {
        char ip[INET6_ADDRSTRLEN] = {0};
        if (check_sockaddr_blocked((const struct sockaddr *)msg->msg_name,
                                   msg->msg_namelen, ip, sizeof(ip))) {
            log_event(g_policy.log_only ? "LOG_ONLY" : "BLOCK", "NET-SENDMSG", ip);
            if (!g_policy.log_only) {
                errno = ECONNREFUSED;
                return -1;
            }
        }
    }

    return real_sendmsg_fn(sockfd, msg, flags);
}

// 拦截 getaddrinfo() DNS 解析，检查域名是否被策略禁止
int getaddrinfo(const char *node, const char *service,
                const struct addrinfo *hints, struct addrinfo **res) {
    if (!real_getaddrinfo_fn) {
        real_getaddrinfo_fn = (int (*)(const char *, const char *,
                                       const struct addrinfo *, struct addrinfo **))
                              dlsym(RTLD_NEXT, "getaddrinfo");
    }

    if (node && g_policy_loaded && is_domain_blocked(node)) {
        log_event(g_policy.log_only ? "LOG_ONLY" : "BLOCK", "DNS", node);
        if (!g_policy.log_only) {
            return EAI_FAIL;
        }
    }

    return real_getaddrinfo_fn(node, service, hints, res);
}

// 拦截 system() 调用，检查命令是否被策略禁止
int system(const char *command) {
    if (!real_system_fn) {
        real_system_fn = (int (*)(const char *))dlsym(RTLD_NEXT, "system");
    }

    if (command && is_cmd_blocked(command)) {
        log_event(g_policy.log_only ? "LOG_ONLY" : "BLOCK", "CMD", command);
        if (!g_policy.log_only) {
            errno = EPERM;
            return -1;
        }
    }

    return real_system_fn(command);
}

// 拦截 execve() 系统调用，检查执行的命令是否被策略禁止
int execve(const char *filename, char *const argv[], char *const envp[]) {
    if (!real_execve_fn) {
        real_execve_fn = (int (*)(const char *, char *const [], char *const []))dlsym(RTLD_NEXT, "execve");
    }

    const char *cmd = filename;
    if (argv && argv[0]) {
        cmd = argv[0];
    }

    if (cmd && is_cmd_blocked(cmd)) {
        log_event(g_policy.log_only ? "LOG_ONLY" : "BLOCK", "CMD", cmd);
        if (!g_policy.log_only) {
            errno = EPERM;
            return -1;
        }
    }

    return real_execve_fn(filename, argv, envp);
}
