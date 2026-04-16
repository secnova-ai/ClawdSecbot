package hermes

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

var (
	configPathOverride string
	configPathMutex    sync.RWMutex
	isAppStoreBuild    bool
	appStoreMutex      sync.RWMutex
)

// SetConfigPath sets config path override from external caller.
func SetConfigPath(path string) {
	configPathMutex.Lock()
	defer configPathMutex.Unlock()
	configPathOverride = strings.TrimSpace(path)
}

// GetConfigPath returns current config path override.
func GetConfigPath() string {
	configPathMutex.RLock()
	defer configPathMutex.RUnlock()
	return configPathOverride
}

// SetAppStoreBuild sets whether current binary runs under App Store restrictions.
func SetAppStoreBuild(isAppStore bool) {
	appStoreMutex.Lock()
	defer appStoreMutex.Unlock()
	isAppStoreBuild = isAppStore
}

// IsAppStoreBuild reports whether current binary runs under App Store restrictions.
func IsAppStoreBuild() bool {
	appStoreMutex.RLock()
	defer appStoreMutex.RUnlock()
	return isAppStoreBuild
}

// HermesConfig captures only fields used by scanner/risk/protection.
type HermesConfig struct {
	Model struct {
		Default  string `yaml:"default"`
		Provider string `yaml:"provider"`
		BaseURL  string `yaml:"base_url"`
		APIKey   string `yaml:"api_key"`
	} `yaml:"model"`
	Terminal struct {
		Backend string `yaml:"backend"`
	} `yaml:"terminal"`
	Approvals struct {
		Mode string `yaml:"mode"`
	} `yaml:"approvals"`
	Security struct {
		RedactSecrets *bool `yaml:"redact_secrets"`
	} `yaml:"security"`
}

func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
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

	candidates := []string{
		filepath.Join(override, "config.yaml"),
		filepath.Join(override, "config.yml"),
	}
	if strings.ToLower(filepath.Base(override)) != ".hermes" {
		candidates = append(candidates,
			filepath.Join(override, ".hermes", "config.yaml"),
			filepath.Join(override, ".hermes", "config.yml"),
		)
	}
	for _, p := range candidates {
		if isRegularFile(p) {
			return p, true
		}
	}
	return "", false
}

// findConfigPath locates Hermes config file.
func findConfigPath() (string, error) {
	configPathMutex.RLock()
	override := strings.TrimSpace(configPathOverride)
	configPathMutex.RUnlock()
	if override != "" {
		if p, ok := resolveConfigPathFromOverride(override); ok {
			return p, nil
		}
	}

	if cfg := strings.TrimSpace(os.Getenv("HERMES_CONFIG")); cfg != "" {
		if p, ok := resolveConfigPathFromOverride(cfg); ok {
			return p, nil
		}
	}
	if home := strings.TrimSpace(os.Getenv("HERMES_HOME")); home != "" {
		if p, ok := resolveConfigPathFromOverride(filepath.Join(home, "config.yaml")); ok {
			return p, nil
		}
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	candidates := []string{
		filepath.Join(homeDir, ".hermes", "config.yaml"),
		filepath.Join(homeDir, ".hermes", "config.yml"),
		filepath.Join(homeDir, ".config", "hermes", "config.yaml"),
		filepath.Join(homeDir, ".config", "hermes", "config.yml"),
	}
	for _, p := range candidates {
		if isRegularFile(p) {
			return p, nil
		}
	}
	return "", fmt.Errorf("hermes config not found")
}

// loadConfig reads and parses Hermes config.
func loadConfig(configPath string) (*HermesConfig, map[string]interface{}, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, nil, err
	}

	var cfg HermesConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, nil, err
	}

	raw := map[string]interface{}{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, nil, err
	}

	return &cfg, raw, nil
}

// saveConfig writes Hermes config back to disk.
func saveConfig(configPath string, rawConfig map[string]interface{}) error {
	data, err := yaml.Marshal(rawConfig)
	if err != nil {
		return fmt.Errorf("marshal config failed: %w", err)
	}
	return os.WriteFile(configPath, data, 0600)
}

func ensureMap(parent map[string]interface{}, key string) map[string]interface{} {
	if parent == nil {
		return map[string]interface{}{}
	}
	if v, ok := parent[key].(map[string]interface{}); ok {
		return v
	}
	if v, ok := parent[key].(map[interface{}]interface{}); ok {
		out := map[string]interface{}{}
		for k, vv := range v {
			ks, ok := k.(string)
			if !ok {
				continue
			}
			out[ks] = vv
		}
		parent[key] = out
		return out
	}
	out := map[string]interface{}{}
	parent[key] = out
	return out
}

func getNestedString(raw map[string]interface{}, keys ...string) string {
	if raw == nil || len(keys) == 0 {
		return ""
	}
	cur := raw
	for i, key := range keys {
		val, ok := cur[key]
		if !ok {
			return ""
		}
		if i == len(keys)-1 {
			s, _ := val.(string)
			return strings.TrimSpace(s)
		}
		next, ok := val.(map[string]interface{})
		if !ok {
			return ""
		}
		cur = next
	}
	return ""
}

func getNestedBool(raw map[string]interface{}, keys ...string) (bool, bool) {
	if raw == nil || len(keys) == 0 {
		return false, false
	}
	cur := raw
	for i, key := range keys {
		val, ok := cur[key]
		if !ok {
			return false, false
		}
		if i == len(keys)-1 {
			b, ok := val.(bool)
			return b, ok
		}
		next, ok := val.(map[string]interface{})
		if !ok {
			return false, false
		}
		cur = next
	}
	return false, false
}
