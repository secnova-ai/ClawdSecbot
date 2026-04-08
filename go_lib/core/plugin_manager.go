package core

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"go_lib/core/logging"
	"go_lib/plugin_sdk"
)

// AssetPluginInstance represents a concrete plugin instance bound to one asset ID.
// Relationship is strictly 1:1, i.e. one asset ID maps to one plugin instance entry.
type AssetPluginInstance struct {
	AssetID   string `json:"asset_id"`
	AssetName string `json:"asset_name"`
	Asset     Asset  `json:"asset"`
	plugin    BotPlugin
}

// PluginManager manages plugin registration by asset type and plugin instances by asset ID.
// Registered plugin is type-level capability; runtime operations are instance-level via asset ID.
type PluginManager struct {
	mu sync.RWMutex
	// registeredPlugins maps asset name -> plugin capability implementation.
	registeredPlugins map[string]BotPlugin
	// instances maps asset ID -> plugin instance entry (1 asset ID : 1 instance).
	instances map[string]*AssetPluginInstance
}

var (
	globalPluginManager *PluginManager
	pluginManagerOnce   sync.Once
)

// GetPluginManager 获取全局插件管理器实例
func GetPluginManager() *PluginManager {
	pluginManagerOnce.Do(func() {
		globalPluginManager = &PluginManager{
			registeredPlugins: make(map[string]BotPlugin),
			instances:         make(map[string]*AssetPluginInstance),
		}
	})
	return globalPluginManager
}

// ResetForTest 清空插件管理器的注册与实例表，仅供测试使用。
// 保留单例本身不重建，避免与外部持有的引用失配。
func (pm *PluginManager) ResetForTest() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.registeredPlugins = make(map[string]BotPlugin)
	pm.instances = make(map[string]*AssetPluginInstance)
}

// Register 注册插件类型能力（按资产名称唯一）
func (pm *PluginManager) Register(plugin BotPlugin) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	assetName := strings.TrimSpace(plugin.GetAssetName())
	pluginID := strings.TrimSpace(plugin.GetID())
	manifest := plugin.GetManifest()
	assetKey := normalizeAssetName(assetName)
	if assetKey == "" {
		logging.Warning("Plugin registration skipped: empty asset name")
		return
	}
	if pluginID == "" {
		logging.Warning("Plugin registration skipped: empty plugin id, asset=%s", assetName)
		return
	}
	if strings.TrimSpace(manifest.PluginID) == "" {
		logging.Warning("Plugin registration skipped: empty manifest.plugin_id, asset=%s", assetName)
		return
	}

	if exist, exists := pm.registeredPlugins[assetKey]; exists {
		logging.Warning("Asset plugin %s already registered, skipping duplicate implementation", exist.GetAssetName())
		return
	}

	pm.registeredPlugins[assetKey] = plugin
	logging.Info("Plugin registered: id=%s, assetName=%s", pluginID, assetName)
}

// GetPluginByAssetName returns the registered plugin capability by asset name.
func (pm *PluginManager) GetPluginByAssetName(assetName string) BotPlugin {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.registeredPlugins[normalizeAssetName(assetName)]
}

// GetPluginByAssetID returns the plugin bound to assetID instance.
func (pm *PluginManager) GetPluginByAssetID(assetID string) BotPlugin {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	inst := pm.instances[strings.TrimSpace(assetID)]
	if inst == nil {
		return nil
	}
	return inst.plugin
}

// GetPluginCount returns count of registered plugin capabilities (asset types).
func (pm *PluginManager) GetPluginCount() int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return len(pm.registeredPlugins)
}

// GetAssetInstanceCount returns count of tracked plugin instances (asset IDs).
func (pm *PluginManager) GetAssetInstanceCount() int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return len(pm.instances)
}

// GetAllPlugins returns all registered plugin capabilities.
func (pm *PluginManager) GetAllPlugins() []BotPlugin {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	plugins := make([]BotPlugin, 0, len(pm.registeredPlugins))
	for _, plugin := range pm.registeredPlugins {
		plugins = append(plugins, plugin)
	}
	return plugins
}

