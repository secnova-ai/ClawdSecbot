package sandbox

import (
	"path/filepath"
	"testing"
)

func TestNormalizeSandboxConfig_DeduplicateAndCleanEntries(t *testing.T) {
	cfg := SandboxConfig{
		GatewayBinaryPath: " /usr/bin/openclaw ",
		GatewayConfigPath: " /tmp/../tmp/openclaw.json ",
		PathPermission: PathPermissionConfig{
			Mode:  PermissionMode(""),
			Paths: []string{" /tmp// ", "/tmp", "", "   "},
		},
		NetworkPermission: NetworkPermissionConfig{
			Outbound: DirectionalNetworkConfig{
				Mode:      ModeWhitelist,
				Addresses: []string{" EXAMPLE.COM:443 ", "example.com:443", "10.0.0.1", " 10.0.0.1 "},
			},
			Inbound: DirectionalNetworkConfig{
				Mode:      PermissionMode("invalid"),
				Addresses: []string{" LOCALHOST:8080 ", "localhost:8080"},
			},
		},
		ShellPermission: ShellPermissionConfig{
			Mode:     ModeWhitelist,
			Commands: []string{" Curl ", "curl", "", "   "},
		},
	}

	got := normalizeSandboxConfig(cfg)

	if got.PathPermission.Mode != ModeBlacklist {
		t.Fatalf("PathPermission.Mode = %q, want %q", got.PathPermission.Mode, ModeBlacklist)
	}
	if got.NetworkPermission.Inbound.Mode != ModeBlacklist {
		t.Fatalf("NetworkPermission.Inbound.Mode = %q, want %q", got.NetworkPermission.Inbound.Mode, ModeBlacklist)
	}
	if got.NetworkPermission.Outbound.Mode != ModeWhitelist {
		t.Fatalf("NetworkPermission.Outbound.Mode = %q, want %q", got.NetworkPermission.Outbound.Mode, ModeWhitelist)
	}
	if got.ShellPermission.Mode != ModeWhitelist {
		t.Fatalf("ShellPermission.Mode = %q, want %q", got.ShellPermission.Mode, ModeWhitelist)
	}

	wantTmpPath := filepath.Clean("/tmp")
	if len(got.PathPermission.Paths) != 1 || got.PathPermission.Paths[0] != wantTmpPath {
		t.Fatalf("PathPermission.Paths = %v, want [%s]", got.PathPermission.Paths, wantTmpPath)
	}
	if len(got.NetworkPermission.Outbound.Addresses) != 2 ||
		got.NetworkPermission.Outbound.Addresses[0] != "example.com:443" ||
		got.NetworkPermission.Outbound.Addresses[1] != "10.0.0.1" {
		t.Fatalf("NetworkPermission.Outbound.Addresses = %v, want [example.com:443 10.0.0.1]", got.NetworkPermission.Outbound.Addresses)
	}
	if len(got.NetworkPermission.Inbound.Addresses) != 1 || got.NetworkPermission.Inbound.Addresses[0] != "localhost:8080" {
		t.Fatalf("NetworkPermission.Inbound.Addresses = %v, want [localhost:8080]", got.NetworkPermission.Inbound.Addresses)
	}
	if len(got.ShellPermission.Commands) != 1 || got.ShellPermission.Commands[0] != "curl" {
		t.Fatalf("ShellPermission.Commands = %v, want [curl]", got.ShellPermission.Commands)
	}
	wantGatewayBinaryPath := filepath.Clean("/usr/bin/openclaw")
	if got.GatewayBinaryPath != wantGatewayBinaryPath {
		t.Fatalf("GatewayBinaryPath = %q, want %q", got.GatewayBinaryPath, wantGatewayBinaryPath)
	}
	wantGatewayConfigPath := filepath.Clean("/tmp/openclaw.json")
	if got.GatewayConfigPath != wantGatewayConfigPath {
		t.Fatalf("GatewayConfigPath = %q, want %q", got.GatewayConfigPath, wantGatewayConfigPath)
	}
}
