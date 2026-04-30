package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"go_lib/core/logging"
	"go_lib/core/proxy"
	"go_lib/core/repository"
	"go_lib/core/shepherd"
)

// ========== 保护状态操作 ==========

// SaveProtectionState 保存保护状态
func SaveProtectionState(jsonStr string) map[string]interface{} {
	var state repository.ProtectionState
	if err := json.Unmarshal([]byte(jsonStr), &state); err != nil {
		logging.Error("Failed to parse protection state JSON: %v", err)
		return errorMessageResult("invalid JSON: " + err.Error())
	}

	repo := repository.NewProtectionRepository(nil)
	if err := repo.SaveProtectionState(&state); err != nil {
		logging.Error("Failed to save protection state: %v", err)
		return errorResult(err)
	}

	return successResult()
}

// GetProtectionState 获取保护状态
func GetProtectionState() map[string]interface{} {
	repo := repository.NewProtectionRepository(nil)
	state, err := repo.GetProtectionState()
	if err != nil {
		logging.Error("Failed to get protection state: %v", err)
		return errorResult(err)
	}

	if state == nil {
		return successDataResult(nil)
	}
	return successDataResult(state)
}

// ClearProtectionState 清空保护状态
func ClearProtectionState() map[string]interface{} {
	repo := repository.NewProtectionRepository(nil)
	if err := repo.ClearProtectionState(); err != nil {
		logging.Error("Failed to clear protection state: %v", err)
		return errorResult(err)
	}

	return successResult()
}

// ========== 保护配置操作 ==========

// SaveProtectionConfig 保存保护配置
// 采用 read-modify-write 模式：如果传入 JSON 未包含 bot_model_config，
// 则保留数据库中已有的 BotModelConfig，避免被覆盖擦除
func SaveProtectionConfig(jsonStr string) map[string]interface{} {
	var config repository.ProtectionConfig
	if err := json.Unmarshal([]byte(jsonStr), &config); err != nil {
		logging.Error("Failed to parse protection config JSON: %v", err)
		return errorMessageResult("invalid JSON: " + err.Error())
	}
	// 此处对原始 JSON 再做一次浅层 Unmarshal 以判断 inherits_default_policy
	// 字段是否被显式携带：Go 的零值 false 无法区分“未传”和“显式传 false”，
	// 必须借助 raw map 才能正确决定是否走“保留继承标记”分支。
	var raw map[string]json.RawMessage
	hasInheritsDefaultPolicy := false
	if err := json.Unmarshal([]byte(jsonStr), &raw); err == nil {
		_, hasInheritsDefaultPolicy = raw["inherits_default_policy"]
	}

	repo := repository.NewProtectionRepository(nil)
	hasUserInputDetectionEnabled := jsonHasKey(jsonStr, "user_input_detection_enabled")

	// 如果传入的配置没有 BotModelConfig，从数据库中读取已有的值并保留。
	// 如果没有显式传入 inherits_default_policy，仅在内容未变化时保留继承标记；
	// 内容有变化则视为资产自定义策略。
	if strings.TrimSpace(config.AssetID) != "" {
		existing, err := repo.GetProtectionConfig(config.AssetID)
		if err == nil && existing != nil {
			if config.BotModelConfig == nil && existing.BotModelConfig != nil {
				config.BotModelConfig = existing.BotModelConfig
			}
			if !hasInheritsDefaultPolicy {
				config.InheritsDefaultPolicy = shouldPreserveInheritedDefaultPolicy(existing, &config)
			}
		}
	}
	if !hasUserInputDetectionEnabled {
		config.UserInputDetectionEnabled = true
		if strings.TrimSpace(config.AssetID) != "" {
			if existing, err := repo.GetProtectionConfig(config.AssetID); err == nil && existing != nil {
				config.UserInputDetectionEnabled = existing.UserInputDetectionEnabled
			}
		}
	}

	if err := repo.SaveProtectionConfig(&config); err != nil {
		logging.Error("Failed to save protection config: %v", err)
		return errorResult(err)
	}

	return successResult()
}

