#include "hooks.h"
#include "policy.h"
#include <MinHook.h>
#include <stdio.h>
#include <ws2tcpip.h>

/* --- Globals --- */
static const SandboxPolicy *g_policy = NULL;
static wchar_t g_dll_path[MAX_PATH] = {0};
static FILE *g_log = NULL;

/* --- Original function pointers --- */
typedef HANDLE (WINAPI *PFN_CreateFileW)(
    LPCWSTR, DWORD, DWORD, LPSECURITY_ATTRIBUTES, DWORD, DWORD, HANDLE);
typedef BOOL (WINAPI *PFN_CreateProcessW)(
    LPCWSTR, LPWSTR, LPSECURITY_ATTRIBUTES, LPSECURITY_ATTRIBUTES,
    BOOL, DWORD, LPVOID, LPCWSTR, LPSTARTUPINFOW, LPPROCESS_INFORMATION);
typedef BOOL (WINAPI *PFN_DeleteFileW)(LPCWSTR);
typedef BOOL (WINAPI *PFN_RemoveDirectoryW)(LPCWSTR);
typedef BOOL (WINAPI *PFN_CreateDirectoryW)(LPCWSTR, LPSECURITY_ATTRIBUTES);
typedef BOOL (WINAPI *PFN_MoveFileW)(LPCWSTR, LPCWSTR);
typedef BOOL (WINAPI *PFN_MoveFileExW)(LPCWSTR, LPCWSTR, DWORD);
typedef HANDLE (WINAPI *PFN_FindFirstFileW)(LPCWSTR, LPWIN32_FIND_DATAW);
typedef HANDLE (WINAPI *PFN_FindFirstFileExW)(
    LPCWSTR, FINDEX_INFO_LEVELS, LPVOID, FINDEX_SEARCH_OPS, LPVOID, DWORD);
typedef int (WSAAPI *PFN_connect)(SOCKET, const struct sockaddr *, int);
typedef int (WSAAPI *PFN_WSAConnect)(SOCKET, const struct sockaddr *, int,
    LPWSABUF, LPWSABUF, LPQOS, LPQOS);
typedef int (WSAAPI *PFN_getaddrinfo)(PCSTR, PCSTR, const ADDRINFOA *, PADDRINFOA *);
typedef int (WSAAPI *PFN_GetAddrInfoW)(PCWSTR, PCWSTR, const ADDRINFOW *, PADDRINFOW *);

static PFN_CreateFileW    fpCreateFileW    = NULL;
static PFN_CreateProcessW fpCreateProcessW = NULL;
static PFN_DeleteFileW    fpDeleteFileW    = NULL;
static PFN_RemoveDirectoryW fpRemoveDirectoryW = NULL;
static PFN_CreateDirectoryW fpCreateDirectoryW = NULL;
static PFN_MoveFileW      fpMoveFileW      = NULL;
static PFN_MoveFileExW    fpMoveFileExW    = NULL;
static PFN_FindFirstFileW fpFindFirstFileW = NULL;
static PFN_FindFirstFileExW fpFindFirstFileExW = NULL;
static PFN_connect        fpConnect        = NULL;
static PFN_WSAConnect     fpWSAConnect     = NULL;
static PFN_getaddrinfo    fpGetAddrInfoA   = NULL;
static PFN_GetAddrInfoW   fpGetAddrInfoW   = NULL;

static void audit_log(const char *action, const char *detail);

// 路径操作检查顺序：
// 目录打开 -> 创建 -> 读取 -> 重命名 -> 写入 -> 删除
#define PATH_OP_DIR_OPEN (1U << 0)
#define PATH_OP_CREATE   (1U << 1)
#define PATH_OP_READ     (1U << 2)
#define PATH_OP_RENAME   (1U << 3)
#define PATH_OP_WRITE    (1U << 4)
#define PATH_OP_DELETE   (1U << 5)

// 按统一顺序选择主操作名称，用于跨平台一致审计。
static const char *select_primary_path_op(unsigned int opMask) {
    if (opMask & PATH_OP_DIR_OPEN) return "PATH-DIR-OPEN";
    if (opMask & PATH_OP_CREATE) return "PATH-CREATE";
    if (opMask & PATH_OP_READ) return "PATH-READ";
    if (opMask & PATH_OP_RENAME) return "PATH-RENAME";
    if (opMask & PATH_OP_WRITE) return "PATH-WRITE";
    if (opMask & PATH_OP_DELETE) return "PATH-DELETE";
    return "PATH-READ";
}

