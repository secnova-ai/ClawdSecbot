package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go_lib/core/repository"
)

func TestRiskTitleKey_AliasAndKnownIDs(t *testing.T) {
	cases := []struct {
		riskID  string
		wantKey string
	}{
		{riskID: "gateway_bind_unsafe", wantKey: "riskNonLoopbackBinding"},
		{riskID: "riskNoAuth", wantKey: "riskNoAuth"},
		{riskID: "gateway_auth_disabled", wantKey: "riskNoAuth"},
		{riskID: "openclaw_1click_rce_vulnerability", wantKey: "riskOneClickRce"},
		{riskID: "nullclaw_1click_rce_vulnerability", wantKey: "riskOneClickRce"},
		{riskID: "model_base_url_public", wantKey: "riskModelBaseUrlPublic"},
	}

	for _, tc := range cases {
		if got := riskTitleKey(tc.riskID); got != tc.wantKey {
			t.Fatalf("riskTitleKey(%q) = %q, want %q", tc.riskID, got, tc.wantKey)
		}
	}
}

func TestRiskDescriptionKey_ExportMapping(t *testing.T) {
	cases := []struct {
		riskID  string
		wantKey string
	}{
		{riskID: "gateway_bind_unsafe", wantKey: "riskNonLoopbackBindingDesc"},
		{riskID: "riskNoAuth", wantKey: "riskNoAuthDesc"},
		{riskID: "gateway_auth_disabled", wantKey: "riskNoAuthDesc"},
		{riskID: "openclaw_1click_rce_vulnerability", wantKey: "riskOneClickRceDesc"},
		{riskID: "nullclaw_1click_rce_vulnerability", wantKey: "riskOneClickRceDesc"},
		{riskID: "terminal_backend_local", wantKey: "riskTerminalBackendLocalDesc"},
	}

	for _, tc := range cases {
		if got := riskDescriptionKey(tc.riskID); got != tc.wantKey {
			t.Fatalf("riskDescriptionKey(%q) = %q, want %q", tc.riskID, got, tc.wantKey)
		}
	}
}

func TestReplaceMitigationDescWithKey(t *testing.T) {
	input := []MitigationInfo{
		{Desc: "old-1", Command: "cmd-1"},
		{Desc: "old-2", Command: "cmd-2"},
	}
	infos := replaceMitigationDescWithKey(input, "riskNonLoopbackBindingDesc")
	if len(infos) != 2 {
		t.Fatalf("len(infos) = %d, want 2", len(infos))
	}
	for i, info := range infos {
		if info.Desc != "riskNonLoopbackBindingDesc" {
			t.Fatalf("infos[%d].Desc = %q, want %q", i, info.Desc, "riskNonLoopbackBindingDesc")
		}
		if info.Command != input[i].Command {
			t.Fatalf("infos[%d].Command = %q, want %q", i, info.Command, input[i].Command)
		}
	}
}

func TestMapRepositoryAuditLogToExportEntry_UsesAuditWindowFields(t *testing.T) {
	record := &repository.AuditLog{
		ID:               "audit-001",
		Timestamp:        "2026-04-21T10:00:00Z",
		RequestID:        "audit-001",
		AssetID:          "openclaw:abc123",
		Model:            "gpt-test",
		RequestContent:   "hello",
		ToolCalls:        `[{"name":"read_file","arguments":"{\"path\":\"a.txt\"}","result":"ok","is_sensitive":true}]`,
		HasRisk:          true,
		RiskLevel:        "SUSPICIOUS",
		RiskReason:       "tool access",
		Action:           "WARN",
		PromptTokens:     10,
		CompletionTokens: 5,
		TotalTokens:      15,
		DurationMs:       120,
	}

	entry := mapRepositoryAuditLogToExportEntry(record)
	if entry.BotID != record.AssetID || entry.LogID != record.ID || entry.RequestID != record.RequestID {
		t.Fatalf("expected identity fields to map from repository record, got %#v", entry)
	}
	if entry.UserRequest != record.RequestContent || entry.RiskCauses != record.RiskReason {
		t.Fatalf("expected content/risk fields to map from repository record, got %#v", entry)
	}
	if entry.TokenCount != 15 || entry.ToolCallCount != 1 || len(entry.ToolCalls) != 1 {
		t.Fatalf("expected token/tool counts from repository record, got %#v", entry)
	}
	if entry.ToolCalls[0].Tool != "read_file" || entry.ToolCalls[0].Parameters != `{"path":"a.txt"}` || entry.ToolCalls[0].Result != "ok" {
		t.Fatalf("expected tool call fields converted for audit.jsonl, got %#v", entry.ToolCalls[0])
	}
}

