# Plugin SDK (Unified Contract)

This branch keeps `plugin-reconstruction` as the runtime baseline and enforces a single plugin SDK contract for all bot plugins.

## Scope

- Source plugins still live under `go_lib/plugins/<plugin_name>/`
- Runtime routing remains `asset_name + asset_id` centric
- SDK metadata/schema are mandatory in `BotPlugin`

## Types

Core SDK types are defined in:

- `go_lib/plugin_sdk/types.go`

Key objects:

- `PluginManifest`
- `AssetUISchema`
- `BuildInstanceID(...)`

## Mandatory Plugin Contract

The host `BotPlugin` contract requires metadata + schema:

```go
type BotPlugin interface {
    GetAssetName() string
    GetID() string
    GetManifest() plugin_sdk.PluginManifest
    GetAssetUISchema() *plugin_sdk.AssetUISchema
}
```

`PluginManager.GetAllPluginInfos()` always returns manifest + schema fields.

## Rules

1. Do not remove legacy `asset_name` / `asset_id` fields in current APIs.
2. `MitigateRisk` requests must include `source_plugin`; no multi-plugin fallback routing.
3. No legacy fallback for old `asset_id=""` records in plugin runtime path.
