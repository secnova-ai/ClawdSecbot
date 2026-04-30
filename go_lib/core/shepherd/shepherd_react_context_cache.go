package shepherd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	guardContextInlineCharLimit = 6000
	guardContextPreviewHalf     = 2400
	guardContextToolMaxChars    = 50000
)

type guardContextCache struct {
	items []guardContextItem
	byID  map[string]guardContextItem
}

type guardContextItem struct {
	ID            string `json:"context_id"`
	Kind          string `json:"kind"`
	OwnerID       string `json:"owner_id,omitempty"`
	Field         string `json:"field"`
	OriginalChars int    `json:"original_chars"`
	Content       string `json:"-"`
}

func newGuardContextCache() *guardContextCache {
	return &guardContextCache{
		byID: make(map[string]guardContextItem),
	}
}

func (c *guardContextCache) HasItems() bool {
	return c != nil && len(c.items) > 0
}

func (c *guardContextCache) Summaries() []guardContextItem {
	if c == nil || len(c.items) == 0 {
		return nil
	}
	out := make([]guardContextItem, len(c.items))
	copy(out, c.items)
	return out
}

func (c *guardContextCache) Lookup(contextID string) (guardContextItem, bool) {
	if c == nil {
		return guardContextItem{}, false
	}
	item, ok := c.byID[strings.TrimSpace(contextID)]
	return item, ok
}

func (c *guardContextCache) TruncateString(kind, ownerID, field, content string) interface{} {
	if utf8.RuneCountInString(content) <= guardContextInlineCharLimit {
		return content
	}
	item := c.add(kind, ownerID, field, content)
	return map[string]interface{}{
		"truncated":      true,
		"context_id":     item.ID,
		"kind":           item.Kind,
		"field":          item.Field,
		"owner_id":       item.OwnerID,
		"original_chars": item.OriginalChars,
		"preview":        previewHeadTail(content, guardContextPreviewHalf),
		"instruction":    "Call get_full_guard_context with this context_id to inspect omitted content when needed for security classification.",
	}
}

func (c *guardContextCache) TruncateJSONValue(kind, ownerID, field string, value interface{}) interface{} {
	if value == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return value
	}
	if utf8.RuneCount(raw) <= guardContextInlineCharLimit {
		return value
	}
	item := c.add(kind, ownerID, field, string(raw))
	return map[string]interface{}{
		"truncated":      true,
		"context_id":     item.ID,
		"kind":           item.Kind,
		"field":          item.Field,
		"owner_id":       item.OwnerID,
		"original_chars": item.OriginalChars,
		"preview_json":   previewHeadTail(string(raw), guardContextPreviewHalf),
		"instruction":    "Call get_full_guard_context with this context_id to inspect omitted JSON when needed for security classification.",
	}
}

func (c *guardContextCache) add(kind, ownerID, field, content string) guardContextItem {
	if c == nil {
		return guardContextItem{}
	}
	sum := sha256.Sum256([]byte(kind + "\x00" + ownerID + "\x00" + field + "\x00" + content))
	id := fmt.Sprintf("guardctx_%s", hex.EncodeToString(sum[:8]))
	if item, ok := c.byID[id]; ok {
		return item
	}
	item := guardContextItem{
		ID:            id,
		Kind:          kind,
		OwnerID:       ownerID,
		Field:         field,
		OriginalChars: utf8.RuneCountInString(content),
		Content:       content,
	}
	c.items = append(c.items, item)
	c.byID[id] = item
	return item
}

func previewHeadTail(content string, half int) string {
	runes := []rune(content)
	if len(runes) <= half*2 {
		return content
	}
	head := string(runes[:half])
	tail := string(runes[len(runes)-half:])
	return head + "\n...[TRUNCATED_MIDDLE]...\n" + tail
}
