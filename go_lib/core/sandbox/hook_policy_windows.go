//go:build windows

package sandbox

import "encoding/json"

// HookConfig defines the policy format for the Windows MinHook sandbox DLL.
// Compatible with the Linux PreloadConfig so both platforms share the same policy model.
type HookConfig struct {
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
	StrictMode        bool     `json:"strict_mode"`
	LogOnly           bool     `json:"log_only"`
	InjectChildren    bool     `json:"inject_children"`
}

// ToPolicyJSON serializes the config to JSON
func (c *HookConfig) ToPolicyJSON() ([]byte, error) {
	return json.MarshalIndent(c, "", "  ")
}

// buildHookConfig converts SandboxConfig to HookConfig for Windows
func buildHookConfig(config SandboxConfig) *HookConfig {
	config = normalizeSandboxConfig(config)

	hc := &HookConfig{
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
		InjectChildren:    true,
	}

	if config.PathPermission.Mode == ModeWhitelist {
		hc.FilePolicyType = "whitelist"
		hc.AllowedPaths = config.PathPermission.Paths
	} else {
		hc.FilePolicyType = "blacklist"
		hc.BlockedPaths = config.PathPermission.Paths
	}

	if config.NetworkPermission.Outbound.Mode == ModeWhitelist {
		hc.NetworkPolicyType = "whitelist"
		ips, domains := classifyAddresses(config.NetworkPermission.Outbound.Addresses)
		hc.AllowedIPs = ips
		hc.AllowedDomains = domains
	} else {
		hc.NetworkPolicyType = "blacklist"
		ips, domains := classifyAddresses(config.NetworkPermission.Outbound.Addresses)
		hc.BlockedIPs = ips
		hc.BlockedDomains = domains
	}

	if config.ShellPermission.Mode == ModeWhitelist {
		hc.CommandPolicyType = "whitelist"
		hc.AllowedCommands = config.ShellPermission.Commands
	} else {
		hc.CommandPolicyType = "blacklist"
		hc.BlockedCommands = config.ShellPermission.Commands
	}

	return hc
}
