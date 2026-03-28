package nullclaw

import (
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"sync"

	"go_lib/chatmodel-routing/adapter"
)

// configPathOverride holds the externally set config directory path
var (
	configPathOverride string
	configPathMutex    sync.RWMutex
	isAppStoreBuild    bool
	appStoreMutex      sync.RWMutex
)

// SetConfigPath sets the config directory path from external source (Flutter)
// This allows the app to work within macOS sandbox with user-authorized paths
func SetConfigPath(path string) {
	configPathMutex.Lock()
	defer configPathMutex.Unlock()
	configPathOverride = path
}

// GetConfigPath returns the currently set config path override
func GetConfigPath() string {
	configPathMutex.RLock()
	defer configPathMutex.RUnlock()
	return configPathOverride
}

// SetAppStoreBuild sets whether this is an App Store build
// App Store builds run in sandbox and cannot execute shell commands
func SetAppStoreBuild(isAppStore bool) {
	appStoreMutex.Lock()
	defer appStoreMutex.Unlock()
	isAppStoreBuild = isAppStore
}

// IsAppStoreBuild returns whether this is an App Store build
func IsAppStoreBuild() bool {
	appStoreMutex.RLock()
	defer appStoreMutex.RUnlock()
	return isAppStoreBuild
}

// NullclawConfig defines the nullclaw configuration structure.
// Nullclaw uses snake_case keys in ~/.nullclaw/config.json.
type NullclawConfig struct {
	Gateway struct {
		Bind            string `json:"bind"` // compatibility field
		Host            string `json:"host"`
		Port            int    `json:"port"`
		RequirePairing  bool   `json:"require_pairing"`
		AllowPublicBind bool   `json:"allow_public_bind"`
		Auth            struct {
			Mode     string `json:"mode"`
			Token    string `json:"token"`
			Enabled  bool   `json:"enabled"`
			Password string `json:"password"`
		} `json:"auth"`
	} `json:"gateway"`
	Agents struct {
		Defaults struct {
			Model   interface{} `json:"model"` // Can be string or object
			Sandbox struct {
				Mode string `json:"mode"`
			} `json:"sandbox"`
		} `json:"defaults"`
	} `json:"agents"`
	Autonomy struct {
		Level         string `json:"level"`
		WorkspaceOnly bool   `json:"workspace_only"`
	} `json:"autonomy"`
	Security struct {
		Sandbox struct {
			Backend string `json:"backend"`
		} `json:"sandbox"`
		Audit struct {
			Enabled       bool `json:"enabled"`
			RetentionDays int  `json:"retention_days"`
		} `json:"audit"`
	} `json:"security"`
	Diagnostics struct {
		LogLLMIO          bool `json:"log_llm_io"`
		LogMessagePayload bool `json:"log_message_payloads"`
	} `json:"diagnostics"`
	Models struct {
		Providers map[string]*NullclawProvider `json:"providers"`
	} `json:"models"`
}

// NullclawProvider represents a model provider configuration.
// Nullclaw uses snake_case keys (api_key/base_url).
type NullclawProvider struct {
	BaseURL string      `json:"base_url"`
	APIKey  interface{} `json:"api_key"`
	API     string      `json:"api"`
}

// findConfigPath locates nullclaw configuration file.
func findConfigPath() (string, error) {
	configPathMutex.RLock()
	override := strings.TrimSpace(configPathOverride)
	configPathMutex.RUnlock()

	if override != "" {
		if p, ok := resolveConfigPathFromOverride(override); ok {
			return p, nil
		}
	}

	if envPath := strings.TrimSpace(os.Getenv("NULLCLAW_CONFIG")); envPath != "" {
		if p, ok := resolveConfigPathFromOverride(envPath); ok {
			return p, nil
		}
	}
	if envHome := strings.TrimSpace(os.Getenv("NULLCLAW_HOME")); envHome != "" {
		if p, ok := resolveConfigPathFromOverride(filepath.Join(envHome, "config.json")); ok {
			return p, nil
		}
	}

	usr, err := user.Current()
	if err != nil {
		return "", err
	}

	candidates := []string{
		filepath.Join(usr.HomeDir, ".nullclaw", "config.json"),
		filepath.Join(usr.HomeDir, ".config", "nullclaw", "config.json"),
		// backward-compat fallback
		filepath.Join(usr.HomeDir, ".nullclaw", "nullclaw.json"),
	}

	for _, p := range candidates {
		if isRegularFile(p) {
			return p, nil
		}
	}

	return "", fmt.Errorf("nullclaw config not found")
}

