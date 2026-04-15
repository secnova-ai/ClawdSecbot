package core

import (
	"testing"
)

// TestDetect_PortAndProcess 验证端口+进程匹配规则
func TestDetect_PortAndProcess(t *testing.T) {
	engine := NewEngine()

	// 加载端口+进程检测规则
	engine.LoadRule(AssetFinderRule{
		Code:      "openclaw_gateway_active",
		Name:      "Openclaw Gateway",
		LifeCycle: RuleLifeCycleRuntime,
		Desc:      "Detects active Openclaw Gateway via Port and Process",
		Expression: RuleExpression{
			Lang: "json_match",
			Expr: `{"ports": [18789], "process_keywords": ["openclaw", "moltbot"]}`,
		},
	})

	// 创建模拟快照：Openclaw正在运行
	snapshot := SystemSnapshot{
		OpenPorts: []int{22, 80, 18789},
		RunningProcesses: []SystemProcess{
			{Pid: 101, Name: "launchd", Cmd: "/sbin/launchd", Path: "/sbin/launchd"},
			// 真实场景：openclaw 进程名称或路径包含 openclaw 关键字
			{Pid: 502, Name: "openclaw-gateway", Cmd: "openclaw gateway", Path: "/usr/local/bin/openclaw-gateway"},
		},
		Services:   []string{"ssh-agent"},
		FileExists: func(path string) bool { return false },
	}

	assets, err := engine.Detect(snapshot)
	if err != nil {
		t.Fatalf("Detection error: %v", err)
	}

	if len(assets) == 0 {
		t.Fatal("Expected to detect Openclaw, but found 0 assets")
	}

	asset := assets[0]
	if len(asset.Ports) != 1 || asset.Ports[0] != 18789 {
		t.Errorf("Expected port [18789], got %v", asset.Ports)
	}

	// 验证进程路径被记录
	if len(asset.ProcessPaths) == 0 {
		t.Error("Expected process paths to be recorded")
	}
	if asset.Metadata["pid"] != "502" {
		t.Errorf("Expected pid metadata '502', got %q", asset.Metadata["pid"])
	}

	t.Logf("Detected Asset: %+v", asset)
}

func TestDetect_PortAndWrapperProcessCommandLine(t *testing.T) {
	engine := NewEngine()

	engine.LoadRule(AssetFinderRule{
		Code:      "openclaw_gateway_active",
		Name:      "Openclaw Gateway",
		LifeCycle: RuleLifeCycleRuntime,
		Desc:      "Detects active Openclaw Gateway via Port and Process",
		Expression: RuleExpression{
			Lang: "json_match",
			Expr: `{"ports": [18789], "process_keywords": ["openclaw"]}`,
		},
	})

	snapshot := SystemSnapshot{
		OpenPorts: []int{18789},
		RunningProcesses: []SystemProcess{
			{
				Pid:  13364,
				Name: "node.exe",
				Cmd:  `"C:\Program Files\nodejs\node.exe" C:\Users\hudy\AppData\Roaming\npm\node_modules\openclaw\dist\index.js gateway --port 18789`,
				Path: `C:\Program Files\nodejs\node.exe`,
			},
		},
		Services:   []string{},
		FileExists: func(path string) bool { return false },
	}

	assets, err := engine.Detect(snapshot)
	if err != nil {
		t.Fatalf("Detection error: %v", err)
	}
	if len(assets) != 1 {
		t.Fatalf("Expected 1 asset, got %d", len(assets))
	}
	if got := assets[0].Metadata["pid"]; got != "13364" {
		t.Fatalf("Expected pid metadata '13364', got %q", got)
	}
	if len(assets[0].ProcessPaths) != 1 || assets[0].ProcessPaths[0] != `C:\Program Files\nodejs\node.exe` {
		t.Fatalf("Expected node executable path to be recorded, got %v", assets[0].ProcessPaths)
	}
}

func TestDetect_ConfigFileOnly_DoesNotFabricatePID(t *testing.T) {
	engine := NewEngine()

	engine.LoadRule(AssetFinderRule{
		Code:      "openclaw_config_exist",
		Name:      "Config File Detection",
		LifeCycle: RuleLifeCycleStatic,
		Desc:      "Detects Openclaw presence via Config File",
		Expression: RuleExpression{
			Lang: "json_match",
			Expr: `{"file_paths": ["~/.openclaw"]}`,
		},
	})

	snapshot := SystemSnapshot{
		OpenPorts:        []int{},
		RunningProcesses: []SystemProcess{},
		Services:         []string{},
		FileExists: func(path string) bool {
			return path == "~/.openclaw"
		},
	}

	assets, err := engine.Detect(snapshot)
	if err != nil {
		t.Fatalf("Detection error: %v", err)
	}
	if len(assets) != 1 {
		t.Fatalf("Expected 1 asset, got %d", len(assets))
	}
	if assets[0].Metadata["pid"] != "" {
		t.Fatalf("Expected empty pid metadata for static-only asset, got %q", assets[0].Metadata["pid"])
	}
}