// 将 CreateFileW 参数映射为路径操作位图。
static unsigned int build_createfile_op_mask(DWORD desiredAccess, DWORD creationDisposition, DWORD flagsAndAttributes) {
    unsigned int opMask = 0;
    if (flagsAndAttributes & FILE_FLAG_BACKUP_SEMANTICS) {
        opMask |= PATH_OP_DIR_OPEN;
    }
    if (creationDisposition == CREATE_NEW ||
        creationDisposition == CREATE_ALWAYS ||
        creationDisposition == OPEN_ALWAYS) {
        opMask |= PATH_OP_CREATE;
    }
    if ((desiredAccess & (GENERIC_READ | FILE_GENERIC_READ | FILE_READ_DATA | FILE_LIST_DIRECTORY)) != 0) {
        opMask |= PATH_OP_READ;
    }
    if ((desiredAccess & (GENERIC_WRITE | FILE_GENERIC_WRITE | FILE_WRITE_DATA | FILE_APPEND_DATA)) != 0 ||
        creationDisposition == TRUNCATE_EXISTING ||
        creationDisposition == CREATE_ALWAYS) {
        opMask |= PATH_OP_WRITE;
    }
    return opMask;
}

static int hook_api_windows_core(const char *procName, LPVOID detour, LPVOID *original) {
    int hooked = 0;

    if (MH_CreateHookApi(L"kernel32", procName, detour, original) == MH_OK) {
        hooked = 1;
    }
    if (MH_CreateHookApi(L"KernelBase", procName, detour, original) == MH_OK) {
        hooked = 1;
    }

    if (!hooked) {
        char msg[160];
        snprintf(msg, sizeof(msg), "failed to hook %s (kernel32/KernelBase)", procName);
        audit_log("WARN", msg);
        return -1;
    }
    return 0;
}

/* --- Audit logging --- */
static void audit_log(const char *action, const char *detail) {
    if (!g_log) return;
    SYSTEMTIME st;
    GetLocalTime(&st);
    fprintf(g_log, "[%04d-%02d-%02d %02d:%02d:%02d] %s: %s\n",
            st.wYear, st.wMonth, st.wDay,
            st.wHour, st.wMinute, st.wSecond,
            action, detail);
    fflush(g_log);
}

// 输出统一沙箱事件日志，格式与 Linux 侧对齐:
// ACTION=<BLOCK|LOG_ONLY> TYPE=<...> TARGET=<...>
static void audit_log_event(const char *action, const char *type, const char *target) {
    if (!g_log) return;
    SYSTEMTIME st;
    GetLocalTime(&st);
    fprintf(g_log, "[%04d-%02d-%02d %02d:%02d:%02d] ACTION=%s TYPE=%s TARGET=%s\n",
            st.wYear, st.wMonth, st.wDay,
            st.wHour, st.wMinute, st.wSecond,
            action ? action : "UNKNOWN",
            type ? type : "",
            target ? target : "");
    fflush(g_log);
}

static void open_audit_log(void) {
    char path[MAX_PATH];
    DWORD len = GetEnvironmentVariableA("SANDBOX_LOG_FILE", path, MAX_PATH);
    if (len > 0 && len < MAX_PATH) {
        g_log = fopen(path, "a");
    }
}

