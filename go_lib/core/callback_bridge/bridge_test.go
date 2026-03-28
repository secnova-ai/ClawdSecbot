package callback_bridge

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// newTestBridge 创建测试用 Bridge，使用自定义回调收集消息，绕过 logging 依赖。
func newTestBridge(callback CallbackFunc) *Bridge {
	ctx, cancel := context.WithCancel(context.Background())
	b := &Bridge{
		callback:          callback,
		logChan:           make(chan string, 1000),
		metricsChan:       make(chan map[string]interface{}, 100),
		securityEventChan: make(chan map[string]interface{}, 100),
		truthRecordChan:   make(chan map[string]interface{}, 100),
		ctx:               ctx,
		cancel:            cancel,
		running:           true,
	}
	b.wg.Add(1)
	go b.publishWorker()
	return b
}

// TestSendSecurityEventDelivery 验证安全事件通过 channel 能正确投递到 callback
func TestSendSecurityEventDelivery(t *testing.T) {
	var mu sync.Mutex
	var received []Message

	bridge := newTestBridge(func(msg string) {
		var m Message
		if err := json.Unmarshal([]byte(msg), &m); err == nil {
			mu.Lock()
			received = append(received, m)
			mu.Unlock()
		}
	})
	defer bridge.Close()

	// 发送安全事件
	bridge.SendSecurityEvent(map[string]interface{}{
		"id":          "sevt_test_1",
		"event_type":  "blocked",
		"action_desc": "test action",
		"risk_type":   "high",
		"source":      "react_agent",
	})

	// 等待 publishWorker 处理（最多 200ms）
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(received) != 1 {
		t.Fatalf("expected 1 message, got %d", len(received))
	}
	if received[0].Type != MessageTypeSecurityEvent {
		t.Errorf("expected type %q, got %q", MessageTypeSecurityEvent, received[0].Type)
	}
	if received[0].Payload["id"] != "sevt_test_1" {
		t.Errorf("expected id 'sevt_test_1', got %v", received[0].Payload["id"])
	}
	if received[0].Payload["event_type"] != "blocked" {
		t.Errorf("expected event_type 'blocked', got %v", received[0].Payload["event_type"])
	}
}

// TestConcurrentSendAll 并发发送 log + metrics + security event，验证无 panic、消息完整
func TestConcurrentSendAll(t *testing.T) {
	var logCount, metricsCount, eventCount atomic.Int64

	bridge := newTestBridge(func(msg string) {
		var m Message
		if err := json.Unmarshal([]byte(msg), &m); err != nil {
			return
		}
		switch m.Type {
		case MessageTypeLog:
			logCount.Add(1)
		case MessageTypeMetrics:
			metricsCount.Add(1)
		case MessageTypeSecurityEvent:
			eventCount.Add(1)
		}
	})
	defer bridge.Close()

	const n = 50
	var wg sync.WaitGroup
	wg.Add(3)

	// 并发发送日志
	go func() {
		defer wg.Done()
		for i := 0; i < n; i++ {
			bridge.SendLog(`{"message":"log"}`)
		}
	}()

	// 并发发送指标
	go func() {
		defer wg.Done()
		for i := 0; i < n; i++ {
			bridge.SendMetrics(map[string]interface{}{"total_tokens": i})
		}
	}()

	// 并发发送安全事件
	go func() {
		defer wg.Done()
		for i := 0; i < n; i++ {
			bridge.SendSecurityEvent(map[string]interface{}{"id": i})
		}
	}()

	wg.Wait()
	// 等待 publishWorker 处理完所有消息
	time.Sleep(300 * time.Millisecond)

	lc := logCount.Load()
	mc := metricsCount.Load()
	ec := eventCount.Load()

	if lc != int64(n) {
		t.Errorf("expected %d logs, got %d", n, lc)
	}
	if mc != int64(n) {
		t.Errorf("expected %d metrics, got %d", n, mc)
	}
	if ec != int64(n) {
		t.Errorf("expected %d security events, got %d", n, ec)
	}
}

// TestSecurityEventChannelFull 验证 channel 满时不会阻塞
func TestSecurityEventChannelFull(t *testing.T) {
	// 创建一个慢回调，让 channel 积压
	bridge := newTestBridge(func(msg string) {
		time.Sleep(10 * time.Millisecond)
	})
	defer bridge.Close()

	done := make(chan struct{})
	go func() {
		// 发送超过 channel 容量的事件（100 + 50 = 150）
		for i := 0; i < 150; i++ {
			bridge.SendSecurityEvent(map[string]interface{}{"id": i})
		}
		close(done)
	}()

	select {
	case <-done:
		// 未阻塞
	case <-time.After(2 * time.Second):
		t.Fatal("SendSecurityEvent blocked when channel is full")
	}
}

func TestSendTruthRecordDelivery(t *testing.T) {
	var mu sync.Mutex
	var received []Message

	bridge := newTestBridge(func(msg string) {
		var m Message
		if err := json.Unmarshal([]byte(msg), &m); err == nil {
			mu.Lock()
			received = append(received, m)
			mu.Unlock()
		}
	})
	defer bridge.Close()

	bridge.SendTruthRecord(map[string]interface{}{
		"request_id": "req_test_1",
		"phase":      "completed",
		"asset_id":   "asset-1",
	})

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(received) != 1 {
		t.Fatalf("expected 1 message, got %d", len(received))
	}
	if received[0].Type != MessageTypeTruthRecord {
		t.Fatalf("expected type %q, got %q", MessageTypeTruthRecord, received[0].Type)
	}
	if received[0].Payload["request_id"] != "req_test_1" {
		t.Fatalf("unexpected request id: %v", received[0].Payload["request_id"])
	}
}
