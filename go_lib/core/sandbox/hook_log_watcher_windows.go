//go:build windows

package sandbox

import (
	"bufio"
	"os"
	"strings"
	"sync"
	"time"

	"go_lib/core/logging"
)

// HookLogEvent represents a parsed event from the hook DLL log file
type HookLogEvent struct {
	Timestamp string
	Action    string // BLOCK/LOG_ONLY or legacy BLOCK_FILE/BLOCK_CMD/BLOCK_NET/...
	Type      string // PATH-READ/CMD/NET/DNS... (new format), optional for legacy lines
	Target    string // target path/domain/ip (new format), optional for legacy lines
	Detail    string // legacy detail or fallback detail
}

// HookLogCallback is called when new hook log events are detected
type HookLogCallback func(event HookLogEvent)

// HookLogWatcher monitors the hook DLL's audit log file and emits events
type HookLogWatcher struct {
	mu       sync.Mutex
	logPath  string
	callback HookLogCallback
	stopCh   chan struct{}
	running  bool
	offset   int64
}

// NewHookLogWatcher creates a watcher for the specified log file
func NewHookLogWatcher(logPath string, cb HookLogCallback) *HookLogWatcher {
	return &HookLogWatcher{
		logPath:  logPath,
		callback: cb,
		stopCh:   make(chan struct{}),
	}
}

// Start begins watching the log file
func (w *HookLogWatcher) Start() {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return
	}
	w.running = true
	w.stopCh = make(chan struct{})
	w.mu.Unlock()

	go w.watchLoop()
}

// Stop stops the watcher
func (w *HookLogWatcher) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.running {
		return
	}
	close(w.stopCh)
	w.running = false
}

func (w *HookLogWatcher) watchLoop() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			w.readNewLines()
		case <-w.stopCh:
			return
		}
	}
}

func (w *HookLogWatcher) readNewLines() {
	f, err := os.Open(w.logPath)
	if err != nil {
		return
	}
	defer f.Close()

	if w.offset > 0 {
		if _, err := f.Seek(w.offset, 0); err != nil {
			w.offset = 0
			return
		}
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if event, ok := parseHookLogLine(line); ok {
			if shouldEmitHookSecurityEvent(event) && w.callback != nil {
				w.callback(event)
			}
		}
	}

	newOffset, _ := f.Seek(0, 1)
	w.offset = newOffset
}

// shouldEmitHookSecurityEvent decides whether a hook log event should be promoted
// into the unified security event pipeline.
// We keep sandbox lifecycle noise in hook log files, but not in UI/database events.
func shouldEmitHookSecurityEvent(event HookLogEvent) bool {
	if event.Action == "BLOCK" || event.Action == "LOG_ONLY" {
		return true
	}
	switch event.Action {
	case "INIT", "CLEANUP", "INJECT":
		return false
	default:
		return true
	}
}

// parseHookLogLine parses two formats:
// 1) New: [2026-03-18 12:00:00] ACTION=BLOCK TYPE=PATH-READ TARGET=C:\Users\a.txt
// 2) Legacy: [2026-03-18 12:00:00] BLOCK_FILE: C:\Users\secret\data.txt
func parseHookLogLine(line string) (HookLogEvent, bool) {
	line = strings.TrimSpace(line)
	if len(line) < 22 || line[0] != '[' {
		return HookLogEvent{}, false
	}

	closeBracket := strings.Index(line, "]")
	if closeBracket < 0 {
		return HookLogEvent{}, false
	}

	timestamp := line[1:closeBracket]
	rest := strings.TrimSpace(line[closeBracket+1:])

	if strings.Contains(rest, "ACTION=") {
		action := extractKV(rest, "ACTION")
		eventType := extractKV(rest, "TYPE")
		target := extractKV(rest, "TARGET")
		if action == "" {
			return HookLogEvent{}, false
		}
		return HookLogEvent{
			Timestamp: timestamp,
			Action:    action,
			Type:      eventType,
			Target:    target,
			Detail:    strings.TrimSpace(rest),
		}, true
	}

	colonIdx := strings.Index(rest, ":")
	if colonIdx < 0 {
		return HookLogEvent{}, false
	}

	action := strings.TrimSpace(rest[:colonIdx])
	detail := strings.TrimSpace(rest[colonIdx+1:])

	return HookLogEvent{
		Timestamp: timestamp,
		Action:    action,
		Type:      "",
		Target:    "",
		Detail:    detail,
	}, true
}