/* --- Inject this DLL into a child process --- */
static void inject_into_child(HANDLE hProcess, DWORD pid) {
    if (g_dll_path[0] == L'\0') return;

    HMODULE hK32 = GetModuleHandleW(L"kernel32.dll");
    if (!hK32) return;

    FARPROC pLoadLib = GetProcAddress(hK32, "LoadLibraryW");
    if (!pLoadLib) return;

    size_t pathBytes = (wcslen(g_dll_path) + 1) * sizeof(wchar_t);
    LPVOID remoteMem = VirtualAllocEx(hProcess, NULL, pathBytes,
                                      MEM_COMMIT | MEM_RESERVE, PAGE_READWRITE);
    if (!remoteMem) return;

    if (!WriteProcessMemory(hProcess, remoteMem, g_dll_path, pathBytes, NULL)) {
        VirtualFreeEx(hProcess, remoteMem, 0, MEM_RELEASE);
        return;
    }

    HANDLE hThread = CreateRemoteThread(hProcess, NULL, 0,
                                        (LPTHREAD_START_ROUTINE)pLoadLib,
                                        remoteMem, 0, NULL);
    if (hThread) {
        WaitForSingleObject(hThread, 5000);
        CloseHandle(hThread);
    }

    char msg[128];
    snprintf(msg, sizeof(msg), "injected hook DLL into child PID %lu", (unsigned long)pid);
    audit_log("INJECT", msg);
}

/* --- Hook: CreateFileW --- */
static HANDLE WINAPI Hook_CreateFileW(
    LPCWSTR lpFileName, DWORD dwDesiredAccess, DWORD dwShareMode,
    LPSECURITY_ATTRIBUTES lpSA, DWORD dwCreation, DWORD dwFlags, HANDLE hTemplate)
{
    if (g_policy && lpFileName) {
        unsigned int opMask = build_createfile_op_mask(dwDesiredAccess, dwCreation, dwFlags);
        const char *opName = select_primary_path_op(opMask);
        PolicyAction act = policy_check_file(g_policy, lpFileName);
        if (act == ACTION_DENY) {
            char narrow[MAX_PATH];
            WideCharToMultiByte(CP_UTF8, 0, lpFileName, -1, narrow, MAX_PATH, NULL, NULL);
            audit_log_event("BLOCK", opName, narrow);
            SetLastError(ERROR_ACCESS_DENIED);
            return INVALID_HANDLE_VALUE;
        }
        if (act == ACTION_LOG) {
            char narrow[MAX_PATH];
            WideCharToMultiByte(CP_UTF8, 0, lpFileName, -1, narrow, MAX_PATH, NULL, NULL);
            audit_log_event("LOG_ONLY", opName, narrow);
        }
    }
    return fpCreateFileW(lpFileName, dwDesiredAccess, dwShareMode,
                         lpSA, dwCreation, dwFlags, hTemplate);
}

/* --- Hook: DeleteFileW --- */
static BOOL WINAPI Hook_DeleteFileW(LPCWSTR lpFileName) {
    if (g_policy && lpFileName) {
        PolicyAction act = policy_check_file(g_policy, lpFileName);
        if (act == ACTION_DENY) {
            char narrow[MAX_PATH];
            WideCharToMultiByte(CP_UTF8, 0, lpFileName, -1, narrow, MAX_PATH, NULL, NULL);
            audit_log_event("BLOCK", "PATH-DELETE", narrow);
            SetLastError(ERROR_ACCESS_DENIED);
            return FALSE;
        }
        if (act == ACTION_LOG) {
            char narrow[MAX_PATH];
            WideCharToMultiByte(CP_UTF8, 0, lpFileName, -1, narrow, MAX_PATH, NULL, NULL);
            audit_log_event("LOG_ONLY", "PATH-DELETE", narrow);
        }
    }
    return fpDeleteFileW(lpFileName);
}

/* --- Hook: RemoveDirectoryW --- */
static BOOL WINAPI Hook_RemoveDirectoryW(LPCWSTR lpPathName) {
    if (g_policy && lpPathName) {
        PolicyAction act = policy_check_file(g_policy, lpPathName);
        if (act == ACTION_DENY) {
            char narrow[MAX_PATH];
            WideCharToMultiByte(CP_UTF8, 0, lpPathName, -1, narrow, MAX_PATH, NULL, NULL);
            audit_log_event("BLOCK", "PATH-DELETE", narrow);
            SetLastError(ERROR_ACCESS_DENIED);
            return FALSE;
        }
        if (act == ACTION_LOG) {
            char narrow[MAX_PATH];
            WideCharToMultiByte(CP_UTF8, 0, lpPathName, -1, narrow, MAX_PATH, NULL, NULL);
            audit_log_event("LOG_ONLY", "PATH-DELETE", narrow);
        }
    }
    return fpRemoveDirectoryW(lpPathName);
}

