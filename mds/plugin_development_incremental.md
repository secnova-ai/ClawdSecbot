# Plugin Development Guide (Unified)

This guide targets the current `plugin-reconstruction` baseline and the unified SDK model.

## 1. Implement Mandatory Contract

Every plugin must implement `core.BotPlugin`:

- `GetAssetName`
- `GetID`
- `GetManifest`
- `GetAssetUISchema`
- `ScanAssets`
- `AssessRisks`
- `MitigateRisk`
- `StartProtection(assetID, config)`
- `StopProtection(assetID)`
- `GetProtectionStatus(assetID)`

## 2. Keep Asset Identity Stable

Use deterministic `asset_id` for instance routing.
`asset_id` must be stable across repeated scans for the same instance.

## 3. Risk Routing

Risk mitigation is strict:

1. Every mitigatable risk must contain `asset_id` (or provide it in `args.asset_id`).
2. Host routes mitigation only by `asset_id` to the bound plugin instance.
3. No fallback traversal across all plugins.

## 4. Testing

Required:

1. Asset scanning tests
2. Multi-instance protection lifecycle tests
3. Risk routing/mitigation tests

Recommended:

1. Metadata/schema contract tests
2. Cross-platform path normalization tests