func resolveConfigPathFromOverride(override string) (string, bool) {
	override = strings.TrimSpace(override)
	if override == "" {
		return "", false
	}

	if isRegularFile(override) {
		return override, true
	}

	if !isDir(override) {
		return "", false
	}

	baseName := strings.ToLower(filepath.Base(override))
	candidates := []string{
		filepath.Join(override, "config.json"),
		filepath.Join(override, "nullclaw.json"),
	}

	// If caller passes parent dir, probe the canonical .nullclaw subdir too.
	if baseName != ".nullclaw" {
		candidates = append(candidates,
			filepath.Join(override, ".nullclaw", "config.json"),
			filepath.Join(override, ".nullclaw", "nullclaw.json"),
		)
	}

	for _, p := range candidates {
		if isRegularFile(p) {
			return p, true
		}
	}

	return "", false
}

func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// loadConfig reads and parses the configuration file.
func loadConfig(configPath string) (*NullclawConfig, map[string]interface{}, error) {
	configData, err := os.ReadFile(configPath)
	if err != nil {
		return nil, nil, err
	}

	normalized := normalizeJSON5(configData)

	var config NullclawConfig
	if err := json.Unmarshal(normalized, &config); err != nil {
		return nil, nil, err
	}

	var rawConfig map[string]interface{}
	if err := json.Unmarshal(normalized, &rawConfig); err != nil {
		return nil, nil, err
	}

	return &config, rawConfig, nil
}

// normalizeJSON5 strips comments and trailing commas from JSON5 content.
func normalizeJSON5(input []byte) []byte {
	withoutComments := stripJSON5Comments(input)
	return stripTrailingCommas(withoutComments)
}

// stripJSON5Comments removes // and /* */ comments while preserving strings.
func stripJSON5Comments(input []byte) []byte {
	var output []byte
	inString := false
	stringDelim := byte(0)
	escapeNext := false

	for i := 0; i < len(input); i++ {
		current := input[i]
		next := byte(0)
		if i+1 < len(input) {
			next = input[i+1]
		}

		if inString {
			output = append(output, current)
			if escapeNext {
				escapeNext = false
				continue
			}
			if current == '\\' {
				escapeNext = true
				continue
			}
			if current == stringDelim {
				inString = false
				stringDelim = 0
			}
			continue
		}

		if current == '"' || current == '\'' {
			inString = true
			stringDelim = current
			output = append(output, current)
			continue
		}

		if current == '/' && next == '/' {
			for i < len(input) && input[i] != '\n' {
				i++
			}
			if i < len(input) {
				output = append(output, '\n')
			}
			continue
		}
		if current == '/' && next == '*' {
			i += 2
			for i < len(input)-1 {
				if input[i] == '*' && input[i+1] == '/' {
					i++
					break
				}
				i++
			}
			continue
		}

		output = append(output, current)
	}

	return output
}

// stripTrailingCommas removes trailing commas from objects and arrays.
func stripTrailingCommas(input []byte) []byte {
	var output []byte
	inString := false
	stringDelim := byte(0)
	escapeNext := false

	for i := 0; i < len(input); i++ {
		current := input[i]
		if inString {
			output = append(output, current)
			if escapeNext {
				escapeNext = false
				continue
			}
			if current == '\\' {
				escapeNext = true
				continue
			}
			if current == stringDelim {
				inString = false
				stringDelim = 0
			}
			continue
		}

		if current == '"' || current == '\'' {
			inString = true
			stringDelim = current
			output = append(output, current)
			continue
		}

		if current == ',' {
			j := i + 1
			for j < len(input) {
				if input[j] == ' ' || input[j] == '\t' || input[j] == '\n' || input[j] == '\r' {
					j++
					continue
				}
				break
			}
			if j < len(input) && (input[j] == '}' || input[j] == ']') {
				continue
			}
		}

		output = append(output, current)
	}

	return output
}

