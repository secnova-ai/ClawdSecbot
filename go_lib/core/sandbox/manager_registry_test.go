package sandbox

import "testing"

func resetSandboxManagerRegistry(t *testing.T) {
	t.Helper()

	sandboxMu.Lock()
	old := sandboxManagers
	sandboxManagers = make(map[string]*SandboxManager)
	sandboxMu.Unlock()

	t.Cleanup(func() {
		sandboxMu.Lock()
		for _, manager := range sandboxManagers {
			if manager != nil {
				_ = manager.Stop()
			}
		}
		sandboxManagers = old
		sandboxMu.Unlock()
	})
}

func TestGetExistingSandboxManagerByKey_DoesNotCreate(t *testing.T) {
	resetSandboxManagerRegistry(t)

	got := GetExistingSandboxManagerByKey("openclaw:a1")
	if got != nil {
		t.Fatal("expected nil when manager does not exist")
	}

	sandboxMu.RLock()
	count := len(sandboxManagers)
	sandboxMu.RUnlock()
	if count != 0 {
		t.Fatalf("expected registry size 0, got %d", count)
	}
}

func TestGetExistingSandboxManagerByKey_ReturnsExisting(t *testing.T) {
	resetSandboxManagerRegistry(t)

	created := GetSandboxManagerByKey("openclaw:a1", t.TempDir())
	got := GetExistingSandboxManagerByKey("openclaw:a1")

	if got == nil {
		t.Fatal("expected existing manager")
	}
	if got != created {
		t.Fatal("expected to get the same manager pointer")
	}
}
