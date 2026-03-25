package openclaw

import "testing"

func TestBuildGatewayInstanceKey_AssetIDFirst(t *testing.T) {
	id := "openclaw:abc123"
	got1 := buildGatewayInstanceKey("Openclaw", id)
	got2 := buildGatewayInstanceKey("WrongName", id)

	if got1 != id {
		t.Fatalf("expected key=%s, got %s", id, got1)
	}
	if got2 != id {
		t.Fatalf("expected key=%s when asset name mismatches, got %s", id, got2)
	}
}

func TestBuildGatewayInstanceKey_FallbackWhenAssetIDEmpty(t *testing.T) {
	got := buildGatewayInstanceKey("Openclaw", "")
	if got != openclawAssetName {
		t.Fatalf("expected fallback key=%s, got %s", openclawAssetName, got)
	}
}