// getPrimaryModelFromConfig extracts the primary model from config.
func getPrimaryModelFromConfig(config *NullclawConfig) (string, error) {
	if config == nil {
		return "", fmt.Errorf("config is nil")
	}

	switch model := config.Agents.Defaults.Model.(type) {
	case string:
		if strings.TrimSpace(model) == "" {
			return "", fmt.Errorf("primary model is empty")
		}
		return strings.TrimSpace(model), nil
	case map[string]interface{}:
		if primary, ok := model["primary"].(string); ok && strings.TrimSpace(primary) != "" {
			return strings.TrimSpace(primary), nil
		}
		return "", fmt.Errorf("primary model not found in config")
	default:
		return "", fmt.Errorf("unsupported model config type")
	}
}

// saveConfig saves the raw config back to file.
func saveConfig(configPath string, rawConfig map[string]interface{}) error {
	data, err := json.MarshalIndent(rawConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	return os.WriteFile(configPath, data, 0600)
}

// GetCurrentModelProvider extracts the current model provider from config.
// Returns provider name and provider base_url.
func GetCurrentModelProvider(config *NullclawConfig, rawConfig map[string]interface{}) (providerName string, baseURL string, err error) {
	primaryModel, err := getPrimaryModelFromConfig(config)
	if err != nil {
		return "", "", err
	}

	providerName = normalizeProviderName(extractProviderFromModel(primaryModel))
	if providerName == "" {
		return "", "", fmt.Errorf("cannot extract provider from model: %s", primaryModel)
	}

	if modelsMap, ok := rawConfig["models"].(map[string]interface{}); ok {
		if providersMap, ok := modelsMap["providers"].(map[string]interface{}); ok {
			if providerMap, ok := providersMap[providerName].(map[string]interface{}); ok {
				if v, ok := providerMap["base_url"].(string); ok {
					baseURL = strings.TrimSpace(v)
				}
				if baseURL == "" {
					if v, ok := providerMap["baseUrl"].(string); ok {
						baseURL = strings.TrimSpace(v)
					}
				}
			}
		}
	}

	if baseURL == "" {
		baseURL = getDefaultBaseURL(providerName)
	}

	return providerName, baseURL, nil
}

// normalizeProviderName trims, lowercases, and removes known internal prefixes.
func normalizeProviderName(providerName string) string {
	normalized := strings.ToLower(strings.TrimSpace(providerName))
	for _, prefix := range []string{"clawdsecbot-", "clawdsector-"} {
		if strings.HasPrefix(normalized, prefix) {
			normalized = strings.TrimPrefix(normalized, prefix)
		}
	}
	return normalized
}

// extractProviderFromModel extracts provider name from model string.
// Expected format: provider/model-id.
func extractProviderFromModel(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return ""
	}
	if i := strings.Index(model, "/"); i > 0 {
		return model[:i]
	}
	return inferProviderFromModel(model)
}

// inferProviderFromModel infers provider from model name using adapter mapping.
func inferProviderFromModel(model string) string {
	return string(adapter.InferProviderFromModel(model))
}

// getDefaultBaseURL returns the default base URL using adapter mapping.
func getDefaultBaseURL(provider string) string {
	return adapter.GetDefaultBaseURL(adapter.NormalizeProviderName(provider))
}

// SetProviderBaseURL updates the provider's base_url in raw config.
func SetProviderBaseURL(rawConfig map[string]interface{}, providerName string, newBaseURL string) error {
	models, ok := rawConfig["models"].(map[string]interface{})
	if !ok {
		models = make(map[string]interface{})
		rawConfig["models"] = models
	}

	providers, ok := models["providers"].(map[string]interface{})
	if !ok {
		providers = make(map[string]interface{})
		models["providers"] = providers
	}

	provider, ok := providers[providerName].(map[string]interface{})
	if !ok {
		provider = make(map[string]interface{})
		providers[providerName] = provider
	}

	provider["base_url"] = newBaseURL
	return nil
}

// providerToVendor maps provider name to proxy vendor string.
func providerToVendor(provider string) string {
	return adapter.ProviderToVendor(adapter.NormalizeProviderName(provider))
}
