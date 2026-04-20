package service

import (
	"encoding/json"
	"strings"

	"go_lib/core"
	"go_lib/core/logging"
	"go_lib/core/repository"
)

// ========== 扫描结果操作 ==========

// SaveScanResult 保存完整的扫描结果（扫描记录+资产+风险）
// 接收JSON格式的扫描结果，解析后存入数据库
func SaveScanResult(resultJSON string) map[string]interface{} {
	var input struct {
		ConfigFound bool              `json:"config_found"`
		ConfigPath  string            `json:"config_path,omitempty"`
		ConfigJSON  string            `json:"config_json,omitempty"`
		Assets      []core.Asset      `json:"assets"`
		Risks       []json.RawMessage `json:"risks"`
	}
	if err := json.Unmarshal([]byte(resultJSON), &input); err != nil {
		logging.Error("Failed to parse scan result JSON: %v", err)
		return errorMessageResult("invalid JSON: " + err.Error())
	}

	// 将风险数据转换为core.Risk（兼容Flutter端的RiskInfo序列化格式）
	var risks []core.Risk
	for _, rawRisk := range input.Risks {
		var risk core.Risk
		if err := json.Unmarshal(rawRisk, &risk); err != nil {
			logging.Warning("Failed to parse risk: %v, skipping", err)
			continue
		}
		risks = append(risks, risk)
	}
	backfillRiskAssetIDs(input.Assets, risks)

	record := &repository.ScanRecord{
		ConfigFound: input.ConfigFound,
		ConfigPath:  input.ConfigPath,
		ConfigJSON:  input.ConfigJSON,
		Assets:      input.Assets,
		Risks:       risks,
	}

	repo := repository.NewScanRepository(nil)
	if err := repo.SaveScanResult(record); err != nil {
		logging.Error("Failed to save scan result: %v", err)
		return errorResult(err)
	}

	// 将默认防护策略同步到当前扫描资产：
	// - 为无配置资产创建继承默认策略的配置
	// - 更新已标记继承默认策略的资产到最新默认策略
	if err := SyncDefaultProtectionPolicyForAssets(input.Assets); err != nil {
		// 同步失败时回滚本次扫描写入，确保“保存+同步”原子化。
		if rollbackErr := repo.DeleteScanResultByID(record.ID); rollbackErr != nil {
			logging.Error("Failed to sync default protection policy after scan save: %v; rollback scan failed: %v", err, rollbackErr)
			return errorMessageResult(err.Error() + "; rollback scan failed: " + rollbackErr.Error())
		}
		logging.Error("Failed to sync default protection policy after scan save, scan rolled back: %v", err)
		return errorResult(err)
	}

	return map[string]interface{}{
		"success": true,
		"scan_id": record.ID,
	}
}

// GetLatestScanResult 获取最新的扫描结果
// 如果没有记录，返回 {"success": true, "data": null}
func GetLatestScanResult() map[string]interface{} {
	repo := repository.NewScanRepository(nil)
	record, err := repo.GetLatestScanResult()
	if err != nil {
		logging.Error("Failed to get latest scan result: %v", err)
		return errorResult(err)
	}

	if record == nil {
		return successDataResult(nil)
	}
	backfillRiskAssetIDs(record.Assets, record.Risks)

	return successDataResult(record)
}

