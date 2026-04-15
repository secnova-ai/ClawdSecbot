package core

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"

	"go_lib/core/logging"
)

// SystemProcess represents a running process in the system snapshot
type SystemProcess struct {
	Pid  int32
	Name string
	Cmd  string
	Path string
}

// SystemSnapshot represents the collected state of the system
// This decouples the collection logic (OS-specific) from the detection logic (Rule-based)
type SystemSnapshot struct {
	OpenPorts        []int
	RunningProcesses []SystemProcess
	Services         []string
	// FileExists is a callback to check if a file exists (allowing mock or real OS check)
	FileExists func(path string) bool
}

// AssetDetectionEngine 资产检测引擎,将规则与系统快照进行匹配
type AssetDetectionEngine struct {
	Rules []AssetFinderRule
}

// NewEngine 创建一个新的检测引擎实例
func NewEngine() *AssetDetectionEngine {
	return &AssetDetectionEngine{
		Rules: make([]AssetFinderRule, 0),
	}
}

// LoadRule 注册一条新的检测规则
func (e *AssetDetectionEngine) LoadRule(rule AssetFinderRule) {
	e.Rules = append(e.Rules, rule)
}

// Detect 扫描系统快照并返回所有匹配的资产
func (e *AssetDetectionEngine) Detect(snapshot SystemSnapshot) ([]Asset, error) {
	var detected []Asset

	logging.Info("开始资产检测,共有 %d 条规则", len(e.Rules))
	logging.Debug("快照信息: 端口数=%d, 进程数=%d, 服务数=%d",
		len(snapshot.OpenPorts), len(snapshot.RunningProcesses), len(snapshot.Services))

	for idx, rule := range e.Rules {
		logging.Debug("处理规则 [%d/%d]: %s (生命周期=%v, 操作系统=%v)",
			idx+1, len(e.Rules), rule.Name, rule.LifeCycle, rule.OS)

		// 1. 检查规则生命周期
		// 同时支持运行时（Runtime）和静态（Static）规则
		if rule.LifeCycle != RuleLifeCycleRuntime && rule.LifeCycle != RuleLifeCycleStatic {
			logging.Debug("跳过规则 %s: 生命周期不匹配 (%v)", rule.Name, rule.LifeCycle)
			continue
		}

		// 2. 检查操作系统适用性
		if len(rule.OS) > 0 {
			matchedOS := false
			for _, os := range rule.OS {
				if os == runtime.GOOS {
					matchedOS = true
					break
				}
			}
			if !matchedOS {
				logging.Debug("跳过规则 %s: 操作系统不匹配 (期望=%v, 当前=%s)",
					rule.Name, rule.OS, runtime.GOOS)
				continue
			}
		}

		// 3. 根据表达式语言分发处理逻辑
		if rule.Expression.Lang == "json_match" {
			if asset := e.matchJSON(snapshot, rule); asset != nil {
				logging.Info("✓ 检测到资产: %s (类型=%s)", asset.Name, asset.Type)
				detected = append(detected, *asset)
			} else {
				logging.Debug("规则 %s 未匹配到资产", rule.Name)
			}
		}
	}

	logging.Info("资产检测完成,共检测到 %d 个资产", len(detected))
	return detected, nil
}