/* --- Hook: CreateDirectoryW --- */
static BOOL WINAPI Hook_CreateDirectoryW(LPCWSTR lpPathName, LPSECURITY_ATTRIBUTES lpSecurityAttributes) {
    if (g_policy && lpPathName) {
        PolicyAction act = policy_check_file(g_policy, lpPathName);
        if (act == ACTION_DENY) {
            char narrow[MAX_PATH];
            WideCharToMultiByte(CP_UTF8, 0, lpPathName, -1, narrow, MAX_PATH, NULL, NULL);
            audit_log_event("BLOCK", "PATH-CREATE", narrow);
            SetLastError(ERROR_ACCESS_DENIED);
            return FALSE;
        }
        if (act == ACTION_LOG) {
            char narrow[MAX_PATH];
            WideCharToMultiByte(CP_UTF8, 0, lpPathName, -1, narrow, MAX_PATH, NULL, NULL);
            audit_log_event("LOG_ONLY", "PATH-CREATE", narrow);
        }
    }
    return fpCreateDirectoryW(lpPathName, lpSecurityAttributes);
}

/* --- Hook: MoveFileW --- */
static BOOL WINAPI Hook_MoveFileW(LPCWSTR lpExistingFileName, LPCWSTR lpNewFileName) {
    if (g_policy) {
        if (lpExistingFileName) {
            PolicyAction srcAct = policy_check_file(g_policy, lpExistingFileName);
            if (srcAct == ACTION_DENY) {
                char narrow[MAX_PATH];
                WideCharToMultiByte(CP_UTF8, 0, lpExistingFileName, -1, narrow, MAX_PATH, NULL, NULL);
                audit_log_event("BLOCK", "PATH-RENAME", narrow);
                SetLastError(ERROR_ACCESS_DENIED);
                return FALSE;
            }
        }
        if (lpNewFileName) {
            PolicyAction dstAct = policy_check_file(g_policy, lpNewFileName);
            if (dstAct == ACTION_DENY) {
                char narrow[MAX_PATH];
                WideCharToMultiByte(CP_UTF8, 0, lpNewFileName, -1, narrow, MAX_PATH, NULL, NULL);
                audit_log_event("BLOCK", "PATH-RENAME", narrow);
                SetLastError(ERROR_ACCESS_DENIED);
                return FALSE;
            }
        }
    }
    return fpMoveFileW(lpExistingFileName, lpNewFileName);
}

/* --- Hook: MoveFileExW --- */
static BOOL WINAPI Hook_MoveFileExW(LPCWSTR lpExistingFileName, LPCWSTR lpNewFileName, DWORD dwFlags) {
    if (g_policy) {
        if (lpExistingFileName) {
            PolicyAction srcAct = policy_check_file(g_policy, lpExistingFileName);
            if (srcAct == ACTION_DENY) {
                char narrow[MAX_PATH];
                WideCharToMultiByte(CP_UTF8, 0, lpExistingFileName, -1, narrow, MAX_PATH, NULL, NULL);
                audit_log_event("BLOCK", "PATH-RENAME", narrow);
                SetLastError(ERROR_ACCESS_DENIED);
                return FALSE;
            }
        }
        if (lpNewFileName) {
            PolicyAction dstAct = policy_check_file(g_policy, lpNewFileName);
            if (dstAct == ACTION_DENY) {
                char narrow[MAX_PATH];
                WideCharToMultiByte(CP_UTF8, 0, lpNewFileName, -1, narrow, MAX_PATH, NULL, NULL);
                audit_log_event("BLOCK", "PATH-RENAME", narrow);
                SetLastError(ERROR_ACCESS_DENIED);
                return FALSE;
            }
        }
    }
    return fpMoveFileExW(lpExistingFileName, lpNewFileName, dwFlags);
}

