package sandbox

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"go_lib/core/logging"
)

// SeatbeltPolicy generates macOS sandbox-exec policy files
type SeatbeltPolicy struct {
	Config SandboxConfig
}

// NewSeatbeltPolicy creates a new policy generator
func NewSeatbeltPolicy(config SandboxConfig) *SeatbeltPolicy {
	return &SeatbeltPolicy{Config: normalizeSandboxConfig(config)}
}

// GeneratePolicy generates a complete Seatbelt policy file content
// Uses simple whitelist/blacklist mode as documented in sandbox-exec.md
func (p *SeatbeltPolicy) GeneratePolicy() (string, error) {
	var sb strings.Builder

	// Header
	sb.WriteString("(version 1)\n")
	sb.WriteString("; Auto-generated sandbox policy for: ")
	sb.WriteString(p.Config.AssetName)
	sb.WriteString("\n")
	sb.WriteString("; For debugging: uncomment (debug deny) to see rejected operations\n")
	sb.WriteString("; (debug deny)\n")
	sb.WriteString("\n")

	// Determine if we're using whitelist mode for any permission
	usingWhitelistMode := p.Config.PathPermission.Mode == ModeWhitelist ||
		p.Config.NetworkPermission.Inbound.Mode == ModeWhitelist ||
		p.Config.NetworkPermission.Outbound.Mode == ModeWhitelist ||
		p.Config.ShellPermission.Mode == ModeWhitelist

	if usingWhitelistMode {
		// Whitelist mode: default deny, then allow specific items
		sb.WriteString("; Whitelist mode: default deny\n")
		sb.WriteString("(deny default)\n\n")

		// Basic system access required for any process
		sb.WriteString("; Basic system access\n")
		sb.WriteString("(allow process-fork)\n")
		sb.WriteString("(allow process-exec)\n")
		sb.WriteString("(allow signal (target self))\n")
		sb.WriteString("(allow sysctl-read)\n")
		sb.WriteString("(allow mach-lookup)\n")
		sb.WriteString("(allow ipc-posix-shm-read-data)\n")
		sb.WriteString("(allow ipc-posix-shm-write-data)\n")
		sb.WriteString("\n")

		// Allow reading system libraries and frameworks
		sb.WriteString("; System libraries and frameworks\n")
		sb.WriteString("(allow file-read*\n")
		sb.WriteString("  (subpath \"/usr/lib\")\n")
		sb.WriteString("  (subpath \"/usr/share\")\n")
		sb.WriteString("  (subpath \"/System/Library\")\n")
		sb.WriteString("  (subpath \"/Library/Frameworks\")\n")
		sb.WriteString("  (subpath \"/private/var/db\")\n")
		sb.WriteString(")\n\n")

		// Allow executing the gateway binary itself
		if p.Config.GatewayBinaryPath != "" {
			sb.WriteString("; Gateway binary execution\n")
			sb.WriteString(fmt.Sprintf("(allow file-read* file-execute (literal \"%s\"))\n", p.Config.GatewayBinaryPath))
			// Also allow the directory containing the binary
			binDir := filepath.Dir(p.Config.GatewayBinaryPath)
			sb.WriteString(fmt.Sprintf("(allow file-read* (subpath \"%s\"))\n", binDir))
			sb.WriteString("\n")
		}

		// Allow reading the gateway config
		if p.Config.GatewayConfigPath != "" {
			sb.WriteString("; Gateway config access\n")
			sb.WriteString(fmt.Sprintf("(allow file-read* (literal \"%s\"))\n", p.Config.GatewayConfigPath))
			configDir := filepath.Dir(p.Config.GatewayConfigPath)
			sb.WriteString(fmt.Sprintf("(allow file-read* file-write* (subpath \"%s\"))\n", configDir))
			sb.WriteString("\n")
		}

		// Temp directory access (required for many operations)
		sb.WriteString("; Temporary directory access\n")
		sb.WriteString("(allow file-read* file-write* (subpath \"/tmp\"))\n")
		sb.WriteString("(allow file-read* file-write* (subpath \"/private/tmp\"))\n")
		sb.WriteString("(allow file-read* file-write* (subpath \"/var/folders\"))\n")
		sb.WriteString("(allow file-read* file-write* (subpath \"/private/var/folders\"))\n")
		sb.WriteString("\n")

		// Generate permission rules for whitelist mode
		sb.WriteString(p.generatePathRules())
		sb.WriteString(p.generateNetworkRules())
		sb.WriteString(p.generateShellRules())
	} else {
		// Blacklist mode: IMPORTANT - deny rules MUST come BEFORE (allow default)
		sb.WriteString("; Blacklist mode: deny specific items first, then allow default\n")

		// Generate deny rules FIRST (before allow default)
		sb.WriteString(p.generatePathRules())
		sb.WriteString(p.generateNetworkRules())
		sb.WriteString(p.generateShellRules())

		// Now allow everything else
		sb.WriteString("; Default allow (after deny rules)\n")
		sb.WriteString("(allow default)\n")
	}

	return sb.String(), nil
}

