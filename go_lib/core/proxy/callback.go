package proxy

import (
	"sync"

	"go_lib/core/callback_bridge"
)

// Global callback bridge for proxy
var globalCallbackBridge *callback_bridge.Bridge
var callbackBridgeMu sync.Mutex

// SetCallbackBridge sets the global callback bridge
func SetCallbackBridge(bridge *callback_bridge.Bridge) {
	callbackBridgeMu.Lock()
	defer callbackBridgeMu.Unlock()
	globalCallbackBridge = bridge
}

func getCallbackBridge() *callback_bridge.Bridge {
	callbackBridgeMu.Lock()
	defer callbackBridgeMu.Unlock()
	return globalCallbackBridge
}

func sendToCallback(message string) {
	bridge := getCallbackBridge()
	if bridge != nil && bridge.IsRunning() {
		bridge.SendLog(message)
	}
}

func sendMetricsToCallback(metrics map[string]interface{}) {
	bridge := getCallbackBridge()
	if bridge != nil && bridge.IsRunning() {
		bridge.SendMetrics(metrics)
	}
}

func sendTruthRecordToCallback(record map[string]interface{}) {
	bridge := getCallbackBridge()
	if bridge != nil && bridge.IsRunning() {
		bridge.SendTruthRecord(record)
	}
}