/* --- Hook: FindFirstFileW --- */
static HANDLE WINAPI Hook_FindFirstFileW(LPCWSTR lpFileName, LPWIN32_FIND_DATAW lpFindFileData) {
    if (g_policy && lpFileName) {
        PolicyAction act = policy_check_file(g_policy, lpFileName);
        if (act == ACTION_DENY) {
            char narrow[MAX_PATH];
            WideCharToMultiByte(CP_UTF8, 0, lpFileName, -1, narrow, MAX_PATH, NULL, NULL);
            audit_log_event("BLOCK", "PATH-DIR-OPEN", narrow);
            SetLastError(ERROR_ACCESS_DENIED);
            return INVALID_HANDLE_VALUE;
        }
        if (act == ACTION_LOG) {
            char narrow[MAX_PATH];
            WideCharToMultiByte(CP_UTF8, 0, lpFileName, -1, narrow, MAX_PATH, NULL, NULL);
            audit_log_event("LOG_ONLY", "PATH-DIR-OPEN", narrow);
        }
    }
    return fpFindFirstFileW(lpFileName, lpFindFileData);
}

/* --- Hook: FindFirstFileExW --- */
static HANDLE WINAPI Hook_FindFirstFileExW(
    LPCWSTR lpFileName, FINDEX_INFO_LEVELS fInfoLevelId, LPVOID lpFindFileData,
    FINDEX_SEARCH_OPS fSearchOp, LPVOID lpSearchFilter, DWORD dwAdditionalFlags)
{
    if (g_policy && lpFileName) {
        PolicyAction act = policy_check_file(g_policy, lpFileName);
        if (act == ACTION_DENY) {
            char narrow[MAX_PATH];
            WideCharToMultiByte(CP_UTF8, 0, lpFileName, -1, narrow, MAX_PATH, NULL, NULL);
            audit_log_event("BLOCK", "PATH-DIR-OPEN", narrow);
            SetLastError(ERROR_ACCESS_DENIED);
            return INVALID_HANDLE_VALUE;
        }
        if (act == ACTION_LOG) {
            char narrow[MAX_PATH];
            WideCharToMultiByte(CP_UTF8, 0, lpFileName, -1, narrow, MAX_PATH, NULL, NULL);
            audit_log_event("LOG_ONLY", "PATH-DIR-OPEN", narrow);
        }
    }
    return fpFindFirstFileExW(lpFileName, fInfoLevelId, lpFindFileData, fSearchOp, lpSearchFilter, dwAdditionalFlags);
}

/* --- Hook: CreateProcessW --- */
static BOOL WINAPI Hook_CreateProcessW(
    LPCWSTR lpAppName, LPWSTR lpCmdLine,
    LPSECURITY_ATTRIBUTES lpPA, LPSECURITY_ATTRIBUTES lpTA,
    BOOL bInherit, DWORD dwFlags, LPVOID lpEnv,
    LPCWSTR lpDir, LPSTARTUPINFOW lpSI, LPPROCESS_INFORMATION lpPI)
{
    if (g_policy) {
        const wchar_t *check = lpCmdLine ? lpCmdLine : lpAppName;
        PolicyAction act = policy_check_command(g_policy, check);
        if (act == ACTION_DENY) {
            char narrow[MAX_PATH];
            WideCharToMultiByte(CP_UTF8, 0, check ? check : L"(null)", -1,
                                narrow, MAX_PATH, NULL, NULL);
            audit_log_event("BLOCK", "CMD", narrow);
            SetLastError(ERROR_ACCESS_DENIED);
            return FALSE;
        }
        if (act == ACTION_LOG) {
            char narrow[MAX_PATH];
            WideCharToMultiByte(CP_UTF8, 0, check ? check : L"(null)", -1,
                                narrow, MAX_PATH, NULL, NULL);
            audit_log_event("LOG_ONLY", "CMD", narrow);
        }
    }

    DWORD realFlags = dwFlags;
    BOOL needResume = FALSE;
    if (g_policy && g_policy->inject_children && !(dwFlags & CREATE_SUSPENDED)) {
        realFlags |= CREATE_SUSPENDED;
        needResume = TRUE;
    }

    BOOL ok = fpCreateProcessW(lpAppName, lpCmdLine, lpPA, lpTA,
                               bInherit, realFlags, lpEnv, lpDir, lpSI, lpPI);
    if (!ok) return FALSE;

    if (g_policy && g_policy->inject_children && lpPI && lpPI->hProcess) {
        inject_into_child(lpPI->hProcess, lpPI->dwProcessId);
    }

    if (needResume && lpPI && lpPI->hThread) {
        ResumeThread(lpPI->hThread);
    }

    return TRUE;
}