// generatePathRules generates file system access rules
func (p *SeatbeltPolicy) generatePathRules() string {
	var sb strings.Builder

	sb.WriteString("; File system access rules\n")

	if len(p.Config.PathPermission.Paths) == 0 {
		// No paths configured, no restrictions
		return sb.String()
	}

	if p.Config.PathPermission.Mode == ModeWhitelist {
		// Whitelist mode: only allow listed paths (used with deny default)
		sb.WriteString("; Whitelist mode - only allow specified paths\n")
		for _, path := range p.Config.PathPermission.Paths {
			expandedPath := expandPath(path)
			sb.WriteString(fmt.Sprintf("(allow file-read* file-write* (subpath \"%s\"))\n", expandedPath))
		}
	} else {
		// Blacklist mode: deny specific paths (deny rules MUST come BEFORE allow default)
		sb.WriteString("; Blacklist mode - deny specified paths\n")
		for _, path := range p.Config.PathPermission.Paths {
			// Use absolute path directly (should already be expanded from UI)
			expandedPath := expandPath(path)
			// Deny both data read and metadata read for complete blocking
			sb.WriteString(fmt.Sprintf("(deny file-read-data file-read-metadata file-write* (subpath \"%s\"))\n", expandedPath))
		}
	}

	sb.WriteString("\n")
	return sb.String()
}

// generateNetworkRules generates network access rules with inbound/outbound separation
func (p *SeatbeltPolicy) generateNetworkRules() string {
	var sb strings.Builder

	sb.WriteString("; Network access rules\n")

	outbound := p.Config.NetworkPermission.Outbound
	inbound := p.Config.NetworkPermission.Inbound

	hasOutboundRules := len(outbound.Addresses) > 0
	hasInboundRules := len(inbound.Addresses) > 0

	if !hasOutboundRules && !hasInboundRules {
		// No addresses configured, no restrictions
		return sb.String()
	}

	// Always allow unix domain socket (required for XPC, launchd, etc.)
	sb.WriteString("; Always allow unix domain socket\n")
	sb.WriteString("(allow network* (remote unix) (local unix))\n\n")

	// --- Outbound rules (network-outbound) ---
	sb.WriteString(p.generateDirectionalRules("outbound", outbound))

	// --- Inbound rules (network-inbound) ---
	sb.WriteString(p.generateDirectionalRules("inbound", inbound))

	return sb.String()
}

// generateDirectionalRules generates rules for a single network direction
// direction: "outbound" or "inbound"
// For outbound: uses network-outbound + (remote ip ...)
// For inbound: uses network-inbound + (local ip ...)
func (p *SeatbeltPolicy) generateDirectionalRules(direction string, config DirectionalNetworkConfig) string {
	var sb strings.Builder

	if len(config.Addresses) == 0 {
		return ""
	}

	var networkOp, ipFilter string
	if direction == "outbound" {
		networkOp = "network-outbound"
		ipFilter = "remote ip"
	} else {
		networkOp = "network-inbound"
		ipFilter = "local ip"
	}

	if config.Mode == ModeWhitelist {
		sb.WriteString(fmt.Sprintf("; %s whitelist - only allow specified addresses\n", direction))
		// In whitelist mode, always allow localhost for gateway communication
		sb.WriteString(fmt.Sprintf("(allow %s (%s \"localhost:*\"))\n", networkOp, ipFilter))
		for _, addr := range config.Addresses {
			formatted := formatNetworkRule(addr)
			if !isValidSandboxNetworkAddress(formatted) {
				logging.Warning("Skipping invalid sandbox %s address '%s': sandbox-exec only supports '*' or 'localhost' as host", direction, addr)
				sb.WriteString(fmt.Sprintf("; SKIPPED (invalid for sandbox-exec): %s\n", addr))
				continue
			}
			sb.WriteString(fmt.Sprintf("(allow %s (%s \"%s\"))\n", networkOp, ipFilter, formatted))
		}
	} else {
		sb.WriteString(fmt.Sprintf("; %s blacklist - deny specified addresses\n", direction))
		for _, addr := range config.Addresses {
			formatted := formatNetworkRule(addr)
			if !isValidSandboxNetworkAddress(formatted) {
				logging.Warning("Skipping invalid sandbox %s address '%s': sandbox-exec only supports '*' or 'localhost' as host", direction, addr)
				sb.WriteString(fmt.Sprintf("; SKIPPED (invalid for sandbox-exec): %s\n", addr))
				continue
			}
			sb.WriteString(fmt.Sprintf("(deny %s (%s \"%s\"))\n", networkOp, ipFilter, formatted))
		}
	}

	sb.WriteString("\n")
	return sb.String()
}

