package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"go_lib/core"
	"go_lib/core/logging"
	"go_lib/core/proxy"
	"go_lib/core/repository"
	"go_lib/core/shepherd"
)

// SyncDefaultProtectionPolicyForAssets 同步默认防护策略到资产实例。
//
// 同步目标包含两类：
// 1. 当前扫描结果中的资产，且尚无资产配置；
// 2. 已存在资产配置且标记为继承默认策略的资产。
//
// 当 assets 为空时，会先全量扫描当前资产，再执行同步，
// 用于“默认策略更新后批量下发”场景。
func SyncDefaultProtectionPolicyForAssets(assets []core.Asset) error {
	repo := repository.NewProtectionRepository(nil)

	defaultConfig, err := repo.GetDefaultProtectionConfig()
	if err != nil {
		return err
	}
	if defaultConfig == nil {
		return nil
	}

	defaultUserRules, foundDefaultUserRules, err := repo.GetShepherdSensitiveActions(
		repository.DefaultProtectionPolicyAssetName,
		repository.DefaultProtectionPolicyAssetID,
	)
	if err != nil {
		logging.Warning("Default policy sync: failed to load default user rules: %v", err)
		defaultUserRules = nil
		foundDefaultUserRules = false
	}
	if !foundDefaultUserRules {
		defaultRules, defaultErr := shepherd.GetDefaultUserRules()
		if defaultErr == nil {
			defaultUserRules = defaultRules.SensitiveActions
			foundDefaultUserRules = true
		}
	}

	resolvedAssets, err := resolveAssetsForDefaultPolicySync(assets)
	if err != nil {
		return err
	}

	targets, err := collectDefaultPolicySyncTargets(repo, resolvedAssets)
	if err != nil {
		return err
	}

	snapshots := make(map[string]*defaultPolicySyncSnapshot, len(targets))
	runtimePlans := make([]*defaultPolicyRuntimePlan, 0, len(targets))
	appliedAssetIDs := make([]string, 0, len(targets))

	for assetID, assetName := range targets {
		existingConfig, err := repo.GetProtectionConfig(assetID)
		if err != nil {
			return err
		}
		previousConfig := cloneProtectionConfigForSync(existingConfig)
		previousUserRules, foundPreviousUserRules, err := repo.GetShepherdSensitiveActions(assetName, assetID)
		if err != nil {
			return err
		}
		snapshots[assetID] = &defaultPolicySyncSnapshot{
			assetID:             assetID,
			assetName:           strings.TrimSpace(assetName),
			previousConfig:      previousConfig,
			previousUserRules:   append([]string(nil), previousUserRules...),
			foundPreviousRules:  foundPreviousUserRules,
			foundCurrentRequest: true,
		}

		config := cloneProtectionConfigForSync(defaultConfig)
		config.AssetID = assetID
		config.AssetName = strings.TrimSpace(assetName)
		config.InheritsDefaultPolicy = true
		if config.AssetName == "" && existingConfig != nil {
			config.AssetName = existingConfig.AssetName
		}

		// 新扫描到的资产（无历史配置）不携带 BotModelConfig：
		// 用户尚未为该资产配置模型，继承默认策略的其他字段即可，模型留空待用户后续配置。
		// 已标记 InheritsDefaultPolicy=true 的老资产，保留从默认克隆的全字段（含 BotModelConfig）。
		if existingConfig == nil {
			config.BotModelConfig = nil
		}

		// 资产继承默认策略时，保持默认策略的 Enabled/AuditOnly 语义。
		// 这里不做“有模型即强制启用”的覆盖，避免默认策略为 disabled 时被误开启。

		if err := repo.SaveProtectionConfig(config); err != nil {
			if rollbackErr := rollbackDefaultPolicySync(repo, appliedAssetIDs, snapshots); rollbackErr != nil {
				return fmt.Errorf("default policy sync failed: %w; rollback failed: %v", err, rollbackErr)
			}
			return err
		}
		appliedAssetIDs = append(appliedAssetIDs, assetID)

		runtimeUserRules := []string(nil)
		if foundDefaultUserRules {
			runtimeUserRules = append([]string(nil), defaultUserRules...)
			if err := repo.SaveShepherdSensitiveActions(config.AssetName, assetID, runtimeUserRules); err != nil {
				if rollbackErr := rollbackDefaultPolicySync(repo, appliedAssetIDs, snapshots); rollbackErr != nil {
					return fmt.Errorf("default policy sync failed: %w; rollback failed: %v", err, rollbackErr)
				}
				return err
			}
		}

		runtimePlans = append(runtimePlans, &defaultPolicyRuntimePlan{
			previousConfig:    previousConfig,
			currentConfig:     config,
			userRules:         runtimeUserRules,
			previousUserRules: append([]string(nil), previousUserRules...),
		})
	}

	// 数据层全部成功后，再统一同步运行时，避免中途失败造成“部分写入”。
	appliedRuntimePlans := make([]*defaultPolicyRuntimePlan, 0, len(runtimePlans))
	for _, runtimePlan := range runtimePlans {
		if err := applyProtectionPolicyRuntimeForSync(runtimePlan.previousConfig, runtimePlan.currentConfig, runtimePlan.userRules); err != nil {
			// 运行时失败时执行双回滚：
			// 1) 回滚运行时已应用变更；
			// 2) 回滚数据库已写入配置。
			rollbackRuntimePlans := append(appliedRuntimePlans, runtimePlan)
			runtimeRollbackErr := rollbackRuntimePlansForDefaultPolicySync(rollbackRuntimePlans)
			dbRollbackErr := rollbackDefaultPolicySync(repo, appliedAssetIDs, snapshots)
			return combineAtomicSyncErrors(err, runtimeRollbackErr, dbRollbackErr)
		}
		appliedRuntimePlans = append(appliedRuntimePlans, runtimePlan)
		logging.Info("Default policy sync: applied to asset %s/%s", runtimePlan.currentConfig.AssetName, runtimePlan.currentConfig.AssetID)
	}

	return nil
}

