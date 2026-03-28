#include "policy.h"
#include <cJSON.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <wchar.h>
#include <ws2tcpip.h>
#include <iphlpapi.h>

#pragma comment(lib, "iphlpapi.lib")

#define MAX_LOCAL_IPS 256
static char g_local_ips[MAX_LOCAL_IPS][64];
static int g_local_ip_count = 0;

static void load_string_array(cJSON *arr, char dest[][MAX_PATH_LEN], int *count, int max) {
    *count = 0;
    if (!cJSON_IsArray(arr)) return;
    cJSON *item;
    cJSON_ArrayForEach(item, arr) {
        if (*count >= max) break;
        if (cJSON_IsString(item) && item->valuestring) {
            strncpy(dest[*count], item->valuestring, MAX_PATH_LEN - 1);
            dest[*count][MAX_PATH_LEN - 1] = '\0';
            (*count)++;
        }
    }
}

static void load_ip_array(cJSON *arr, char dest[][64], int *count, int max) {
    *count = 0;
    if (!cJSON_IsArray(arr)) return;
    cJSON *item;
    cJSON_ArrayForEach(item, arr) {
        if (*count >= max) break;
        if (cJSON_IsString(item) && item->valuestring) {
            strncpy(dest[*count], item->valuestring, 63);
            dest[*count][63] = '\0';
            (*count)++;
        }
    }
}

static int is_ip_in_list(const char *ip) {
    if (!ip) return 0;
    for (int i = 0; i < g_local_ip_count; ++i) {
        if (strcmp(g_local_ips[i], ip) == 0) return 1;
    }
    return 0;
}

static void collect_local_ips(void) {
    g_local_ip_count = 0;

    ULONG flags = GAA_FLAG_SKIP_ANYCAST | GAA_FLAG_SKIP_MULTICAST | GAA_FLAG_SKIP_DNS_SERVER;
    ULONG family = AF_UNSPEC;
    ULONG size = 15000;
    IP_ADAPTER_ADDRESSES *addresses = (IP_ADAPTER_ADDRESSES *)malloc(size);
    if (!addresses) return;

    ULONG ret = GetAdaptersAddresses(family, flags, NULL, addresses, &size);
    if (ret == ERROR_BUFFER_OVERFLOW) {
        free(addresses);
        addresses = (IP_ADAPTER_ADDRESSES *)malloc(size);
        if (!addresses) return;
        ret = GetAdaptersAddresses(family, flags, NULL, addresses, &size);
    }

    if (ret != NO_ERROR) {
        free(addresses);
        return;
    }

    for (IP_ADAPTER_ADDRESSES *adapter = addresses; adapter != NULL; adapter = adapter->Next) {
        for (IP_ADAPTER_UNICAST_ADDRESS *ua = adapter->FirstUnicastAddress; ua != NULL; ua = ua->Next) {
            if (!ua->Address.lpSockaddr) continue;

            char ip[64] = {0};
            if (ua->Address.lpSockaddr->sa_family == AF_INET) {
                struct sockaddr_in *sin = (struct sockaddr_in *)ua->Address.lpSockaddr;
                InetNtopA(AF_INET, &sin->sin_addr, ip, sizeof(ip));
            } else if (ua->Address.lpSockaddr->sa_family == AF_INET6) {
                struct sockaddr_in6 *sin6 = (struct sockaddr_in6 *)ua->Address.lpSockaddr;
                InetNtopA(AF_INET6, &sin6->sin6_addr, ip, sizeof(ip));
            } else {
                continue;
            }

            if (ip[0] == '\0' || is_ip_in_list(ip)) {
                continue;
            }
            if (g_local_ip_count >= MAX_LOCAL_IPS) {
                break;
            }
            strncpy(g_local_ips[g_local_ip_count], ip, sizeof(g_local_ips[g_local_ip_count]) - 1);
            g_local_ips[g_local_ip_count][sizeof(g_local_ips[g_local_ip_count]) - 1] = '\0';
            g_local_ip_count++;
        }
    }

    free(addresses);
}

