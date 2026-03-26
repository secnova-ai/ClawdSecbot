package openclaw

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

// OpenclawConfig defines the configuration structure
type OpenclawConfig struct {
	Gateway struct {
		Bind string `json:"bind"`
		Host string `json:"host"`
		Port int    `json:"port"`
		Auth struct {
			Mode     string `json:"mode"`
			Token    string `json:"token"`
			Enabled  bool   `json:"enabled"`
			Password string `json:"password"`
		} `json:"auth"`
		TrustedProxies []string `json:"trustedProxies"`
	} `json:"gateway"`
	Agents struct {
		Defaults struct {
			Model   interface{} `json:"model"` // Can be string or object
			Sandbox struct {
				Mode string `json:"mode"`
			} `json:"sandbox"`
		} `json:"defaults"`
	} `json:"agents"`
	Models struct {
		Mode      string                       `json:"mode"`
		Providers map[string]*OpenclawProvider `json:"providers"`
	} `json:"models"`
	Logging struct {
		RedactSensitive string `json:"redactSensitive"`
	} `json:"logging"`
}

// OpenclawProvider represents a model provider configuration
type OpenclawProvider struct {
	BaseURL string `json:"baseUrl"`
	APIKey  string `json:"apiKey"`
	API     string `json:"api"`
}

// findConfigPath locates the Openclaw configuration file
func findConfigPath() (string, error) {
	// First, check if an external path has been set (from Flutter/sandbox)
	configPathMutex.RLock()
	override := configPathOverride
	configPathMutex.RUnlock()

	if override != "" {
		// Check for config files within the override directory
		configFiles := []string{
			filepath.Join(override, "openclaw.json"),
			filepath.Join(override, "moltbot.json"),
			filepath.Join(override, "clawdbot.json"),
		}
		for _, p := range configFiles {
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
		}
		// If the override itself is a directory like .openclaw, check parent patterns
		baseName := filepath.Base(override)
		if baseName == ".openclaw" || baseName == ".moltbot" || baseName == ".clawdbot" {
			configFiles := []string{
				filepath.Join(override, "openclaw.json"),
				filepath.Join(override, "moltbot.json"),
				filepath.Join(override, "clawdbot.json"),
			}
			for _, p := range configFiles {
				if _, err := os.Stat(p); err == nil {
					return p, nil
				}
			}
		}
	}

	// Fall back to default paths
	usr, err := user.Current()
	if err != nil {
		return "", err
	}

	paths := []string{
		filepath.Join(usr.HomeDir, ".openclaw", "openclaw.json"),
		filepath.Join(usr.HomeDir, ".moltbot", "moltbot.json"),
		filepath.Join(usr.HomeDir, ".clawdbot", "clawdbot.json"),
		filepath.Join(usr.HomeDir, ".clawdbot", "moltbot.json"),
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("config not found")
}

// loadConfig reads and parses the configuration file
func loadConfig(configPath string) (*OpenclawConfig, map[string]interface{}, error) {
	configData, err := os.ReadFile(configPath)
	if err != nil {
		return nil, nil, err
	}

	normalized := normalizeJSON5(configData)

	var config OpenclawConfig
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
func getPrimaryModelFromConfig(config *OpenclawConfig) (string, error) {
	if config == nil {
		return "", fmt.Errorf("config is nil")
	}

	switch model := config.Agents.Defaults.Model.(type) {
	case string:
		if strings.TrimSpace(model) == "" {
			return "", fmt.Errorf("primary model is empty")
		}
		return model, nil
	case map[string]interface{}:
		if primary, ok := model["primary"].(string); ok && strings.TrimSpace(primary) != "" {
			return primary, nil
		}
		return "", fmt.Errorf("primary model not found in config")
	default:
		return "", fmt.Errorf("unsupported model config type")
	}
}

// saveConfig saves the raw config back to file
func saveConfig(configPath string, rawConfig map[string]interface{}) error {
	data, err := json.MarshalIndent(rawConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	return os.WriteFile(configPath, data, 0644)
}

// GetCurrentModelProvider extracts the current model provider from config
// Returns provider name and the original baseUrl
func GetCurrentModelProvider(config *OpenclawConfig, rawConfig map[string]interface{}) (providerName string, baseURL string, err error) {
	// Parse model config to get primary model
	var primaryModel string

	switch m := config.Agents.Defaults.Model.(type) {
	case string:
		primaryModel = m
	case map[string]interface{}:
		if p, ok := m["primary"].(string); ok {
			primaryModel = p
		}
	}

	if primaryModel == "" {
		return "", "", fmt.Errorf("no primary model configured")
	}

	// Extract provider from model string (format: provider/model)
	providerName = extractProviderFromModel(primaryModel)
	providerName = normalizeProviderName(providerName)
	if providerName == "" {
		return "", "", fmt.Errorf("cannot extract provider from model: %s", primaryModel)
	}

	// Get provider's baseUrl from config
	if config.Models.Providers != nil {
		if provider, ok := config.Models.Providers[providerName]; ok && provider != nil {
			baseURL = provider.BaseURL
		}
	}

	// If no custom provider config, use default baseUrl for known providers
	if baseURL == "" {
		baseURL = getDefaultBaseURL(providerName)
	}

	return providerName, baseURL, nil
}

// normalizeProviderName trims, lowercases, and removes known prefixes.
func normalizeProviderName(providerName string) string {
	normalized := strings.ToLower(strings.TrimSpace(providerName))
	const prefix = "clawdsecbot-"
	if strings.HasPrefix(normalized, prefix) {
		return strings.TrimPrefix(normalized, prefix)
	}
	return normalized
}

// extractProviderFromModel extracts provider name from model string
func extractProviderFromModel(model string) string {
	// Model format: provider/model-name
	for i, c := range model {
		if c == '/' {
			return model[:i]
		}
	}
	// If no slash, try to infer from model name
	return inferProviderFromModel(model)
}

// inferProviderFromModel infers provider from model name using adapter's unified mapping.
func inferProviderFromModel(model string) string {
	return string(adapter.InferProviderFromModel(model))
}

// getDefaultBaseURL returns the default base URL using adapter's unified mapping.
func getDefaultBaseURL(provider string) string {
	return adapter.GetDefaultBaseURL(adapter.NormalizeProviderName(provider))
}

// SetProviderBaseURL updates the provider's baseUrl in the raw config
func SetProviderBaseURL(rawConfig map[string]interface{}, providerName string, newBaseURL string) error {
	// Ensure models.providers exists
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

	provider["baseUrl"] = newBaseURL
	return nil
}

// providerToVendor maps provider name to proxy vendor string using adapter's unified mapping.
func providerToVendor(provider string) string {
	return adapter.ProviderToVendor(adapter.NormalizeProviderName(provider))
}