// matchJSON 实现针对 "json_match" 语言的简单匹配逻辑
// 逻辑：Rule 内的 Criteria 字段之间是 AND 关系；同一字段的列表内是 OR 关系.
func (e *AssetDetectionEngine) matchJSON(snapshot SystemSnapshot, rule AssetFinderRule) *Asset {
	var criteria AssetMatchCriteria
	if err := json.Unmarshal([]byte(rule.Expression.Expr), &criteria); err != nil {
		// 记录错误或忽略无效规则
		logging.Warning("规则 %s 的表达式解析失败: %v", rule.Name, err)
		return nil
	}

	// 至少需要定义一种匹配条件
	if len(criteria.Ports) == 0 && len(criteria.ProcessKeywords) == 0 &&
		len(criteria.ServiceNames) == 0 && len(criteria.FilePaths) == 0 {
		logging.Debug("规则 %s 没有定义任何匹配条件", rule.Name)
		return nil
	}

	logging.Debug("规则 %s 匹配条件: 端口=%v, 进程关键字=%v, 服务名=%v, 文件路径=%v",
		rule.Name, criteria.Ports, criteria.ProcessKeywords, criteria.ServiceNames, criteria.FilePaths)

	currentAsset := Asset{
		Name:     rule.Name,
		Type:     "Service", // 默认为服务类型
		Metadata: make(map[string]string),
	}

	// 1. 检查端口 (Ports) - 维度内 OR,维度间 AND
	if len(criteria.Ports) > 0 {
		portMatched := false
		for _, criteriaPort := range criteria.Ports {
			for _, openPort := range snapshot.OpenPorts {
				if criteriaPort == openPort {
					portMatched = true
					currentAsset.Ports = append(currentAsset.Ports, openPort)
					logging.Debug("规则 %s: 匹配到端口 %d", rule.Name, openPort)
				}
			}
		}
		if !portMatched {
			logging.Debug("规则 %s: 端口条件未满足 (需要=%v)", rule.Name, criteria.Ports)
			return nil // 端口条件未满足
		}
	}

	// 2. 检查进程 (Processes) - 维度内 OR,维度间 AND
	// 只匹配进程名称和进程路径，不匹配命令行参数，避免误报（如vim编辑包含关键字的文件）
	if len(criteria.ProcessKeywords) > 0 {
		procMatched := false
		matchedPIDs := make([]string, 0)
		for _, key := range criteria.ProcessKeywords {
			key = strings.ToLower(key)
			for _, proc := range snapshot.RunningProcesses {
				// 只匹配进程名称或进程路径，不匹配命令行参数
				if processMatchesKeyword(proc, key) {
					procMatched = true
					currentAsset.ProcessPaths = append(currentAsset.ProcessPaths, proc.Path)
					matchedPIDs = append(matchedPIDs, fmt.Sprintf("%d", proc.Pid))
					logging.Debug("规则 %s: 匹配到进程 %s (路径=%s, 关键字=%s)", rule.Name, proc.Name, proc.Path, key)
				}
			}
		}
		if !procMatched {
			logging.Debug("规则 %s: 进程条件未满足 (需要关键字=%v)", rule.Name, criteria.ProcessKeywords)
			return nil // 进程条件未满足
		}
	}

	// 3. 检查服务 (Services) - 维度内 OR,维度间 AND
	if len(criteria.ServiceNames) > 0 {
		serviceMatched := false
		for _, serviceName := range criteria.ServiceNames {
			for _, runningService := range snapshot.Services {
				if strings.Contains(strings.ToLower(runningService), strings.ToLower(serviceName)) {
					serviceMatched = true
					currentAsset.ServiceName = runningService
					logging.Debug("规则 %s: 匹配到服务 %s", rule.Name, runningService)
				}
			}
		}
		if !serviceMatched {
			logging.Debug("规则 %s: 服务条件未满足 (需要=%v)", rule.Name, criteria.ServiceNames)
			return nil // 服务条件未满足
		}
	}

	// 4. 检查文件 (Files) - 维度内 OR,维度间 AND
	if len(criteria.FilePaths) > 0 {
		fileMatched := false
		if snapshot.FileExists != nil {
			for _, path := range criteria.FilePaths {
				// 在 Collector 中已经处理了 ~ 扩展,这里直接检查
				logging.Debug("规则 %s: 检查文件路径 %s", rule.Name, path)
				if snapshot.FileExists(path) {
					fileMatched = true
					currentAsset.Metadata["config_path"] = path
					logging.Debug("规则 %s: 匹配到文件 %s", rule.Name, path)
				}
			}
		}
		if !fileMatched {
			logging.Debug("规则 %s: 文件条件未满足 (需要=%v)", rule.Name, criteria.FilePaths)
			return nil // 文件条件未满足
		}
	}

	if len(criteria.ProcessKeywords) > 0 {
		matchedPIDs := collectMatchedProcessPIDs(snapshot.RunningProcesses, currentAsset.ProcessPaths)
		if len(matchedPIDs) > 0 {
			currentAsset.Metadata["pid"] = strings.Join(matchedPIDs, ", ")
		}
	}

	currentAsset.ProcessPaths = uniqueStrings(currentAsset.ProcessPaths)
	return &currentAsset
}

// uniqueStrings 字符串切片去重
func uniqueStrings(input []string) []string {
	keys := make(map[string]bool)
	list := []string{}
	for _, entry := range input {
		if entry == "" {
			continue
		}
		if _, value := keys[entry]; !value {
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list
}

func collectMatchedProcessPIDs(processes []SystemProcess, processPaths []string) []string {
	if len(processes) == 0 || len(processPaths) == 0 {
		return []string{}
	}

	pathSet := make(map[string]struct{}, len(processPaths))
	for _, path := range processPaths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		pathSet[path] = struct{}{}
	}

	pids := make([]string, 0)
	for _, proc := range processes {
		path := strings.TrimSpace(proc.Path)
		if path == "" {
			continue
		}
		if _, ok := pathSet[path]; !ok {
			continue
		}
		pids = append(pids, fmt.Sprintf("%d", proc.Pid))
	}

	return uniqueStrings(pids)
}

func processMatchesKeyword(proc SystemProcess, keyword string) bool {
	name := strings.ToLower(strings.TrimSpace(proc.Name))
	path := strings.ToLower(strings.TrimSpace(proc.Path))
	cmd := strings.ToLower(strings.TrimSpace(proc.Cmd))

	if strings.Contains(name, keyword) || strings.Contains(path, keyword) {
		return true
	}

	if !isWrapperRuntimeProcess(name, path) {
		return false
	}

	return strings.Contains(cmd, keyword)
}

func isWrapperRuntimeProcess(name, path string) bool {
	for _, token := range []string{
		"node", "npm", "npx", "bun", "deno",
		"python", "pythonw", "py",
		"ruby", "java", "javaw",
	} {
		if strings.Contains(name, token) || strings.Contains(path, token) {
			return true
		}
	}
	return false
}
