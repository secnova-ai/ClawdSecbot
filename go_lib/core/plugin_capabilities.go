package core

import (
	"encoding/json"
	"fmt"
	"strings"

	"go_lib/core/logging"
)

// SkillScanCapability defines optional plugin capability for skill security scan flows.
type SkillScanCapability interface {
	StartSkillSecurityScan(skillPath, modelConfigJSON string) string
	GetSkillSecurityScanLog(scanID string) string
	GetSkillSecurityScanResult(scanID string) string
	CancelSkillSecurityScan(scanID string) string
	StartBatchSkillScan() string
	GetBatchSkillScanLog(batchID string) string
	GetBatchSkillScanResults(batchID string) string
	CancelBatchSkillScan(batchID string) string
}

// ModelConnectionCapability defines optional plugin capability for model connectivity tests.
type ModelConnectionCapability interface {
	TestModelConnection(configJSON string) string
}

// SkillManagementCapability defines optional plugin capability for skill/file management.
type SkillManagementCapability interface {
	DeleteSkill(skillPath string) string
}

// GatewaySandboxCapability defines optional plugin capability for gateway sandbox synchronization.
type GatewaySandboxCapability interface {
	SyncGatewaySandbox() string
	SyncGatewaySandboxByAsset(assetID string) string
	HasInitialBackup() string
	RestoreToInitialConfig() string
}

// ApplicationLifecycleCapability defines optional plugin capability for
// application-exit orchestration and best-effort bot state restoration.
type ApplicationLifecycleCapability interface {
	OnAppExit(assetID string) string
	RestoreBotDefaultState(assetID string) string
}

func capabilityError(err error) string {
	payload, marshalErr := json.Marshal(map[string]interface{}{
		"success": false,
		"error":   err.Error(),
	})
	if marshalErr != nil {
		return `{"success":false,"error":"internal error"}`
	}
	return string(payload)
}

func pickPluginFromDiscoveredAssets(matched []BotPlugin) BotPlugin {
	if len(matched) == 0 {
		return nil
	}

	counts := GetPluginManager().getAssetInstanceCountsByPlugin()
	bestCount := 0
	var best BotPlugin
	tied := false
	for _, plugin := range matched {
		count := counts[normalizeAssetName(plugin.GetAssetName())]
		if count > bestCount {
			bestCount = count
			best = plugin
			tied = false
		} else if count > 0 && count == bestCount {
			tied = true
		}
	}
	if bestCount > 0 && !tied {
		return best
	}
	return nil
}

func pickLegacyDefaultPlugin(matched []BotPlugin) BotPlugin {
	// Preserve historical behavior when no explicit asset was provided.
	for _, plugin := range matched {
		if normalizeAssetName(plugin.GetAssetName()) == "openclaw" {
			return plugin
		}
	}
	return nil
}

func resolvePluginByCapability(assetName, capability string, supports func(BotPlugin) bool) (BotPlugin, error) {
	pm := GetPluginManager()
	assetName = strings.TrimSpace(assetName)
	if assetName != "" {
		plugin := pm.GetPluginByAssetName(assetName)
		if plugin == nil {
			return nil, fmt.Errorf("no plugin found for asset: %s", assetName)
		}
		if !supports(plugin) {
			return nil, fmt.Errorf("plugin %s does not support capability: %s", plugin.GetAssetName(), capability)
		}
		return plugin, nil
	}

	plugins := pm.getAllPluginsDeterministic()
	matched := make([]BotPlugin, 0, len(plugins))
	for _, plugin := range plugins {
		if supports(plugin) {
			matched = append(matched, plugin)
		}
	}

	if len(matched) == 0 {
		return nil, fmt.Errorf("no plugin supports capability: %s", capability)
	}
	if len(matched) > 1 {
		if plugin := pickPluginFromDiscoveredAssets(matched); plugin != nil {
			return plugin, nil
		}
		if plugin := pickLegacyDefaultPlugin(matched); plugin != nil {
			return plugin, nil
		}
		return nil, fmt.Errorf("multiple plugins support capability %s; specify asset_name", capability)
	}
	return matched[0], nil
}

