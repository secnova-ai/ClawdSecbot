package sandbox

import "testing"

func resetProcessMonitorRegistry(t *testing.T) {
	t.Helper()

	monitorMu.Lock()
	old := processMonitors
	processMonitors = make(map[string]*ProcessMonitor)
	monitorMu.Unlock()

	t.Cleanup(func() {
		monitorMu.Lock()
		for _, monitor := range processMonitors {
			if monitor != nil {
				monitor.Stop()
			}
		}
		processMonitors = old
		monitorMu.Unlock()
	})
}

func TestGetProcessMonitorByKey_IsolatedByAssetID(t *testing.T) {
	resetProcessMonitorRegistry(t)

	first := GetProcessMonitorByKey("openclaw:a1", "Openclaw", "gateway-a")
	second := GetProcessMonitorByKey("openclaw:a2", "Openclaw", "gateway-a")

	if first == nil || second == nil {
		t.Fatal("expected monitors to be created")
	}
	if first == second {
		t.Fatal("expected distinct monitors for different asset_id keys")
	}

	monitorMu.RLock()
	count := len(processMonitors)
	monitorMu.RUnlock()
	if count != 2 {
		t.Fatalf("expected 2 monitor entries, got %d", count)
	}
}

func TestGetProcessMonitorByKey_ReusesSameAssetID(t *testing.T) {
	resetProcessMonitorRegistry(t)

	first := GetProcessMonitorByKey("openclaw:a1", "Openclaw", "gateway-a")
	second := GetProcessMonitorByKey("openclaw:a1", "ChangedName", "gateway-b")

	if first != second {
		t.Fatal("expected same monitor instance for same asset_id key")
	}

	monitorMu.RLock()
	count := len(processMonitors)
	monitorMu.RUnlock()
	if count != 1 {
		t.Fatalf("expected 1 monitor entry, got %d", count)
	}
}

func TestRemoveProcessMonitorByKey_RemovesOnlyTargetAsset(t *testing.T) {
	resetProcessMonitorRegistry(t)

	first := GetProcessMonitorByKey("openclaw:a1", "Openclaw", "gateway-a")
	second := GetProcessMonitorByKey("openclaw:a2", "Openclaw", "gateway-a")
	if first == nil || second == nil {
		t.Fatal("expected monitors to be created")
	}

	RemoveProcessMonitorByKey("openclaw:a1")

	monitorMu.RLock()
	_, firstExists := processMonitors["openclaw:a1"]
	_, secondExists := processMonitors["openclaw:a2"]
	monitorMu.RUnlock()

	if firstExists {
		t.Fatal("expected first monitor to be removed")
	}
	if !secondExists {
		t.Fatal("expected second monitor to remain")
	}
}
