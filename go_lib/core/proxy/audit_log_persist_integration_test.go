package proxy

import (
	"path/filepath"
	"testing"
	"time"

	"go_lib/core/repository"
)

func TestAuditChainTracker_PersistsLogToSQLite(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "audit_log_integration.db")
	if err := repository.InitDB(dbPath); err != nil {
		t.Fatalf("init db failed: %v", err)
	}
	defer repository.CloseDB()

	tracker := NewAuditChainTracker()
	msgs := buildMessagesFromRaw(t, `{
	  "model":"gpt-test",
	  "messages":[
	    {"role":"user","content":"scan workspace"}
	  ]
	}`)
	tracker.StartFromRequest("req_integration", "openclaw", "openclaw:a1", "gpt-test", msgs)

	repo := repository.NewAuditLogRepository(nil)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		count, err := repo.GetAuditLogCount(false, "", "openclaw:a1", "")
		if err == nil && count > 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}

	count, err := repo.GetAuditLogCount(false, "", "openclaw:a1", "")
	if err != nil {
		t.Fatalf("query audit log count failed: %v", err)
	}
	t.Fatalf("expected persisted audit log count > 0, got %d", count)
}