func extractKV(s, key string) string {
	prefix := key + "="
	start := strings.Index(s, prefix)
	if start < 0 {
		return ""
	}
	start += len(prefix)
	rest := s[start:]

	nextKeys := []string{" ACTION=", " TYPE=", " TARGET="}
	end := len(rest)
	for _, marker := range nextKeys {
		if idx := strings.Index(rest, marker); idx >= 0 && idx < end {
			end = idx
		}
	}
	return strings.TrimSpace(rest[:end])
}

// MapHookEventToSecurityEvent maps a hook log event to the standard event type/risk classification
func MapHookEventToSecurityEvent(event HookLogEvent) (eventType, actionDesc, riskType, source string) {
	source = "sandbox_hook"

	switch {
	case event.Action == "BLOCK":
		eventType = "blocked"
		riskType = hookEventToRiskType(event.Type, event.Action)
		if event.Target != "" {
			actionDesc = "Sandbox blocked: " + event.Type + " " + event.Target
		} else {
			actionDesc = "Sandbox blocked: " + event.Detail
		}
	case event.Action == "LOG_ONLY":
		eventType = "tool_execution"
		riskType = hookEventToRiskType(event.Type, event.Action)
		if event.Target != "" {
			actionDesc = "Sandbox logged: " + event.Type + " " + event.Target
		} else {
			actionDesc = "Sandbox logged: " + event.Detail
		}
	case strings.HasPrefix(event.Action, "BLOCK"):
		eventType = "blocked"
		riskType = hookActionToRiskType(event.Action)
		actionDesc = "Sandbox blocked: " + event.Detail
	case strings.HasPrefix(event.Action, "LOG"):
		eventType = "tool_execution"
		riskType = hookActionToRiskType(event.Action)
		actionDesc = "Sandbox logged: " + event.Detail
	case event.Action == "INJECT":
		eventType = "other"
		riskType = ""
		actionDesc = "Sandbox injected into child: " + event.Detail
	default:
		eventType = "other"
		riskType = ""
		actionDesc = event.Action + ": " + event.Detail
	}
	return
}

func hookEventToRiskType(eventType string, action string) string {
	t := strings.ToUpper(eventType)
	switch {
	case strings.HasPrefix(t, "PATH-"):
		return "unauthorized_file_access"
	case t == "CMD":
		return "unauthorized_command"
	case strings.HasPrefix(t, "NET"), t == "DNS":
		return "unauthorized_network"
	default:
		if action == "BLOCK" || action == "LOG_ONLY" {
			return "unknown"
		}
		return "unknown"
	}
}

func hookActionToRiskType(action string) string {
	switch action {
	case "BLOCK_FILE", "LOG_FILE":
		return "unauthorized_file_access"
	case "BLOCK_CMD", "LOG_CMD":
		return "unauthorized_command"
	case "BLOCK_NET", "LOG_NET":
		return "unauthorized_network"
	default:
		return "unknown"
	}
}

// Global watcher registry
var (
	hookWatchers   = make(map[string]*HookLogWatcher)
	hookWatchersMu sync.Mutex
)

// StartHookLogWatcher starts a log watcher for an asset's hook log
func StartHookLogWatcher(assetName, logPath string, cb HookLogCallback) {
	StartHookLogWatcherByKey(assetName, logPath, cb)
}

// StartHookLogWatcherByKey starts a log watcher with an explicit instance key.
func StartHookLogWatcherByKey(assetKey, logPath string, cb HookLogCallback) {
	hookWatchersMu.Lock()
	defer hookWatchersMu.Unlock()

	if old, exists := hookWatchers[assetKey]; exists {
		old.Stop()
	}

	w := NewHookLogWatcher(logPath, cb)
	hookWatchers[assetKey] = w
	w.Start()
	logging.Info("[Sandbox] Started hook log watcher for %s: %s", assetKey, logPath)
}

// StopHookLogWatcher stops the log watcher for an asset
func StopHookLogWatcher(assetName string) {
	StopHookLogWatcherByKey(assetName)
}

// StopHookLogWatcherByKey stops the log watcher for an explicit instance key.
func StopHookLogWatcherByKey(assetKey string) {
	hookWatchersMu.Lock()
	defer hookWatchersMu.Unlock()

	if w, exists := hookWatchers[assetKey]; exists {
		w.Stop()
		delete(hookWatchers, assetKey)
	}
}

// StopAllHookLogWatchers stops all active watchers
func StopAllHookLogWatchers() {
	hookWatchersMu.Lock()
	defer hookWatchersMu.Unlock()

	for _, w := range hookWatchers {
		w.Stop()
	}
	hookWatchers = make(map[string]*HookLogWatcher)
}