// shouldPreserveInheritedDefaultPolicy 判断在 UI 未携带 inherits_default_policy
// 字段时是否保留继承标记。
//
// BotModelConfig 不参与对比：它由 SaveBotModelConfig 单独维护，
// SaveProtectionConfig 上游已经按需从数据库回填了相同的 BotModelConfig，
// 此处再比一次属于无意义比较。
func shouldPreserveInheritedDefaultPolicy(existing, incoming *repository.ProtectionConfig) bool {
	if existing == nil || incoming == nil || !existing.InheritsDefaultPolicy {
		return false
	}
	return existing.AssetName == incoming.AssetName &&
		existing.AssetID == incoming.AssetID &&
		existing.Enabled == incoming.Enabled &&
		existing.AuditOnly == incoming.AuditOnly &&
		existing.SandboxEnabled == incoming.SandboxEnabled &&
		existing.GatewayBinaryPath == incoming.GatewayBinaryPath &&
		existing.GatewayConfigPath == incoming.GatewayConfigPath &&
		existing.SingleSessionTokenLimit == incoming.SingleSessionTokenLimit &&
		existing.DailyTokenLimit == incoming.DailyTokenLimit &&
		equalJSONConfig(existing.PathPermission, incoming.PathPermission) &&
		equalJSONConfig(existing.NetworkPermission, incoming.NetworkPermission) &&
		equalJSONConfig(existing.ShellPermission, incoming.ShellPermission)
}

// equalJSONConfig 判断两个权限 JSON 是否等价。
//
// 业务约定：默认策略保存“无规则”时可能为空字符串，UI 端则会发送
// {"mode":"blacklist","paths":[]} 这类显式空规则。两者语义等价，
// 这里把空字符串与“列表字段全为空”的 JSON 视为相等，避免 UI 仅打开未改动
// 就把 inherits_default_policy 误置为 0。
func equalJSONConfig(left, right string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == right {
		return true
	}
	if left == "" || right == "" {
		other := left
		if other == "" {
			other = right
		}
		return isEmptyPermissionJSON(other)
	}

	leftCanonical, ok := canonicalizeJSON(left)
	if !ok {
		return false
	}
	rightCanonical, ok := canonicalizeJSON(right)
	if !ok {
		return false
	}
	if leftCanonical == rightCanonical {
		return true
	}
	return isEmptyPermissionJSON(left) && isEmptyPermissionJSON(right)
}

// canonicalizeJSON 将 JSON 文本反序列化后再序列化，得到稳定的 canonical 字符串。
// Go 的 map[string]interface{} marshal 时按键排序，可消除字段顺序差异。
func canonicalizeJSON(raw string) (string, bool) {
	var value interface{}
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return "", false
	}
	bytes, err := json.Marshal(value)
	if err != nil {
		return "", false
	}
	return string(bytes), true
}

// isEmptyPermissionJSON 判断 JSON 表示的权限规则是否等同于无规则。
// 所有列表字段（paths/addresses/commands）为空即视为没有任何实际规则，
// 与数据库里保存的空字符串语义等价。
func isEmptyPermissionJSON(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return true
	}
	var value interface{}
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return false
	}
	return permissionListsAreEmpty(value)
}

func permissionListsAreEmpty(value interface{}) bool {
	switch v := value.(type) {
	case map[string]interface{}:
		for key, child := range v {
			switch key {
			case "paths", "addresses", "commands":
				list, ok := child.([]interface{})
				if !ok || len(list) > 0 {
					return false
				}
			case "inbound", "outbound":
				if !permissionListsAreEmpty(child) {
					return false
				}
			}
		}
		return true
	case []interface{}:
		return len(v) == 0
	default:
		return true
	}
}