int policy_load(const char *path, SandboxPolicy *out) {
    memset(out, 0, sizeof(*out));
    out->inject_children = true;

    FILE *f = fopen(path, "rb");
    if (!f) return -1;

    fseek(f, 0, SEEK_END);
    long len = ftell(f);
    fseek(f, 0, SEEK_SET);

    if (len <= 0 || len > 1024 * 1024) {
        fclose(f);
        return -2;
    }

    char *buf = (char *)malloc(len + 1);
    if (!buf) { fclose(f); return -3; }
    fread(buf, 1, len, f);
    buf[len] = '\0';
    fclose(f);

    cJSON *root = cJSON_Parse(buf);
    free(buf);
    if (!root) return -4;

    cJSON *val;

    val = cJSON_GetObjectItem(root, "file_policy_type");
    if (val && cJSON_IsString(val)) {
        out->file_policy = (strcmp(val->valuestring, "whitelist") == 0)
                               ? POLICY_WHITELIST : POLICY_BLACKLIST;
    }
    load_string_array(cJSON_GetObjectItem(root, "blocked_paths"),
                      out->blocked_paths, &out->blocked_paths_count, MAX_POLICY_ENTRIES);
    load_string_array(cJSON_GetObjectItem(root, "allowed_paths"),
                      out->allowed_paths, &out->allowed_paths_count, MAX_POLICY_ENTRIES);

    val = cJSON_GetObjectItem(root, "command_policy_type");
    if (val && cJSON_IsString(val)) {
        out->command_policy = (strcmp(val->valuestring, "whitelist") == 0)
                                  ? POLICY_WHITELIST : POLICY_BLACKLIST;
    }
    load_string_array(cJSON_GetObjectItem(root, "blocked_commands"),
                      out->blocked_commands, &out->blocked_commands_count, MAX_POLICY_ENTRIES);
    load_string_array(cJSON_GetObjectItem(root, "allowed_commands"),
                      out->allowed_commands, &out->allowed_commands_count, MAX_POLICY_ENTRIES);

    val = cJSON_GetObjectItem(root, "network_policy_type");
    if (val && cJSON_IsString(val)) {
        out->network_policy = (strcmp(val->valuestring, "whitelist") == 0)
                                  ? POLICY_WHITELIST : POLICY_BLACKLIST;
    }
    load_ip_array(cJSON_GetObjectItem(root, "blocked_ips"),
                  out->blocked_ips, &out->blocked_ips_count, MAX_POLICY_ENTRIES);
    load_ip_array(cJSON_GetObjectItem(root, "allowed_ips"),
                  out->allowed_ips, &out->allowed_ips_count, MAX_POLICY_ENTRIES);
    load_string_array(cJSON_GetObjectItem(root, "blocked_domains"),
                      out->blocked_domains, &out->blocked_domains_count, MAX_POLICY_ENTRIES);
    load_string_array(cJSON_GetObjectItem(root, "allowed_domains"),
                      out->allowed_domains, &out->allowed_domains_count, MAX_POLICY_ENTRIES);

    val = cJSON_GetObjectItem(root, "strict_mode");
    if (val) out->strict_mode = cJSON_IsTrue(val);

    val = cJSON_GetObjectItem(root, "log_only");
    if (val) out->log_only = cJSON_IsTrue(val);

    val = cJSON_GetObjectItem(root, "inject_children");
    if (val) out->inject_children = cJSON_IsTrue(val);

    cJSON_Delete(root);
    collect_local_ips();
    return 0;
}

/* Wide-char to narrow for path matching */
static void wchar_to_utf8(const wchar_t *src, char *dst, int maxlen) {
    WideCharToMultiByte(CP_UTF8, 0, src, -1, dst, maxlen, NULL, NULL);
}

static void normalize_windows_path(const char *src, char *dst, size_t dstlen) {
    if (!src || !dst || dstlen == 0) return;
    strncpy(dst, src, dstlen - 1);
    dst[dstlen - 1] = '\0';

    for (char *p = dst; *p; p++) {
        if (*p == '/') *p = '\\';
    }

    if (_strnicmp(dst, "\\\\?\\", 4) == 0 || _strnicmp(dst, "\\\\.\\", 4) == 0) {
        memmove(dst, dst + 4, strlen(dst + 4) + 1);
    } else if (_strnicmp(dst, "\\??\\", 4) == 0) {
        memmove(dst, dst + 4, strlen(dst + 4) + 1);
    }

    size_t n = strlen(dst);
    while (n > 3 && (dst[n - 1] == '\\' || dst[n - 1] == '/')) {
        dst[n - 1] = '\0';
        n--;
    }
}

static int path_matches(const char *pattern, const char *path) {
    char norm_pattern[MAX_PATH_LEN];
    char norm_path[MAX_PATH_LEN];
    normalize_windows_path(pattern, norm_pattern, sizeof(norm_pattern));
    normalize_windows_path(path, norm_path, sizeof(norm_path));

    size_t plen = strlen(norm_pattern);
    if (plen == 0) return 0;
    if (_strnicmp(norm_path, norm_pattern, plen) != 0) return 0;

    /* Prefix boundary check: exact match OR next char is separator. */
    if (strlen(norm_path) == plen) return 1;
    char next = norm_path[plen];
    return next == '\\' || next == '/';
}

static int is_cmd_boundary_char(char c) {
    return c == '\0' || c == ' ' || c == '\t' || c == '\n' || c == '\r' ||
           c == '/' || c == '\\' || c == '"' || c == '\'' || c == '=' ||
           c == ':' || c == ';' || c == '|' || c == '&';
}

static int command_matches(const char *cmd, const char *pattern) {
    if (!cmd || !pattern) return 0;
    size_t plen = strlen(pattern);
    if (plen == 0) return 0;

    const char *cursor = cmd;
    while (*cursor) {
        if (_strnicmp(cursor, pattern, plen) == 0) {
            char left = (cursor == cmd) ? '\0' : cursor[-1];
            char right = cursor[plen];
            if (is_cmd_boundary_char(left) && is_cmd_boundary_char(right)) {
                return 1;
            }
        }
        cursor++;
    }
    return 0;
}

/* IP 通配符匹配：支持 10.0.*.* 这类模式 */
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

