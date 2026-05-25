package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAssetJSONSerialization(t *testing.T) {
	asset := Asset{
		Name:         "Test Asset",
		Type:         "Service",
		Version:      "1.0.0",
		Ports:        []int{8080, 443},
		ServiceName:  "test-service",
		ProcessPaths: []string{"/usr/bin/test", "/opt/test/bin"},
		Metadata: map[string]string{
			"workspace": "/home/user/project",
			"config":    "/etc/test.conf",
		},
	}

	// Test marshaling
	data, err := json.Marshal(asset)
	if err != nil {
		t.Fatalf("Failed to marshal asset: %v", err)
	}

	// Test unmarshaling
	var unmarshaled Asset
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal asset: %v", err)
	}

	if unmarshaled.Name != asset.Name {
		t.Errorf("Expected Name %s, got %s", asset.Name, unmarshaled.Name)
	}
	if unmarshaled.Type != asset.Type {
		t.Errorf("Expected Type %s, got %s", asset.Type, unmarshaled.Type)
	}
	if unmarshaled.Version != asset.Version {
		t.Errorf("Expected Version %s, got %s", asset.Version, unmarshaled.Version)
	}
	if len(unmarshaled.Ports) != 2 {
		t.Errorf("Expected 2 ports, got %d", len(unmarshaled.Ports))
	}
	if unmarshaled.ServiceName != asset.ServiceName {
		t.Errorf("Expected ServiceName %s, got %s", asset.ServiceName, unmarshaled.ServiceName)
	}
	if len(unmarshaled.ProcessPaths) != 2 {
		t.Errorf("Expected 2 ProcessPaths, got %d", len(unmarshaled.ProcessPaths))
	}
	if unmarshaled.Metadata["workspace"] != "/home/user/project" {
		t.Errorf("Expected workspace metadata")
	}
}

func TestAssetMatchCriteriaPorts(t *testing.T) {
	criteria := AssetMatchCriteria{
		Ports: []int{8080, 443, 22},
	}

	data, err := json.Marshal(criteria)
	if err != nil {
		t.Fatalf("Failed to marshal criteria: %v", err)
	}

	var unmarshaled AssetMatchCriteria
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal criteria: %v", err)
	}

	if len(unmarshaled.Ports) != 3 {
		t.Errorf("Expected 3 ports, got %d", len(unmarshaled.Ports))
	}
}

func TestAssetMatchCriteriaProcessKeywords(t *testing.T) {
	criteria := AssetMatchCriteria{
		ProcessKeywords: []string{"openclaw", "gateway", "agent"},
	}

	data, err := json.Marshal(criteria)
	if err != nil {
		t.Fatalf("Failed to marshal criteria: %v", err)
	}

	var unmarshaled AssetMatchCriteria
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal criteria: %v", err)
	}

	if len(unmarshaled.ProcessKeywords) != 3 {
		t.Errorf("Expected 3 keywords, got %d", len(unmarshaled.ProcessKeywords))
	}
}

func TestAssetMatchCriteriaServiceNames(t *testing.T) {
	criteria := AssetMatchCriteria{
		ServiceNames: []string{"ssh", "httpd", "mysql"},
	}

	data, err := json.Marshal(criteria)
	if err != nil {
		t.Fatalf("Failed to marshal criteria: %v", err)
	}

	var unmarshaled AssetMatchCriteria
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal criteria: %v", err)
	}

	if len(unmarshaled.ServiceNames) != 3 {
		t.Errorf("Expected 3 service names, got %d", len(unmarshaled.ServiceNames))
	}
}

func TestAssetMatchCriteriaFilePaths(t *testing.T) {
	criteria := AssetMatchCriteria{
		FilePaths: []string{"~/.openclaw", "/etc/openclaw/config.yaml"},
	}

	data, err := json.Marshal(criteria)
	if err != nil {
		t.Fatalf("Failed to marshal criteria: %v", err)
	}

	var unmarshaled AssetMatchCriteria
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal criteria: %v", err)
	}

	if len(unmarshaled.FilePaths) != 2 {
		t.Errorf("Expected 2 file paths, got %d", len(unmarshaled.FilePaths))
	}
}

func TestAssetMatchCriteriaCombined(t *testing.T) {
	criteria := AssetMatchCriteria{
		Ports:           []int{8080},
		ProcessKeywords: []string{"node"},
		ServiceNames:    []string{"test-service"},
		FilePaths:       []string{"~/.config"},
	}

	data, err := json.Marshal(criteria)
	if err != nil {
		t.Fatalf("Failed to marshal criteria: %v", err)
	}

	var unmarshaled AssetMatchCriteria
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal criteria: %v", err)
	}

	if unmarshaled.Ports == nil || len(unmarshaled.Ports) != 1 {
		t.Error("Ports not properly marshaled")
	}
	if unmarshaled.ProcessKeywords == nil || len(unmarshaled.ProcessKeywords) != 1 {
		t.Error("ProcessKeywords not properly marshaled")
	}
	if unmarshaled.ServiceNames == nil || len(unmarshaled.ServiceNames) != 1 {
		t.Error("ServiceNames not properly marshaled")
	}
	if unmarshaled.FilePaths == nil || len(unmarshaled.FilePaths) != 1 {
		t.Error("FilePaths not properly marshaled")
	}
}