func jsonHasKey(raw, key string) bool {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return false
	}
	_, ok := payload[key]
	return ok
}

// GetProtectionConfig returns the protection config for the specified asset instance.
func GetProtectionConfig(assetID string) map[string]interface{} {
	repo := repository.NewProtectionRepository(nil)
	config, err := repo.GetProtectionConfig(assetID)
	if err != nil {
		logging.Error("Failed to get protection config: %v", err)
		return errorResult(err)
	}

	if config == nil {
		return successDataResult(nil)
	}
	return successDataResult(config)
}

// GetEnabledProtectionConfigs 获取所有启用的保护配置
func GetEnabledProtectionConfigs() map[string]interface{} {
	repo := repository.NewProtectionRepository(nil)
	configs, err := repo.GetEnabledProtectionConfigs()
	if err != nil {
		logging.Error("Failed to get enabled protection configs: %v", err)
		return errorResult(err)
	}

	return successDataResult(configs)
}

// GetActiveProtectionCount 获取正在防护中的资产数量
func GetActiveProtectionCount() map[string]interface{} {
	repo := repository.NewProtectionRepository(nil)
	configs, err := repo.GetEnabledProtectionConfigs()
	if err != nil {
		logging.Error("Failed to get enabled protection configs: %v", err)
		return errorResult(err)
	}

	count := len(configs)
	logging.Info("Active protection count: %d", count)
	return successDataResult(map[string]interface{}{"count": count})
}