func (pm *PluginManager) bindAssetInstance(plugin BotPlugin, asset Asset) {
	assetID := strings.TrimSpace(asset.ID)
	if assetID == "" {
		logging.Warning("Skip binding plugin instance for %s: empty asset ID", plugin.GetAssetName())
		return
	}

	assetName := strings.TrimSpace(asset.SourcePlugin)
	if assetName == "" {
		assetName = plugin.GetAssetName()
	}
	assetName = strings.TrimSpace(assetName)
	if normalizeAssetName(assetName) == "" {
		assetName = plugin.GetAssetName()
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()
	if exist := pm.instances[assetID]; exist != nil {
		if normalizeAssetName(exist.AssetName) != normalizeAssetName(assetName) {
			logging.Warning("Asset ID collision detected: id=%s existingAsset=%s newAsset=%s", assetID, exist.AssetName, assetName)
			return
		}
	}
	pm.instances[assetID] = &AssetPluginInstance{
		AssetID:   assetID,
		AssetName: assetName,
		Asset:     asset,
		plugin:    plugin,
	}
}

func (pm *PluginManager) resolvePluginInstance(assetID string) (*AssetPluginInstance, error) {
	assetID = strings.TrimSpace(assetID)
	if assetID == "" {
		return nil, fmt.Errorf("assetID is required")
	}

	pm.mu.RLock()
	inst := pm.instances[assetID]
	pm.mu.RUnlock()
	if inst != nil {
		return inst, nil
	}
	return nil, fmt.Errorf("asset instance not found: assetID %s, run asset scan first", assetID)
}

// ScanAllAssets scans assets via all registered plugins and binds plugin instances by asset ID.
func (pm *PluginManager) ScanAllAssets() ([]Asset, error) {
	plugins := pm.GetAllPlugins()
	logging.Info("Starting asset scan with %d plugins", len(plugins))

	var allAssets []Asset
	for _, plugin := range plugins {
		assetName := plugin.GetAssetName()
		logging.Info("Scanning assets with plugin assetName=%s", assetName)
		assets, err := plugin.ScanAssets()
		if err != nil {
			logging.Warning("Plugin %s scan failed: %v", assetName, err)
			continue
		}

		scannedAssetIDs := make(map[string]struct{}, len(assets))
		for i := range assets {
			if strings.TrimSpace(assets[i].SourcePlugin) == "" {
				assets[i].SourcePlugin = assetName
			}
			assetID := strings.TrimSpace(assets[i].ID)
			if assetID != "" {
				scannedAssetIDs[assetID] = struct{}{}
			}
			pm.bindAssetInstance(plugin, assets[i])
		}
		pm.reconcilePluginInstances(assetName, scannedAssetIDs)

		logging.Info("Plugin %s found %d assets", assetName, len(assets))
		allAssets = append(allAssets, assets...)
	}

	logging.Info("Asset scan completed, total assets: %d", len(allAssets))
	return allAssets, nil
}

// ScanAssetsByPlugin scans assets for a single registered plugin and binds
// discovered asset instances for later protection/risk routing.
func (pm *PluginManager) ScanAssetsByPlugin(assetName string) ([]Asset, error) {
	plugin := pm.GetPluginByAssetName(assetName)
	if plugin == nil {
		return nil, fmt.Errorf("plugin not found: %s", assetName)
	}

	pluginAssetName := plugin.GetAssetName()
	logging.Info("Scanning assets with single plugin assetName=%s", pluginAssetName)
	assets, err := plugin.ScanAssets()
	if err != nil {
		return nil, err
	}

	for i := range assets {
		if strings.TrimSpace(assets[i].SourcePlugin) == "" {
			assets[i].SourcePlugin = pluginAssetName
		}
		pm.bindAssetInstance(plugin, assets[i])
	}

	logging.Info("Single plugin %s found %d assets", pluginAssetName, len(assets))
	return assets, nil
}

// AssessAllRisks evaluates risks via all registered plugins.
// Automatically injects SourcePlugin and best-effort asset_id for downstream routing.
func (pm *PluginManager) AssessAllRisks(scannedHashes map[string]bool) ([]Risk, error) {
	plugins := pm.GetAllPlugins()
	logging.Info("Starting risk assessment with %d plugins", len(plugins))

	var allRisks []Risk
	for _, plugin := range plugins {
		assetName := plugin.GetAssetName()
		singleAssetID := pm.getSingleAssetIDByPlugin(assetName)
		logging.Info("Assessing risks with plugin: %s", assetName)
		pluginAssets := pm.getAssetsByPlugin(assetName)
		risks, err := plugin.AssessRisks(scannedHashes, pluginAssets)
		if err != nil {
			logging.Warning("Plugin %s risk assessment failed: %v", assetName, err)
			continue
		}

		vulnRisks, err := BuildVulnerabilityRisks(plugin, pluginAssets)
		if err != nil {
			logging.Warning("Plugin %s vulnerability matching failed: %v", assetName, err)
		} else {
			risks = append(risks, vulnRisks...)
		}

		for i := range risks {
			risks[i].SourcePlugin = assetName
			if risks[i].Args == nil {
				risks[i].Args = map[string]interface{}{}
			}
			if _, exists := risks[i].Args["asset_name"]; !exists {
				risks[i].Args["asset_name"] = assetName
			}
			riskAssetID := normalizeRiskAssetID(risks[i].AssetID)
			if riskAssetID == "" {
				riskAssetID = normalizeRiskAssetID(anyToString(risks[i].Args["asset_id"]))
			}
			if riskAssetID == "" {
				riskAssetID = singleAssetID
			}
			if riskAssetID != "" {
				risks[i].AssetID = riskAssetID
				if _, exists := risks[i].Args["asset_id"]; !exists {
					risks[i].Args["asset_id"] = riskAssetID
				}
			}
		}
		logging.Info("Plugin %s found %d risks", assetName, len(risks))
		allRisks = append(allRisks, risks...)
	}

	logging.Info("Risk assessment completed, total risks: %d", len(allRisks))
	return allRisks, nil
}

func (pm *PluginManager) getAssetsByPlugin(assetName string) []Asset {
	key := normalizeAssetName(assetName)
	if key == "" {
		return []Asset{}
	}

	pm.mu.RLock()
	defer pm.mu.RUnlock()

	assets := make([]Asset, 0)
	for _, inst := range pm.instances {
		if inst == nil {
			continue
		}
		instKey := normalizeAssetName(inst.AssetName)
		if instKey == "" && inst.plugin != nil {
			instKey = normalizeAssetName(inst.plugin.GetAssetName())
		}
		if instKey != key {
			continue
		}
		assets = append(assets, inst.Asset)
	}
	return assets
}

// MitigateRisk routes a mitigation request by asset_id to the corresponding plugin instance.
// AssessRisksByPlugin evaluates risks for a single registered plugin.
func (pm *PluginManager) AssessRisksByPlugin(assetName string, scannedHashes map[string]bool) ([]Risk, error) {
	plugin := pm.GetPluginByAssetName(assetName)
	if plugin == nil {
		return nil, fmt.Errorf("plugin not found: %s", assetName)
	}

	pluginAssetName := plugin.GetAssetName()
	logging.Info("Assessing risks with single plugin: %s", pluginAssetName)
	risks, err := plugin.AssessRisks(scannedHashes)
	if err != nil {
		return nil, err
	}

	for i := range risks {
		risks[i].SourcePlugin = pluginAssetName
	}
	logging.Info("Single plugin %s found %d risks", pluginAssetName, len(risks))
	return risks, nil
}
func (pm *PluginManager) MitigateRisk(riskInfoJSON string) string {
	var req map[string]interface{}
	if err := json.Unmarshal([]byte(riskInfoJSON), &req); err != nil {
		return mitigationErrorResult("invalid json")
	}

	assetID := normalizeRiskAssetID(anyToString(req["asset_id"]))
	if assetID == "" {
		if args, ok := req["args"].(map[string]interface{}); ok {
			assetID = normalizeRiskAssetID(anyToString(args["asset_id"]))
		}
	}
	if assetID == "" {
		return mitigationErrorResult("asset_id is required")
	}

	inst, err := pm.resolvePluginInstance(assetID)
	if err != nil {
		return mitigationErrorResult(err.Error())
	}

	req["asset_id"] = assetID
	if _, exists := req["source_plugin"]; !exists {
		req["source_plugin"] = inst.AssetName
	}

	normalizedReq, err := json.Marshal(req)
	if err != nil {
		return mitigationErrorResult("invalid mitigation payload")
	}
	return normalizeMitigationResponse(inst.plugin.MitigateRisk(string(normalizedReq)))
}

func mitigationErrorResult(message string) string {
	payload := map[string]interface{}{
		"success": false,
		"data":    nil,
		"error":   message,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return `{"success":false,"data":null,"error":"internal marshal error"}`
	}
	return string(b)
}

func normalizeMitigationResponse(raw string) string {
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return mitigationErrorResult("plugin returned invalid json")
	}

	success, ok := payload["success"].(bool)
	if !ok {
		success = false
		payload["success"] = false
	}

	if _, ok := payload["data"]; !ok {
		payload["data"] = nil
	}
	if _, ok := payload["error"]; !ok {
		if success {
			payload["error"] = nil
		} else {
			payload["error"] = "unknown plugin error"
		}
	}

	b, err := json.Marshal(payload)
	if err != nil {
		return mitigationErrorResult("failed to encode mitigation response")
	}
	return string(b)
}

