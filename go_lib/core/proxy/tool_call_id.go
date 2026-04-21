package proxy

import (
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/openai/openai-go"
)

var proxyToolCallIDCounter uint64

func nextProxyToolCallID(existing map[string]struct{}) string {
	for {
		n := atomic.AddUint64(&proxyToolCallIDCounter, 1)
		candidate := "tc" + strconv.FormatUint(n, 36)
		if existing == nil {
			return candidate
		}
		if _, ok := existing[candidate]; ok {
			continue
		}
		return candidate
	}
}

func ensureResponseToolCallIDs(toolCalls []openai.ChatCompletionMessageToolCall) bool {
	if len(toolCalls) == 0 {
		return false
	}

	existing := make(map[string]struct{}, len(toolCalls))
	for _, tc := range toolCalls {
		id := strings.TrimSpace(tc.ID)
		if id == "" {
			continue
		}
		existing[id] = struct{}{}
	}

	changed := false
	for i := range toolCalls {
		if strings.TrimSpace(toolCalls[i].ID) != "" {
			continue
		}
		newID := nextProxyToolCallID(existing)
		toolCalls[i].ID = newID
		existing[newID] = struct{}{}
		changed = true
	}
	return changed
}