// SetProtectionEnabled 设置保护启用状态
func SetProtectionEnabled(jsonStr string) map[string]interface{} {
	var input struct {
		AssetName string `json:"asset_name"`
		AssetID   string `json:"asset_id"`
		Enabled   bool   `json:"enabled"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &input); err != nil {
		return errorMessageResult("invalid JSON: " + err.Error())
	}

	repo := repository.NewProtectionRepository(nil)
	if err := repo.SetProtectionEnabled(input.AssetID, input.Enabled); err != nil {
		logging.Error("Failed to set protection enabled: %v", err)
		return errorResult(err)
	}

	return successResult()
}

// DeleteProtectionConfig removes the protection config for the specified asset instance.
func DeleteProtectionConfig(assetID string) map[string]interface{} {
	repo := repository.NewProtectionRepository(nil)
	if err := repo.DeleteProtectionConfig(assetID); err != nil {
		logging.Error("Failed to delete protection config: %v", err)
		return errorResult(err)
	}

	return successResult()
}

// ========== 保护统计操作 ==========

// SaveProtectionStatistics 保存保护统计
func SaveProtectionStatistics(jsonStr string) map[string]interface{} {
	var stats repository.ProtectionStatistics
	if err := json.Unmarshal([]byte(jsonStr), &stats); err != nil {
		logging.Error("Failed to parse protection statistics JSON: %v", err)
		return errorMessageResult("invalid JSON: " + err.Error())
	}

	repo := repository.NewProtectionRepository(nil)
	if err := repo.SaveProtectionStatistics(&stats); err != nil {
		logging.Error("Failed to save protection statistics: %v", err)
		return errorResult(err)
	}

	return successResult()
}

// GetProtectionStatistics returns the protection statistics for the specified asset instance.
func GetProtectionStatistics(assetID string) map[string]interface{} {
	repo := repository.NewProtectionRepository(nil)
	stats, err := repo.GetProtectionStatistics(assetID)
	if err != nil {
		logging.Error("Failed to get protection statistics: %v", err)
		return errorResult(err)
	}

	if stats == nil {
		return successDataResult(nil)
	}
	return successDataResult(stats)
}

// ClearProtectionStatistics clears the protection statistics for the specified asset instance.
func ClearProtectionStatistics(assetID string) map[string]interface{} {
	repo := repository.NewProtectionRepository(nil)
	if err := repo.ClearProtectionStatistics(assetID); err != nil {
		logging.Error("Failed to clear protection statistics: %v", err)
		return errorResult(err)
	}

	return successResult()
}

// ========== Shepherd规则操作 ==========

// GetShepherdRules returns the structured Shepherd rules for the specified asset instance.
func GetShepherdRules(assetID string) map[string]interface{} {
	if strings.TrimSpace(assetID) == "" {
		defaultRules, err := shepherd.GetDefaultUserRules()
		if err != nil {
			logging.Error("Failed to load default shepherd rules: %v", err)
			return errorResult(err)
		}
		return successDataResult(defaultRules)
	}
	repo := repository.NewProtectionRepository(nil)
	raw, found, err := repo.GetShepherdRulesRaw(assetID)
	if err != nil {
		logging.Error("Failed to get shepherd rules: %v", err)
		return errorResult(err)
	}
	if !found {
		defaultRules, defaultErr := shepherd.GetDefaultUserRules()
		if defaultErr != nil {
			logging.Error("Failed to load default shepherd rules: %v", defaultErr)
			return errorResult(defaultErr)
		}
		return successDataResult(defaultRules)
	}
	rules, err := shepherd.DecodeUserRulesJSON([]byte(raw))
	if err != nil {
		logging.Error("Failed to parse shepherd rules: %v", err)
		return errorResult(err)
	}
	return successDataResult(rules)
}

// SaveShepherdRules saves structured Shepherd rules for an asset instance.
func SaveShepherdRules(jsonStr string) map[string]interface{} {
	var input struct {
		AssetName     string                  `json:"asset_name"`
		AssetID       string                  `json:"asset_id"`
		SemanticRules []shepherd.SemanticRule `json:"semantic_rules"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &input); err != nil {
		return errorMessageResult("invalid JSON: " + err.Error())
	}

	if strings.TrimSpace(input.AssetID) == "" {
		return errorResult(fmt.Errorf("asset_id is required"))
	}

	rules := &shepherd.UserRules{
		SemanticRules: input.SemanticRules,
	}
	rulesJSON, err := json.Marshal(rules)
	if err != nil {
		return errorResult(err)
	}

	repo := repository.NewProtectionRepository(nil)
	if err := repo.SaveShepherdRulesRaw(input.AssetName, input.AssetID, string(rulesJSON)); err != nil {
		logging.Error("Failed to save shepherd user rules: %v", err)
		return errorResult(err)
	}
	applyRuntimeUserRules(input.AssetID, rules)

	if result := proxy.UpdateShepherdRulesByAssetInternal(input.AssetID, string(rulesJSON)); result != "ok" {
		return errorMessageResult(result)
	}

	return successResult()
}

func applyRuntimeUserRules(assetID string, rules *shepherd.UserRules) {
	pp := proxy.GetProxyProtectionByAsset(assetID)
	if pp == nil || !pp.IsRunning() {
		return
	}
	pp.UpdateUserRulesConfig(rules)
}

// ========== 全局操作 ==========

// ClearAllData 清空所有运行数据
func ClearAllData() map[string]interface{} {
	repo := repository.NewProtectionRepository(nil)
	if err := repo.ClearAllData(); err != nil {
		logging.Error("Failed to clear all data: %v", err)
		return errorResult(err)
	}

	return successResult()
}

// SaveHomeDirectoryPermission 保存Home目录授权状态
func SaveHomeDirectoryPermission(jsonStr string) map[string]interface{} {
	var input struct {
		Authorized     bool   `json:"authorized"`
		AuthorizedPath string `json:"authorized_path"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &input); err != nil {
		return errorMessageResult("invalid JSON: " + err.Error())
	}

	repo := repository.NewProtectionRepository(nil)
	if err := repo.SaveHomeDirectoryPermission(input.Authorized, input.AuthorizedPath); err != nil {
		logging.Error("Failed to save home directory permission: %v", err)
		return errorResult(err)
	}

	return successResult()
}