func backfillRiskAssetIDs(assets []core.Asset, risks []core.Risk) {
	if len(assets) == 0 || len(risks) == 0 {
		return
	}

	pluginAssetIDs := make(map[string][]string)
	for _, asset := range assets {
		assetID := strings.TrimSpace(asset.ID)
		if assetID == "" {
			continue
		}

		pluginKey := normalizePluginKey(asset.SourcePlugin)
		if pluginKey == "" {
			pluginKey = normalizePluginKey(asset.Name)
		}
		if pluginKey == "" {
			continue
		}

		pluginAssetIDs[pluginKey] = appendUniqueString(pluginAssetIDs[pluginKey], assetID)
	}

	for i := range risks {
		if strings.TrimSpace(risks[i].AssetID) != "" {
			continue
		}

		if risks[i].Args != nil {
			if existing := strings.TrimSpace(toString(risks[i].Args["asset_id"])); existing != "" {
				risks[i].AssetID = existing
				continue
			}
		}

		pluginKey := normalizePluginKey(risks[i].SourcePlugin)
		if pluginKey == "" && risks[i].Args != nil {
			pluginKey = normalizePluginKey(toString(risks[i].Args["source_plugin"]))
		}
		if pluginKey == "" && risks[i].Args != nil {
			pluginKey = normalizePluginKey(toString(risks[i].Args["asset_name"]))
		}
		if pluginKey == "" {
			continue
		}

		ids := pluginAssetIDs[pluginKey]
		if len(ids) != 1 {
			continue
		}

		if risks[i].Args == nil {
			risks[i].Args = map[string]interface{}{}
		}
		risks[i].AssetID = ids[0]
		risks[i].Args["asset_id"] = ids[0]
	}
}

func appendUniqueString(list []string, value string) []string {
	for _, item := range list {
		if item == value {
			return list
		}
	}
	return append(list, value)
}

func normalizePluginKey(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}

func toString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// ========== 技能扫描操作 ==========

// GetScannedSkillHashes 获取所有已扫描技能的哈希值列表
func GetScannedSkillHashes() map[string]interface{} {
	repo := repository.NewSkillSecurityScanRepository(nil)
	hashes, err := repo.GetScannedSkillHashes()
	if err != nil {
		logging.Error("Failed to get scanned skill hashes: %v", err)
		return errorResult(err)
	}

	return successDataResult(hashes)
}

// SaveSkillScanResult 保存技能扫描结果
func SaveSkillScanResult(jsonStr string) map[string]interface{} {
	var record repository.SkillScanRecord
	if err := json.Unmarshal([]byte(jsonStr), &record); err != nil {
		logging.Error("Failed to parse skill scan result JSON: %v", err)
		return errorMessageResult("invalid JSON: " + err.Error())
	}

	repo := repository.NewSkillSecurityScanRepository(nil)
	if err := repo.SaveSkillScanResult(&record); err != nil {
		logging.Error("Failed to save skill scan result: %v", err)
		return errorResult(err)
	}

	return successResult()
}

// GetSkillScanByHash 根据哈希值查询技能扫描结果
func GetSkillScanByHash(hash string) map[string]interface{} {
	repo := repository.NewSkillSecurityScanRepository(nil)
	record, err := repo.GetSkillScanByHash(hash)
	if err != nil {
		logging.Error("Failed to get skill scan by hash: %v", err)
		return errorResult(err)
	}

	if record == nil {
		return successDataResult(nil)
	}
	return successDataResult(record)
}

// DeleteSkillScan 根据技能哈希删除扫描记录
func DeleteSkillScan(skillHash string) map[string]interface{} {
	repo := repository.NewSkillSecurityScanRepository(nil)
	if err := repo.DeleteSkillScan(skillHash); err != nil {
		logging.Error("Failed to delete skill scan: %v", err)
		return errorResult(err)
	}

	return successResult()
}

// GetRiskySkills 获取所有有安全风险的技能
func GetRiskySkills() map[string]interface{} {
	repo := repository.NewSkillSecurityScanRepository(nil)
	records, err := repo.GetRiskySkills()
	if err != nil {
		logging.Error("Failed to get risky skills: %v", err)
		return errorResult(err)
	}

	return successDataResult(records)
}

// GetAllSkillScans retrieves all skill scan records
func GetAllSkillScans() map[string]interface{} {
	repo := repository.NewSkillSecurityScanRepository(nil)
	records, err := repo.GetAllSkillScans()
	if err != nil {
		logging.Error("Failed to get all skill scans: %v", err)
		return errorResult(err)
	}

	return successDataResult(records)
}

// TrustSkill marks a skill as trusted (user accepts known risks)
func TrustSkill(skillHash string) map[string]interface{} {
	repo := repository.NewSkillSecurityScanRepository(nil)
	if err := repo.TrustSkill(skillHash); err != nil {
		logging.Error("Failed to trust skill: %v", err)
		return errorResult(err)
	}
	return successResult()
}
