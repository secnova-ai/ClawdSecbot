package dintalclaw

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"go_lib/core"
	"go_lib/core/logging"
	"go_lib/core/scanner"
)

// dintalclawAssetName 资产内部名称（小写，用于数据库键和指纹计算）
const dintalclawAssetName = "dintalclaw"

// dintalclawProcessKeywords 识别 DinTalClaw 进程的关键字
var dintalclawProcessKeywords = []string{"agentmain.py", "stapp.py", "launch.pyw"}

// installRootDir 安装根目录名称
const installRootDir = "portable_package"

// configFileName 配置文件名称
const configFileName = "mykey.py"

var (
	configPathOverride string
	configPathMutex    sync.RWMutex
)

// SetConfigPath 设置外部传入的配置路径
func SetConfigPath(path string) {
	configPathMutex.Lock()
	defer configPathMutex.Unlock()
	configPathOverride = path
}

// GetConfigPath 获取当前配置路径覆盖
func GetConfigPath() string {
	configPathMutex.RLock()
	defer configPathMutex.RUnlock()
	return configPathOverride
}

// DintalclawAssetScanner DinTalClaw 资产扫描器
type DintalclawAssetScanner struct {
	configPath string
	collector  core.Collector
}

// NewDintalclawAssetScanner 创建扫描器实例
func NewDintalclawAssetScanner(configPath string) *DintalclawAssetScanner {
	return &DintalclawAssetScanner{
		configPath: configPath,
	}
}

// WithCollector 设置自定义采集器（用于测试）
func (s *DintalclawAssetScanner) WithCollector(c core.Collector) *DintalclawAssetScanner {
	s.collector = c
	return s
}

// ScanAssets 执行资产扫描
// 流程：
//  1. 通过规则引擎执行运行时进程检测
//  2. 通过深度目录搜索执行静态安装检测
//  3. 合并结果，丰富资产属性（运行时监听信息、配置路径等）
func (s *DintalclawAssetScanner) ScanAssets() ([]core.Asset, error) {
	assets, err := scanner.ScanSingleMergedAsset(scanner.PluginAssetScanOptions{
		AssetName:  dintalclawAssetName,
		AssetType:  "Service",
		ConfigPath: s.configPath,
		Collector:  s.collector,
		RulesJSON:  dintalclawRulesJSON,
		Enrich:     s.enrichAsset,
	})

	if len(assets) == 0 && err == nil {
		logging.Info("[DintalclawScanner] No runtime match, trying deep install search")
		return s.scanByInstallSearch()
	}

	// 运行态扫描会将 ports/process_paths 参与指纹，dintalclaw 端口/进程路径在重启后易变化。
	// 统一改为基于配置路径的稳定 asset_id，保证监控窗口与防护实例稳定绑定。
	for i := range assets {
		s.rewriteStableAssetID(&assets[i])
	}

	return assets, err
}

// scanByInstallSearch 从用户 HOME 向下搜索最多 3 层，定位 portable_package/mykey.py
func (s *DintalclawAssetScanner) scanByInstallSearch() ([]core.Asset, error) {
	root := findInstallRoot()
	if root == "" {
		return []core.Asset{}, nil
	}

	configPath := filepath.Join(root, configFileName)
	asset := core.Asset{
		Name:         dintalclawAssetName,
		Type:         "Service",
		SourcePlugin: dintalclawAssetName,
		Metadata: map[string]string{
			"config_path":  configPath,
			"install_root": root,
			"detected_by":  "install_search",
		},
	}

	s.enrichAsset(&asset)

	s.rewriteStableAssetID(&asset)

	logging.Info("[DintalclawScanner] Found install at %s, asset_id=%s", root, asset.ID)
	return []core.Asset{asset}, nil
}

// rewriteStableAssetID 重写 dintalclaw 资产 ID，确保运行态与安装态一致稳定。
// 仅用 assetName + configPath 计算指纹，排除 ports/processPaths 等易变字段。
func (s *DintalclawAssetScanner) rewriteStableAssetID(asset *core.Asset) {
	if asset == nil {
		return
	}
	configPath := strings.TrimSpace(asset.Metadata["config_path"])
	if configPath == "" {
		configPath = strings.TrimSpace(s.configPath)
	}
	prev := asset.ID
	asset.ID = core.ComputeAssetID(dintalclawAssetName, configPath)
	if prev != "" && prev != asset.ID {
		logging.Info("[DintalclawScanner] Rewrote volatile asset_id %s -> %s (stable)", prev, asset.ID)
	}
}

