package coclaw

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

const proxyProviderPrefix = "clawdsecbot-"

type providerAliasHijackResult struct {
	OriginalProviderName string
	ProxyProviderName    string
	OriginalPrimary      string
	ProxyPrimary         string
	ProxyProvider        map[string]interface{}
}

type providerAliasRestoreResult struct {
	ProxyProviderName string
	PreviousPrimary   string
	RestoredPrimary   string
}

// hijackProviderAlias 通过新增代理 provider 改写 primary，避免直接修改 CoClaw 原 provider。
func hijackProviderAlias(raw map[string]interface{}, proxyBaseURL string) (providerAliasHijackResult, bool, error) {
	result := providerAliasHijackResult{}
	proxyBaseURL = strings.TrimSpace(proxyBaseURL)
	if proxyBaseURL == "" {
		return result, false, fmt.Errorf("proxy base URL is empty")
	}

	primary, ok := readPrimary(raw)
	if !ok {
		return result, false, fmt.Errorf("primary model not found")
	}
	providerName, modelID := splitPrimary(raw, primary)
	if strings.TrimSpace(providerName) == "" || strings.TrimSpace(modelID) == "" {
		return result, false, fmt.Errorf("invalid primary model: %s", primary)
	}

	originalProviderName := stripProxyProviderPrefix(providerName)
	proxyProviderName := proxyProviderName(originalProviderName)
	proxyPrimary := proxyProviderName + "/" + modelID

	providers, ok := providersMap(raw)
	if !ok {
		return result, false, fmt.Errorf("providers not found")
	}
	originalProvider, ok := providers[originalProviderName].(map[string]interface{})
	if !ok {
		return result, false, fmt.Errorf("provider not found: %s", originalProviderName)
	}

	proxyProvider, err := cloneMap(originalProvider)
	if err != nil {
		return result, false, fmt.Errorf("clone provider failed: %w", err)
	}
	proxyProvider["baseUrl"] = proxyBaseURL

	changed := strings.TrimSpace(primary) != proxyPrimary || !reflect.DeepEqual(providers[proxyProviderName], proxyProvider)
	providers[proxyProviderName] = proxyProvider
	if err := writePrimary(raw, proxyPrimary); err != nil {
		return result, false, err
	}

	result.OriginalProviderName = originalProviderName
	result.ProxyProviderName = proxyProviderName
	result.OriginalPrimary = primary
	result.ProxyPrimary = proxyPrimary
	result.ProxyProvider = proxyProvider
	return result, changed, nil
}

// restoreProviderAlias 删除代理 provider，并优先使用初始备份恢复 primary。
func restoreProviderAlias(raw map[string]interface{}, backup map[string]interface{}) (providerAliasRestoreResult, bool, error) {
	result := providerAliasRestoreResult{}
	primary, ok := readPrimary(raw)
	if !ok {
		return result, false, nil
	}
	providerName, modelID := splitPrimary(raw, primary)
	if !isProxyProvider(providerName) {
		return result, false, nil
	}

	restoredPrimary, ok := readPrimary(backup)
	if !ok || strings.TrimSpace(restoredPrimary) == "" {
		restoredPrimary = stripProxyProviderPrefix(providerName) + "/" + modelID
	}

	providers, ok := providersMap(raw)
	if !ok {
		return result, false, fmt.Errorf("providers not found")
	}
	delete(providers, providerName)
	if err := writePrimary(raw, restoredPrimary); err != nil {
		return result, false, err
	}

	result.ProxyProviderName = providerName
	result.PreviousPrimary = primary
	result.RestoredPrimary = restoredPrimary
	return result, true, nil
}

func proxyProviderName(providerName string) string {
	providerName = strings.TrimSpace(providerName)
	if isProxyProvider(providerName) {
		return providerName
	}
	return proxyProviderPrefix + providerName
}

func isProxyProvider(providerName string) bool {
	return strings.HasPrefix(strings.TrimSpace(providerName), proxyProviderPrefix)
}

func stripProxyProviderPrefix(providerName string) string {
	providerName = strings.TrimSpace(providerName)
	if isProxyProvider(providerName) {
		return strings.TrimPrefix(providerName, proxyProviderPrefix)
	}
	return providerName
}

func readPrimary(raw map[string]interface{}) (string, bool) {
	agents, ok := raw["agents"].(map[string]interface{})
	if !ok {
		return "", false
	}
	defaults, ok := agents["defaults"].(map[string]interface{})
	if !ok {
		return "", false
	}
	switch model := defaults["model"].(type) {
	case string:
		model = strings.TrimSpace(model)
		return model, model != ""
	case map[string]interface{}:
		primary, ok := model["primary"].(string)
		primary = strings.TrimSpace(primary)
		return primary, ok && primary != ""
	default:
		return "", false
	}
}

func writePrimary(raw map[string]interface{}, primary string) error {
	primary = strings.TrimSpace(primary)
	if primary == "" {
		return fmt.Errorf("primary model is empty")
	}
	agents := ensureMap(raw, "agents")
	defaults := ensureMap(agents, "defaults")
	model, ok := defaults["model"].(map[string]interface{})
	if !ok {
		model = map[string]interface{}{}
		if current, ok := defaults["model"].(string); ok && strings.TrimSpace(current) != "" {
			model["primary"] = strings.TrimSpace(current)
		}
		defaults["model"] = model
	}
	model["primary"] = primary
	return nil
}

func splitPrimary(raw map[string]interface{}, primary string) (string, string) {
	parts := strings.SplitN(strings.TrimSpace(primary), "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return findProviderByModelID(raw, primary), primary
}

func findProviderByModelID(raw map[string]interface{}, modelID string) string {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return ""
	}
	providers, ok := providersMap(raw)
	if !ok {
		return ""
	}
	for providerName, providerValue := range providers {
		provider, ok := providerValue.(map[string]interface{})
		if !ok {
			continue
		}
		models, ok := provider["models"].([]interface{})
		if !ok {
			continue
		}
		for _, modelValue := range models {
			switch model := modelValue.(type) {
			case string:
				if strings.TrimSpace(model) == modelID {
					return strings.TrimSpace(providerName)
				}
			case map[string]interface{}:
				if id, ok := model["id"].(string); ok && strings.TrimSpace(id) == modelID {
					return strings.TrimSpace(providerName)
				}
			}
		}
	}
	return ""
}

func providersMap(raw map[string]interface{}) (map[string]interface{}, bool) {
	models, ok := raw["models"].(map[string]interface{})
	if !ok {
		return nil, false
	}
	providers, ok := models["providers"].(map[string]interface{})
	return providers, ok
}

func ensureMap(parent map[string]interface{}, key string) map[string]interface{} {
	if value, ok := parent[key].(map[string]interface{}); ok {
		return value
	}
	value := map[string]interface{}{}
	parent[key] = value
	return value
}

func cloneMap(input map[string]interface{}) (map[string]interface{}, error) {
	data, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	var output map[string]interface{}
	if err := json.Unmarshal(data, &output); err != nil {
		return nil, err
	}
	return output, nil
}