// resolveAssetsForDefaultPolicySync 解析本轮需要参与默认策略同步的资产列表。
//
// 当调用方未传入资产列表时（例如默认策略更新后的全量同步场景），
// 这里会主动重新扫描当前资产，确保“未落配置的新资产”也能被同步。
func resolveAssetsForDefaultPolicySync(assets []core.Asset) ([]core.Asset, error) {
	if len(assets) > 0 {
		return assets, nil
	}

	pm := core.GetPluginManager()
	if pm == nil {
		return nil, errWithMessage("plugin manager is not initialized")
	}

	scannedAssets, err := pm.ScanAllAssets()
	if err != nil {
		return nil, err
	}
	logging.Info("Default policy sync: scanned %d assets for full synchronization", len(scannedAssets))
	return scannedAssets, nil
}

// collectDefaultPolicySyncTargets 收集需要同步默认策略的资产集合。
func collectDefaultPolicySyncTargets(repo *repository.ProtectionRepository, assets []core.Asset) (map[string]string, error) {
	targets := make(map[string]string, len(assets))

	// 1) 已有配置中，明确标记为继承默认策略的资产。
	existingConfigs, err := repo.GetAllProtectionConfigs()
	if err != nil {
		return nil, err
	}
	for _, cfg := range existingConfigs {
		if cfg == nil {
			continue
		}
		if repository.IsDefaultProtectionPolicyAssetID(cfg.AssetID) {
			continue
		}
		if !cfg.InheritsDefaultPolicy {
			continue
		}
		assetID := strings.TrimSpace(cfg.AssetID)
		if assetID == "" {
			continue
		}
		targets[assetID] = strings.TrimSpace(cfg.AssetName)
	}

	// 2) 当前扫描资产里“尚无配置”的资产，也要继承默认策略。
	for _, asset := range assets {
		assetID := strings.TrimSpace(asset.ID)
		assetName := strings.TrimSpace(asset.Name)
		if assetID == "" || assetName == "" {
			continue
		}
		existingConfig, err := repo.GetProtectionConfig(assetID)
		if err != nil {
			return nil, err
		}
		if existingConfig == nil {
			targets[assetID] = assetName
			continue
		}
		if existingConfig.InheritsDefaultPolicy {
			if targets[assetID] == "" {
				targets[assetID] = assetName
			}
		}
	}

	return targets, nil
}

// cloneProtectionConfigForSync 克隆防护配置，避免引用共享导致串改。
func cloneProtectionConfigForSync(config *repository.ProtectionConfig) *repository.ProtectionConfig {
	if config == nil {
		return nil
	}
	cloned := *config
	if config.BotModelConfig != nil {
		botModelCopy := *config.BotModelConfig
		cloned.BotModelConfig = &botModelCopy
	}
	return &cloned
}

