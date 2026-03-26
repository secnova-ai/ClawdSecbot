package sandbox

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"go_lib/core/logging"
)

// PermissionMode defines whitelist or blacklist mode
type PermissionMode string

const (
	ModeWhitelist PermissionMode = "whitelist"
	ModeBlacklist PermissionMode = "blacklist"
)

// PathPermissionConfig defines path access permissions
type PathPermissionConfig struct {
	Mode  PermissionMode `json:"mode"`
	Paths []string       `json:"paths"`
}

// DirectionalNetworkConfig defines network config for a single direction (inbound or outbound)
type DirectionalNetworkConfig struct {
	Mode      PermissionMode `json:"mode"`
	Addresses []string       `json:"addresses"`
}

// NetworkPermissionConfig defines network access permissions with inbound/outbound separation
// - Outbound: controls connections initiated by the process -> network-outbound + (remote ip ...)
// - Inbound: controls connections to the process -> network-inbound + (local ip ...)
type NetworkPermissionConfig struct {
	Inbound  DirectionalNetworkConfig `json:"inbound"`
	Outbound DirectionalNetworkConfig `json:"outbound"`
}

// ShellPermissionConfig defines shell command permissions
type ShellPermissionConfig struct {
	Mode     PermissionMode `json:"mode"`
	Commands []string       `json:"commands"`
}

// SandboxConfig contains all sandbox configuration
type SandboxConfig struct {
	AssetName         string                  `json:"asset_name"`
	GatewayBinaryPath string                  `json:"gateway_binary_path"`
	GatewayConfigPath string                  `json:"gateway_config_path"`
	PathPermission    PathPermissionConfig    `json:"path_permission"`
	NetworkPermission NetworkPermissionConfig `json:"network_permission"`
	ShellPermission   ShellPermissionConfig   `json:"shell_permission"`
}

// 规范化沙箱配置，保证三平台策略生成行为一致。
func normalizeSandboxConfig(config SandboxConfig) SandboxConfig {
	config.PathPermission.Mode = normalizePermissionMode(config.PathPermission.Mode)
	config.NetworkPermission.Inbound.Mode = normalizePermissionMode(config.NetworkPermission.Inbound.Mode)
	config.NetworkPermission.Outbound.Mode = normalizePermissionMode(config.NetworkPermission.Outbound.Mode)
	config.ShellPermission.Mode = normalizePermissionMode(config.ShellPermission.Mode)

	config.PathPermission.Paths = normalizePathEntries(config.PathPermission.Paths)
	config.NetworkPermission.Inbound.Addresses = normalizeNetworkEntries(config.NetworkPermission.Inbound.Addresses)
	config.NetworkPermission.Outbound.Addresses = normalizeNetworkEntries(config.NetworkPermission.Outbound.Addresses)
	config.ShellPermission.Commands = normalizeCommandEntries(config.ShellPermission.Commands)

	config.GatewayBinaryPath = normalizePathValue(config.GatewayBinaryPath)
	config.GatewayConfigPath = normalizePathValue(config.GatewayConfigPath)
	return config
}

// 统一权限模式默认值，异常值回退为黑名单模式。
func normalizePermissionMode(mode PermissionMode) PermissionMode {
	if mode == ModeWhitelist {
		return ModeWhitelist
	}
	return ModeBlacklist
}

// 规范化单个路径值。
func normalizePathValue(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	return filepath.Clean(expandPath(trimmed))
}

// 规范化路径列表：去空、展开、规整、去重。
func normalizePathEntries(paths []string) []string {
	return normalizeUnique(paths, func(v string) string {
		return normalizePathValue(v)
	})
}

// 规范化命令列表：去空、转小写、去重。
func normalizeCommandEntries(commands []string) []string {
	return normalizeUnique(commands, func(v string) string {
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return ""
		}
		return strings.ToLower(trimmed)
	})
}

// 规范化网络地址列表：去空、转小写、去重。
func normalizeNetworkEntries(addresses []string) []string {
	return normalizeUnique(addresses, func(v string) string {
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return ""
		}
		return strings.ToLower(trimmed)
	})
}

// 通用列表规范化与稳定去重。
func normalizeUnique(values []string, transform func(string) string) []string {
	if len(values) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		normalized := transform(value)
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

// SandboxStatus represents the current sandbox status
type SandboxStatus struct {
	Running          bool   `json:"running"`
	ManagedPID       int    `json:"managed_pid"`
	PolicyPath       string `json:"policy_path"`
	AssetName        string `json:"asset_name"`
	GatewayBinary    string `json:"gateway_binary"`
	ErrorMessage     string `json:"error,omitempty"`
	SandboxSupported bool   `json:"sandbox_supported"`
}

// 以下为跨平台共享的辅助函数

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(homeDir, path[2:])
	}
	return path
}

func sanitizeAssetName(name string) string {
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		" ", "_",
		".", "_",
		":", "_",
	)
	return replacer.Replace(name)
}

// SanitizeAssetNamePublic is the exported version of sanitizeAssetName
func SanitizeAssetNamePublic(name string) string {
	return sanitizeAssetName(name)
}

// isDomainName checks whether an address string is a domain name (not an IP)
func isDomainName(addr string) bool {
	host := addr
	if h, _, err := net.SplitHostPort(addr); err == nil {
		host = h
	}
	if net.ParseIP(host) != nil {
		return false
	}
	return strings.Contains(host, ".")
}

// resolveDomainsToIPs resolves a list of domain names to IP addresses
func resolveDomainsToIPs(domains []string) []string {
	var ips []string
	for _, domain := range domains {
		host := domain
		if h, _, err := net.SplitHostPort(domain); err == nil {
			host = h
		}
		resolved, err := net.LookupHost(host)
		if err != nil {
			logging.Warning("[Sandbox] DNS resolve failed for %s: %v", domain, err)
			continue
		}
		ips = append(ips, resolved...)
		logging.Info("[Sandbox] Resolved %s -> %v", domain, resolved)
	}
	return ips
}

// classifyAddresses splits addresses into IPs and domains, resolving domains to IPs as well
func classifyAddresses(addresses []string) (ips []string, domains []string) {
	for _, addr := range addresses {
		if isDomainName(addr) {
			domains = append(domains, addr)
		} else {
			ips = append(ips, addr)
		}
	}
	resolvedIPs := resolveDomainsToIPs(domains)
	ips = append(ips, resolvedIPs...)
	return
}

func resolveCommandPath(cmd string) string {
	if filepath.IsAbs(cmd) {
		return cmd
	}
	searchPaths := []string{"/usr/bin", "/usr/local/bin", "/bin", "/sbin", "/usr/sbin"}
	if runtime.GOOS == "windows" {
		systemRoot := os.Getenv("SystemRoot")
		if systemRoot == "" {
			systemRoot = `C:\Windows`
		}
		searchPaths = []string{
			filepath.Join(systemRoot, "System32"),
			filepath.Join(systemRoot),
			filepath.Join(os.Getenv("ProgramFiles")),
			filepath.Join(os.Getenv("ProgramFiles(x86)")),
		}
	}
	for _, dir := range searchPaths {
		fullPath := filepath.Join(dir, cmd)
		if _, err := os.Stat(fullPath); err == nil {
			return fullPath
		}
	}
	return ""
}
