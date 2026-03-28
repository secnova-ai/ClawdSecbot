package sandbox

import (
	"testing"
)

func TestIsDomainName(t *testing.T) {
	tests := []struct {
		addr string
		want bool
	}{
		{"192.168.1.1", false},
		{"10.0.0.1", false},
		{"127.0.0.1", false},
		{"::1", false},
		{"2001:db8::1", false},
		{"www.baidu.com", true},
		{"example.com", true},
		{"sub.domain.example.com", true},
		{"192.168.1.1:8080", false},
		{"www.baidu.com:443", true},
		{"*.example.com", true},
		{"10.0.*.*", false},
		{"192.168.1.10:8080", false},
		{"localhost", false},
		{"", false},
		{"*", false},
	}

	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			if got := isDomainName(tt.addr); got != tt.want {
				t.Errorf("isDomainName(%q) = %v, want %v", tt.addr, got, tt.want)
			}
		})
	}
}

func TestClassifyAddresses_NormalizeAndSplit(t *testing.T) {
	ips, domains := classifyAddresses([]string{
		"10.0.*.*",
		"192.168.1.10:8080",
		"EXAMPLE.COM:443",
		"*.Example.com",
		"10.0.*.*",
	})

	if !containsAll(ips, []string{"10.0.*.*", "192.168.1.10"}) {
		t.Fatalf("ips = %v, want contain [10.0.*.* 192.168.1.10]", ips)
	}
	if !containsAll(domains, []string{"example.com", "*.example.com"}) {
		t.Fatalf("domains = %v, want contain [example.com *.example.com]", domains)
	}
}

func TestBuildPreloadConfig_PathPermission(t *testing.T) {
	tests := []struct {
		name        string
		mode        PermissionMode
		paths       []string
		wantType    string
		wantBlocked []string
		wantAllowed []string
	}{
		{
			name:        "blacklist mode",
			mode:        ModeBlacklist,
			paths:       []string{"/etc/passwd", "/home/secret"},
			wantType:    "blacklist",
			wantBlocked: []string{"/etc/passwd", "/home/secret"},
			wantAllowed: []string{},
		},
		{
			name:        "whitelist mode",
			mode:        ModeWhitelist,
			paths:       []string{"/tmp", "/var/log"},
			wantType:    "whitelist",
			wantBlocked: []string{},
			wantAllowed: []string{"/tmp", "/var/log"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := SandboxConfig{
				PathPermission: PathPermissionConfig{
					Mode:  tt.mode,
					Paths: tt.paths,
				},
			}
			pc := buildPreloadConfig(cfg)

			if pc.FilePolicyType != tt.wantType {
				t.Errorf("FilePolicyType = %q, want %q", pc.FilePolicyType, tt.wantType)
			}
			if !stringSliceEqual(pc.BlockedPaths, tt.wantBlocked) {
				t.Errorf("BlockedPaths = %v, want %v", pc.BlockedPaths, tt.wantBlocked)
			}
			if !stringSliceEqual(pc.AllowedPaths, tt.wantAllowed) {
				t.Errorf("AllowedPaths = %v, want %v", pc.AllowedPaths, tt.wantAllowed)
			}
		})
	}
}

func TestBuildPreloadConfig_CommandPermission(t *testing.T) {
	tests := []struct {
		name        string
		mode        PermissionMode
		commands    []string
		wantType    string
		wantBlocked []string
		wantAllowed []string
	}{
		{
			name:        "blacklist shell commands",
			mode:        ModeBlacklist,
			commands:    []string{"rm -rf", "curl"},
			wantType:    "blacklist",
			wantBlocked: []string{"rm -rf", "curl"},
			wantAllowed: []string{},
		},
		{
			name:        "whitelist shell commands",
			mode:        ModeWhitelist,
			commands:    []string{"ls", "cat"},
			wantType:    "whitelist",
			wantBlocked: []string{},
			wantAllowed: []string{"ls", "cat"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := SandboxConfig{
				ShellPermission: ShellPermissionConfig{
					Mode:     tt.mode,
					Commands: tt.commands,
				},
			}
			pc := buildPreloadConfig(cfg)

			if pc.CommandPolicyType != tt.wantType {
				t.Errorf("CommandPolicyType = %q, want %q", pc.CommandPolicyType, tt.wantType)
			}
			if !stringSliceEqual(pc.BlockedCommands, tt.wantBlocked) {
				t.Errorf("BlockedCommands = %v, want %v", pc.BlockedCommands, tt.wantBlocked)
			}
			if !stringSliceEqual(pc.AllowedCommands, tt.wantAllowed) {
				t.Errorf("AllowedCommands = %v, want %v", pc.AllowedCommands, tt.wantAllowed)
			}
		})
	}
}

