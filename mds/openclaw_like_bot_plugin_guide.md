# Openclaw-like Bot Plugin Adaptation Guide

**[中文文档](openclaw_like_bot_plugin_guide_zh-CN.md)**

This guide explains how to adapt ClawSecbot to other Openclaw-like bots through the plugin mechanism.

## 1. Scope

Use this guide when your target bot has architecture similar to Openclaw:

- It has a local workspace/config file/process/gateway.
- It talks to LLM APIs via HTTP/HTTPS.
- You can reroute its LLM traffic by updating config and restarting process.

## 2. Core Principles

1. Asset ID is the only instance identity.
2. Asset instance and plugin instance are strictly 1:1.
3. Plugin type registration and runtime instance binding are separated.
4. Core must remain bot-agnostic.

## 3. Runtime Model

- Type level: plugin is registered by `asset_name` in `PluginManager`.
- Instance level: each discovered asset (`asset_id`) is bound to exactly one plugin instance entry.
- All runtime operations use `(assetName, assetID)` and finally route by `assetID`.

Code references:

- `go_lib/core/plugin.go`
- `go_lib/core/plugin_manager.go`
- `go_lib/core/asset.go`

## 4. Required Interface

Implement `core.BotPlugin` in your plugin package:

```go
type BotPlugin interface {
    GetAssetName() string
    ScanAssets() ([]Asset, error)
    AssessRisks(scannedHashes map[string]bool) ([]Risk, error)
    MitigateRisk(riskInfo string) string
    StartProtection(assetID string, config ProtectionConfig) error
    StopProtection(assetID string) error
    GetProtectionStatus(assetID string) ProtectionStatus
}
```

Optional lifecycle hooks:

```go
type ProtectionLifecycleHooks interface {
    OnProtectionStart(ctx *ProtectionContext) (map[string]interface{}, error)
    OnBeforeProxyStop(ctx *ProtectionContext)
}
```

`OnProtectionStart` is where bot-specific traffic hijacking is usually implemented (for example: rewrite bot config, point endpoint to local proxy, restart gateway process).

## 5. Asset Discovery and Deterministic ID

### 5.1 Build stable fingerprint

Use `core.ComputeAssetID(name, configPath)` to generate deterministic IDs.

Rules:

- Only `name` + `config_path` participate in the fingerprint; both must be stable and canonical.
- Runtime-dynamic data (`ports`, `process_paths`, `pid`, `service_name`, etc.) MUST NOT enter the fingerprint; otherwise starting/stopping the bot will drift `asset_id` and break policy/protection binding.
- Do not include volatile data (timestamps/temp paths/random values).

### 5.2 SourcePlugin

For every discovered asset, fill `asset.SourcePlugin = plugin.GetAssetName()`.

### 5.3 Shared Runtime Claim Ownership

Two products may present different user-facing identities while recognizing the
same compatible runtime. A matching runtime family plus canonical
`config_path` identifies that shared runtime; matching ports, PIDs, and process
paths are product evidence only and must not affect `asset_id`.

Implement the optional `core.RuntimeAssetClaimProvider` when a plugin shares a
runtime family with another plugin. Return a canonical config path, variant ID,
and deterministic priority. Use `IsFallback` only for the generic compatibility
plugin. Do not use registration order to decide ownership.

`ScanAllAssets` is the authoritative arbitration path. It chooses one winner,
binds only that plugin instance, and sends its risk/protection routing to the
winning product. A product plugin may retain its own display name and UI
sections even when it reuses a generic fallback asset ID for compatibility.

Ordinary claim arbitration must not silently migrate persisted cross-product
asset IDs. Such a migration requires an explicit database migration and
application version update.

## 6. Multi-instance Protection Flow

1. Scan assets (`ScanAllAssets`) and bind asset IDs to plugin instances.
2. User selects one asset and triggers one-click protection.
3. UI passes `assetName + assetID` to FFI.
4. Core routes to plugin instance and starts proxy/sandbox for this asset.
5. Status/query/stop all operate by the same `assetID`.

