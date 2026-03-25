package core

import (
	"encoding/json"
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

func TestComputeAssetID_DeterministicAndOrderInsensitive(t *testing.T) {
	id1 := ComputeAssetID(
		"Openclaw",
		"/Users/test/.openclaw/config.json",
		[]int{3000, 13436},
		[]string{"/usr/local/bin/openclaw", "/Applications/Openclaw.app"},
	)
	id2 := ComputeAssetID(
		"openclaw",
		"/Users/test/.openclaw/config.json",
		[]int{13436, 3000},
		[]string{"/Applications/Openclaw.app", "/usr/local/bin/openclaw"},
	)

	if id1 != id2 {
		t.Fatalf("expected deterministic id, got id1=%s id2=%s", id1, id2)
	}
}

func TestComputeAssetID_UniqueAcrossPlugins(t *testing.T) {
	openID := ComputeAssetID(
		"Openclaw",
		"/Users/test/.bot/config.json",
		[]int{3000},
		[]string{"/usr/local/bin/bot"},
	)
	nullID := ComputeAssetID(
		"Nullclaw",
		"/Users/test/.bot/config.json",
		[]int{3000},
		[]string{"/usr/local/bin/bot"},
	)

	if openID == nullID {
		t.Fatalf("asset id collision across plugin types: %s", openID)
	}
}

func TestComputeAssetID_UniqueForDifferentFingerprint(t *testing.T) {
	id1 := ComputeAssetID(
		"Openclaw",
		"/Users/test/.openclaw/config-a.json",
		[]int{3000},
		[]string{"/usr/local/bin/openclaw"},
	)
	id2 := ComputeAssetID(
		"Openclaw",
		"/Users/test/.openclaw/config-b.json",
		[]int{3000},
		[]string{"/usr/local/bin/openclaw"},
	)

	if id1 == id2 {
		t.Fatalf("expected different ids for different fingerprint, got %s", id1)
	}
}
