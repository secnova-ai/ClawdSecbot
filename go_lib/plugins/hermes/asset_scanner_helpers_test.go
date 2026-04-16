package hermes

import (
	"testing"

	"go_lib/core"
)

func TestHelperStatusFunctions(t *testing.T) {
	if got := fallbackString("", "fallback"); got != "fallback" {
		t.Fatalf("fallbackString empty mismatch: %q", got)
	}
	if got := fallbackString(" value ", "fallback"); got != "value" {
		t.Fatalf("fallbackString trim mismatch: %q", got)
	}

	if terminalBackendStatus("remote") != "safe" {
		t.Fatal("expected remote terminal backend to be safe")
	}
	if terminalBackendStatus("local") != "warning" {
		t.Fatal("expected local terminal backend to be warning")
	}
	if terminalBackendStatus("unknown") != "neutral" {
		t.Fatal("expected unknown terminal backend to be neutral")
	}

	if approvalModeStatus("off") != "danger" {
		t.Fatal("expected approvals off to be danger")
	}
	if approvalModeStatus("yolo") != "danger" {
		t.Fatal("expected approvals yolo to be danger")
	}
	if approvalModeStatus("manual") != "safe" {
		t.Fatal("expected approvals manual to be safe")
	}
	if approvalModeStatus("smart") != "safe" {
		t.Fatal("expected approvals smart to be safe")
	}
	if approvalModeStatus("unknown") != "neutral" {
		t.Fatal("expected unknown approvals mode to be neutral")
	}

	if redactStatus("true") != "safe" {
		t.Fatal("expected redact true to be safe")
	}
	if redactStatus("false") != "danger" {
		t.Fatal("expected redact false to be danger")
	}
	if redactStatus("unknown") != "neutral" {
		t.Fatal("expected unknown redact status to be neutral")
	}
}

func TestBuildRuntimeSection(t *testing.T) {
	if buildRuntimeSection(nil) != nil {
		t.Fatal("expected nil runtime section for nil asset")
	}

	a := &core.Asset{Metadata: map[string]string{}, ProcessPaths: nil}
	if buildRuntimeSection(a) != nil {
		t.Fatal("expected nil runtime section when runtime fields are empty")
	}

	a = &core.Asset{
		Metadata:     map[string]string{"pid": "4321"},
		ProcessPaths: []string{"/usr/local/bin/hermes"},
	}
	section := buildRuntimeSection(a)
	if section == nil {
		t.Fatal("expected runtime section")
	}
	if section.Title != "Runtime" {
		t.Fatalf("unexpected runtime section title: %s", section.Title)
	}
	if len(section.Items) != 2 {
		t.Fatalf("expected 2 runtime items, got %d", len(section.Items))
	}
}

func TestRewriteStableAssetID_FallbackToScannerConfigPath(t *testing.T) {
	scanner := NewHermesAssetScanner("/tmp/hermes/config.yaml")
	asset := &core.Asset{ID: "volatile", Metadata: map[string]string{}}
	scanner.rewriteStableAssetID(asset)

	want := core.ComputeAssetID(hermesAssetName, "/tmp/hermes/config.yaml", nil, nil)
	if asset.ID != want {
		t.Fatalf("asset id mismatch: got=%s want=%s", asset.ID, want)
	}
}
