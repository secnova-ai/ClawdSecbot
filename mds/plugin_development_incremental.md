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

Only the normalized asset name and canonical `config_path` participate in the
fingerprint. Ports, process paths, PIDs, and other runtime evidence must not
change an existing asset ID.

## 3. Shared Runtime Claims

When more than one plugin can identify the same compatible runtime, implement
the optional `core.RuntimeAssetClaimProvider` capability. Do not add the method
to `core.BotPlugin`.

Each claim must provide a runtime family, canonical config path, product
variant, and priority. Mark `IsFallback` only on a generic compatibility
plugin. Product-specific evidence may choose the winner, but it must not enter
the `asset_id` fingerprint.

`PluginManager.ScanAllAssets` is the authoritative arbitration path: it scans
all plugins, resolves shared-runtime claims, removes losing instances, and then
binds the winner. Test both the winning and losing product paths, including a
rescan that transfers an existing fallback asset ID. Cross-product persisted ID
migrations require an explicit database migration and application version
update.

## 4. Risk Routing

Risk mitigation is strict:

1. Every mitigatable risk must contain `asset_id` (or provide it in `args.asset_id`).
2. Host routes mitigation only by `asset_id` to the bound plugin instance.
3. No fallback traversal across all plugins.

## 5. Testing

Required:

1. Asset scanning tests
2. Multi-instance protection lifecycle tests
3. Risk routing/mitigation tests
4. Aggregate claim-arbitration winner and loser tests when the plugin shares a runtime family

Recommended:

1. Metadata/schema contract tests
2. Cross-platform path normalization tests
