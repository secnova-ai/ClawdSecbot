package core

import "go_lib/plugin_sdk"

// BotPlugin defines the standard contract for bot security plugins.
// Each plugin (for example Openclaw) implements this interface and is
// compiled into the shared dynamic library for Flutter FFI usage.
//
// Every plugin must implement this interface and register itself in init().
type BotPlugin interface {
	// ========== Identity ==========

	// GetAssetName returns the asset type handled by this plugin.
	GetAssetName() string

	// GetID returns a stable plugin ID (independent of asset instances).
	GetID() string

	// GetManifest returns plugin metadata used by host-side aggregation.
	GetManifest() plugin_sdk.PluginManifest

	// GetAssetUISchema returns the declarative asset card schema.
	GetAssetUISchema() *plugin_sdk.AssetUISchema

	// RequiresBotModelConfig reports whether this plugin's protection
	// implementation depends on explicit Bot model configuration.
	//
	// true  -> protection startup must include bot_model config
	// false -> plugin can resolve forwarding target without bot_model config
	RequiresBotModelConfig() bool

	// ========== Asset Discovery ==========

	// ScanAssets discovers assets and returns the current asset instances.
	// Each returned Asset must have a unique ID computed via ComputeAssetID().
	ScanAssets() ([]Asset, error)

	// ========== Risk Assessment ==========

	// AssessRisks evaluates risks for discovered assets.
	AssessRisks(scannedHashes map[string]bool, assets []Asset) ([]Risk, error)

	// GetVulnInfoJSON returns plugin-scoped vulnerability definitions.
	GetVulnInfoJSON() []byte

	// CompareVulnerabilityVersion compares current asset version with the
	// vulnerability checkpoint version.
	// Returns -1 when current < target, 0 when equal, 1 when current > target.
	// ok=false indicates the plugin cannot compare the provided versions.
	CompareVulnerabilityVersion(current, target string) (int, bool)

	// MitigateRisk handles mitigation requests.
	// riskInfo is a JSON string containing risk ID, args, form_data, etc.
	// Returns a JSON string with {"success": bool, "message": ..., "error": ...}
	MitigateRisk(riskInfo string) string

	// ========== Protection Control ==========

	// StartProtection starts protection for the specified asset instance.
	StartProtection(assetID string, config ProtectionConfig) error

	// StopProtection stops protection for the specified asset instance.
	StopProtection(assetID string) error

	// GetProtectionStatus returns current protection status for assetID.
	GetProtectionStatus(assetID string) ProtectionStatus
}

// ProtectionConfig stores protection runtime settings.
type ProtectionConfig struct {
	// SandboxEnabled enables sandbox protection.
	SandboxEnabled bool `json:"sandbox_enabled"`

	// ProxyEnabled enables proxy protection.
	ProxyEnabled bool `json:"proxy_enabled"`

	// ProxyPort is the local proxy port.
	ProxyPort int `json:"proxy_port"`

	// AuditOnly means detect-only mode without blocking.
	AuditOnly bool `json:"audit_only"`

	// TargetURL is the upstream LLM endpoint.
	TargetURL string `json:"target_url"`

	// TargetVendor is the upstream provider name.
	TargetVendor string `json:"target_vendor"`
}

// ProtectionStatus reports current protection runtime state.
type ProtectionStatus struct {
	// Running indicates protection runtime status.
	Running bool `json:"running"`

	// ProxyRunning indicates proxy runtime status.
	ProxyRunning bool `json:"proxy_running"`

	// ProxyPort is the current proxy port.
	ProxyPort int `json:"proxy_port"`

	// SandboxActive indicates sandbox activation state.
	SandboxActive bool `json:"sandbox_active"`

	// AuditOnly indicates detect-only mode.
	AuditOnly bool `json:"audit_only"`

	// Error stores an optional runtime error.
	Error string `json:"error,omitempty"`
}

// ProtectionContext contains runtime data required by plugin hooks.
type ProtectionContext struct {
	// AssetID identifies the protected asset instance.
	AssetID string
	// ProxyPort is the active proxy port.
	ProxyPort int
	// BackupDir points to the backup directory.
	BackupDir string
	// Config is the resolved protection config.
	Config ProtectionConfig
}

// ProtectionLifecycleHooks defines optional lifecycle hooks around proxy runtime.
type ProtectionLifecycleHooks interface {
	// OnProtectionStart runs before proxy start.
	OnProtectionStart(ctx *ProtectionContext) (map[string]interface{}, error)

	// OnBeforeProxyStop runs before proxy stop for cleanup.
	OnBeforeProxyStop(ctx *ProtectionContext)
}

// ProxyForwardingTarget is the resolved upstream target for proxy forwarding.
// Plugins that do not require explicit Bot model configuration can provide
// this target from their own runtime config files.
type ProxyForwardingTarget struct {
	Provider string
	BaseURL  string
	APIKey   string
}

// ProxyForwardingTargetResolver defines optional plugin capability to resolve
// forwarding target dynamically for a specific asset instance.
type ProxyForwardingTargetResolver interface {
	ResolveProxyForwardingTarget(assetID string) (*ProxyForwardingTarget, error)
}