/* --- Hook: connect (Winsock) --- */
static int WSAAPI Hook_connect(SOCKET s, const struct sockaddr *name, int namelen) {
    if (g_policy && name) {
        char ip[64] = {0};
        int port = 0;
        if (name->sa_family == AF_INET) {
            struct sockaddr_in *sin = (struct sockaddr_in *)name;
            inet_ntop(AF_INET, &sin->sin_addr, ip, sizeof(ip));
            port = ntohs(sin->sin_port);
        } else if (name->sa_family == AF_INET6) {
            struct sockaddr_in6 *sin6 = (struct sockaddr_in6 *)name;
            inet_ntop(AF_INET6, &sin6->sin6_addr, ip, sizeof(ip));
            port = ntohs(sin6->sin6_port);
        }
        if (ip[0]) {
            PolicyAction act = policy_check_network(g_policy, ip, port);
            if (act == ACTION_DENY) {
                char msg[128];
                snprintf(msg, sizeof(msg), "%s:%d", ip, port);
                audit_log_event("BLOCK", "NET", msg);
                WSASetLastError(WSAEACCES);
                return SOCKET_ERROR;
            }
            if (act == ACTION_LOG) {
                char msg[128];
                snprintf(msg, sizeof(msg), "%s:%d", ip, port);
                audit_log_event("LOG_ONLY", "NET", msg);
            }
        }
    }
    return fpConnect(s, name, namelen);
}

/* --- Hook: WSAConnect --- */
static int WSAAPI Hook_WSAConnect(
    SOCKET s, const struct sockaddr *name, int namelen,
    LPWSABUF lpCallerData, LPWSABUF lpCalleeData,
    LPQOS lpSQOS, LPQOS lpGQOS)
{
    if (g_policy && name) {
        char ip[64] = {0};
        int port = 0;
        if (name->sa_family == AF_INET) {
            struct sockaddr_in *sin = (struct sockaddr_in *)name;
            inet_ntop(AF_INET, &sin->sin_addr, ip, sizeof(ip));
            port = ntohs(sin->sin_port);
        }
        if (ip[0]) {
            PolicyAction act = policy_check_network(g_policy, ip, port);
            if (act == ACTION_DENY) {
                char msg[128];
                snprintf(msg, sizeof(msg), "%s:%d (WSAConnect)", ip, port);
                audit_log_event("BLOCK", "NET", msg);
                WSASetLastError(WSAEACCES);
                return SOCKET_ERROR;
            }
        }
    }
    return fpWSAConnect(s, name, namelen, lpCallerData, lpCalleeData, lpSQOS, lpGQOS);
}

/* --- Hook: getaddrinfo (ANSI) --- */
static int WSAAPI Hook_getaddrinfo(
    PCSTR pNodeName, PCSTR pServiceName, const ADDRINFOA *pHints, PADDRINFOA *ppResult)
{
    if (g_policy && pNodeName) {
        PolicyAction act = policy_check_domain(g_policy, pNodeName);
        if (act == ACTION_DENY) {
            audit_log_event("BLOCK", "DNS", pNodeName);
            return EAI_FAIL;
        }
        if (act == ACTION_LOG) {
            audit_log_event("LOG_ONLY", "DNS", pNodeName);
        }
    }
    return fpGetAddrInfoA(pNodeName, pServiceName, pHints, ppResult);
}

/* --- Hook: GetAddrInfoW (Unicode) --- */
static int WSAAPI Hook_GetAddrInfoW(
    PCWSTR pNodeName, PCWSTR pServiceName, const ADDRINFOW *pHints, PADDRINFOW *ppResult)
{
    if (g_policy && pNodeName) {
        char nodeUtf8[MAX_PATH] = {0};
        WideCharToMultiByte(CP_UTF8, 0, pNodeName, -1, nodeUtf8, MAX_PATH, NULL, NULL);

        PolicyAction act = policy_check_domain(g_policy, nodeUtf8);
        if (act == ACTION_DENY) {
            audit_log_event("BLOCK", "DNS", nodeUtf8);
            return EAI_FAIL;
        }
        if (act == ACTION_LOG) {
            audit_log_event("LOG_ONLY", "DNS", nodeUtf8);
        }
    }
    return fpGetAddrInfoW(pNodeName, pServiceName, pHints, ppResult);
}