func TestExportService_AppendMissingAuditLogsFromRepository_DedupesByLogID(t *testing.T) {
	setupPolicyRuntimeTestDB(t)

	repo := repository.NewAuditLogRepository(nil)
	for _, record := range []*repository.AuditLog{
		{
			ID:             "audit-existing",
			Timestamp:      "2026-04-21T10:00:00Z",
			RequestID:      "audit-existing",
			AssetID:        "bot-1",
			RequestContent: "already exported",
			Action:         "ALLOW",
		},
		{
			ID:             "audit-new",
			Timestamp:      "2026-04-21T10:01:00Z",
			RequestID:      "audit-new",
			AssetID:        "bot-1",
			RequestContent: "new log",
			Action:         "WARN",
			RiskLevel:      "SUSPICIOUS",
			RiskReason:     "test risk",
		},
	} {
		if err := repo.SaveAuditLog(record); err != nil {
			t.Fatalf("SaveAuditLog(%s) failed: %v", record.ID, err)
		}
	}

	auditFile := filepath.Join(t.TempDir(), "audit.jsonl")
	existing, err := json.Marshal(&AuditLogEntry{LogID: "audit-existing"})
	if err != nil {
		t.Fatalf("marshal existing entry: %v", err)
	}
	if err := os.WriteFile(auditFile, append(existing, '\n'), 0644); err != nil {
		t.Fatalf("write existing audit file: %v", err)
	}

	svc := &ExportServiceImpl{running: true, auditFile: auditFile}
	if err := svc.InitializeExportedAuditLogIDs(); err != nil {
		t.Fatalf("InitializeExportedAuditLogIDs failed: %v", err)
	}
	if err := svc.AppendMissingAuditLogsFromRepository(); err != nil {
		t.Fatalf("AppendMissingAuditLogsFromRepository failed: %v", err)
	}
	if err := svc.AppendMissingAuditLogsFromRepository(); err != nil {
		t.Fatalf("second AppendMissingAuditLogsFromRepository failed: %v", err)
	}

	raw, err := os.ReadFile(auditFile)
	if err != nil {
		t.Fatalf("read audit file: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 exported lines after de-dupe, got %d: %s", len(lines), raw)
	}
	var appended AuditLogEntry
	if err := json.Unmarshal([]byte(lines[1]), &appended); err != nil {
		t.Fatalf("unmarshal appended line: %v", err)
	}
	if appended.LogID != "audit-new" || appended.UserRequest != "new log" || appended.RiskCauses != "test risk" {
		t.Fatalf("expected missing audit row to be appended from repository, got %#v", appended)
	}
}

func TestExportService_AppendCompletedAuditLogsFromRepository_SkipsIncompleteRows(t *testing.T) {
	setupPolicyRuntimeTestDB(t)

	repo := repository.NewAuditLogRepository(nil)
	for _, record := range []*repository.AuditLog{
		{
			ID:             "audit-complete",
			Timestamp:      "2026-04-21T10:02:00Z",
			RequestID:      "audit-complete",
			AssetID:        "bot-1",
			RequestContent: "finished request",
			OutputContent:  "finished response",
			Action:         "ALLOW",
		},
		{
			ID:             "audit-incomplete",
			Timestamp:      "2026-04-21T10:03:00Z",
			RequestID:      "audit-incomplete",
			AssetID:        "bot-1",
			RequestContent: "still running",
			Action:         "ALLOW",
		},
	} {
		if err := repo.SaveAuditLog(record); err != nil {
			t.Fatalf("SaveAuditLog(%s) failed: %v", record.ID, err)
		}
	}

	auditFile := filepath.Join(t.TempDir(), "audit.jsonl")
	svc := &ExportServiceImpl{running: true, auditFile: auditFile}
	if err := svc.InitializeExportedAuditLogIDs(); err != nil {
		t.Fatalf("InitializeExportedAuditLogIDs failed: %v", err)
	}
	if err := svc.AppendCompletedAuditLogsFromRepository(); err != nil {
		t.Fatalf("AppendCompletedAuditLogsFromRepository failed: %v", err)
	}

	raw, err := os.ReadFile(auditFile)
	if err != nil {
		t.Fatalf("read audit file: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected only the complete row to be exported, got %d: %s", len(lines), raw)
	}
	var appended AuditLogEntry
	if err := json.Unmarshal([]byte(lines[0]), &appended); err != nil {
		t.Fatalf("unmarshal appended line: %v", err)
	}
	if appended.LogID != "audit-complete" {
		t.Fatalf("expected complete row to be appended, got %#v", appended)
	}
}

