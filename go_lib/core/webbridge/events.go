package webbridge

import (
	"fmt"
	"net/http"
	"sync"
	"time"
)

// EventHub fans out callback_bridge messages to SSE clients.
type EventHub struct {
	mu      sync.RWMutex
	clients map[chan string]struct{}
}

func NewEventHub() *EventHub {
	return &EventHub{clients: make(map[chan string]struct{})}
}

func (h *EventHub) Publish(message string) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for ch := range h.clients {
		select {
		case ch <- message:
		default:
			// Drop oldest message for this client to avoid global blocking.
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- message:
			default:
			}
		}
	}
}

func (h *EventHub) subscribe() (chan string, func()) {
	ch := make(chan string, 128)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()

	unsubscribe := func() {
		h.mu.Lock()
		if _, ok := h.clients[ch]; ok {
			delete(h.clients, ch)
			close(ch)
		}
		h.mu.Unlock()
	}
	return ch, unsubscribe
}

func (h *EventHub) ServeSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch, unsubscribe := h.subscribe()
	defer unsubscribe()

	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()
		}
	}
}