/* --- Public API --- */

const wchar_t *hooks_get_dll_path(void) {
    return g_dll_path;
}

int hooks_install(const SandboxPolicy *policy) {
    g_policy = policy;

    /* Resolve our own DLL path for child injection */
    HMODULE hSelf = NULL;
    GetModuleHandleExW(GET_MODULE_HANDLE_EX_FLAG_FROM_ADDRESS |
                       GET_MODULE_HANDLE_EX_FLAG_UNCHANGED_REFCOUNT,
                       (LPCWSTR)hooks_install, &hSelf);
    if (hSelf) {
        GetModuleFileNameW(hSelf, g_dll_path, MAX_PATH);
    }

    open_audit_log();
    audit_log("INIT", "sandbox hook initializing");

    if (MH_Initialize() != MH_OK) {
        audit_log("ERROR", "MH_Initialize failed");
        return -1;
    }

    /* File/process APIs (try both kernel32 + KernelBase) */
    hook_api_windows_core("CreateFileW", (LPVOID)Hook_CreateFileW, (LPVOID *)&fpCreateFileW);
    hook_api_windows_core("DeleteFileW", (LPVOID)Hook_DeleteFileW, (LPVOID *)&fpDeleteFileW);
    hook_api_windows_core("RemoveDirectoryW", (LPVOID)Hook_RemoveDirectoryW, (LPVOID *)&fpRemoveDirectoryW);
    hook_api_windows_core("CreateDirectoryW", (LPVOID)Hook_CreateDirectoryW, (LPVOID *)&fpCreateDirectoryW);
    hook_api_windows_core("MoveFileW", (LPVOID)Hook_MoveFileW, (LPVOID *)&fpMoveFileW);
    hook_api_windows_core("MoveFileExW", (LPVOID)Hook_MoveFileExW, (LPVOID *)&fpMoveFileExW);
    hook_api_windows_core("FindFirstFileW", (LPVOID)Hook_FindFirstFileW, (LPVOID *)&fpFindFirstFileW);
    hook_api_windows_core("FindFirstFileExW", (LPVOID)Hook_FindFirstFileExW, (LPVOID *)&fpFindFirstFileExW);
    hook_api_windows_core("CreateProcessW", (LPVOID)Hook_CreateProcessW, (LPVOID *)&fpCreateProcessW);

    /* connect (ws2_32) */
    if (MH_CreateHookApi(L"ws2_32", "connect",
                         (LPVOID)Hook_connect, (LPVOID *)&fpConnect) != MH_OK) {
        audit_log("WARN", "failed to hook connect");
    }

    /* WSAConnect (ws2_32) */
    if (MH_CreateHookApi(L"ws2_32", "WSAConnect",
                         (LPVOID)Hook_WSAConnect, (LPVOID *)&fpWSAConnect) != MH_OK) {
        audit_log("WARN", "failed to hook WSAConnect");
    }

    /* getaddrinfo (ws2_32) */
    if (MH_CreateHookApi(L"ws2_32", "getaddrinfo",
                         (LPVOID)Hook_getaddrinfo, (LPVOID *)&fpGetAddrInfoA) != MH_OK) {
        audit_log("WARN", "failed to hook getaddrinfo");
    }

    /* GetAddrInfoW (ws2_32) */
    if (MH_CreateHookApi(L"ws2_32", "GetAddrInfoW",
                         (LPVOID)Hook_GetAddrInfoW, (LPVOID *)&fpGetAddrInfoW) != MH_OK) {
        audit_log("WARN", "failed to hook GetAddrInfoW");
    }

    if (MH_EnableHook(MH_ALL_HOOKS) != MH_OK) {
        audit_log("ERROR", "MH_EnableHook failed");
        MH_Uninitialize();
        return -2;
    }

    audit_log("INIT", "all hooks installed successfully");
    return 0;
}

void hooks_remove(void) {
    MH_DisableHook(MH_ALL_HOOKS);
    MH_Uninitialize();
    audit_log("CLEANUP", "hooks removed");
    if (g_log) {
        fclose(g_log);
        g_log = NULL;
    }
}