// TestDetect_ConfigFile 验证配置文件匹配规则
func TestDetect_ConfigFile(t *testing.T) {
	engine := NewEngine()

	engine.LoadRule(AssetFinderRule{
		Code:      "openclaw_config_exist",
		Name:      "Config File Detection",
		LifeCycle: RuleLifeCycleStatic,
		Desc:      "Detects Openclaw presence via Config File",
		Expression: RuleExpression{
			Lang: "json_match",
			Expr: `{"file_paths": ["~/.openclaw", "~/.moltbot", "~/.clawdbot"]}`,
		},
	})

	snapshot := SystemSnapshot{
		OpenPorts:        []int{},
		RunningProcesses: []SystemProcess{},
		Services:         []string{},
		FileExists: func(path string) bool {
			return path == "~/.openclaw"
		},
	}

	assets, err := engine.Detect(snapshot)
	if err != nil {
		t.Fatalf("Detection error: %v", err)
	}

	if len(assets) == 0 {
		t.Fatal("Expected to detect config file asset")
	}

	if assets[0].Metadata["config_path"] != "~/.openclaw" {
		t.Errorf("Expected config_path '~/.openclaw', got '%s'", assets[0].Metadata["config_path"])
	}
}

// TestDetect_NoMatch 验证无匹配时返回空列表
func TestDetect_NoMatch(t *testing.T) {
	engine := NewEngine()

	engine.LoadRule(AssetFinderRule{
		Code:      "test_rule",
		Name:      "Test Rule",
		LifeCycle: RuleLifeCycleRuntime,
		Desc:      "Test rule",
		Expression: RuleExpression{
			Lang: "json_match",
			Expr: `{"ports": [18789], "process_keywords": ["openclaw"]}`,
		},
	})

	// 系统中没有Openclaw
	snapshot := SystemSnapshot{
		OpenPorts: []int{22, 80, 443},
		RunningProcesses: []SystemProcess{
			{Pid: 1, Name: "nginx", Cmd: "nginx -g daemon off", Path: "/usr/sbin/nginx"},
		},
		Services:   []string{"nginx"},
		FileExists: func(path string) bool { return false },
	}

	assets, err := engine.Detect(snapshot)
	if err != nil {
		t.Fatalf("Detection error: %v", err)
	}

	if len(assets) != 0 {
		t.Errorf("Expected 0 assets, got %d", len(assets))
	}
}

// TestDetect_RuleLifeCycleFilter 验证规则生命周期过滤
func TestDetect_RuleLifeCycleFilter(t *testing.T) {
	engine := NewEngine()

	// 加载一条无效生命周期的规则
	engine.LoadRule(AssetFinderRule{
		Code:      "invalid_lifecycle",
		Name:      "Invalid Lifecycle Rule",
		LifeCycle: RuleLifeCycle(99), // 无效的生命周期
		Desc:      "Should be skipped",
		Expression: RuleExpression{
			Lang: "json_match",
			Expr: `{"ports": [18789]}`,
		},
	})

	snapshot := SystemSnapshot{
		OpenPorts:        []int{18789},
		RunningProcesses: []SystemProcess{},
		Services:         []string{},
		FileExists:       func(path string) bool { return false },
	}

	assets, err := engine.Detect(snapshot)
	if err != nil {
		t.Fatalf("Detection error: %v", err)
	}

	if len(assets) != 0 {
		t.Errorf("Expected rule with invalid lifecycle to be skipped, got %d assets", len(assets))
	}
}

// TestDetect_MultipleRules 验证多规则同时检测
func TestDetect_MultipleRules(t *testing.T) {
	engine := NewEngine()

	// 端口+进程规则
	engine.LoadRule(AssetFinderRule{
		Code:      "rule_runtime",
		Name:      "Runtime Detection",
		LifeCycle: RuleLifeCycleRuntime,
		Desc:      "Port + Process",
		Expression: RuleExpression{
			Lang: "json_match",
			Expr: `{"ports": [18789], "process_keywords": ["openclaw"]}`,
		},
	})

	// 配置文件规则
	engine.LoadRule(AssetFinderRule{
		Code:      "rule_static",
		Name:      "Static Detection",
		LifeCycle: RuleLifeCycleStatic,
		Desc:      "Config file",
		Expression: RuleExpression{
			Lang: "json_match",
			Expr: `{"file_paths": ["~/.openclaw"]}`,
		},
	})

	snapshot := SystemSnapshot{
		OpenPorts: []int{18789},
		RunningProcesses: []SystemProcess{
			{Pid: 502, Name: "openclaw", Cmd: "openclaw gateway", Path: "/usr/local/bin/openclaw"},
		},
		Services: []string{},
		FileExists: func(path string) bool {
			return path == "~/.openclaw"
		},
	}

	assets, err := engine.Detect(snapshot)
	if err != nil {
		t.Fatalf("Detection error: %v", err)
	}

	if len(assets) != 2 {
		t.Fatalf("Expected 2 assets from 2 rules, got %d", len(assets))
	}
}

