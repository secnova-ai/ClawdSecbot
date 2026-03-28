package nullclaw

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const samplePlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>com.nullclaw.gateway</string>
  <key>ProgramArguments</key>
  <array>
    <string>/usr/local/bin/nullclaw</string>
    <string>gateway</string>
    <string>start</string>
  </array>
</dict>
</plist>
`

func TestInjectSandboxIntoPlist_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()
	plist := filepath.Join(tmpDir, "agent.plist")
	if err := os.WriteFile(plist, []byte(samplePlist), 0644); err != nil {
		t.Fatal(err)
	}

	policyPath := filepath.Join(tmpDir, "botsec_Nullclaw.sb")

	mod1, err := injectSandboxIntoPlist(plist, policyPath)
	if err != nil {
		t.Fatalf("inject failed: %v", err)
	}
	if !mod1 {
		t.Fatalf("expected modified on first inject")
	}

	// second inject should be idempotent
	mod2, err := injectSandboxIntoPlist(plist, policyPath)
	if err != nil {
		t.Fatalf("second inject failed: %v", err)
	}
	if mod2 {
		t.Fatalf("expected not modified on second inject")
	}

	b, _ := os.ReadFile(plist)
	s := string(b)
	if !strings.Contains(s, "<string>/usr/bin/sandbox-exec</string>") {
		t.Fatalf("expected sandbox-exec inserted")
	}
	if !strings.Contains(s, "<string>"+policyPath+"</string>") {
		t.Fatalf("expected policy path inserted")
	}
}

func TestInjectSandboxIntoPlist_UpdatePolicyPath(t *testing.T) {
	tmpDir := t.TempDir()
	plist := filepath.Join(tmpDir, "agent.plist")
	if err := os.WriteFile(plist, []byte(samplePlist), 0644); err != nil {
		t.Fatal(err)
	}

	p1 := filepath.Join(tmpDir, "p1.sb")
	p2 := filepath.Join(tmpDir, "p2.sb")

	_, err := injectSandboxIntoPlist(plist, p1)
	if err != nil {
		t.Fatalf("inject p1 failed: %v", err)
	}

	mod, err := injectSandboxIntoPlist(plist, p2)
	if err != nil {
		t.Fatalf("inject p2 failed: %v", err)
	}
	if !mod {
		t.Fatalf("expected modified when policy path changes")
	}

	b, _ := os.ReadFile(plist)
	s := string(b)
	if strings.Contains(s, "<string>"+p1+"</string>") {
		t.Fatalf("expected old policy path removed")
	}
	if !strings.Contains(s, "<string>"+p2+"</string>") {
		t.Fatalf("expected new policy path present")
	}
}

func TestRemoveSandboxFromPlist_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()
	plist := filepath.Join(tmpDir, "agent.plist")
	if err := os.WriteFile(plist, []byte(samplePlist), 0644); err != nil {
		t.Fatal(err)
	}

	policyPath := filepath.Join(tmpDir, "p.sb")
	_, err := injectSandboxIntoPlist(plist, policyPath)
	if err != nil {
		t.Fatalf("inject failed: %v", err)
	}

	mod1, err := removeSandboxFromPlist(plist)
	if err != nil {
		t.Fatalf("remove failed: %v", err)
	}
	if !mod1 {
		t.Fatalf("expected modified on remove")
	}

	mod2, err := removeSandboxFromPlist(plist)
	if err != nil {
		t.Fatalf("second remove failed: %v", err)
	}
	if mod2 {
		t.Fatalf("expected idempotent remove (not modified)")
	}

	b, _ := os.ReadFile(plist)
	s := string(b)
	if strings.Contains(s, "sandbox-exec") {
		t.Fatalf("expected sandbox-exec removed")
	}
}

func TestRestartNullclawGatewaySimple_BinaryNotFound(t *testing.T) {
	// 确保 PATH 中不存在 nullclaw 二进制
	// 保存原始 PATH 并设置一个空路径
	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", t.TempDir())
	defer os.Setenv("PATH", originalPath)

	// 清除可能缓存的配置路径，使资产扫描也找不到二进制
	oldConfigPath := GetConfigPath()
	SetConfigPath("")
	defer SetConfigPath(oldConfigPath)

	err := restartNullclawGatewaySimple()
	if err == nil {
		t.Fatal("expected error when nullclaw binary not found")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestRestartNullclawGatewaySimple_WithMockBinary(t *testing.T) {
	// 创建一个模拟的 nullclaw 二进制脚本
	tmpDir := t.TempDir()
	mockBinary := filepath.Join(tmpDir, "nullclaw")
	// 脚本记录收到的命令参数
	logFile := filepath.Join(tmpDir, "commands.log")
	script := `#!/bin/sh
echo "$@" >> ` + logFile + `
exit 0
`
	if err := os.WriteFile(mockBinary, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	// 确保 mock binary 可执行
	if _, err := exec.LookPath(mockBinary); err != nil {
		// 直接验证文件存在且可执行
		info, err := os.Stat(mockBinary)
		if err != nil {
			t.Fatalf("mock binary not found: %v", err)
		}
		if info.Mode()&0111 == 0 {
			t.Fatal("mock binary not executable")
		}
	}

	// 设置 PATH 指向临时目录
	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", tmpDir)
	defer os.Setenv("PATH", originalPath)

	// 清除配置路径以使用 PATH 查找
	oldConfigPath := GetConfigPath()
	SetConfigPath("")
	defer SetConfigPath(oldConfigPath)

	err := restartNullclawGatewaySimple()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 验证 stop 和 start 命令都被调用了
	logData, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("read log failed: %v", err)
	}
	logContent := string(logData)

	if !strings.Contains(logContent, "service stop") {
		t.Error("expected 'service stop' command to be called")
	}
	if !strings.Contains(logContent, "service start") {
		t.Error("expected 'service start' command to be called")
	}
}