func StartSkillSecurityScanByPlugin(assetName, skillPath, modelConfigJSON string) string {
	plugin, err := resolvePluginByCapability(assetName, "skill_scan", func(p BotPlugin) bool {
		_, ok := p.(SkillScanCapability)
		return ok
	})
	if err != nil {
		return capabilityError(err)
	}
	return plugin.(SkillScanCapability).StartSkillSecurityScan(skillPath, modelConfigJSON)
}

func GetSkillSecurityScanLogByPlugin(assetName, scanID string) string {
	plugin, err := resolvePluginByCapability(assetName, "skill_scan", func(p BotPlugin) bool {
		_, ok := p.(SkillScanCapability)
		return ok
	})
	if err != nil {
		return capabilityError(err)
	}
	return plugin.(SkillScanCapability).GetSkillSecurityScanLog(scanID)
}

func GetSkillSecurityScanResultByPlugin(assetName, scanID string) string {
	plugin, err := resolvePluginByCapability(assetName, "skill_scan", func(p BotPlugin) bool {
		_, ok := p.(SkillScanCapability)
		return ok
	})
	if err != nil {
		return capabilityError(err)
	}
	return plugin.(SkillScanCapability).GetSkillSecurityScanResult(scanID)
}

func CancelSkillSecurityScanByPlugin(assetName, scanID string) string {
	plugin, err := resolvePluginByCapability(assetName, "skill_scan", func(p BotPlugin) bool {
		_, ok := p.(SkillScanCapability)
		return ok
	})
	if err != nil {
		return capabilityError(err)
	}
	return plugin.(SkillScanCapability).CancelSkillSecurityScan(scanID)
}

func StartBatchSkillScanByPlugin(assetName string) string {
	plugin, err := resolvePluginByCapability(assetName, "skill_scan", func(p BotPlugin) bool {
		_, ok := p.(SkillScanCapability)
		return ok
	})
	if err != nil {
		return capabilityError(err)
	}
	return plugin.(SkillScanCapability).StartBatchSkillScan()
}

func GetBatchSkillScanLogByPlugin(assetName, batchID string) string {
	plugin, err := resolvePluginByCapability(assetName, "skill_scan", func(p BotPlugin) bool {
		_, ok := p.(SkillScanCapability)
		return ok
	})
	if err != nil {
		return capabilityError(err)
	}
	return plugin.(SkillScanCapability).GetBatchSkillScanLog(batchID)
}

func GetBatchSkillScanResultsByPlugin(assetName, batchID string) string {
	plugin, err := resolvePluginByCapability(assetName, "skill_scan", func(p BotPlugin) bool {
		_, ok := p.(SkillScanCapability)
		return ok
	})
	if err != nil {
		return capabilityError(err)
	}
	return plugin.(SkillScanCapability).GetBatchSkillScanResults(batchID)
}

func CancelBatchSkillScanByPlugin(assetName, batchID string) string {
	plugin, err := resolvePluginByCapability(assetName, "skill_scan", func(p BotPlugin) bool {
		_, ok := p.(SkillScanCapability)
		return ok
	})
	if err != nil {
		return capabilityError(err)
	}
	return plugin.(SkillScanCapability).CancelBatchSkillScan(batchID)
}

func TestModelConnectionByPlugin(assetName, configJSON string) string {
	plugin, err := resolvePluginByCapability(assetName, "model_connection_test", func(p BotPlugin) bool {
		_, ok := p.(ModelConnectionCapability)
		return ok
	})
	if err != nil {
		return capabilityError(err)
	}
	return plugin.(ModelConnectionCapability).TestModelConnection(configJSON)
}

// parseCapabilityResult 解析 capability 返回 JSON，提取 success/error。
func parseCapabilityResult(raw string) (bool, string) {
	parsed := make(map[string]interface{})
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return false, fmt.Sprintf("invalid capability response: %v", err)
	}
	if success, ok := parsed["success"].(bool); ok && success {
		return true, ""
	}
	if errMsg, ok := parsed["error"].(string); ok && strings.TrimSpace(errMsg) != "" {
		return false, errMsg
	}
	return false, "capability returned unsuccessful result"
}