// buildPolicyProxyConfigForSync 构建运行时代理配置。
func buildPolicyProxyConfigForSync(config *repository.ProtectionConfig) (*proxy.ProtectionConfig, error) {
	if config == nil {
		return nil, errWithMessage("protection config is nil")
	}
	if config.AssetName == "" || config.AssetID == "" {
		return nil, errWithMessage("asset identity is incomplete")
	}
	if config.BotModelConfig == nil {
		return nil, errWithMessage("bot model config is required")
	}

	securityModel, err := repository.NewSecurityModelConfigRepository(nil).Get()
	if err != nil {
		return nil, err
	}
	if securityModel == nil {
		return nil, errWithMessage("security model config is required")
	}

	return &proxy.ProtectionConfig{
		AssetName:     config.AssetName,
		AssetID:       config.AssetID,
		SecurityModel: securityModel,
		BotModel: &proxy.BotModelConfig{
			Provider:  config.BotModelConfig.Provider,
			BaseURL:   config.BotModelConfig.BaseURL,
			APIKey:    config.BotModelConfig.APIKey,
			Model:     config.BotModelConfig.Model,
			SecretKey: config.BotModelConfig.SecretKey,
		},
		Runtime: &proxy.ProtectionRuntimeConfig{
			AuditOnly:               config.AuditOnly,
			SingleSessionTokenLimit: config.SingleSessionTokenLimit,
			DailyTokenLimit:         config.DailyTokenLimit,
		},
	}, nil
}

// applyProtectionPolicyRuntimeForSync 将策略变化同步到运行时代理。
// 返回 error 表示运行时同步失败，供上层触发原子回滚。
func applyProtectionPolicyRuntimeForSync(previousConfig, config *repository.ProtectionConfig, userRules []string) error {
	if config == nil || config.AssetName == "" || config.AssetID == "" {
		return nil
	}
	if repository.IsDefaultProtectionPolicyAssetID(config.AssetID) {
		return nil
	}

	wasEnabled := previousConfig != nil && previousConfig.Enabled
	oldSandboxEnabled := previousConfig != nil && previousConfig.SandboxEnabled

	if !wasEnabled && config.Enabled {
		// 没有 BotModelConfig 时静默跳过代理启动：
		// 典型场景是新扫描资产继承默认策略但尚未配置模型，此时无法启动代理，
		// 也不应把同步流程标记为失败（业务规则：不做有模型即强制启用的覆盖）。
		if config.BotModelConfig == nil {
			logging.Info("Default policy sync: skip starting proxy for asset %s/%s due to missing bot model config", config.AssetName, config.AssetID)
			return nil
		}
		protectionConfig, err := buildPolicyProxyConfigForSync(config)
		if err != nil {
			return err
		}
		configJSON, err := json.Marshal(protectionConfig)
		if err != nil {
			return err
		}
		startResult := proxy.StartProtectionProxyInternal(string(configJSON))
		if strings.Contains(startResult, `"success":false`) {
			return fmt.Errorf("failed to start protection proxy for botId=%s: %s", config.AssetID, startResult)
		}
		return nil
	}

	if wasEnabled && !config.Enabled {
		stopResult := proxy.StopProtectionProxyByAssetInternal(config.AssetID)
		if strings.Contains(stopResult, `"success":false`) {
			return fmt.Errorf("failed to stop protection proxy for botId=%s: %s", config.AssetID, stopResult)
		}
		return nil
	}

	runningProxy := proxy.GetProxyProtectionByAsset(config.AssetID)
	if runningProxy != nil && runningProxy.IsRunning() {
		runtimeCfg := &proxy.ProtectionRuntimeConfig{
			AuditOnly:               config.AuditOnly,
			SingleSessionTokenLimit: config.SingleSessionTokenLimit,
			DailyTokenLimit:         config.DailyTokenLimit,
		}
		runningProxy.UpdateProtectionConfig(runtimeCfg)
		if userRules != nil {
			runningProxy.UpdateUserRules(userRules)
		}
	}

	if config.SandboxEnabled || oldSandboxEnabled {
		// 沙箱同步失败不阻断策略下发：与 handler_protection 的运行时同步保持一致，
		// 部分插件不提供 gateway_sandbox 能力属正常场景。
		if syncResult := core.SyncGatewaySandboxByAssetAndPlugin(config.AssetName, config.AssetID); strings.Contains(syncResult, `"success":false`) {
			logging.Warning("Default policy sync: failed to sync gateway sandbox for asset %s/%s: %s", config.AssetName, config.AssetID, syncResult)
		}
	}
	return nil
}