Important:

- There is no independent plugin instance ID.
- The asset ID is the plugin instance identity.

## 7. Recommended Adaptation Steps

1. Create a new package under `go_lib/plugins/<yourbot>/`.
2. Add plugin struct with per-asset status map (`map[assetID]ProtectionStatus`).
3. Register in `init()` via `core.GetPluginManager().Register(plugin)`.
4. Implement scanner.
5. Implement risk assessment and mitigation templates.
6. Implement protection hooks.
7. Verify one-click protection can run concurrently for multiple assets of same bot type.

## 8. Model Configuration Timing

For Openclaw-like products, model config may be unknown before first scan (no asset ID yet).

Recommended UX and logic:

1. Do not force bot model config at first app launch.
2. After asset scan, open protection config by selected asset.
3. On save, validate bot model first.
4. If security model is missing, open security model config dialog.
5. Allow "Reuse bot model config" for better UX.

## 9. Core vs Plugin Responsibilities

Core responsibilities:

- Plugin registry/routing.
- ShepherdGate analysis and generic guard skills.
- LLM routing proxy, audit, callback bridge.
- DB/repository/service common capabilities.

Plugin responsibilities:

- Bot-specific asset recognition.
- Bot-specific risk rules and one-click fix.
- Bot-specific traffic hijack mechanism.
- Bot-specific process restart/recovery details.

## 10. Validation Checklist

- Deterministic asset ID is stable across repeated scans.
- Different assets get different IDs.
- One asset ID maps to one runtime plugin instance entry.
- A shared runtime has exactly one aggregate-scan winner and one routing owner.
- Start/stop/status all work with correct `assetID`.
- `OnProtectionStart` modifies only target asset instance.
- Rollback in `OnBeforeProxyStop` is idempotent.
- Multiple asset windows can run protection independently.
- Logs/metrics/security events can stream to UI in real time.
- `./scripts/run_with_pprof.sh` builds and starts successfully.

## 11. Minimal Skeleton

```go
package yourbot

import (
    "sync"
    "go_lib/core"
)

type YourBotPlugin struct {
    mu sync.RWMutex
    protectionStatuses map[string]core.ProtectionStatus
}

var plugin *YourBotPlugin

func init() {
    plugin = &YourBotPlugin{protectionStatuses: make(map[string]core.ProtectionStatus)}
    core.GetPluginManager().Register(plugin)
}

func (p *YourBotPlugin) GetAssetName() string { return "yourbot" }

func (p *YourBotPlugin) ScanAssets() ([]core.Asset, error) {
    // detect config/process/ports; compute asset ID from name + canonical config path only
    return nil, nil
}

func (p *YourBotPlugin) AssessRisks(scannedHashes map[string]bool) ([]core.Risk, error) {
    return nil, nil
}

func (p *YourBotPlugin) MitigateRisk(riskInfo string) string { return `{"success":true}` }

func (p *YourBotPlugin) StartProtection(assetID string, cfg core.ProtectionConfig) error {
    p.mu.Lock()
    defer p.mu.Unlock()
    p.protectionStatuses[assetID] = core.ProtectionStatus{Running: true, ProxyRunning: true, ProxyPort: cfg.ProxyPort}
    return nil
}

func (p *YourBotPlugin) StopProtection(assetID string) error {
    p.mu.Lock()
    defer p.mu.Unlock()
    p.protectionStatuses[assetID] = core.ProtectionStatus{Running: false, ProxyRunning: false}
    return nil
}

func (p *YourBotPlugin) GetProtectionStatus(assetID string) core.ProtectionStatus {
    p.mu.RLock()
    defer p.mu.RUnlock()
    return p.protectionStatuses[assetID]
}
```

## 12. Notes

- Keep comments in Chinese and logs in English to match project conventions.
- Add unit tests for scanner, ID determinism, and multi-instance protection behavior.
- Avoid adding bot-specific branches into `go_lib/core`.
