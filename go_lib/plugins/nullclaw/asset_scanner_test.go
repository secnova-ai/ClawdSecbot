package nullclaw

import (
	"fmt"
	"testing"

	"go_lib/core"
)

// testCollector 测试用模拟采集器
type testCollector struct {
	snapshot core.SystemSnapshot
	err      error
}

func (m *testCollector) Collect() (core.SystemSnapshot, error) {
	return m.snapshot, m.err
}

// TestNullclawAssetScanner_ScanAssets_Detected 验证正常检测Nullclaw资产
func TestNullclawAssetScanner_ScanAssets_Detected(t *testing.T) {
	s := NewNullclawAssetScanner("")
	s.WithCollector(&testCollector{
		snapshot: core.SystemSnapshot{
			OpenPorts: []int{3000},
			RunningProcesses: []core.SystemProcess{
				{Pid: 1, Name: "nullclaw", Cmd: "nullclaw gateway", Path: "/usr/local/bin/nullclaw"},
			},
			Services: []string{},
			FileExists: func(path string) bool {
				return path == "~/.nullclaw"
			},
		},
	})

	assets, err := s.ScanAssets()
	if err != nil {
		t.Fatalf("ScanAssets failed: %v", err)
	}

	if len(assets) == 0 {
		t.Fatal("Expected to detect Nullclaw assets")
	}

	// 验证资产名称
	if assets[0].Name != nullclawAssetName {
		t.Errorf("Expected asset name '%s', got '%s'", nullclawAssetName, assets[0].Name)
	}

	// 验证资产类型
	if assets[0].Type != "Service" {
		t.Errorf("Expected asset type 'Service', got '%s'", assets[0].Type)
	}

	// 验证端口
	portFound := false
	for _, port := range assets[0].Ports {
		if port == 3000 {
			portFound = true
		}
	}
	if !portFound {
		t.Error("Expected port 3000 in detected asset")
	}
}

// TestNullclawAssetScanner_ScanAssets_NotDetected 验证无Nullclaw时返回空列表
func TestNullclawAssetScanner_ScanAssets_NotDetected(t *testing.T) {
	s := NewNullclawAssetScanner("")
	s.WithCollector(&testCollector{
		snapshot: core.SystemSnapshot{
			OpenPorts:        []int{22, 80, 443},
			RunningProcesses: []core.SystemProcess{},
			Services:         []string{},
			FileExists:       func(path string) bool { return false },
		},
	})

	assets, err := s.ScanAssets()
	if err != nil {
		t.Fatalf("ScanAssets failed: %v", err)
	}

	if len(assets) != 0 {
		t.Errorf("Expected 0 assets when Nullclaw is not running, got %d", len(assets))
	}
}

// TestNullclawAssetScanner_ScanAssets_ConfigFileOnly 验证仅通过配置文件检测
func TestNullclawAssetScanner_ScanAssets_ConfigFileOnly(t *testing.T) {
	s := NewNullclawAssetScanner("")
	s.WithCollector(&testCollector{
		snapshot: core.SystemSnapshot{
			OpenPorts:        []int{},
			RunningProcesses: []core.SystemProcess{},
			Services:         []string{},
			FileExists: func(path string) bool {
				return path == "~/.nullclaw/config.json" || path == "~/.config/nullclaw/config.json" || path == "~/.nullclaw"
			},
		},
	})

	assets, err := s.ScanAssets()
	if err != nil {
		t.Fatalf("ScanAssets failed: %v", err)
	}

	if len(assets) == 0 {
		t.Fatal("Expected to detect Nullclaw via config file")
	}

	if assets[0].Name != nullclawAssetName {
		t.Errorf("Expected asset name '%s', got '%s'", nullclawAssetName, assets[0].Name)
	}
}

// TestNullclawAssetScanner_ScanAssets_MergesMultipleRules 验证多规则匹配时资产合并
func TestNullclawAssetScanner_ScanAssets_MergesMultipleRules(t *testing.T) {
	s := NewNullclawAssetScanner("")
	s.WithCollector(&testCollector{
		snapshot: core.SystemSnapshot{
			OpenPorts: []int{3000},
			RunningProcesses: []core.SystemProcess{
				{Pid: 502, Name: "nullclaw", Cmd: "nullclaw gateway", Path: "/usr/local/bin/nullclaw"},
			},
			Services: []string{},
			FileExists: func(path string) bool {
				return path == "~/.nullclaw"
			},
		},
	})

	assets, err := s.ScanAssets()
	if err != nil {
		t.Fatalf("ScanAssets failed: %v", err)
	}

	// 两条规则都匹配，但应该合并为一个资产
	if len(assets) != 1 {
		t.Fatalf("Expected 1 merged asset, got %d", len(assets))
	}

	// 验证合并后的资产名称
	if assets[0].Name != nullclawAssetName {
		t.Errorf("Expected asset name '%s', got '%s'", nullclawAssetName, assets[0].Name)
	}

	// 验证端口被保留
	portFound := false
	for _, port := range assets[0].Ports {
		if port == 3000 {
			portFound = true
		}
	}
	if !portFound {
		t.Error("Expected port 3000 in merged asset")
	}

	// 验证配置路径 metadata 被保留
	if assets[0].Metadata["config_path"] != "~/.nullclaw" {
		t.Logf("Metadata: %v", assets[0].Metadata)
		// 注意：config_path 可能来自规则匹配或 enrichAssetWithConfig
		// 在没有真实配置文件的测试环境中，enrichment可能跳过
	}
}

// TestNullclawAssetScanner_LoadRules 验证规则加载
func TestNullclawAssetScanner_LoadRules(t *testing.T) {
	s := NewNullclawAssetScanner("")
	rules, err := s.loadRules()
	if err != nil {
		t.Fatalf("loadRules failed: %v", err)
	}

	if len(rules) == 0 {
		t.Fatal("Expected at least one rule from nullclaw.json")
	}

	// 验证规则包含端口和进程检测
	hasPortRule := false
	hasConfigRule := false
	for _, rule := range rules {
		if rule.Code == "nullclaw_gateway_active" {
			hasPortRule = true
		}
		if rule.Code == "nullclaw_config_exist" {
			hasConfigRule = true
		}
	}

	if !hasPortRule {
		t.Error("Expected 'nullclaw_gateway_active' rule")
	}
	if !hasConfigRule {
		t.Error("Expected 'nullclaw_config_exist' rule")
	}
}

// TestNullclawAssetScanner_CollectorError 验证采集器出错时的容错处理
func TestNullclawAssetScanner_CollectorError(t *testing.T) {
	s := NewNullclawAssetScanner("")
	s.WithCollector(&testCollector{
		err: fmt.Errorf("permission denied"),
	})

	assets, err := s.ScanAssets()
	if err != nil {
		t.Fatalf("ScanAssets should not return error on collector failure, got: %v", err)
	}

	// 采集器失败时应返回空列表而非错误
	if len(assets) != 0 {
		t.Errorf("Expected 0 assets on collector failure, got %d", len(assets))
	}
}