// TestDetect_ServiceMatch 验证服务名称匹配
func TestDetect_ServiceMatch(t *testing.T) {
	engine := NewEngine()

	engine.LoadRule(AssetFinderRule{
		Code:      "test_service",
		Name:      "Service Detection",
		LifeCycle: RuleLifeCycleRuntime,
		Desc:      "Detects via service name",
		Expression: RuleExpression{
			Lang: "json_match",
			Expr: `{"service_names": ["openclaw"]}`,
		},
	})

	snapshot := SystemSnapshot{
		OpenPorts:        []int{},
		RunningProcesses: []SystemProcess{},
		Services:         []string{"com.openclaw.gateway", "ssh-agent"},
		FileExists:       func(path string) bool { return false },
	}

	assets, err := engine.Detect(snapshot)
	if err != nil {
		t.Fatalf("Detection error: %v", err)
	}

	if len(assets) == 0 {
		t.Fatal("Expected to detect service asset")
	}

	if assets[0].ServiceName != "com.openclaw.gateway" {
		t.Errorf("Expected service name 'com.openclaw.gateway', got '%s'", assets[0].ServiceName)
	}
}

// TestDetect_InvalidExpression 验证无效表达式的处理
func TestDetect_InvalidExpression(t *testing.T) {
	engine := NewEngine()

	engine.LoadRule(AssetFinderRule{
		Code:      "invalid_expr",
		Name:      "Invalid Expression",
		LifeCycle: RuleLifeCycleRuntime,
		Desc:      "Has invalid JSON expression",
		Expression: RuleExpression{
			Lang: "json_match",
			Expr: `{invalid json`,
		},
	})

	snapshot := SystemSnapshot{
		OpenPorts:        []int{18789},
		RunningProcesses: []SystemProcess{},
		Services:         []string{},
		FileExists:       func(path string) bool { return false },
	}

	// 不应该panic，应该优雅处理
	assets, err := engine.Detect(snapshot)
	if err != nil {
		t.Fatalf("Detection should not return error for invalid expression: %v", err)
	}

	if len(assets) != 0 {
		t.Error("Expected 0 assets for invalid expression")
	}
}

// TestDetect_ProcessMatchExcludesCmdArgs 验证进程匹配不包含命令行参数，避免误报
// 例如：vim 编辑 ~/.openclaw/openclaw.json 不应该被识别为 openclaw 资产
func TestDetect_ProcessMatchExcludesCmdArgs(t *testing.T) {
	engine := NewEngine()

	// 加载进程关键字匹配规则
	engine.LoadRule(AssetFinderRule{
		Code:      "openclaw_process",
		Name:      "Openclaw Process Detection",
		LifeCycle: RuleLifeCycleRuntime,
		Desc:      "Detects Openclaw via process keywords",
		Expression: RuleExpression{
			Lang: "json_match",
			Expr: `{"process_keywords": ["openclaw"]}`,
		},
	})

	// 模拟场景：vim 正在编辑包含 openclaw 关键字的文件
	// 这不应该被识别为 openclaw 资产
	snapshot := SystemSnapshot{
		OpenPorts: []int{},
		RunningProcesses: []SystemProcess{
			// vim 编辑包含 "openclaw" 关键字的文件，命令行包含关键字但进程名和路径不包含
			{Pid: 1234, Name: "vim", Cmd: "vim /Users/user/.openclaw/openclaw.json", Path: "/usr/bin/vim"},
			// 真正的 openclaw 进程
			{Pid: 5678, Name: "openclaw-gateway", Cmd: "openclaw gateway", Path: "/usr/local/bin/openclaw-gateway"},
		},
		Services:   []string{},
		FileExists: func(path string) bool { return false },
	}

	assets, err := engine.Detect(snapshot)
	if err != nil {
		t.Fatalf("Detection error: %v", err)
	}

	if len(assets) == 0 {
		t.Fatal("Expected to detect openclaw process")
	}

	// 应该只检测到一个资产
	if len(assets) != 1 {
		t.Errorf("Expected 1 asset, got %d", len(assets))
	}

	asset := assets[0]

	// 应该只包含 openclaw-gateway 进程，不包含 vim
	if len(asset.ProcessPaths) != 1 {
		t.Errorf("Expected 1 process path, got %d: %v", len(asset.ProcessPaths), asset.ProcessPaths)
	}

	if len(asset.ProcessPaths) > 0 && asset.ProcessPaths[0] != "/usr/local/bin/openclaw-gateway" {
		t.Errorf("Expected process path '/usr/local/bin/openclaw-gateway', got '%s'", asset.ProcessPaths[0])
	}

	// 确保没有包含 vim 进程
	for _, path := range asset.ProcessPaths {
		if path == "/usr/bin/vim" {
			t.Error("vim process should not be included in detected assets")
		}
	}

	t.Logf("Detected Asset: %+v", asset)
}