func (pm *PluginManager) reconcilePluginInstances(assetName string, scannedAssetIDs map[string]struct{}) {
	key := normalizeAssetName(assetName)
	if key == "" {
		return
	}
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for assetID, inst := range pm.instances {
		if inst == nil {
			delete(pm.instances, assetID)
			continue
		}

		instKey := normalizeAssetName(inst.AssetName)
		if instKey == "" && inst.plugin != nil {
			instKey = normalizeAssetName(inst.plugin.GetAssetName())
		}
		if instKey != key {
			continue
		}

		if _, exists := scannedAssetIDs[strings.TrimSpace(assetID)]; exists {
			continue
		}

		delete(pm.instances, assetID)
		logging.Info("Pruned stale plugin instance: plugin=%s, assetID=%s", assetName, assetID)
	}
}

func (pm *PluginManager) getSingleAssetIDByPlugin(assetName string) string {
	key := normalizeAssetName(assetName)
	if key == "" {
		return ""
	}
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	count := 0
	singleID := ""
	for assetID, inst := range pm.instances {
		if inst == nil {
			continue
		}
		instKey := normalizeAssetName(inst.AssetName)
		if instKey == "" && inst.plugin != nil {
			instKey = normalizeAssetName(inst.plugin.GetAssetName())
		}
		if instKey != key {
			continue
		}
		count++
		singleID = strings.TrimSpace(assetID)
		if count > 1 {
			return ""
		}
	}
	return singleID
}