func TestExportService_AppendMissingAuditLogsFromRepository_AppendsNewestLast(t *testing.T) {
	setupPolicyRuntimeTestDB(t)

	repo := repository.NewAuditLogRepository(nil)
	for _, record := range []*repository.AuditLog{
		{
			ID:             "audit-old",
			Timestamp:      "2026-04-21T10:00:00Z",
			RequestID:      "audit-old",
			AssetID:        "bot-1",
			RequestContent: "old log",
			Action:         "ALLOW",
		},
		{
			ID:             "audit-new",
			Timestamp:      "2026-04-21T10:01:00Z",
			RequestID:      "audit-new",
			AssetID:        "bot-1",
			RequestContent: "new log",
			Action:         "ALLOW",
		},
	} {
		if err := repo.SaveAuditLog(record); err != nil {
			t.Fatalf("SaveAuditLog(%s) failed: %v", record.ID, err)
		}
	}

	auditFile := filepath.Join(t.TempDir(), "audit.jsonl")
	svc := &ExportServiceImpl{running: true, auditFile: auditFile}
	if err := svc.InitializeExportedAuditLogIDs(); err != nil {
		t.Fatalf("InitializeExportedAuditLogIDs failed: %v", err)
	}
	if err := svc.AppendMissingAuditLogsFromRepository(); err != nil {
		t.Fatalf("AppendMissingAuditLogsFromRepository failed: %v", err)
	}

	raw, err := os.ReadFile(auditFile)
	if err != nil {
		t.Fatalf("read audit file: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 exported lines, got %d: %s", len(lines), raw)
	}
	var first, last AuditLogEntry
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("unmarshal first line: %v", err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &last); err != nil {
		t.Fatalf("unmarshal last line: %v", err)
	}
	if first.LogID != "audit-old" || last.LogID != "audit-new" {
		t.Fatalf("expected newest log to be last, got first=%s last=%s", first.LogID, last.LogID)
	}
}

func TestExportService_AppendMissingAuditLogsFromRepository_UsesStartWatermark(t *testing.T) {
	setupPolicyRuntimeTestDB(t)

	repo := repository.NewAuditLogRepository(nil)
	for _, record := range []*repository.AuditLog{
		{
			ID:             "audit-before-start",
			Timestamp:      "2026-04-21T10:00:00Z",
			RequestID:      "audit-before-start",
			AssetID:        "bot-1",
			RequestContent: "old log",
			Action:         "ALLOW",
		},
		{
			ID:             "audit-after-start",
			Timestamp:      "2026-04-21T10:01:00Z",
			RequestID:      "audit-after-start",
			AssetID:        "bot-1",
			RequestContent: "new log",
			Action:         "ALLOW",
		},
	} {
		if err := repo.SaveAuditLog(record); err != nil {
			t.Fatalf("SaveAuditLog(%s) failed: %v", record.ID, err)
		}
	}

	auditFile := filepath.Join(t.TempDir(), "audit.jsonl")
	svc := &ExportServiceImpl{
		running:            true,
		auditFile:          auditFile,
		auditExportStartAt: "2026-04-21T10:00:30Z",
	}
	if err := svc.InitializeExportedAuditLogIDs(); err != nil {
		t.Fatalf("InitializeExportedAuditLogIDs failed: %v", err)
	}
	if err := svc.AppendMissingAuditLogsFromRepository(); err != nil {
		t.Fatalf("AppendMissingAuditLogsFromRepository failed: %v", err)
	}

	raw, err := os.ReadFile(auditFile)
	if err != nil {
		t.Fatalf("read audit file: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected only logs after start watermark, got %d: %s", len(lines), raw)
	}
	var appended AuditLogEntry
	if err := json.Unmarshal([]byte(lines[0]), &appended); err != nil {
		t.Fatalf("unmarshal appended line: %v", err)
	}
	if appended.LogID != "audit-after-start" {
		t.Fatalf("expected only post-start log to be exported, got %#v", appended)
	}
}
