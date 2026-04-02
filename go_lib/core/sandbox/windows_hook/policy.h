#ifndef SANDBOX_POLICY_H
#define SANDBOX_POLICY_H

#include <stdbool.h>
#include <wchar.h>

#define MAX_POLICY_ENTRIES 256
#define MAX_PATH_LEN 1024

typedef enum {
    POLICY_BLACKLIST,
    POLICY_WHITELIST
} PolicyType;

typedef struct {
    PolicyType file_policy;
    char blocked_paths[MAX_POLICY_ENTRIES][MAX_PATH_LEN];
    int blocked_paths_count;
    char allowed_paths[MAX_POLICY_ENTRIES][MAX_PATH_LEN];
    int allowed_paths_count;

    PolicyType command_policy;
    char blocked_commands[MAX_POLICY_ENTRIES][MAX_PATH_LEN];
    int blocked_commands_count;
    char allowed_commands[MAX_POLICY_ENTRIES][MAX_PATH_LEN];
    int allowed_commands_count;

    PolicyType network_policy;
    char blocked_ips[MAX_POLICY_ENTRIES][64];
    int blocked_ips_count;
    char allowed_ips[MAX_POLICY_ENTRIES][64];
    int allowed_ips_count;
    char blocked_domains[MAX_POLICY_ENTRIES][MAX_PATH_LEN];
    int blocked_domains_count;
    char allowed_domains[MAX_POLICY_ENTRIES][MAX_PATH_LEN];
    int allowed_domains_count;

    bool strict_mode;
    bool log_only;
    bool inject_children;
} SandboxPolicy;

typedef enum {
    ACTION_ALLOW,
    ACTION_DENY,
    ACTION_LOG
} PolicyAction;

/* Load policy from a JSON file. Returns 0 on success. */
int policy_load(const char *path, SandboxPolicy *out);

/* Check if a file path is allowed by the policy. */
PolicyAction policy_check_file(const SandboxPolicy *p, const wchar_t *path);

/* Check if a command line is allowed by the policy. */
PolicyAction policy_check_command(const SandboxPolicy *p, const wchar_t *cmdline);

/* Check if a network address is allowed by the policy. */
PolicyAction policy_check_network(const SandboxPolicy *p, const char *ip, int port);

/* Check if a domain name is allowed by the policy. */
PolicyAction policy_check_domain(const SandboxPolicy *p, const char *domain);

#endif /* SANDBOX_POLICY_H */
