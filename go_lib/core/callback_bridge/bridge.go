package callback_bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go_lib/core/logging"
)

type MessageType string

const (
	MessageTypeLog           MessageType = "log"
	MessageTypeMetrics       MessageType = "metrics"
	MessageTypeStatus        MessageType = "status"
	MessageTypeVersionUpdate MessageType = "version_update"
	MessageTypeSecurityEvent MessageType = "security_event"
	MessageTypeTruthRecord   MessageType = "truth_record"
)

type Message struct {
	Type      MessageType            `json:"type"`
	Timestamp int64                  `json:"timestamp"`
	Payload   map[string]interface{} `json:"payload"`
}

type CallbackFunc func(message string)

type Bridge struct {
	callback CallbackFunc

	logChan           chan string
	metricsChan       chan map[string]interface{}
	securityEventChan chan map[string]interface{}
	truthRecordChan   chan map[string]interface{}

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	running bool
	mu      sync.Mutex
}

func NewBridge(callback CallbackFunc) (*Bridge, error) {
	if callback == nil {
		return nil, fmt.Errorf("callback function is required")
	}

	ctx, cancel := context.WithCancel(context.Background())
	bridge := &Bridge{
		callback:          callback,
		logChan:           make(chan string, 1000),
		metricsChan:       make(chan map[string]interface{}, 100),
		securityEventChan: make(chan map[string]interface{}, 100),
		truthRecordChan:   make(chan map[string]interface{}, 200),
		ctx:               ctx,
		cancel:            cancel,
		running:           true,
	}

	bridge.wg.Add(1)
	go bridge.publishWorker()

	logging.Info("[CallbackBridge] Callback bridge initialized")
	return bridge, nil
}

func (b *Bridge) publishWorker() {
	defer b.wg.Done()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	var logBatch []string
	var metricsBatch []map[string]interface{}

	for {
		select {
		case <-b.ctx.Done():
			b.flushLogs(logBatch)
			b.flushMetrics(metricsBatch)
			b.drainSecurityEvents()
			b.drainTruthRecords()
			return

		case log := <-b.logChan:
			logBatch = append(logBatch, log)
			if len(logBatch) >= 5 {
				b.flushLogs(logBatch)
				logBatch = logBatch[:0]
			}

		case metrics := <-b.metricsChan:
			metricsBatch = append(metricsBatch, metrics)
			if len(metricsBatch) >= 5 {
				b.flushMetrics(metricsBatch)
				metricsBatch = metricsBatch[:0]
			}

		case event := <-b.securityEventChan:
			b.flushSecurityEvents([]map[string]interface{}{event})

		case record := <-b.truthRecordChan:
			b.flushTruthRecords([]map[string]interface{}{record})

		case <-ticker.C:
			if len(logBatch) > 0 {
				b.flushLogs(logBatch)
				logBatch = logBatch[:0]
			}
			if len(metricsBatch) > 0 {
				b.flushMetrics(metricsBatch)
				metricsBatch = metricsBatch[:0]
			}
		}
	}
}

func (b *Bridge) invokeCallback(data []byte) error {
	b.mu.Lock()
	callback := b.callback
	running := b.running
	b.mu.Unlock()

	if !running || callback == nil {
		return fmt.Errorf("bridge not running")
	}
	if len(data) == 0 {
		return fmt.Errorf("empty data")
	}
	if data[0] != '{' || data[len(data)-1] != '}' {
		return fmt.Errorf("invalid JSON format")
	}

	defer func() {
		if r := recover(); r != nil {
			logging.Error("[CallbackBridge] Recovered from callback panic: %v", r)
		}
	}()

	callback(string(data))
	return nil
}

func (b *Bridge) flushLogs(logs []string) {
	for _, log := range logs {
		msg := b.newLogMessage(log)
		if data, err := json.Marshal(msg); err == nil {
			_ = b.invokeCallback(data)
		}
	}
}