// generateShellRules generates process execution rules
func (p *SeatbeltPolicy) generateShellRules() string {
	var sb strings.Builder

	sb.WriteString("; Process execution rules\n")

	if len(p.Config.ShellPermission.Commands) == 0 {
		// No commands configured, no restrictions (allow default will handle it)
		return sb.String()
	}

	if p.Config.ShellPermission.Mode == ModeWhitelist {
		// Whitelist mode: only allow listed commands (used with deny default)
		sb.WriteString("; Whitelist mode - only allow specified commands\n")
		// Always allow basic shell commands in whitelist mode
		sb.WriteString("(allow process-exec (literal \"/bin/sh\"))\n")
		sb.WriteString("(allow process-exec (literal \"/bin/bash\"))\n")
		sb.WriteString("(allow process-exec (literal \"/usr/bin/env\"))\n")
		for _, cmd := range p.Config.ShellPermission.Commands {
			cmdPath := resolveCommandPath(cmd)
			if cmdPath != "" {
				sb.WriteString(fmt.Sprintf("(allow process-exec (literal \"%s\"))\n", cmdPath))
			}
		}
	} else {
		// Blacklist mode: deny specific commands (must come before allow default)
		sb.WriteString("; Blacklist mode - deny specified commands\n")
		for _, cmd := range p.Config.ShellPermission.Commands {
			cmdPath := resolveCommandPath(cmd)
			if cmdPath != "" {
				sb.WriteString(fmt.Sprintf("(deny process-exec (literal \"%s\"))\n", cmdPath))
			}
		}
	}

	sb.WriteString("\n")
	return sb.String()
}

// SavePolicyFile saves the policy to a file
func (p *SeatbeltPolicy) SavePolicyFile(policyDir string) (string, error) {
	content, err := p.GeneratePolicy()
	if err != nil {
		return "", err
	}

	// Create policy directory if not exists
	if err := os.MkdirAll(policyDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create policy directory: %w", err)
	}

	// Generate policy file path
	policyPath := filepath.Join(policyDir, fmt.Sprintf("botsec_%s.sb", sanitizeAssetName(p.Config.AssetName)))

	// Write policy file
	if err := os.WriteFile(policyPath, []byte(content), 0600); err != nil {
		return "", fmt.Errorf("failed to write policy file: %w", err)
	}

	logging.Info("Sandbox policy saved to: %s", policyPath)
	return policyPath, nil
}

// Seatbelt helper functions

func formatNetworkRule(addr string) string {
	// If no port specified, add wildcard
	if !strings.Contains(addr, ":") {
		return addr + ":*"
	}
	return addr
}

// isValidSandboxNetworkAddress checks if a network address is valid for sandbox-exec.
// sandbox-exec only accepts "*" or "localhost" as the host in (remote ip "host:port") rules.
// Returns true if valid, false otherwise.
func isValidSandboxNetworkAddress(addr string) bool {
	// Parse host part
	host := addr
	if idx := strings.LastIndex(addr, ":"); idx >= 0 {
		host = addr[:idx]
	}

	// sandbox-exec only allows * or localhost as host
	switch strings.ToLower(host) {
	case "*", "localhost", "127.0.0.1":
		return true
	default:
		return false
	}
}

// isIPAddress checks if the given string looks like an IP address (not * or localhost)
func isIPAddress(host string) bool {
	return net.ParseIP(host) != nil
}

// ValidateNetworkAddresses checks all network addresses and returns a list of invalid ones
// with human-readable error messages.
func ValidateNetworkAddresses(addresses []string) []string {
	var errors []string
	for _, addr := range addresses {
		if !isValidSandboxNetworkAddress(addr) {
			host := addr
			if idx := strings.LastIndex(addr, ":"); idx >= 0 {
				host = addr[:idx]
			}
			if isIPAddress(host) {
				errors = append(errors, fmt.Sprintf(
					"'%s': sandbox-exec does not support specific IP addresses, only '*' or 'localhost' are allowed as host",
					addr))
			} else if strings.Contains(addr, "/") {
				errors = append(errors, fmt.Sprintf(
					"'%s': sandbox-exec does not support CIDR notation, only '*' or 'localhost' are allowed as host",
					addr))
			} else {
				errors = append(errors, fmt.Sprintf(
					"'%s': sandbox-exec does not support domain names, only '*' or 'localhost' are allowed as host",
					addr))
			}
		}
	}
	return errors
}
