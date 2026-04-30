package core

import (
	"crypto/sha256"
	"fmt"
	"strconv"
	"strings"
)

// Asset 定义检测到的资产结构
type Asset struct {
	// ID is a deterministic fingerprint hash uniquely identifying this asset instance.
	// Computed from name + config_path via ComputeAssetID(); runtime-dynamic fields such
	// as ports/process_paths are intentionally excluded so the ID is stable across restarts.
	ID string `json:"id"`
	// SourcePlugin is the asset name of the plugin that discovered this asset.
	SourcePlugin string `json:"source_plugin"`
	// Name 资产名称（例如："Moltbot Gateway"）
	Name string `json:"name"`
	// Type 资产类型（例如："Service", "Application"）
	Type string `json:"type"`
	// Version 资产版本（如果可检测）
	Version string `json:"version"`
	// Ports 该资产当前开放的端口列表
	Ports []int `json:"ports"`
	// ServiceName 如果作为系统服务运行（systemd/launchd 等）的服务名称
	ServiceName string `json:"service_name"`
	// ProcessPaths 与该资产关联的可执行文件路径列表
	ProcessPaths []string `json:"process_paths"`
	// Metadata 包含额外信息,如工作空间路径、数据存储目录等
	Metadata map[string]string `json:"metadata"`
	// DisplaySections provides structured display hints for the UI layer.
	// Each plugin populates this with its own config sections so the UI can
	// render asset details generically without hardcoding plugin-specific fields.
	DisplaySections []DisplaySection `json:"display_sections,omitempty"`
}

// DisplaySection describes a group of key-value items for UI rendering.
type DisplaySection struct {
	// Title is the section heading, e.g. "Gateway Configuration"
	Title string `json:"title"`
	// Icon is a short identifier mapped to an icon on the UI side, e.g. "globe", "box"
	Icon string `json:"icon"`
	// Items are the key-value pairs within this section
	Items []DisplayItem `json:"items"`
}

// DisplayItem is a single key-value entry with a safety status indicator.
type DisplayItem struct {
	// Label is the display label, e.g. "Bind"
	Label string `json:"label"`
	// Value is the display value, e.g. "127.0.0.1"
	Value string `json:"value"`
	// Status indicates the safety state: "safe", "warning", "danger", or "neutral"
	Status string `json:"status"`
}

// ComputeAssetID generates a deterministic fingerprint ID for an asset instance.
// The ID is composed of the lowercase asset name and a 12-char hex hash derived from
// the config path. Runtime-dynamic attributes (ports, process paths, pid, etc.) are
// intentionally excluded so the ID stays stable regardless of whether the bot is
// currently running. Same instance scanned at different times always produces the
// same ID, which is required for protection/policy binding by asset_id.
func ComputeAssetID(name string, configPath string) string {
	nameLower := strings.ToLower(name)

	parts := []string{"name=" + nameLower}
	if configPath != "" {
		parts = append(parts, "config="+configPath)
	}

	canonical := strings.Join(parts, "|")
	hash := sha256.Sum256([]byte(canonical))
	shortHash := fmt.Sprintf("%x", hash[:6]) // 12 hex chars

	return nameLower + ":" + shortHash
}

// AttachAssetMainProcessPID records a plugin-resolved main process PID on the
// asset and keeps the structured display section in sync for generic UI cards.
func AttachAssetMainProcessPID(asset *Asset, pid int) {
	if asset == nil || pid <= 0 {
		return
	}
	if asset.Metadata == nil {
		asset.Metadata = make(map[string]string)
	}

	value := strconv.Itoa(pid)
	asset.Metadata["main_pid"] = value
	asset.Metadata["pid"] = value
	upsertPIDDisplayItem(asset, value)
}

func upsertPIDDisplayItem(asset *Asset, value string) {
	runtimeIndex := -1
	for i := range asset.DisplaySections {
		if strings.EqualFold(strings.TrimSpace(asset.DisplaySections[i].Title), "Runtime") {
			runtimeIndex = i
		}
		for j := range asset.DisplaySections[i].Items {
			label := strings.TrimSpace(asset.DisplaySections[i].Items[j].Label)
			if strings.EqualFold(label, "PID") || strings.EqualFold(label, "Main PID") {
				asset.DisplaySections[i].Items[j].Label = "Main PID"
				asset.DisplaySections[i].Items[j].Value = value
				asset.DisplaySections[i].Items[j].Status = "neutral"
				return
			}
		}
	}

	pidItem := DisplayItem{Label: "Main PID", Value: value, Status: "neutral"}
	if runtimeIndex >= 0 {
		items := asset.DisplaySections[runtimeIndex].Items
		asset.DisplaySections[runtimeIndex].Items = append([]DisplayItem{pidItem}, items...)
		return
	}

	asset.DisplaySections = append(asset.DisplaySections, DisplaySection{
		Title: "Runtime",
		Icon:  "monitor",
		Items: []DisplayItem{pidItem},
	})
}

// AssetMatchCriteria 定义简单的 "json_match" 匹配规则结构
// 对应 RuleExpression 中 lang 为 "json_match" 时的 expr 字段内容
type AssetMatchCriteria struct {
	// Ports 需要匹配的端口列表
	Ports []int `json:"ports,omitempty"`
	// ProcessKeywords 进程名称或命令行参数中包含的关键字
	ProcessKeywords []string `json:"process_keywords,omitempty"`
	// ServiceNames 服务名称列表（支持部分匹配）
	ServiceNames []string `json:"service_names,omitempty"`
	// FilePaths 需要检查是否存在的文件路径列表
	FilePaths []string `json:"file_paths,omitempty"`
}