// findInstallRoot 从 HOME 向下搜索最多 3 层，查找 portable_package/mykey.py
func findInstallRoot() string {
	configPathMutex.RLock()
	override := strings.TrimSpace(configPathOverride)
	configPathMutex.RUnlock()

	if override != "" {
		dir := filepath.Dir(override)
		if filepath.Base(dir) == installRootDir {
			if _, err := os.Stat(override); err == nil {
				return dir
			}
		}
	}

	usr, err := user.Current()
	if err != nil {
		return ""
	}
	homeDir := usr.HomeDir

	target := filepath.Join(installRootDir, configFileName)

	for depth := 0; depth <= 3; depth++ {
		found := searchAtDepth(homeDir, target, 0, depth)
		if found != "" {
			return filepath.Dir(found)
		}
	}

	return ""
}

// searchAtDepth 递归搜索指定深度的目录
func searchAtDepth(dir, target string, current, maxDepth int) string {
	candidate := filepath.Join(dir, target)
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}

	if current >= maxDepth {
		return ""
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		found := searchAtDepth(filepath.Join(dir, name), target, current+1, maxDepth)
		if found != "" {
			return found
		}
	}

	return ""
}

// enrichAsset 丰富资产属性：配置路径、安装根目录、运行时监听信息
func (s *DintalclawAssetScanner) enrichAsset(asset *core.Asset) {
	if asset.Metadata == nil {
		asset.Metadata = make(map[string]string)
	}

	root := asset.Metadata["install_root"]
	if root == "" {
		root = findInstallRoot()
	}

	configPath := ""
	if root != "" {
		configPath = filepath.Join(root, configFileName)
		asset.Metadata["install_root"] = root
		asset.Metadata["config_path"] = configPath
	}

	processInfos := collectRuntimeProcessInfos()

	var listenerStrs []string
	var ports []int
	seen := make(map[string]bool)
	for _, info := range processInfos {
		for _, listener := range info.Listeners {
			if seen[listener] {
				continue
			}
			seen[listener] = true
			listenerStrs = append(listenerStrs, listener)

			parts := strings.Split(listener, ":")
			if len(parts) >= 2 {
				port, err := strconv.Atoi(parts[len(parts)-1])
				if err == nil {
					ports = append(ports, port)
				}
			}
		}
	}

	if len(ports) > 0 {
		asset.Ports = ports
	}
	if len(listenerStrs) > 0 {
		asset.Metadata["runtime_listeners"] = strings.Join(listenerStrs, ", ")
	}

	assetStatus := resolveAssetStatus(processInfos, root)
	asset.Metadata["asset_status"] = assetStatus

	version := extractVersionFromInstallRoot(root)
	if version != "" {
		asset.Version = version
		asset.Metadata["asset_version"] = version
	}

	statusItems := []core.DisplayItem{
		{Label: "Status", Value: assetStatus, Status: "neutral"},
	}
	if version != "" {
		statusItems = append(statusItems, core.DisplayItem{
			Label: "Version", Value: version, Status: "neutral",
		})
	}

	asset.DisplaySections = []core.DisplaySection{
		{
			Title: "Asset Status",
			Icon:  "settings",
			Items: statusItems,
		},
	}

	logPath := ""
	if root != "" {
		logPath = filepath.Join(root, "temp")
		asset.Metadata["log_path"] = logPath
	}

	basicInfoItems := []core.DisplayItem{
		{Label: "Install Path", Value: root, Status: "neutral"},
		{Label: "Config File", Value: configPath, Status: "neutral"},
		{Label: "Log Path", Value: logPath, Status: "neutral"},
	}
	asset.DisplaySections = append(asset.DisplaySections, core.DisplaySection{
		Title: "Basic Info",
		Icon:  "folder",
		Items: basicInfoItems,
	})

	var processNames []string
	var processPIDs []string
	var processListeners []string
	listenerSeen := make(map[string]bool)
	hasPublicListener := false
	for _, info := range processInfos {
		processNames = append(processNames, info.Name)
		processPIDs = append(processPIDs, strconv.Itoa(info.PID))

		if len(info.Listeners) > 0 {
			if hasNonLoopbackListener(info.Listeners) {
				hasPublicListener = true
			}
			for _, listener := range info.Listeners {
				if listenerSeen[listener] {
					continue
				}
				listenerSeen[listener] = true
				processListeners = append(processListeners, listener)
			}
		}
	}
	if len(processListeners) == 0 {
		processListeners = []string{"N/A"}
	}
	listenerStatus := "safe"
	if len(processInfos) == 0 {
		listenerStatus = "neutral"
	} else if hasPublicListener {
		listenerStatus = "warning"
	}

	// 已安装未运行时不展示进程信息区块，避免卡片出现无意义的 N/A 项。
	if assetStatus != "installed_not_running" {
		processNameValue := "N/A"
		if len(processNames) > 0 {
			processNameValue = strings.Join(processNames, ", ")
		}
		processPIDValue := "N/A"
		if len(processPIDs) > 0 {
			processPIDValue = strings.Join(processPIDs, ", ")
		}
		processItems := []core.DisplayItem{
			{Label: "Process Name", Value: processNameValue, Status: "neutral"},
			{Label: "PID", Value: processPIDValue, Status: "neutral"},
			{Label: "Listener Address", Value: strings.Join(processListeners, ", "), Status: listenerStatus},
		}

		asset.DisplaySections = append(asset.DisplaySections, core.DisplaySection{
			Title: "Process Info",
			Icon:  "radio",
			Items: processItems,
		})
	}
}