func TestAssetEmpty(t *testing.T) {
	asset := Asset{}

	data, err := json.Marshal(asset)
	if err != nil {
		t.Fatalf("Failed to marshal empty asset: %v", err)
	}

	var unmarshaled Asset
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal empty asset: %v", err)
	}

	// All fields should be zero values
	if unmarshaled.Name != "" {
		t.Errorf("Expected empty Name, got %s", unmarshaled.Name)
	}
	if unmarshaled.Ports != nil {
		t.Error("Expected nil Ports")
	}
	if unmarshaled.Metadata != nil {
		t.Error("Expected nil Metadata")
	}
}

func TestComputeAssetID_DeterministicAndCaseInsensitive(t *testing.T) {
	id1 := ComputeAssetID("Openclaw", "/Users/test/.openclaw/config.json")
	id2 := ComputeAssetID("openclaw", "/Users/test/.openclaw/config.json")

	if id1 != id2 {
		t.Fatalf("expected deterministic id, got id1=%s id2=%s", id1, id2)
	}
}

// TestComputeAssetID_IgnoresRuntimeDynamics guards the invariant that
// ports/process_paths or any other runtime-dynamic info must NOT drift the ID
// when the bot starts/stops. The ID depends only on name + config_path.
func TestComputeAssetID_IgnoresRuntimeDynamics(t *testing.T) {
	// Before protection starts: bot not running, no ports/processes observed.
	idBeforeStart := ComputeAssetID("Openclaw", "/Users/test/.openclaw/config.json")
	// After protection restarts openclaw: the same instance now exposes ports
	// and process paths. ID must not change.
	idAfterStart := ComputeAssetID("Openclaw", "/Users/test/.openclaw/config.json")

	if idBeforeStart != idAfterStart {
		t.Fatalf("asset_id must be stable across runtime state changes, got before=%s after=%s",
			idBeforeStart, idAfterStart)
	}
}

func TestComputeAssetID_UniqueAcrossPlugins(t *testing.T) {
	openID := ComputeAssetID("Openclaw", "/Users/test/.bot/config.json")
	nullID := ComputeAssetID("Nullclaw", "/Users/test/.bot/config.json")

	if openID == nullID {
		t.Fatalf("asset id collision across plugin types: %s", openID)
	}
}

func TestComputeAssetID_UniqueForDifferentConfigPath(t *testing.T) {
	id1 := ComputeAssetID("Openclaw", "/Users/test/.openclaw/config-a.json")
	id2 := ComputeAssetID("Openclaw", "/Users/test/.openclaw/config-b.json")

	if id1 == id2 {
		t.Fatalf("expected different ids for different config_path, got %s", id1)
	}
}

func TestResolveStableConfigPathFingerprint_DirectoryAndFileEquivalent(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".openclaw")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	configFile := filepath.Join(configDir, "openclaw.json")
	if err := os.WriteFile(configFile, []byte(`{}`), 0644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	dirFingerprint := ResolveStableConfigPathFingerprint(configDir)
	fileFingerprint := ResolveStableConfigPathFingerprint(configFile)
	if dirFingerprint != fileFingerprint {
		t.Fatalf("expected same fingerprint path, dir=%s file=%s", dirFingerprint, fileFingerprint)
	}
	if dirFingerprint != configFile {
		t.Fatalf("expected resolved config file path %s, got %s", configFile, dirFingerprint)
	}

	idFromDir := ComputeAssetID("Openclaw", dirFingerprint)
	idFromFile := ComputeAssetID("Openclaw", fileFingerprint)
	if idFromDir != idFromFile {
		t.Fatalf("expected same asset id, dir=%s file=%s", idFromDir, idFromFile)
	}
}

func TestResolveStableConfigPathFingerprint_UsesPathManagerHome(t *testing.T) {
	tmpDir := t.TempDir()
	pmHome := filepath.Join(tmpDir, "pm-home")
	sysHome := filepath.Join(tmpDir, "sys-home")
	configDir := filepath.Join(pmHome, ".openclaw")
	configFile := filepath.Join(configDir, "openclaw.json")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(configFile, []byte(`{}`), 0644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}
	_ = GetPathManager().ResetForTest(tmpDir, pmHome)
	t.Setenv("HOME", sysHome)

	got := ResolveStableConfigPathFingerprint("~/.openclaw/openclaw.json")
	if got != configFile {
		t.Fatalf("expected PathManager-based path %s, got %s", configFile, got)
	}
}
