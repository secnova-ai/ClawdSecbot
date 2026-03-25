package shepherd

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go_lib/core/logging"
	"go_lib/core/repository"
)

// SecurityEvent represents a security event recorded by ReAct Agent or heuristic detection.
type SecurityEvent struct {
	ID         string `json:"id"`
	BotID      string `json:"bot_id,omitempty"`
	Timestamp  string `json:"timestamp"`
	EventType  string `json:"event_type"`  // tool_execution | blocked | other
	ActionDesc string `json:"action_desc"` // Action description (LLM generated)
	RiskType   string `json:"risk_type"`   // Risk type
	Detail     string `json:"detail"`      // Additional detail
	Source     string `json:"source"`      // react_agent | heuristic
	AssetName  string `json:"asset_name,omitempty"`
	AssetID    string `json:"asset_id,omitempty"`
	RequestID  string `json:"request_id,omitempty"`
}

// SecurityEventCallback is a function to notify external systems of security events.
type SecurityEventCallback func(event SecurityEvent)

// SecurityEventBuffer is an in-memory buffer for security events.
type SecurityEventBuffer struct {
	mu       sync.Mutex
	events   []SecurityEvent
	maxLen   int
	callback SecurityEventCallback
	exportCb SecurityEventCallback
}

var (
	securityEventBuffer = &SecurityEventBuffer{
		events: make([]SecurityEvent, 0),
		maxLen: 1000,
	}
	securityEventIDCounter int64
	securityEventMu        sync.Mutex
)

// GetSecurityEventBuffer returns the global security event buffer.
func GetSecurityEventBuffer() *SecurityEventBuffer {
	return securityEventBuffer
}

// SetCallback sets the external callback for security events (e.g. Bridge push to Flutter).
func (b *SecurityEventBuffer) SetCallback(cb SecurityEventCallback) {
	b.mu.Lock()
	b.callback = cb
	b.mu.Unlock()
}

// SetExportCallback sets callback for export pipeline writes.
func (b *SecurityEventBuffer) SetExportCallback(cb SecurityEventCallback) {
	b.mu.Lock()
	b.exportCb = cb
	b.mu.Unlock()
}

// generateSecurityEventID generates a unique security event ID.
func generateSecurityEventID() string {
	securityEventMu.Lock()
	defer securityEventMu.Unlock()
	securityEventIDCounter++
	return fmt.Sprintf("sevt_%d_%d", time.Now().UnixNano(), securityEventIDCounter)
}

// AddSecurityEvent appends a security event to the buffer, persists to SQLite and pushes via callback.
func (b *SecurityEventBuffer) AddSecurityEvent(event SecurityEvent) {
	b.mu.Lock()
	if event.ID == "" {
		event.ID = generateSecurityEventID()
	}
	if event.Timestamp == "" {
		event.Timestamp = time.Now().Format(time.RFC3339)
	}
	b.events = append(b.events, event)
	if len(b.events) > b.maxLen {
		b.events = b.events[len(b.events)-b.maxLen:]
	}
	cb := b.callback
	exportCb := b.exportCb
	b.mu.Unlock()

	// Persist to SQLite
	record := &repository.SecurityEventRecord{
		ID:         event.ID,
		Timestamp:  event.Timestamp,
		EventType:  event.EventType,
		ActionDesc: event.ActionDesc,
		RiskType:   event.RiskType,
		Detail:     event.Detail,
		Source:     event.Source,
		AssetName:  event.AssetName,
		AssetID:    event.AssetID,
		RequestID:  event.RequestID,
	}
	repo := repository.NewSecurityEventRepository(nil)
	if err := repo.SaveSecurityEventsBatch([]*repository.SecurityEventRecord{record}); err != nil {
		logging.Warning("[SecurityEvent] Failed to persist event %s: %v", event.ID, err)
	}

	// Push via callback
	if cb != nil {
		cb(event)
	}
	if exportCb != nil {
		exportCb(event)
	}
}

// GetAndClearSecurityEvents returns all events and clears the buffer.
func (b *SecurityEventBuffer) GetAndClearSecurityEvents() []SecurityEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	events := b.events
	b.events = make([]SecurityEvent, 0)
	return events
}

// GetSecurityEventCount returns the number of events in the buffer.
func (b *SecurityEventBuffer) GetSecurityEventCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.events)
}

// ==================== FFI wrapper functions ====================

// GetPendingSecurityEventsInternal gets and clears pending security events.
func GetPendingSecurityEventsInternal() string {
	events := securityEventBuffer.GetAndClearSecurityEvents()
	jsonBytes, err := json.Marshal(events)
	if err != nil {
		return "[]"
	}
	return string(jsonBytes)
}

// ClearSecurityEventsBufferInternal clears the security events buffer.
func ClearSecurityEventsBufferInternal() string {
	securityEventBuffer.mu.Lock()
	securityEventBuffer.events = make([]SecurityEvent, 0)
	securityEventBuffer.mu.Unlock()
	return `{"success":true}`
}