// Listener 运行时监听地址端口
type Listener struct {
	Address string
	Port    int
	PID     int
}

// ProcessRuntimeInfo 运行时进程信息
type ProcessRuntimeInfo struct {
	Name      string
	PID       int
	Listeners []string
}

// collectRuntimeProcessInfos 采集 dintalclaw 主进程及子进程的进程与监听信息
func collectRuntimeProcessInfos() []ProcessRuntimeInfo {
	pids := findDintalclawPIDs()
	if len(pids) == 0 {
		return nil
	}

	allPIDs := make(map[int]bool)
	for _, pid := range pids {
		allPIDs[pid] = true
		children := getChildPIDs(pid)
		for _, child := range children {
			allPIDs[child] = true
		}
	}

	var orderedPIDs []int
	for pid := range allPIDs {
		orderedPIDs = append(orderedPIDs, pid)
	}
	sort.Ints(orderedPIDs)

	var infos []ProcessRuntimeInfo
	for _, pid := range orderedPIDs {
		pidListeners := getListenersForPID(pid)

		listenerSet := make(map[string]bool)
		var listenerStrs []string
		for _, l := range pidListeners {
			addrPort := fmt.Sprintf("%s:%d", l.Address, l.Port)
			if listenerSet[addrPort] {
				continue
			}
			listenerSet[addrPort] = true
			listenerStrs = append(listenerStrs, addrPort)
		}
		sort.Strings(listenerStrs)

		name := getProcessNameForPID(pid)
		if name == "" {
			name = "unknown"
		}

		infos = append(infos, ProcessRuntimeInfo{
			Name:      name,
			PID:       pid,
			Listeners: listenerStrs,
		})
	}

	return infos
}

// resolveAssetStatus 解析资产状态文案 key
func resolveAssetStatus(processInfos []ProcessRuntimeInfo, installRoot string) string {
	hasLaunch := false
	hasStapp := false
	hasAgent := false

	for _, info := range processInfos {
		switch info.Name {
		case "launch.pyw":
			hasLaunch = true
		case "stapp.py":
			hasStapp = true
		case "agentmain.py":
			hasAgent = true
		}
	}

	if hasLaunch {
		return "frontend_mode_running"
	}
	if hasStapp {
		return "browser_mode_running"
	}
	if hasAgent {
		return "cli_mode_running"
	}
	if installRoot != "" {
		return "installed_not_running"
	}
	return "installed_not_running"
}

// hasNonLoopbackListener 判断是否存在非 loopback 监听
func hasNonLoopbackListener(listeners []string) bool {
	for _, listener := range listeners {
		parts := strings.SplitN(listener, ":", 2)
		if len(parts) != 2 {
			continue
		}
		addr := parts[0]
		if addr != "127.0.0.1" && addr != "::1" && addr != "localhost" {
			return true
		}
	}
	return false
}

func extractVersionFromInstallRoot(installRoot string) string {
	if installRoot == "" {
		return ""
	}
	parentDir := filepath.Base(filepath.Dir(installRoot))
	re := regexp.MustCompile(`(?i)^dintalclaw-v([0-9]+\.[0-9]+\.[0-9]+)(?:-[a-z0-9_]+)?$`)
	matches := re.FindStringSubmatch(parentDir)
	if len(matches) == 2 {
		return matches[1]
	}
	return ""
}
