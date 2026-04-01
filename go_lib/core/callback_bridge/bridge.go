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

// publishFlushTickerInterval 周期刷新 log/metrics 尾部积压与 TruthRecord 合并快照(Dart FFI).
const publishFlushTickerInterval = 200 * time.Millisecond

func (b *Bridge) publishWorker() {
	defer b.wg.Done()

	ticker := time.NewTicker(publishFlushTickerInterval)
	defer ticker.Stop()

	var logBatch []string
	var metricsBatch []map[string]interface{}
	// 同一周期内同一 request_id 只保留最新快照, 避免流式更新对 Dart 连发数十次 FFI(Windows UI 易未响应).
	truthPending := make(map[string]map[string]interface{})

	for {
		select {
		case <-b.ctx.Done():
			for {
				select {
				case record := <-b.truthRecordChan:
					if rid := truthRecordRequestID(record); rid != "" {
						truthPending[rid] = record
					} else {
						b.flushTruthRecords([]map[string]interface{}{record})
					}
				default:
					b.flushLogs(logBatch)
					b.flushMetrics(metricsBatch)
					b.drainSecurityEvents()
					b.flushTruthRecordsCoalesced(truthPending)
					return
				}
			}

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
			rid := truthRecordRequestID(record)
			if rid != "" {
				truthPending[rid] = record
				// 终态立即推送, 避免再等 ticker; 并从 pending 移除以免重复发送.
				if ph, _ := record["phase"].(string); ph == "completed" || ph == "stopped" {
					b.flushTruthRecords([]map[string]interface{}{record})
					delete(truthPending, rid)
				}
			} else {
				b.flushTruthRecords([]map[string]interface{}{record})
			}

		case <-ticker.C:
			if len(logBatch) > 0 {
				b.flushLogs(logBatch)
				logBatch = logBatch[:0]
			}
			if len(metricsBatch) > 0 {
				b.flushMetrics(metricsBatch)
				metricsBatch = metricsBatch[:0]
			}
			if len(truthPending) > 0 {
				b.flushTruthRecordsCoalesced(truthPending)
				truthPending = make(map[string]map[string]interface{})
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

// truthRecordRequestID 从快照 map 中取 request_id, 用于合并同一请求的突发更新.
func truthRecordRequestID(record map[string]interface{}) string {
	if v, ok := record["request_id"].(string); ok {
		return v
	}
	return ""
}

// flushTruthRecordsCoalesced 将 pending 中每个 request_id 的最新一条推入 Dart(通常每 ticker 周期一次).
func (b *Bridge) flushTruthRecordsCoalesced(pending map[string]map[string]interface{}) {
	if len(pending) == 0 {
		return
	}
	for _, record := range pending {
		b.flushTruthRecords([]map[string]interface{}{record})
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