func (b *Bridge) flushMetrics(metricsList []map[string]interface{}) {
	for _, metrics := range metricsList {
		msg := b.newMetricsMessage(metrics)
		if data, err := json.Marshal(msg); err == nil {
			_ = b.invokeCallback(data)
		}
	}
}

func (b *Bridge) flushSecurityEvents(events []map[string]interface{}) {
	for _, event := range events {
		msg := &Message{
			Type:      MessageTypeSecurityEvent,
			Timestamp: time.Now().UnixMilli(),
			Payload:   event,
		}
		if data, err := json.Marshal(msg); err == nil {
			_ = b.invokeCallback(data)
		}
	}
}

func (b *Bridge) flushTruthRecords(records []map[string]interface{}) {
	for _, record := range records {
		msg := &Message{
			Type:      MessageTypeTruthRecord,
			Timestamp: time.Now().UnixMilli(),
			Payload:   record,
		}
		if data, err := json.Marshal(msg); err == nil {
			_ = b.invokeCallback(data)
		}
	}
}

func (b *Bridge) drainSecurityEvents() {
	for {
		select {
		case event := <-b.securityEventChan:
			b.flushSecurityEvents([]map[string]interface{}{event})
		default:
			return
		}
	}
}

func (b *Bridge) drainTruthRecords() {
	for {
		select {
		case record := <-b.truthRecordChan:
			b.flushTruthRecords([]map[string]interface{}{record})
		default:
			return
		}
	}
}

func (b *Bridge) newLogMessage(jsonStr string) *Message {
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &payload); err != nil {
		payload = map[string]interface{}{"message": jsonStr}
	}
	return &Message{
		Type:      MessageTypeLog,
		Timestamp: time.Now().UnixMilli(),
		Payload:   payload,
	}
}

func (b *Bridge) newMetricsMessage(metrics map[string]interface{}) *Message {
	return &Message{
		Type:      MessageTypeMetrics,
		Timestamp: time.Now().UnixMilli(),
		Payload:   metrics,
	}
}

func (b *Bridge) newStatusMessage(status map[string]interface{}) *Message {
	return &Message{
		Type:      MessageTypeStatus,
		Timestamp: time.Now().UnixMilli(),
		Payload:   status,
	}
}

func (b *Bridge) SendLog(log string) {
	select {
	case b.logChan <- log:
	default:
		select {
		case <-b.logChan:
		default:
		}
		select {
		case b.logChan <- log:
		default:
		}
	}
}

func (b *Bridge) SendMetrics(metrics map[string]interface{}) {
	select {
	case b.metricsChan <- metrics:
	default:
		select {
		case <-b.metricsChan:
		default:
		}
		select {
		case b.metricsChan <- metrics:
		default:
		}
	}
}

func (b *Bridge) SendStatus(status map[string]interface{}) {
	msg := b.newStatusMessage(status)
	if data, err := json.Marshal(msg); err == nil {
		_ = b.invokeCallback(data)
	}
}

func (b *Bridge) SendVersionUpdate(versionInfo map[string]interface{}) {
	msg := &Message{
		Type:      MessageTypeVersionUpdate,
		Timestamp: time.Now().UnixMilli(),
		Payload:   versionInfo,
	}
	if data, err := json.Marshal(msg); err == nil {
		_ = b.invokeCallback(data)
	}
}

func (b *Bridge) SendSecurityEvent(event map[string]interface{}) {
	select {
	case b.securityEventChan <- event:
	default:
	}
}

func (b *Bridge) SendTruthRecord(record map[string]interface{}) {
	select {
	case b.truthRecordChan <- record:
	default:
		select {
		case <-b.truthRecordChan:
		default:
		}
		select {
		case b.truthRecordChan <- record:
		default:
		}
	}
}

func (b *Bridge) IsRunning() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.running
}

func (b *Bridge) Close() error {
	b.mu.Lock()
	if !b.running {
		b.mu.Unlock()
		return nil
	}
	b.running = false
	b.mu.Unlock()

	b.cancel()
	b.wg.Wait()

	logging.Info("[CallbackBridge] Callback bridge closed")
	return nil
}
