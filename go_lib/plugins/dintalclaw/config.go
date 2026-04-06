package dintalclaw

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"go_lib/core/logging"
)

// DintalclawConfig 配置表示（从 mykey.py 中解析的 Python dict 块）
type DintalclawConfig struct {
	ConfigPath  string
	InstallRoot string
	Blocks      map[string]map[string]string
	RawContent  string
}

// ProviderConfig 表示一个有效的 LLM provider 配置块
type ProviderConfig struct {
	BlockName string `json:"block_name"` // 如 oai_config、oai_config2、claude_config
	Provider  string `json:"provider"`   // "openai" 或 "anthropic"
	APIKey    string `json:"apikey"`
	APIBase   string `json:"apibase"`
	Model     string `json:"model"`
}

// dictBlockRegex 匹配 Python dict 赋值块: varname = { ... }
var dictBlockRegex = regexp.MustCompile(`(?ms)^(\w+)\s*=\s*\{([^}]*)\}`)

// dictEntryRegex 匹配 dict 内的键值对: 'key': 'value' 或 "key": "value"
var dictEntryRegex = regexp.MustCompile(`['"](\w+)['"]\s*:\s*['"]([^'"]*?)['"]`)

// simpleAssignRegex 匹配简单赋值: varname = 'value' 或 varname = "value"
var simpleAssignRegex = regexp.MustCompile(`(?m)^(\w+)\s*=\s*['"]([^'"]*?)['"]`)

// commentedProxyRegex 匹配被注释的 proxy 行
var commentedProxyRegex = regexp.MustCompile(`(?m)^#\s*proxy\s*=\s*['"]([^'"]*?)['"]`)

// findConfigPathForDintalclaw 查找 dintalclaw 配置文件路径
func findConfigPathForDintalclaw() (string, error) {
	root := findInstallRoot()
	if root == "" {
		return "", fmt.Errorf("dintalclaw install root not found")
	}
	configPath := filepath.Join(root, configFileName)
	if _, err := os.Stat(configPath); err != nil {
		return "", fmt.Errorf("config file not found: %s", configPath)
	}
	return configPath, nil
}

// loadDintalclawConfig 读取并解析 mykey.py 配置文件
func loadDintalclawConfig(configPath string) (*DintalclawConfig, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read config failed: %w", err)
	}

	content := string(data)
	config := &DintalclawConfig{
		ConfigPath:  configPath,
		InstallRoot: filepath.Dir(configPath),
		Blocks:      make(map[string]map[string]string),
		RawContent:  content,
	}

	matches := dictBlockRegex.FindAllStringSubmatch(content, -1)
	for _, m := range matches {
		blockName := strings.TrimSpace(m[1])
		blockBody := m[2]
		entries := make(map[string]string)
		entryMatches := dictEntryRegex.FindAllStringSubmatch(blockBody, -1)
		for _, em := range entryMatches {
			entries[em[1]] = em[2]
		}
		config.Blocks[blockName] = entries
	}

	simpleMatches := simpleAssignRegex.FindAllStringSubmatch(content, -1)
	for _, sm := range simpleMatches {
		varName := strings.TrimSpace(sm[1])
		if _, exists := config.Blocks[varName]; !exists {
			config.Blocks[varName] = map[string]string{"_value": sm[2]}
		}
	}

	return config, nil
}

// GetProviderConfigs 从已解析的配置中提取所有有效的 oai_config* 和 claude_config* 块,
// 按块名前缀映射到 "openai" 或 "anthropic" provider。
// 排除 clawdsecbot 自动注入的 oai_config0 块。
func GetProviderConfigs(config *DintalclawConfig) []ProviderConfig {
	var providers []ProviderConfig

	for blockName, entries := range config.Blocks {
		var provider string
		if strings.HasPrefix(blockName, "oai_config") {
			provider = "openai"
		} else if strings.HasPrefix(blockName, "claude_config") {
			provider = "anthropic"
		} else {
			continue
		}

		apiKey := entries["apikey"]
		if apiKey == "" {
			continue
		}

		// 排除 clawdsecbot 自动注入块
		if apiKey == "clawdsecbot-proxy-key" {
			continue
		}

		pc := ProviderConfig{
			BlockName: blockName,
			Provider:  provider,
			APIKey:    apiKey,
			APIBase:   entries["apibase"],
			Model:     entries["model"],
		}
		providers = append(providers, pc)
	}

	return providers
}

// GetProviderConfigsFromFile 从 mykey.py 文件中直接提取所有有效 provider 配置
func GetProviderConfigsFromFile(configPath string) ([]ProviderConfig, error) {
	config, err := loadDintalclawConfig(configPath)
	if err != nil {
		return nil, err
	}
	return GetProviderConfigs(config), nil
}