func DeleteSkillByPlugin(assetName, skillPath string) string {
	assetName = strings.TrimSpace(assetName)
	skillPath = strings.TrimSpace(skillPath)

	if assetName != "" {
		plugin, err := resolvePluginByCapability(assetName, "delete_skill", func(p BotPlugin) bool {
			_, ok := p.(SkillManagementCapability)
			return ok
		})
		if err != nil {
			return capabilityError(err)
		}
		return plugin.(SkillManagementCapability).DeleteSkill(skillPath)
	}

	pm := GetPluginManager()
	plugins := pm.getAllPluginsDeterministic()
	matched := make([]BotPlugin, 0, len(plugins))
	for _, plugin := range plugins {
		if _, ok := plugin.(SkillManagementCapability); ok {
			matched = append(matched, plugin)
		}
	}
	if len(matched) == 0 {
		return capabilityError(fmt.Errorf("no plugin supports capability: delete_skill"))
	}
	if len(matched) == 1 {
		return matched[0].(SkillManagementCapability).DeleteSkill(skillPath)
	}
	if skillPath == "" {
		return capabilityError(fmt.Errorf("skill_path is required when multiple plugins support capability: delete_skill"))
	}

	logging.Info("[DeleteSkillByPlugin] auto routing delete skill path: %s", skillPath)
	failedReasons := make([]string, 0, len(matched))
	for _, plugin := range matched {
		response := plugin.(SkillManagementCapability).DeleteSkill(skillPath)
		success, errMsg := parseCapabilityResult(response)
		if success {
			logging.Info(
				"[DeleteSkillByPlugin] delete skill succeeded via plugin=%s path=%s",
				plugin.GetAssetName(),
				skillPath,
			)
			return response
		}
		failedReasons = append(
			failedReasons,
			fmt.Sprintf("%s: %s", plugin.GetAssetName(), errMsg),
		)
	}

	logging.Warning(
		"[DeleteSkillByPlugin] delete skill failed after auto routing path=%s errors=%s",
		skillPath,
		strings.Join(failedReasons, "; "),
	)
	return capabilityError(
		fmt.Errorf(
			"failed to route delete_skill by path %q: %s",
			skillPath,
			strings.Join(failedReasons, "; "),
		),
	)
}

func SyncGatewaySandboxByPlugin(assetName string) string {
	plugin, err := resolvePluginByCapability(assetName, "gateway_sandbox", func(p BotPlugin) bool {
		_, ok := p.(GatewaySandboxCapability)
		return ok
	})
	if err != nil {
		return capabilityError(err)
	}
	return plugin.(GatewaySandboxCapability).SyncGatewaySandbox()
}

func SyncGatewaySandboxByAssetAndPlugin(assetName, assetID string) string {
	assetID = strings.TrimSpace(assetID)
	assetName = strings.TrimSpace(assetName)

	// Instance isolation must always be driven by asset_id when provided.
	if assetID != "" {
		plugin := GetPluginManager().GetPluginByAssetID(assetID)
		if plugin == nil {
			return capabilityError(fmt.Errorf("no plugin found for asset_id: %s", assetID))
		}
		cap, ok := plugin.(GatewaySandboxCapability)
		if !ok {
			return capabilityError(fmt.Errorf("plugin %s does not support capability: gateway_sandbox", plugin.GetAssetName()))
		}
		return cap.SyncGatewaySandboxByAsset(assetID)
	}

	plugin, err := resolvePluginByCapability(assetName, "gateway_sandbox", func(p BotPlugin) bool {
		_, ok := p.(GatewaySandboxCapability)
		return ok
	})
	if err != nil {
		return capabilityError(err)
	}
	return plugin.(GatewaySandboxCapability).SyncGatewaySandbox()
}

