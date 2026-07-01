package readyclaw

import (
	"fmt"
	"strings"

	"go_lib/core"
)

func assessReadyClawConfigRisks() []core.Risk {
	risks := []core.Risk{}

	cfg, _, configPath, err := loadConfig()
	if err != nil {
		return append(risks, core.Risk{
			ID:          "readyclaw_config_unreadable",
			SourcePlugin: readyclawPluginID,
			Title:       "ReadyClaw Config Unreadable",
			Description: fmt.Sprintf("ReadyClaw config could not be loaded: %v", err),
			Level:       core.RiskLevelHigh,
			Args:        map[string]interface{}{"config_path": configPath},
		})
	}

	protocol := strings.TrimSpace(valueString(cfg.Values, "LLM_PROTOCOL"))
	if protocol != "" && !strings.EqualFold(protocol, "openai") {
		risks = append(risks, core.Risk{
			ID:          "readyclaw_unsupported_llm_protocol",
			SourcePlugin: readyclawPluginID,
			Title:       "ReadyClaw LLM Protocol Unsupported",
			Description: "ReadyClaw protection currently supports OpenAI-compatible LLM configuration only.",
			Level:       core.RiskLevelMedium,
			Args: map[string]interface{}{
				"config_path": configPath,
				"protocol":    protocol,
			},
		})
	}

	baseURL := strings.TrimSpace(valueString(cfg.Values, "LLM_BASE_URL"))
	if baseURL == "" {
		risks = append(risks, core.Risk{
			ID:          "readyclaw_upstream_missing",
			SourcePlugin: readyclawPluginID,
			Title:       "ReadyClaw Upstream Missing",
			Description: "ReadyClaw LLM_BASE_URL is empty, so the protection proxy cannot resolve the original upstream.",
			Level:       core.RiskLevelHigh,
			Args:        map[string]interface{}{"config_path": configPath},
		})
	} else if isReadyClawProxyURL(baseURL) {
		risks = append(risks, core.Risk{
			ID:          "readyclaw_upstream_points_to_local_proxy",
			SourcePlugin: readyclawPluginID,
			Title:       "ReadyClaw Upstream Already Points to Local Proxy",
			Description: "ReadyClaw LLM_BASE_URL already points to localhost. Restore the original upstream before starting a new protection session.",
			Level:       core.RiskLevelMedium,
			Args: map[string]interface{}{
				"config_path": configPath,
				"base_url":    baseURL,
			},
		})
	}

	modelName := strings.TrimSpace(valueString(cfg.Values, "LLM_MODEL_NAME"))
	if modelName == "" {
		risks = append(risks, core.Risk{
			ID:          "readyclaw_model_missing",
			SourcePlugin: readyclawPluginID,
			Title:       "ReadyClaw Model Missing",
			Description: "ReadyClaw LLM_MODEL_NAME is empty. Requests may fail or be routed ambiguously.",
			Level:       core.RiskLevelMedium,
			Args:        map[string]interface{}{"config_path": configPath},
		})
	}

	return risks
}

