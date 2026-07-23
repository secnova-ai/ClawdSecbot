package dwtsclaw

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"go_lib/core"
)

var dwtsclawInstallRootOverride string

type assetScanner struct {
	collector core.Collector
}

func newAssetScanner() *assetScanner {
	return &assetScanner{}
}

func (s *assetScanner) withCollector(collector core.Collector) *assetScanner {
	s.collector = collector
	return s
}

func (s *assetScanner) scan() ([]core.Asset, error) {
	raw, configPath, err := loadDWTSClawConfig()
	if err != nil {
		return []core.Asset{}, nil
	}
	collector := s.collector
	if collector == nil {
		collector = core.NewCollector(filepath.Dir(configPath))
	}
	snapshot, err := collector.Collect()
	if err != nil {
		return []core.Asset{}, nil
	}

	evidence := collectProductEvidence(snapshot)
	if len(evidence.evidence) == 0 {
		return []core.Asset{}, nil
	}

	canonicalConfigPath := core.ResolveStableConfigPathFingerprint(configPath)
	metadata := map[string]string{
		"config_path":      canonicalConfigPath,
		"product_evidence": strings.Join(evidence.evidence, ","),
	}
	if evidence.installRoot != "" {
		metadata["install_root"] = evidence.installRoot
		metadata["gateway_entry_path"] = filepath.Join(evidence.installRoot, "resources", "cfmind", "openclaw.mjs")
	}
	if evidence.pid > 0 {
		metadata["pid"] = strconv.Itoa(evidence.pid)
	}
	if gatewayPort := readGatewayPort(configPath); gatewayPort > 0 {
		metadata["gateway_port"] = strconv.Itoa(gatewayPort)
	}
	if primary := readPrimaryModel(raw); primary != "" {
		metadata["model_primary"] = primary
	}

	asset := core.Asset{
		ID:           core.ComputeAssetID(dwtsclawAssetName, canonicalConfigPath),
		SourcePlugin: dwtsclawAssetName,
		Name:         dwtsclawAssetName,
		Type:         "Application",
		Metadata:     metadata,
		ProcessPaths: evidence.processPaths,
	}
	if port, err := strconv.Atoi(metadata["gateway_port"]); err == nil && port > 0 {
		asset.Ports = []int{port}
	}
	asset.DisplaySections = buildDisplaySections(asset)
	return []core.Asset{asset}, nil
}

type productEvidence struct {
	installRoot  string
	pid          int
	processPaths []string
	evidence     []string
}

func collectProductEvidence(snapshot core.SystemSnapshot) productEvidence {
	result := productEvidence{}
	for _, root := range candidateInstallRoots() {
		if !isDWTSClawInstallRoot(root) {
			continue
		}
		result.installRoot = root
		result.evidence = append(result.evidence, "install-root")
		break
	}

	for _, process := range snapshot.RunningProcesses {
		kind := dwtsclawProcessEvidence(process)
		if kind == "" {
			continue
		}
		result.evidence = append(result.evidence, kind)
		if result.pid == 0 && process.Pid > 0 {
			result.pid = int(process.Pid)
		}
		if path := strings.TrimSpace(process.Path); path != "" {
			result.processPaths = append(result.processPaths, path)
			if result.installRoot == "" {
				if root := installRootFromProcessPath(path); isDWTSClawInstallRoot(root) {
					result.installRoot = root
					result.evidence = append(result.evidence, "install-root")
				}
			}
		}
	}

	result.evidence = sortedStrings(result.evidence)
	result.processPaths = sortedUniquePaths(result.processPaths)
	return result
}

func candidateInstallRoots() []string {
	if override := strings.TrimSpace(dwtsclawInstallRootOverride); override != "" {
		return []string{override}
	}

	candidates := make([]string, 0, 8)
	if root := strings.TrimSpace(os.Getenv("DWTSCLAW_INSTALL_ROOT")); root != "" {
		candidates = append(candidates, root)
	}
	for _, programFiles := range []string{os.Getenv("ProgramFiles(x86)"), os.Getenv("ProgramFiles")} {
		programFiles = strings.TrimSpace(programFiles)
		if programFiles == "" {
			continue
		}
		candidates = append(candidates,
			filepath.Join(programFiles, "天枢Buddy"),
			filepath.Join(programFiles, "DWTDClaw", "DWTSClaw"),
		)
	}
	candidates = append(candidates, discoverPlatformInstallRoots()...)
	return sortedUniquePaths(candidates)
}

func isDWTSClawInstallRoot(root string) bool {
	root = strings.TrimSpace(root)
	if root == "" {
		return false
	}
	if info, err := os.Stat(filepath.Join(root, "DWTSClaw.exe")); err != nil || info.IsDir() {
		return false
	}
	if info, err := os.Stat(filepath.Join(root, "resources", "cfmind", "openclaw.mjs")); err != nil || info.IsDir() {
		return false
	}
	return true
}

func dwtsclawProcessEvidence(process core.SystemProcess) string {
	name := strings.ToLower(strings.TrimSpace(process.Name))
	path := strings.ToLower(strings.TrimSpace(process.Path))
	cmd := strings.ToLower(strings.TrimSpace(process.Cmd))
	if strings.Contains(name, "dwtsclaw") || strings.Contains(path, "dwtsclaw") {
		return "process:dwtsclaw"
	}
	if strings.Contains(name, "cfmind") && (strings.Contains(path, "dwtsclaw") || strings.Contains(cmd, "dwtsclaw")) {
		return "process:cfmind"
	}
	return ""
}

func installRootFromProcessPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	path = filepath.Clean(path)
	if strings.EqualFold(filepath.Base(path), "DWTSClaw.exe") {
		return filepath.Dir(path)
	}
	runtimeDir := filepath.Dir(path)
	if strings.EqualFold(filepath.Base(runtimeDir), "cfmind") &&
		strings.EqualFold(filepath.Base(filepath.Dir(runtimeDir)), "resources") {
		return filepath.Dir(filepath.Dir(runtimeDir))
	}
	return ""
}

func sortedUniquePaths(paths []string) []string {
	unique := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path != "" {
			unique[path] = struct{}{}
		}
	}
	result := make([]string, 0, len(unique))
	for path := range unique {
		result = append(result, path)
	}
	sort.Strings(result)
	return result
}

func buildDisplaySections(asset core.Asset) []core.DisplaySection {
	metadata := asset.Metadata
	items := []core.DisplayItem{
		{Label: "Config", Value: metadata["config_path"], Status: "neutral"},
		{Label: "Evidence", Value: metadata["product_evidence"], Status: "neutral"},
	}
	if root := metadata["install_root"]; root != "" {
		items = append(items, core.DisplayItem{Label: "Install Root", Value: root, Status: "neutral"})
	}
	sections := []core.DisplaySection{{Title: "DWTSClaw", Icon: "monitor", Items: items}}
	if port := metadata["gateway_port"]; port != "" {
		sections = append(sections, core.DisplaySection{
			Title: "Gateway",
			Icon:  "globe",
			Items: []core.DisplayItem{{Label: "Port", Value: port, Status: "neutral"}},
		})
	}
	return sections
}
