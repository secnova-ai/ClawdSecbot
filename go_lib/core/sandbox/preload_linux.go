package sandbox

import (
	"encoding/json"
)

// PreloadConfig LD_PRELOAD 沙箱策略配置，与 preload.c 的 JSON 格式兼容
type PreloadConfig struct {
	FilePolicyType    string   `json:"file_policy_type"`
	BlockedPaths      []string `json:"blocked_paths"`
	AllowedPaths      []string `json:"allowed_paths"`
	CommandPolicyType string   `json:"command_policy_type"`
	BlockedCommands   []string `json:"blocked_commands"`
	AllowedCommands   []string `json:"allowed_commands"`
	NetworkPolicyType string   `json:"network_policy_type"`
	BlockedIPs        []string `json:"blocked_ips"`
	AllowedIPs        []string `json:"allowed_ips"`
	BlockedDomains    []string `json:"blocked_domains"`
	AllowedDomains    []string `json:"allowed_domains"`
	GatewayBinaryPath string   `json:"gateway_binary_path,omitempty"`
	GatewayConfigPath string   `json:"gateway_config_path,omitempty"`
	StrictMode        bool     `json:"strict_mode"`
	LogOnly           bool     `json:"log_only"`
}

// ToPolicyJSON 将配置序列化为策略 JSON 字节
func (c *PreloadConfig) ToPolicyJSON() ([]byte, error) {
	return json.MarshalIndent(c, "", "  ")
}

// buildPreloadConfig 将 SandboxConfig 转换为 PreloadConfig (域名自动解析为 IP 提供双重拦截)
func buildPreloadConfig(config SandboxConfig) *PreloadConfig {
	config = normalizeSandboxConfig(config)

	pc := &PreloadConfig{
		FilePolicyType:    "blacklist",
		BlockedPaths:      []string{},
		AllowedPaths:      []string{},
		CommandPolicyType: "blacklist",
		BlockedCommands:   []string{},
		AllowedCommands:   []string{},
		NetworkPolicyType: "blacklist",
		BlockedIPs:        []string{},
		AllowedIPs:        []string{},
		BlockedDomains:    []string{},
		AllowedDomains:    []string{},
	}

	// 路径权限
	if config.PathPermission.Mode == ModeWhitelist {
		pc.FilePolicyType = "whitelist"
		pc.AllowedPaths = config.PathPermission.Paths
	} else {
		pc.FilePolicyType = "blacklist"
		pc.BlockedPaths = config.PathPermission.Paths
	}

	// 网络权限 (出栈): 分离域名和 IP，域名同时解析为 IP 提供双重拦截
	if config.NetworkPermission.Outbound.Mode == ModeWhitelist {
		pc.NetworkPolicyType = "whitelist"
		ips, domains := classifyAddresses(config.NetworkPermission.Outbound.Addresses)
		pc.AllowedIPs = ips
		pc.AllowedDomains = domains
	} else {
		pc.NetworkPolicyType = "blacklist"
		ips, domains := classifyAddresses(config.NetworkPermission.Outbound.Addresses)
		pc.BlockedIPs = ips
		pc.BlockedDomains = domains
	}

	pc.GatewayBinaryPath = config.GatewayBinaryPath
	pc.GatewayConfigPath = config.GatewayConfigPath

	// Shell 权限
	if config.ShellPermission.Mode == ModeWhitelist {
		pc.CommandPolicyType = "whitelist"
		pc.AllowedCommands = config.ShellPermission.Commands
	} else {
		pc.CommandPolicyType = "blacklist"
		pc.BlockedCommands = config.ShellPermission.Commands
	}

	return pc
}