func anyToString(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

func normalizeRiskAssetID(assetID string) string {
	return strings.TrimSpace(assetID)
}

func (pm *PluginManager) getAllPluginsDeterministic() []BotPlugin {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	keys := make([]string, 0, len(pm.registeredPlugins))
	for key := range pm.registeredPlugins {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	plugins := make([]BotPlugin, 0, len(keys))
	for _, key := range keys {
		plugins = append(plugins, pm.registeredPlugins[key])
	}
	return plugins
}

// getAssetInstanceCountsByPlugin returns discovered asset instance counts keyed by normalized asset name.
func (pm *PluginManager) getAssetInstanceCountsByPlugin() map[string]int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	counts := make(map[string]int)
	for _, inst := range pm.instances {
		if inst == nil {
			continue
		}
		key := normalizeAssetName(inst.AssetName)
		if key == "" && inst.plugin != nil {
			key = normalizeAssetName(inst.plugin.GetAssetName())
		}
		if key == "" {
			continue
		}
		counts[key]++
	}
	return counts
}

// StartProtection starts protection by asset instance.
func (pm *PluginManager) StartProtection(assetID string, config ProtectionConfig) error {
	inst, err := pm.resolvePluginInstance(assetID)
	if err != nil {
		return err
	}

	logging.Info("Starting protection for asset instance: asset=%s, id=%s", inst.AssetName, inst.AssetID)
	if err := inst.plugin.StartProtection(inst.AssetID, config); err != nil {
		logging.Error("Failed to start protection for asset %s/%s: %v", inst.AssetName, inst.AssetID, err)
		return err
	}

	logging.Info("Protection started for asset instance: asset=%s, id=%s", inst.AssetName, inst.AssetID)
	return nil
}

// StopProtection stops protection by asset instance.
func (pm *PluginManager) StopProtection(assetID string) error {
	inst, err := pm.resolvePluginInstance(assetID)
	if err != nil {
		return err
	}

	logging.Info("Stopping protection for asset instance: asset=%s, id=%s", inst.AssetName, inst.AssetID)
	if err := inst.plugin.StopProtection(inst.AssetID); err != nil {
		logging.Error("Failed to stop protection for asset %s/%s: %v", inst.AssetName, inst.AssetID, err)
		return err
	}

	logging.Info("Protection stopped for asset instance: asset=%s, id=%s", inst.AssetName, inst.AssetID)
	return nil
}

// GetProtectionStatus gets protection status by asset instance.
func (pm *PluginManager) GetProtectionStatus(assetID string) (ProtectionStatus, error) {
	inst, err := pm.resolvePluginInstance(assetID)
	if err != nil {
		return ProtectionStatus{}, err
	}
	return inst.plugin.GetProtectionStatus(inst.AssetID), nil
}

// GetAllProtectionStatus returns status map keyed by assetID.
func (pm *PluginManager) GetAllProtectionStatus() map[string]ProtectionStatus {
	pm.mu.RLock()
	instances := make([]*AssetPluginInstance, 0, len(pm.instances))
	for _, inst := range pm.instances {
		instances = append(instances, inst)
	}
	pm.mu.RUnlock()

	statuses := make(map[string]ProtectionStatus, len(instances))
	for _, inst := range instances {
		if inst == nil || inst.plugin == nil || strings.TrimSpace(inst.AssetID) == "" {
			continue
		}
		statuses[inst.AssetID] = inst.plugin.GetProtectionStatus(inst.AssetID)
	}
	return statuses
}

// PluginInfo describes registered plugin capability and bound instance count.
type PluginInfo struct {
	AssetName     string `json:"asset_name"`
	InstanceCount int    `json:"instance_count"`
	// RequiresBotModelConfig indicates whether protection startup must provide
	// bot_model config for this plugin.
	RequiresBotModelConfig bool `json:"requires_bot_model_config"`

	// Canonical metadata fields from BotPlugin contract.
	ID                 string                     `json:"id,omitempty"`
	PluginID           string                     `json:"plugin_id,omitempty"`
	BotType            string                     `json:"bot_type,omitempty"`
	DisplayName        string                     `json:"display_name,omitempty"`
	APIVersion         string                     `json:"api_version,omitempty"`
	Capabilities       []string                   `json:"capabilities,omitempty"`
	SupportedPlatforms []string                   `json:"supported_platforms,omitempty"`
	Manifest           *plugin_sdk.PluginManifest `json:"manifest,omitempty"`
	AssetUISchema      *plugin_sdk.AssetUISchema  `json:"asset_ui_schema,omitempty"`
}

// GetAllPluginInfos returns registered plugin capabilities with current instance counts.
func (pm *PluginManager) GetAllPluginInfos() []PluginInfo {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	instanceCountByAsset := make(map[string]int)
	for _, inst := range pm.instances {
		if inst == nil {
			continue
		}
		instanceCountByAsset[normalizeAssetName(inst.AssetName)]++
	}

	infos := make([]PluginInfo, 0, len(pm.registeredPlugins))
	for key, plugin := range pm.registeredPlugins {
		manifest := plugin.GetManifest()
		info := PluginInfo{
			AssetName:              plugin.GetAssetName(),
			InstanceCount:          instanceCountByAsset[key],
			RequiresBotModelConfig: plugin.RequiresBotModelConfig(),
			ID:                     strings.TrimSpace(plugin.GetID()),
			PluginID:               strings.TrimSpace(manifest.PluginID),
			BotType:                strings.TrimSpace(manifest.BotType),
			DisplayName:            strings.TrimSpace(manifest.DisplayName),
			APIVersion:             strings.TrimSpace(manifest.APIVersion),
			Capabilities:           append([]string{}, manifest.Capabilities...),
			SupportedPlatforms: append([]string{},
				manifest.SupportedPlatforms...),
		}
		manifestCopy := manifest
		info.Manifest = &manifestCopy
		info.AssetUISchema = cloneAssetUISchema(plugin.GetAssetUISchema())

		infos = append(infos, info)
	}
	return infos
}

func cloneAssetUISchema(schema *plugin_sdk.AssetUISchema) *plugin_sdk.AssetUISchema {
	if schema == nil {
		return nil
	}
	return schema.Clone()
}

func normalizeAssetName(assetName string) string {
	return strings.ToLower(strings.TrimSpace(assetName))
}