// injectOaiConfig0 向 mykey.py 顶部注入 oai_config0 块（指向本地代理）
func injectOaiConfig0(configPath string, proxyURL string, apiKey string, modelName string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read config failed: %w", err)
	}
	content := string(data)

	if strings.Contains(content, "oai_config0") {
		logging.Info("[DintalclawConfig] oai_config0 already exists, updating")
		content = removeOaiConfig0Block(content)
	}

	block := buildOaiConfig0Block(proxyURL, apiKey, modelName)
	trimmed := strings.TrimLeft(content, "\n")
	content = block + "\n\n" + trimmed

	if err := os.WriteFile(configPath, []byte(content), 0600); err != nil {
		return fmt.Errorf("write config failed: %w", err)
	}

	logging.Info("[DintalclawConfig] Injected oai_config0 with proxy=%s", proxyURL)
	return nil
}

// buildOaiConfig0Block 构造注入的 oai_config0 Python dict 块
func buildOaiConfig0Block(proxyURL string, apiKey string, modelName string) string {
	if apiKey == "" {
		apiKey = "clawdsecbot-proxy-key"
	}
	if strings.TrimSpace(modelName) == "" {
		modelName = "clawdsecbot-proxy"
	}
	return fmt.Sprintf(`# --- clawdsecbot proxy provider (auto-injected, do not edit) ---
oai_config0 = {
    'apikey': '%s',
    'apibase': '%s',
    'model': '%s'
}
# --- end clawdsecbot proxy provider ---`, apiKey, proxyURL, modelName)
}

// removeOaiConfig0Block 从内容中移除 oai_config0 注入块
func removeOaiConfig0Block(content string) string {
	startMarker := "# --- clawdsecbot proxy provider (auto-injected, do not edit) ---"
	endMarker := "# --- end clawdsecbot proxy provider ---"

	startIdx := strings.Index(content, startMarker)
	endIdx := strings.Index(content, endMarker)

	if startIdx >= 0 && endIdx >= 0 {
		endIdx += len(endMarker)
		for endIdx < len(content) && content[endIdx] == '\n' {
			endIdx++
		}
		content = content[:startIdx] + content[endIdx:]
	} else {
		re := regexp.MustCompile(`(?ms)^oai_config0\s*=\s*\{[^}]*\}\s*\n?`)
		content = re.ReplaceAllString(content, "")
	}

	return content
}

// removeOaiConfig0FromFile 从配置文件中移除 oai_config0 注入块
func removeOaiConfig0FromFile(configPath string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read config failed: %w", err)
	}

	content := string(data)
	newContent := removeOaiConfig0Block(content)

	if content == newContent {
		logging.Info("[DintalclawConfig] No oai_config0 block found, nothing to remove")
		return nil
	}

	if err := os.WriteFile(configPath, []byte(newContent), 0600); err != nil {
		return fmt.Errorf("write config failed: %w", err)
	}

	logging.Info("[DintalclawConfig] Removed oai_config0 block from %s", configPath)
	return nil
}

// injectProxyLine 向 mykey.py 注入或取消注释 proxy = "..." 行
func injectProxyLine(configPath string, proxyURL string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read config failed: %w", err)
	}
	content := string(data)

	activeProxyRegex := regexp.MustCompile(`(?m)^proxy\s*=\s*['"]([^'"]*?)['"]`)
	if activeProxyRegex.MatchString(content) {
		content = activeProxyRegex.ReplaceAllString(content, fmt.Sprintf(`proxy = "%s"`, proxyURL))
	} else if commentedProxyRegex.MatchString(content) {
		content = commentedProxyRegex.ReplaceAllString(content, fmt.Sprintf(`proxy = "%s"`, proxyURL))
	} else {
		content = content + fmt.Sprintf("\nproxy = \"%s\"\n", proxyURL)
	}

	if err := os.WriteFile(configPath, []byte(content), 0600); err != nil {
		return fmt.Errorf("write config failed: %w", err)
	}

	logging.Info("[DintalclawConfig] Set proxy=%s in %s", proxyURL, configPath)
	return nil
}

// removeProxyLine 从 mykey.py 中注释掉 proxy 行
func removeProxyLine(configPath string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read config failed: %w", err)
	}
	content := string(data)

	activeProxyRegex := regexp.MustCompile(`(?m)^proxy\s*=\s*['"]([^'"]*?)['"]`)
	if !activeProxyRegex.MatchString(content) {
		return nil
	}

	content = activeProxyRegex.ReplaceAllStringFunc(content, func(match string) string {
		return "# " + match
	})

	if err := os.WriteFile(configPath, []byte(content), 0600); err != nil {
		return fmt.Errorf("write config failed: %w", err)
	}

	logging.Info("[DintalclawConfig] Commented out proxy line in %s", configPath)
	return nil
}
