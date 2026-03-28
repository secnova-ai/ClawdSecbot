package openclaw

import (
	"reflect"
	"testing"
)

func TestBuildGatewayRuntimeStateKeysPrefersAssetIDAndKeepsLegacyAssetName(t *testing.T) {
	got := buildGatewayRuntimeStateKeys(openclawAssetName, "asset-123")
	want := []string{"asset-123", openclawAssetName}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected runtime state keys: got %v want %v", got, want)
	}
}

func TestBuildGatewayRuntimeStateKeysDeduplicatesEmptyOrSameValues(t *testing.T) {
	got := buildGatewayRuntimeStateKeys(openclawAssetName, openclawAssetName)
	want := []string{openclawAssetName}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected deduplicated runtime state keys: got %v want %v", got, want)
	}
}