func TestBuildPreloadConfig_NetworkIPOnly(t *testing.T) {
	cfg := SandboxConfig{
		NetworkPermission: NetworkPermissionConfig{
			Outbound: DirectionalNetworkConfig{
				Mode:      ModeBlacklist,
				Addresses: []string{"10.0.0.1", "192.168.1.1"},
			},
		},
	}
	pc := buildPreloadConfig(cfg)

	if pc.NetworkPolicyType != "blacklist" {
		t.Errorf("NetworkPolicyType = %q, want %q", pc.NetworkPolicyType, "blacklist")
	}
	if !stringSliceEqual(pc.BlockedIPs, []string{"10.0.0.1", "192.168.1.1"}) {
		t.Errorf("BlockedIPs = %v, want [10.0.0.1 192.168.1.1]", pc.BlockedIPs)
	}
	if len(pc.BlockedDomains) != 0 {
		t.Errorf("BlockedDomains should be empty, got %v", pc.BlockedDomains)
	}
}

func TestBuildPreloadConfig_NetworkDomainClassification(t *testing.T) {
	cfg := SandboxConfig{
		NetworkPermission: NetworkPermissionConfig{
			Outbound: DirectionalNetworkConfig{
				Mode:      ModeBlacklist,
				Addresses: []string{"10.0.0.1", "www.example.com", "192.168.1.1"},
			},
		},
	}
	pc := buildPreloadConfig(cfg)

	if !containsAll(pc.BlockedDomains, []string{"www.example.com"}) {
		t.Errorf("BlockedDomains should contain www.example.com, got %v", pc.BlockedDomains)
	}
	if !containsAll(pc.BlockedIPs, []string{"10.0.0.1", "192.168.1.1"}) {
		t.Errorf("BlockedIPs should contain 10.0.0.1 and 192.168.1.1, got %v", pc.BlockedIPs)
	}
}

func TestBuildPreloadConfig_ToPolicyJSON(t *testing.T) {
	cfg := SandboxConfig{
		PathPermission: PathPermissionConfig{
			Mode:  ModeBlacklist,
			Paths: []string{"/etc/passwd"},
		},
	}
	pc := buildPreloadConfig(cfg)

	data, err := pc.ToPolicyJSON()
	if err != nil {
		t.Fatalf("ToPolicyJSON() error: %v", err)
	}

	json := string(data)
	if len(json) == 0 {
		t.Error("ToPolicyJSON() returned empty JSON")
	}
	if !containsStr(json, `"blocked_paths"`) {
		t.Error("JSON should contain blocked_paths field")
	}
	if !containsStr(json, `"/etc/passwd"`) {
		t.Error("JSON should contain /etc/passwd value")
	}
	if !containsStr(json, `"blocked_domains"`) {
		t.Error("JSON should contain blocked_domains field")
	}
}

func TestBuildPreloadConfig_GatewayPaths(t *testing.T) {
	cfg := SandboxConfig{
		GatewayBinaryPath: "/usr/bin/openclaw",
		GatewayConfigPath: "/home/user/.openclaw/openclaw.json",
	}
	pc := buildPreloadConfig(cfg)

	if pc.GatewayBinaryPath != "/usr/bin/openclaw" {
		t.Errorf("GatewayBinaryPath = %q, want %q", pc.GatewayBinaryPath, "/usr/bin/openclaw")
	}
	if pc.GatewayConfigPath != "/home/user/.openclaw/openclaw.json" {
		t.Errorf("GatewayConfigPath = %q, want %q", pc.GatewayConfigPath, "/home/user/.openclaw/openclaw.json")
	}

	data, err := pc.ToPolicyJSON()
	if err != nil {
		t.Fatalf("ToPolicyJSON() error: %v", err)
	}
	json := string(data)
	if !containsStr(json, `"gateway_binary_path"`) {
		t.Error("JSON should contain gateway_binary_path field")
	}
	if !containsStr(json, `"/usr/bin/openclaw"`) {
		t.Error("JSON should contain gateway binary path value")
	}
	if !containsStr(json, `"gateway_config_path"`) {
		t.Error("JSON should contain gateway_config_path field")
	}
}

func TestBuildPreloadConfig_GatewayPathsOmitEmpty(t *testing.T) {
	cfg := SandboxConfig{}
	pc := buildPreloadConfig(cfg)

	data, err := pc.ToPolicyJSON()
	if err != nil {
		t.Fatalf("ToPolicyJSON() error: %v", err)
	}
	json := string(data)
	if containsStr(json, `"gateway_binary_path"`) {
		t.Error("JSON should omit empty gateway_binary_path")
	}
	if containsStr(json, `"gateway_config_path"`) {
		t.Error("JSON should omit empty gateway_config_path")
	}
}

// --- 测试辅助函数 ---

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func containsAll(slice []string, targets []string) bool {
	m := make(map[string]bool, len(slice))
	for _, s := range slice {
		m[s] = true
	}
	for _, t := range targets {
		if !m[t] {
			return false
		}
	}
	return true
}

func containsStr(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) > len(substr) && findSubstr(s, substr))
}

func findSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
