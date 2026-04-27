package qclaw

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go_lib/core"
	"go_lib/core/logging"
	"go_lib/core/scanner"
)

//go:embed qclaw.json
var qclawRulesJSON []byte

type assetScanner struct {
	configPath string
	collector  core.Collector
}

func newAssetScanner(configPath string) *assetScanner {
	return &assetScanner{configPath: configPath}
}

func (s *assetScanner) withCollector(c core.Collector) *assetScanner {
	s.collector = c
	return s
}

func (s *assetScanner) scan() ([]core.Asset, error) {
	return scanner.ScanSingleMergedAsset(scanner.PluginAssetScanOptions{
		AssetName:  qclawAssetName,
		AssetType:  "Service",
		ConfigPath: s.configPath,
		Collector:  s.collector,
		RulesJSON:  qclawRulesJSON,
		Enrich:     s.enrichAsset,
	})
}

func (s *assetScanner) enrichAsset(asset *core.Asset) {
	config, _, configPath, err := loadConfig()
	if err != nil {
		logging.Warning("[QClaw] Cannot load config for enrichment: %v", err)
		return
	}

	if asset.Metadata == nil {
		asset.Metadata = make(map[string]string)
	}
	asset.Metadata["config_path"] = configPath

	bind := config.Gateway.Bind
	if bind == "" {
		bind = config.Gateway.Host
	}
	if bind == "" {
		bind = "127.0.0.1"
	}
	asset.Metadata["gateway_bind"] = bind

	port := config.Gateway.Port
	if port == 0 {
		port = 28789
	}
	asset.Metadata["gateway_port"] = fmt.Sprintf("%d", port)

	authMode := config.Gateway.Auth.Mode
	if authMode == "" && config.Gateway.Auth.Enabled {
		authMode = "enabled"
	}
	if authMode == "" {
		authMode = "disabled"
	}
	asset.Metadata["auth_mode"] = authMode

	redact := config.Logging.RedactSensitive
	if redact == "" {
		redact = "on"
	}
	asset.Metadata["logging_redact"] = redact

	skillDirs, skillDirsErr := getSkillsDirs()
	if skillDirsErr == nil && len(skillDirs) > 0 {
		asset.Metadata["skills_dirs"] = strings.Join(skillDirs, "\n")
		if len(skillDirs) > 0 {
			asset.Metadata["user_skills_dir"] = skillDirs[0]
		}
		if len(skillDirs) > 1 {
			asset.Metadata["builtin_skills_dir"] = skillDirs[1]
		}
	}

	if statePath, err := findStatePath(); err == nil {
		if state, err := loadState(statePath); err == nil {
			asset.Metadata["state_path"] = statePath
			asset.Metadata["auth_gateway_base_url"] = state.AuthGatewayBaseURL
			asset.Metadata["node_binary"] = state.CLI.NodeBinary
			asset.Metadata["openclaw_mjs"] = state.CLI.OpenclawMjs
			if strings.TrimSpace(state.StateDir) != "" {
				asset.Metadata["state_dir"] = state.StateDir
			}
		}
	}
	if strings.TrimSpace(asset.Metadata["node_binary"]) == "" {
		asset.Metadata["node_binary"] = qclawDefaultNodeBinaryPath()
	}
	// Windows can read PE FileVersion; other platforms return an empty version.
	if exe := strings.TrimSpace(asset.Metadata["node_binary"]); exe != "" {
		if v := strings.TrimSpace(readQClawExecutableVersion(exe)); v != "" {
			asset.Version = v
		}
	}
	if strings.TrimSpace(asset.Metadata["openclaw_mjs"]) == "" {
		asset.Metadata["openclaw_mjs"] = qclawDefaultOpenclawMjsPath()
	}
	if strings.TrimSpace(asset.Metadata["builtin_skills_dir"]) == "" {
		asset.Metadata["builtin_skills_dir"] = qclawDefaultBuiltinSkillsDir()
	}

	if homeDir, err := userCurrentHomeDir(); err == nil {
		if logBaseDir := qclawLogsDir(homeDir); strings.TrimSpace(logBaseDir) != "" {
			asset.Metadata["logs_dir"] = logBaseDir
		}
		if appDataDir := qclawAppDataDir(homeDir); strings.TrimSpace(appDataDir) != "" {
			asset.Metadata["roaming_dir"] = appDataDir
		}
	}

	if appData := strings.TrimSpace(os.Getenv("APPDATA")); appData != "" && asset.Metadata["roaming_dir"] == "" {
		asset.Metadata["roaming_dir"] = filepath.Join(appData, "QClaw")
	}

	asset.DisplaySections = []core.DisplaySection{
		{
			Title: "Gateway",
			Icon:  "globe",
			Items: []core.DisplayItem{
				{Label: "Bind", Value: bind, Status: safeStatus(bind == "127.0.0.1" || bind == "loopback" || bind == "::1")},
				{Label: "Port", Value: asset.Metadata["gateway_port"], Status: "neutral"},
				{Label: "Auth", Value: authMode, Status: safeStatus(authMode != "disabled")},
			},
		},
		{
			Title: "Config",
			Icon:  "file",
			Items: []core.DisplayItem{
				{Label: "Path", Value: configPath, Status: "neutral"},
				{Label: "User Skills", Value: asset.Metadata["user_skills_dir"], Status: "neutral"},
				{Label: "Built-in Skills", Value: asset.Metadata["builtin_skills_dir"], Status: "neutral"},
			},
		},
		{
			Title: "Runtime",
			Icon:  "terminal",
			Items: []core.DisplayItem{
				{Label: "State", Value: asset.Metadata["state_path"], Status: "neutral"},
				{Label: "Auth Gateway", Value: asset.Metadata["auth_gateway_base_url"], Status: "neutral"},
				{Label: "Node Binary", Value: asset.Metadata["node_binary"], Status: "neutral"},
			},
		},
		{
			Title: "Logs",
			Icon:  "folder",
			Items: []core.DisplayItem{
				{Label: "Path", Value: asset.Metadata["logs_dir"], Status: "neutral"},
			},
		},
	}
}

func safeStatus(ok bool) string {
	if ok {
		return "safe"
	}
	return "danger"
}