func HasInitialBackupByPlugin(assetName string) string {
	plugin, err := resolvePluginByCapability(assetName, "gateway_sandbox", func(p BotPlugin) bool {
		_, ok := p.(GatewaySandboxCapability)
		return ok
	})
	if err != nil {
		return capabilityError(err)
	}
	return plugin.(GatewaySandboxCapability).HasInitialBackup()
}

// HasInitialBackupByAssetID resolves the plugin from assetID and checks initial backup.
func HasInitialBackupByAssetID(assetID string) string {
	assetID = strings.TrimSpace(assetID)
	if assetID == "" {
		return HasInitialBackupByPlugin("")
	}
	plugin := GetPluginManager().GetPluginByAssetID(assetID)
	if plugin == nil {
		return capabilityError(fmt.Errorf("no plugin found for asset_id: %s", assetID))
	}
	cap, ok := plugin.(GatewaySandboxCapability)
	if !ok {
		return capabilityError(fmt.Errorf("plugin %s does not support capability: gateway_sandbox", plugin.GetAssetName()))
	}
	return cap.HasInitialBackup()
}

func RestoreToInitialConfigByPlugin(assetName string) string {
	plugin, err := resolvePluginByCapability(assetName, "gateway_sandbox", func(p BotPlugin) bool {
		_, ok := p.(GatewaySandboxCapability)
		return ok
	})
	if err != nil {
		return capabilityError(err)
	}
	return plugin.(GatewaySandboxCapability).RestoreToInitialConfig()
}

// RestoreToInitialConfigByAssetID resolves the plugin from assetID and restores initial config.
func RestoreToInitialConfigByAssetID(assetID string) string {
	assetID = strings.TrimSpace(assetID)
	if assetID == "" {
		return RestoreToInitialConfigByPlugin("")
	}
	plugin := GetPluginManager().GetPluginByAssetID(assetID)
	if plugin == nil {
		return capabilityError(fmt.Errorf("no plugin found for asset_id: %s", assetID))
	}
	cap, ok := plugin.(GatewaySandboxCapability)
	if !ok {
		return capabilityError(fmt.Errorf("plugin %s does not support capability: gateway_sandbox", plugin.GetAssetName()))
	}
	return cap.RestoreToInitialConfig()
}

func NotifyAppExitByPlugin(assetName, assetID string) string {
	assetID = strings.TrimSpace(assetID)
	if assetID != "" {
		plugin := GetPluginManager().GetPluginByAssetID(assetID)
		if plugin == nil {
			return capabilityError(fmt.Errorf("no plugin found for asset_id: %s", assetID))
		}
		cap, ok := plugin.(ApplicationLifecycleCapability)
		if !ok {
			return capabilityError(fmt.Errorf("plugin %s does not support capability: application_exit", plugin.GetAssetName()))
		}
		return cap.OnAppExit(assetID)
	}
	plugin, err := resolvePluginByCapability(assetName, "application_exit", func(p BotPlugin) bool {
		_, ok := p.(ApplicationLifecycleCapability)
		return ok
	})
	if err != nil {
		return capabilityError(err)
	}
	return plugin.(ApplicationLifecycleCapability).OnAppExit("")
}

func RestoreBotDefaultStateByPlugin(assetName, assetID string) string {
	assetID = strings.TrimSpace(assetID)
	if assetID != "" {
		plugin := GetPluginManager().GetPluginByAssetID(assetID)
		if plugin == nil {
			return capabilityError(fmt.Errorf("no plugin found for asset_id: %s", assetID))
		}
		cap, ok := plugin.(ApplicationLifecycleCapability)
		if !ok {
			return capabilityError(fmt.Errorf("plugin %s does not support capability: restore_bot_default_state", plugin.GetAssetName()))
		}
		return cap.RestoreBotDefaultState(assetID)
	}
	plugin, err := resolvePluginByCapability(assetName, "restore_bot_default_state", func(p BotPlugin) bool {
		_, ok := p.(ApplicationLifecycleCapability)
		return ok
	})
	if err != nil {
		return capabilityError(err)
	}
	return plugin.(ApplicationLifecycleCapability).RestoreBotDefaultState("")
}
