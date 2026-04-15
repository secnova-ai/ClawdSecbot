package proxy

import "testing"

func TestBuildAssetKey(t *testing.T) {
	tests := []struct {
		name    string
		assetID string
		want    string
	}{
		{
			name:    "default key",
			assetID: "",
			want:    defaultProxyAssetKey,
		},
		{
			name:    "asset id key",
			assetID: "asset-1",
			want:    "asset-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildAssetKey(tt.assetID); got != tt.want {
				t.Fatalf("buildAssetKey(%q)=%q, want %q", tt.assetID, got, tt.want)
			}
		})
	}
}

func TestGetProxyProtectionByAsset(t *testing.T) {
	proxyInstanceMu.Lock()
	oldMap := proxyByAssetKey
	proxyByAssetKey = make(map[string]*ProxyProtection)
	proxyInstanceMu.Unlock()
	t.Cleanup(func() {
		proxyInstanceMu.Lock()
		proxyByAssetKey = oldMap
		proxyInstanceMu.Unlock()
	})

	pp := &ProxyProtection{port: 12345}
	key := buildAssetKey("asset-42")

	proxyInstanceMu.Lock()
	proxyByAssetKey[key] = pp
	proxyInstanceMu.Unlock()

	got := GetProxyProtectionByAsset("asset-42")
	if got != pp {
		t.Fatalf("expected to fetch proxy by asset key")
	}
}

func TestGetProxyForOperationLocked_ByAsset(t *testing.T) {
	proxyInstanceMu.Lock()
	oldMap := proxyByAssetKey
	oldActive := activeAssetKey
	oldInstance := proxyInstance

	pp := &ProxyProtection{port: 18080}
	proxyByAssetKey = map[string]*ProxyProtection{
		buildAssetKey("asset-1"): pp,
	}
	activeAssetKey = defaultProxyAssetKey
	proxyInstance = nil

	got := getProxyForOperationLocked("asset-1")
	proxyInstanceMu.Unlock()

	t.Cleanup(func() {
		proxyInstanceMu.Lock()
		proxyByAssetKey = oldMap
		activeAssetKey = oldActive
		proxyInstance = oldInstance
		proxyInstanceMu.Unlock()
	})

	if got != pp {
		t.Fatalf("expected asset-scoped proxy, got %#v", got)
	}
}

func TestGetProxyForOperationLocked_DefaultFallsBackToActive(t *testing.T) {
	proxyInstanceMu.Lock()
	oldMap := proxyByAssetKey
	oldActive := activeAssetKey
	oldInstance := proxyInstance

	pp := &ProxyProtection{port: 19090}
	key := buildAssetKey("asset-2")
	proxyByAssetKey = map[string]*ProxyProtection{
		key: pp,
	}
	activeAssetKey = key
	proxyInstance = nil

	got := getProxyForOperationLocked("")
	proxyInstanceMu.Unlock()

	t.Cleanup(func() {
		proxyInstanceMu.Lock()
		proxyByAssetKey = oldMap
		activeAssetKey = oldActive
		proxyInstance = oldInstance
		proxyInstanceMu.Unlock()
	})

	if got != pp {
		t.Fatalf("expected active proxy fallback, got %#v", got)
	}
}