PolicyAction policy_check_file(const SandboxPolicy *p, const wchar_t *wpath) {
    if (!wpath) return ACTION_ALLOW;
    char path[MAX_PATH_LEN];
    wchar_to_utf8(wpath, path, MAX_PATH_LEN);

    if (p->file_policy == POLICY_WHITELIST) {
        for (int i = 0; i < p->allowed_paths_count; i++) {
            if (path_matches(p->allowed_paths[i], path)) return ACTION_ALLOW;
        }
        return p->log_only ? ACTION_LOG : ACTION_DENY;
    }

    /* Blacklist */
    for (int i = 0; i < p->blocked_paths_count; i++) {
        if (path_matches(p->blocked_paths[i], path)) {
            return p->log_only ? ACTION_LOG : ACTION_DENY;
        }
    }
    return ACTION_ALLOW;
}

PolicyAction policy_check_command(const SandboxPolicy *p, const wchar_t *wcmd) {
    if (!wcmd) return ACTION_ALLOW;
    char cmd[MAX_PATH_LEN];
    wchar_to_utf8(wcmd, cmd, MAX_PATH_LEN);
    _strlwr(cmd);

    if (p->command_policy == POLICY_WHITELIST) {
        for (int i = 0; i < p->allowed_commands_count; i++) {
            char lower[MAX_PATH_LEN];
            strncpy(lower, p->allowed_commands[i], MAX_PATH_LEN - 1);
            lower[MAX_PATH_LEN - 1] = '\0';
            _strlwr(lower);
            if (command_matches(cmd, lower)) return ACTION_ALLOW;
        }
        return p->log_only ? ACTION_LOG : ACTION_DENY;
    }

    for (int i = 0; i < p->blocked_commands_count; i++) {
        char lower[MAX_PATH_LEN];
        strncpy(lower, p->blocked_commands[i], MAX_PATH_LEN - 1);
        lower[MAX_PATH_LEN - 1] = '\0';
        _strlwr(lower);
        if (command_matches(cmd, lower)) {
            return p->log_only ? ACTION_LOG : ACTION_DENY;
        }
    }
    return ACTION_ALLOW;
}

PolicyAction policy_check_network(const SandboxPolicy *p, const char *ip, int port) {
    if (!ip) return ACTION_ALLOW;

    if (p->network_policy == POLICY_WHITELIST) {
        if (is_ip_in_list(ip)) return ACTION_ALLOW;
        for (int i = 0; i < p->allowed_ips_count; i++) {
            if (ip_pattern_matches(ip, p->allowed_ips[i])) return ACTION_ALLOW;
        }
        return p->log_only ? ACTION_LOG : ACTION_DENY;
    }

    for (int i = 0; i < p->blocked_ips_count; i++) {
        if (ip_pattern_matches(ip, p->blocked_ips[i])) {
            return p->log_only ? ACTION_LOG : ACTION_DENY;
        }
    }
    return ACTION_ALLOW;
}

static int domain_matches(const char *domain, const char *pattern) {
    if (!domain || !pattern) return 0;

    // 支持通配符子域名模式: *.example.com
    if (pattern[0] == '*' && pattern[1] == '.') {
        const char *suffix = pattern + 2;
        size_t dlen = strlen(domain);
        size_t slen = strlen(suffix);
        if (slen == 0 || dlen < slen) return 0;
        if (_stricmp(domain + (dlen - slen), suffix) != 0) return 0;

        // 允许匹配根域名和子域名，避免 "*.example.com" 规则遗漏 "example.com"
        if (dlen == slen) return 1;
        return domain[dlen - slen - 1] == '.';
    }

    if (_stricmp(domain, pattern) == 0) return 1;

    size_t dlen = strlen(domain);
    size_t plen = strlen(pattern);
    if (dlen > plen + 1) {
        const char *suffix = domain + (dlen - plen);
        if (suffix[-1] == '.' && _stricmp(suffix, pattern) == 0) return 1;
    }
    return 0;
}

static int is_numeric_ip(const char *host) {
    struct in_addr addr4;
    struct in6_addr addr6;
    return InetPtonA(AF_INET, host, &addr4) == 1 || InetPtonA(AF_INET6, host, &addr6) == 1;
}

PolicyAction policy_check_domain(const SandboxPolicy *p, const char *domain) {
    if (!p || !domain || domain[0] == '\0') return ACTION_ALLOW;
    if (is_numeric_ip(domain)) return ACTION_ALLOW;

    if (p->network_policy == POLICY_WHITELIST) {
        if (p->allowed_domains_count == 0 && p->allowed_ips_count == 0) return ACTION_ALLOW;
        for (int i = 0; i < p->allowed_domains_count; i++) {
            if (domain_matches(domain, p->allowed_domains[i])) return ACTION_ALLOW;
        }
        return p->log_only ? ACTION_LOG : ACTION_DENY;
    }

    for (int i = 0; i < p->blocked_domains_count; i++) {
        if (domain_matches(domain, p->blocked_domains[i])) {
            return p->log_only ? ACTION_LOG : ACTION_DENY;
        }
    }
    return ACTION_ALLOW;
}