type defaultPolicySyncSnapshot struct {
	assetID             string
	assetName           string
	previousConfig      *repository.ProtectionConfig
	previousUserRules   []string
	foundPreviousRules  bool
	foundCurrentRequest bool
}

type defaultPolicyRuntimePlan struct {
	previousConfig    *repository.ProtectionConfig
	currentConfig     *repository.ProtectionConfig
	userRules         []string
	previousUserRules []string
}

// rollbackDefaultPolicySync 回滚已应用的默认策略同步变更，保证同步过程具备原子语义。
func rollbackDefaultPolicySync(
	repo *repository.ProtectionRepository,
	appliedAssetIDs []string,
	snapshots map[string]*defaultPolicySyncSnapshot,
) error {
	for index := len(appliedAssetIDs) - 1; index >= 0; index-- {
		assetID := appliedAssetIDs[index]
		snapshot, exists := snapshots[assetID]
		if !exists || snapshot == nil || !snapshot.foundCurrentRequest {
			continue
		}

		if snapshot.previousConfig == nil {
			if err := repo.DeleteProtectionConfig(assetID); err != nil {
				return err
			}
		} else {
			if err := repo.SaveProtectionConfig(snapshot.previousConfig); err != nil {
				return err
			}
		}

		if snapshot.foundPreviousRules {
			assetName := snapshot.assetName
			if snapshot.previousConfig != nil && strings.TrimSpace(snapshot.previousConfig.AssetName) != "" {
				assetName = strings.TrimSpace(snapshot.previousConfig.AssetName)
			}
			if err := repo.SaveShepherdSensitiveActions(assetName, assetID, snapshot.previousUserRules); err != nil {
				return err
			}
			continue
		}

		if err := repo.DeleteShepherdSensitiveActions(assetID); err != nil {
			return err
		}
	}

	logging.Info("Default policy sync: rollback completed for %d assets", len(appliedAssetIDs))
	return nil
}

// rollbackRuntimePlansForDefaultPolicySync 逆序回滚已应用的运行时策略变化。
func rollbackRuntimePlansForDefaultPolicySync(appliedPlans []*defaultPolicyRuntimePlan) error {
	for index := len(appliedPlans) - 1; index >= 0; index-- {
		plan := appliedPlans[index]
		if plan == nil || plan.currentConfig == nil {
			continue
		}

		restoreConfig := plan.previousConfig
		restoreUserRules := append([]string(nil), plan.previousUserRules...)
		if restoreConfig == nil {
			// 该资产此前无配置，回滚时将运行时恢复为关闭状态。
			restoreConfig = cloneProtectionConfigForSync(plan.currentConfig)
			restoreConfig.Enabled = false
			restoreConfig.AuditOnly = false
			restoreConfig.SandboxEnabled = false
		}

		if err := applyProtectionPolicyRuntimeForSync(plan.currentConfig, restoreConfig, restoreUserRules); err != nil {
			return err
		}
	}
	logging.Info("Default policy sync: runtime rollback completed for %d assets", len(appliedPlans))
	return nil
}

func combineAtomicSyncErrors(runtimeErr, runtimeRollbackErr, dbRollbackErr error) error {
	if runtimeRollbackErr == nil && dbRollbackErr == nil {
		return runtimeErr
	}
	if runtimeRollbackErr != nil && dbRollbackErr != nil {
		return fmt.Errorf("runtime sync failed: %v; runtime rollback failed: %v; db rollback failed: %v", runtimeErr, runtimeRollbackErr, dbRollbackErr)
	}
	if runtimeRollbackErr != nil {
		return fmt.Errorf("runtime sync failed: %v; runtime rollback failed: %v", runtimeErr, runtimeRollbackErr)
	}
	return fmt.Errorf("runtime sync failed: %v; db rollback failed: %v", runtimeErr, dbRollbackErr)
}

func errWithMessage(message string) error {
	return &syncServiceError{message: message}
}

type syncServiceError struct {
	message string
}

func (e *syncServiceError) Error() string {
	return e.message
}
