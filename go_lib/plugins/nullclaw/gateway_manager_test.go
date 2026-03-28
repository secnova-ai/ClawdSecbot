package nullclaw

import "testing"

func TestBuildGatewayInstanceKey_AssetIDFirst(t *testing.T) {
	id := "nullclaw:abc123"
	got1 := buildGatewayInstanceKey("Nullclaw", id)
	got2 := buildGatewayInstanceKey("WrongName", id)

	if got1 != id {
		t.Fatalf("expected key=%s, got %s", id, got1)
	}
	if got2 != id {
		t.Fatalf("expected key=%s when asset name mismatches, got %s", id, got2)
	}
}

func TestBuildGatewayInstanceKey_FallbackWhenAssetIDEmpty(t *testing.T) {
	got := buildGatewayInstanceKey("Nullclaw", "")
	if got != nullclawAssetName {
		t.Fatalf("expected fallback key=%s, got %s", nullclawAssetName, got)
	}
}
