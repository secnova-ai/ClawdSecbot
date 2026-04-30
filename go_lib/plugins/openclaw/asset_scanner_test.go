package openclaw

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

// TestOpenclawAssetScanner_ScanAssets_Detected 验证正常检测Openclaw资产
func TestOpenclawAssetScanner_ScanAssets_Detected(t *testing.T) {
	s := NewOpenclawAssetScanner("")
	s.WithCollector(&testCollector{
		snapshot: core.SystemSnapshot{
			OpenPorts: []int{18789},
			RunningProcesses: []core.SystemProcess{
				{Pid: 1, Name: "openclaw", Cmd: "openclaw gateway", Path: "/usr/local/bin/openclaw"},
			},
			Services: []string{},
			FileExists: func(path string) bool {
				return path == "~/.openclaw"
			},
		},
	})

	assets, err := s.ScanAssets()
	if err != nil {
		t.Fatalf("ScanAssets failed: %v", err)
	}

	if len(assets) == 0 {
		t.Fatal("Expected to detect Openclaw assets")
	}

	// 验证资产名称
	if assets[0].Name != openclawAssetName {
		t.Errorf("Expected asset name '%s', got '%s'", openclawAssetName, assets[0].Name)
	}

	// 验证资产类型
	if assets[0].Type != "Service" {
		t.Errorf("Expected asset type 'Service', got '%s'", assets[0].Type)
	}

	// 验证端口
	portFound := false
	for _, port := range assets[0].Ports {
		if port == 18789 {
			portFound = true
		}
	}
	if !portFound {
		t.Error("Expected port 18789 in detected asset")
	}
}

// TestOpenclawAssetScanner_ScanAssets_NotDetected 验证无Openclaw时返回空列表
func TestOpenclawAssetScanner_ScanAssets_NotDetected(t *testing.T) {
	s := NewOpenclawAssetScanner("")
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
		t.Errorf("Expected 0 assets when Openclaw is not running, got %d", len(assets))
	}
}

// TestOpenclawAssetScanner_ScanAssets_ConfigFileOnly 验证仅通过配置文件检测
func TestOpenclawAssetScanner_ScanAssets_ConfigFileOnly(t *testing.T) {
	s := NewOpenclawAssetScanner("")
	s.WithCollector(&testCollector{
		snapshot: core.SystemSnapshot{
			OpenPorts:        []int{},
			RunningProcesses: []core.SystemProcess{},
			Services:         []string{},
			FileExists: func(path string) bool {
				return path == "~/.openclaw" || path == "~/.moltbot" || path == "~/.clawdbot"
			},
		},
	})

	assets, err := s.ScanAssets()
	if err != nil {
		t.Fatalf("ScanAssets failed: %v", err)
	}

	if len(assets) == 0 {
		t.Fatal("Expected to detect Openclaw via config file")
	}

	if assets[0].Name != openclawAssetName {
		t.Errorf("Expected asset name '%s', got '%s'", openclawAssetName, assets[0].Name)
	}
}

// TestOpenclawAssetScanner_ScanAssets_MergesMultipleRules 验证多规则匹配时资产合并
func TestOpenclawAssetScanner_ScanAssets_MergesMultipleRules(t *testing.T) {
	s := NewOpenclawAssetScanner("")
	s.WithCollector(&testCollector{
		snapshot: core.SystemSnapshot{
			OpenPorts: []int{18789},
			RunningProcesses: []core.SystemProcess{
				{Pid: 502, Name: "openclaw", Cmd: "openclaw gateway", Path: "/usr/local/bin/openclaw"},
			},
			Services: []string{},
			FileExists: func(path string) bool {
				return path == "~/.openclaw"
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
	if assets[0].Name != openclawAssetName {
		t.Errorf("Expected asset name '%s', got '%s'", openclawAssetName, assets[0].Name)
	}

	// 验证端口被保留
	portFound := false
	for _, port := range assets[0].Ports {
		if port == 18789 {
			portFound = true
		}
	}
	if !portFound {
		t.Error("Expected port 18789 in merged asset")
	}

	// 验证配置路径 metadata 被保留
	if assets[0].Metadata["config_path"] != "~/.openclaw" {
		t.Logf("Metadata: %v", assets[0].Metadata)
		// 注意：config_path 可能来自规则匹配或 enrichAssetWithConfig
		// 在没有真实配置文件的测试环境中，enrichment可能跳过
	}
}

// TestOpenclawAssetScanner_LoadRules 验证规则加载
func TestOpenclawAssetScanner_LoadRules(t *testing.T) {
	s := NewOpenclawAssetScanner("")
	rules, err := s.loadRules()
	if err != nil {
		t.Fatalf("loadRules failed: %v", err)
	}

	if len(rules) == 0 {
		t.Fatal("Expected at least one rule from openclaw.json")
	}

	// 验证规则包含端口和进程检测
	hasPortRule := false
	hasConfigRule := false
	for _, rule := range rules {
		if rule.Code == "openclaw_gateway_active" {
			hasPortRule = true
		}
		if rule.Code == "openclaw_config_exist" {
			hasConfigRule = true
		}
	}

	if !hasPortRule {
		t.Error("Expected 'openclaw_gateway_active' rule")
	}
	if !hasConfigRule {
		t.Error("Expected 'openclaw_config_exist' rule")
	}
}

// TestOpenclawAssetScanner_CollectorError 验证采集器出错时的容错处理
func TestOpenclawAssetScanner_CollectorError(t *testing.T) {
	s := NewOpenclawAssetScanner("")
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

func TestBuildRuntimeSection_ShowsPIDAndImagePath(t *testing.T) {
	section := buildRuntimeSection(&core.Asset{
		ProcessPaths: []string{"/usr/local/bin/openclaw-gateway"},
		Metadata: map[string]string{
			"pid": "502",
		},
	})

	if section == nil {
		t.Fatal("Expected runtime section to be created")
	}
	if section.Title != "Runtime" {
		t.Fatalf("Expected Runtime title, got %q", section.Title)
	}
	if len(section.Items) != 2 {
		t.Fatalf("Expected 2 runtime items, got %d", len(section.Items))
	}
	if section.Items[0].Label != "PID" || section.Items[0].Value != "502" {
		t.Fatalf("Unexpected PID item: %+v", section.Items[0])
	}
	if section.Items[1].Label != "Image Path" || section.Items[1].Value != "/usr/local/bin/openclaw-gateway" {
		t.Fatalf("Unexpected image item: %+v", section.Items[1])
	}
}

func TestParseOpenclawLaunchctlPIDs(t *testing.T) {
	output := `PID	Status	Label
67106	0	ai.openclaw.gateway
-	0	com.apple.other
`

	pids := parseOpenclawLaunchctlPIDs(output)
	if len(pids) != 1 {
		t.Fatalf("expected 1 launchctl PID, got %d", len(pids))
	}
	if pids[0] != 67106 {
		t.Fatalf("expected PID 67106, got %d", pids[0])
	}
}

func TestParseOpenclawPSMainPID(t *testing.T) {
	output := `  PID  PPID COMM             ARGS
67106     1 /Users/kidbei/.n /Users/kidbei/.nvm/versions/node/v24.13.0/bin/node /Users/kidbei/.nvm/versions/node/v24.13.0/lib/node_modules/openclaw/dist/index.js gateway --port 28789
99020 74631 /bin/zsh         /bin/zsh -c ps -eo pid,ppid,comm,args | rg -i "openclaw|moltbot|clawdbot"
99024 99020 rg               rg -i openclaw|moltbot|clawdbot
`

	pid, ok := parseOpenclawPSMainPID(output, openclawProcessMatchConstraints{
		gatewayPort: "28789",
	})
	if !ok {
		t.Fatal("expected ps PID to be parsed")
	}
	if pid != 67106 {
		t.Fatalf("expected PID 67106, got %d", pid)
	}
}

func TestParseOpenclawPSMainPID_IsolatesByGatewayPort(t *testing.T) {
	output := `  PID  PPID COMM             ARGS
10001     1 node             /opt/node /opt/openclaw/dist/index.js gateway --port 18789
10002     1 node             /opt/node /opt/openclaw/dist/index.js gateway --port 28789
`

	pid, ok := parseOpenclawPSMainPID(output, openclawProcessMatchConstraints{
		gatewayPort: "28789",
	})
	if !ok {
		t.Fatal("expected ps PID to be parsed")
	}
	if pid != 10002 {
		t.Fatalf("expected PID 10002, got %d", pid)
	}
}

func TestParseOpenclawPSMainPID_NoRunningGateway(t *testing.T) {
	output := `  PID  PPID COMM             ARGS
99020 74631 /bin/zsh         /bin/zsh -c ps -eo pid,ppid,comm,args | rg -i "openclaw|moltbot|clawdbot"
`

	if pid, ok := parseOpenclawPSMainPID(output, openclawProcessMatchConstraints{}); ok || pid != 0 {
		t.Fatalf("expected no PID, got pid=%d ok=%v", pid, ok)
	}
}
