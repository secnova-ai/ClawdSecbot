package main

/*
#include <stdlib.h>

// DartCallback Dart 消息回调函数类型
typedef void (*DartCallback)(const char* message);

// 存储 Dart 回调指针
static DartCallback dartCallback = NULL;

// setDartCallback 设置 Dart 回调
static inline void setDartCallback(DartCallback cb) {
    dartCallback = cb;
}

// clearDartCallback 清除 Dart 回调
static inline void clearDartCallback() {
    dartCallback = NULL;
}

// isDartCallbackSet 检查回调是否已设置
static inline int isDartCallbackSet() {
    return dartCallback != NULL ? 1 : 0;
}

// invokeDartCallback 调用 Dart 回调
static inline void invokeDartCallback(const char* msg) {
    if (dartCallback != NULL) {
        dartCallback(msg);
    }
}
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/pprof"
	"os"
	"sync"
	"unsafe"

	"go_lib/chatmodel-routing/adapter"
	"go_lib/core"
	"go_lib/core/callback_bridge"
	"go_lib/core/proxy"
	"go_lib/core/sandbox"
	"go_lib/core/service"
	"go_lib/core/shepherd"

	// Import all plugins to trigger init() registration
	_ "go_lib/plugins/nullclaw"
	_ "go_lib/plugins/openclaw"
)

func init() {
	resetSignals()
	startPprofIfNeeded()
}

func startPprofIfNeeded() {
	pprofPort := os.Getenv("BOTSEC_PPROF_PORT")
	if pprofPort == "" {
		return
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	addr := "127.0.0.1:" + pprofPort
	fmt.Printf("[pprof] profiling server: http://%s/debug/pprof/\n", addr)

	go func() {
		if err := http.ListenAndServe(addr, mux); err != nil {
			fmt.Fprintf(os.Stderr, "[pprof] server failed: %v\n", err)
		}
	}()
}

// ==================== 辅助函数 ====================

func jsonToCString(v interface{}) *C.char {
	b, err := json.Marshal(v)
	if err != nil {
		return C.CString(`{"success":false,"error":"marshal error"}`)
	}
	return C.CString(string(b))
}

func errorCString(err error) *C.char {
	return jsonToCString(map[string]interface{}{
		"success": false,
		"error":   err.Error(),
	})
}

// ==================== 全局初始化 FFI ====================

//export InitPathsFFI
func InitPathsFFI(workspaceDirC, homeDirC *C.char) *C.char {
	workspaceDir := C.GoString(workspaceDirC)
	homeDir := C.GoString(homeDirC)

	result, err := core.Initialize(workspaceDir, homeDir)
	if err != nil {
		return errorCString(err)
	}
	return jsonToCString(result)
}

//export InitLoggingFFI
func InitLoggingFFI(logDirC *C.char) *C.char {
	logDir := C.GoString(logDirC)
	result, err := core.InitLogging(logDir)
	if err != nil {
		return errorCString(err)
	}
	return jsonToCString(result)
}

// ==================== 插件管理 FFI ====================

//export GetPluginsFFI
func GetPluginsFFI() *C.char {
	return jsonToCString(core.GetRegisteredPlugins())
}

// ==================== 资产扫描 FFI ====================

//export ScanAssetsFFI
func ScanAssetsFFI() *C.char {
	result, err := core.ScanAllAssets()
	if err != nil {
		return errorCString(err)
	}
	return jsonToCString(result)
}

// ==================== 风险评估 FFI ====================

//export AssessRisksFFI
func AssessRisksFFI(scannedHashesC *C.char) *C.char {
	result, err := core.AssessAllRisksFromString(C.GoString(scannedHashesC))
	if err != nil {
		return errorCString(err)
	}
	return jsonToCString(result)
}

// ==================== 风险缓解 FFI ====================

//export MitigateRiskFFI
func MitigateRiskFFI(riskInfoC *C.char) *C.char {
	result := core.MitigateRiskByPlugin(C.GoString(riskInfoC))
	return C.CString(result)
}

// ==================== 防护控制 FFI ====================

//export StartProtectionFFI
func StartProtectionFFI(assetNameC, assetIDC, configC *C.char) *C.char {
	err := core.StartProtectionByAsset(C.GoString(assetNameC), C.GoString(assetIDC), C.GoString(configC))
	if err != nil {
		return errorCString(err)
	}
	return jsonToCString(map[string]interface{}{"success": true})
}

//export StopProtectionFFI
func StopProtectionFFI(assetNameC, assetIDC *C.char) *C.char {
	err := core.StopProtectionByAsset(C.GoString(assetNameC), C.GoString(assetIDC))
	if err != nil {
		return errorCString(err)
	}
	return jsonToCString(map[string]interface{}{"success": true})
}

//export GetProtectionStatusFFI
func GetProtectionStatusFFI(assetNameC, assetIDC *C.char) *C.char {
	status, err := core.GetProtectionStatusByAsset(C.GoString(assetNameC), C.GoString(assetIDC))
	if err != nil {
		return errorCString(err)
	}
	return jsonToCString(core.SuccessResult(status))
}

//export GetAllProtectionStatusFFI
func GetAllProtectionStatusFFI() *C.char {
	return jsonToCString(core.SuccessResult(core.GetAllProtectionStatuses()))
}

// ==================== 数据库操作 FFI ====================

//export InitDatabase
func InitDatabase(dbPathC *C.char) *C.char {
	return jsonToCString(service.InitializeDatabase(C.GoString(dbPathC)))
}

//export CloseDatabase
func CloseDatabase() *C.char {
	return jsonToCString(service.CloseDatabase())
}

//export SaveScanResult
func SaveScanResult(resultJSONC *C.char) *C.char {
	return jsonToCString(service.SaveScanResult(C.GoString(resultJSONC)))
}

//export GetLatestScanResult
func GetLatestScanResult() *C.char {
	return jsonToCString(service.GetLatestScanResult())
}

//export GetScannedSkillHashes
func GetScannedSkillHashes() *C.char {
	return jsonToCString(service.GetScannedSkillHashes())
}

//export SaveSkillScanResult
func SaveSkillScanResult(jsonC *C.char) *C.char {
	return jsonToCString(service.SaveSkillScanResult(C.GoString(jsonC)))
}

//export GetSkillScanByHash
func GetSkillScanByHash(hashC *C.char) *C.char {
	return jsonToCString(service.GetSkillScanByHash(C.GoString(hashC)))
}

//export DeleteSkillScanFFI
func DeleteSkillScanFFI(skillNameC *C.char) *C.char {
	return jsonToCString(service.DeleteSkillScan(C.GoString(skillNameC)))
}

//export GetRiskySkills
func GetRiskySkills() *C.char {
	return jsonToCString(service.GetRiskySkills())
}

//export TrustSkillScan
func TrustSkillScan(skillNameC *C.char) *C.char {
	return jsonToCString(service.TrustSkill(C.GoString(skillNameC)))
}

//export GetAllSkillScansFFI
func GetAllSkillScansFFI() *C.char {
	return jsonToCString(service.GetAllSkillScans())
}

// ==================== 防护状态数据库 FFI ====================

//export SaveProtectionStateFFI
func SaveProtectionStateFFI(jsonC *C.char) *C.char {
	return jsonToCString(service.SaveProtectionState(C.GoString(jsonC)))
}

//export GetProtectionStateFFI
func GetProtectionStateFFI() *C.char {
	return jsonToCString(service.GetProtectionState())
}

//export ClearProtectionStateFFI
func ClearProtectionStateFFI() *C.char {
	return jsonToCString(service.ClearProtectionState())
}

//export SaveProtectionConfigFFI
func SaveProtectionConfigFFI(jsonC *C.char) *C.char {
	return jsonToCString(service.SaveProtectionConfig(C.GoString(jsonC)))
}

//export GetProtectionConfigFFI
func GetProtectionConfigFFI(assetNameC, assetIDC *C.char) *C.char {
	return jsonToCString(service.GetProtectionConfig(C.GoString(assetNameC), C.GoString(assetIDC)))
}

//export GetEnabledProtectionConfigsFFI
func GetEnabledProtectionConfigsFFI() *C.char {
	return jsonToCString(service.GetEnabledProtectionConfigs())
}

//export GetActiveProtectionCountFFI
func GetActiveProtectionCountFFI() *C.char {
	return jsonToCString(service.GetActiveProtectionCount())
}

//export SetProtectionEnabledFFI
func SetProtectionEnabledFFI(jsonC *C.char) *C.char {
	return jsonToCString(service.SetProtectionEnabled(C.GoString(jsonC)))
}

//export DeleteProtectionConfigFFI
func DeleteProtectionConfigFFI(assetNameC, assetIDC *C.char) *C.char {
	return jsonToCString(service.DeleteProtectionConfig(C.GoString(assetNameC), C.GoString(assetIDC)))
}

//export SaveProtectionStatisticsFFI
func SaveProtectionStatisticsFFI(jsonC *C.char) *C.char {
	return jsonToCString(service.SaveProtectionStatistics(C.GoString(jsonC)))
}

//export GetProtectionStatisticsFFI
func GetProtectionStatisticsFFI(assetNameC, assetIDC *C.char) *C.char {
	return jsonToCString(service.GetProtectionStatistics(C.GoString(assetNameC), C.GoString(assetIDC)))
}

//export ClearProtectionStatisticsFFI
func ClearProtectionStatisticsFFI(assetNameC, assetIDC *C.char) *C.char {
	return jsonToCString(service.ClearProtectionStatistics(C.GoString(assetNameC), C.GoString(assetIDC)))
}

//export GetShepherdSensitiveActionsFFI
func GetShepherdSensitiveActionsFFI(assetNameC, assetIDC *C.char) *C.char {
	return jsonToCString(service.GetShepherdSensitiveActions(C.GoString(assetNameC), C.GoString(assetIDC)))
}

//export SaveShepherdSensitiveActionsFFI
func SaveShepherdSensitiveActionsFFI(jsonC *C.char) *C.char {
	return jsonToCString(service.SaveShepherdSensitiveActions(C.GoString(jsonC)))
}

//export ClearAllDataFFI
func ClearAllDataFFI() *C.char {
	return jsonToCString(service.ClearAllData())
}

//export SaveHomeDirectoryPermissionFFI
func SaveHomeDirectoryPermissionFFI(jsonC *C.char) *C.char {
	return jsonToCString(service.SaveHomeDirectoryPermission(C.GoString(jsonC)))
}

// ==================== 审计日志 FFI ====================

//export SaveAuditLogFFI
func SaveAuditLogFFI(jsonC *C.char) *C.char {
	return jsonToCString(service.SaveAuditLog(C.GoString(jsonC)))
}

//export SaveAuditLogsBatchFFI
func SaveAuditLogsBatchFFI(jsonC *C.char) *C.char {
	return jsonToCString(service.SaveAuditLogsBatch(C.GoString(jsonC)))
}

//export GetAuditLogsFFI
func GetAuditLogsFFI(jsonC *C.char) *C.char {
	return jsonToCString(service.GetAuditLogs(C.GoString(jsonC)))
}

//export GetAuditLogCountFFI
func GetAuditLogCountFFI(jsonC *C.char) *C.char {
	return jsonToCString(service.GetAuditLogCount(C.GoString(jsonC)))
}

//export GetAuditLogStatisticsFFI
func GetAuditLogStatisticsFFI() *C.char {
	return jsonToCString(service.GetAuditLogStatistics(`{}`))
}

//export GetAuditLogStatisticsByFilterFFI
func GetAuditLogStatisticsByFilterFFI(jsonC *C.char) *C.char {
	return jsonToCString(service.GetAuditLogStatistics(C.GoString(jsonC)))
}

//export GetAuditLogAssetsFFI
func GetAuditLogAssetsFFI() *C.char {
	return jsonToCString(service.GetAuditLogAssets())
}

//export GetAuditLogStatisticsWithFilterFFI
func GetAuditLogStatisticsWithFilterFFI(jsonC *C.char) *C.char {
	return jsonToCString(service.GetAuditLogStatisticsWithFilter(C.GoString(jsonC)))
}

//export CleanOldAuditLogsFFI
func CleanOldAuditLogsFFI(jsonC *C.char) *C.char {
	return jsonToCString(service.CleanOldAuditLogs(C.GoString(jsonC)))
}

//export ClearAllAuditLogsFFI
func ClearAllAuditLogsFFI() *C.char {
	return jsonToCString(service.ClearAllAuditLogs(`{}`))
}

//export ClearAllAuditLogsByFilterFFI
func ClearAllAuditLogsByFilterFFI(jsonC *C.char) *C.char {
	return jsonToCString(service.ClearAllAuditLogs(C.GoString(jsonC)))
}

//export ClearAuditLogsWithFilterFFI
func ClearAuditLogsWithFilterFFI(jsonC *C.char) *C.char {
	return jsonToCString(service.ClearAuditLogsWithFilter(C.GoString(jsonC)))
}

// ==================== 安全事件 FFI ====================

//export SaveSecurityEventsBatchFFI
func SaveSecurityEventsBatchFFI(jsonC *C.char) *C.char {
	return jsonToCString(service.SaveSecurityEventsBatch(C.GoString(jsonC)))
}

//export GetSecurityEventsFFI
func GetSecurityEventsFFI(jsonC *C.char) *C.char {
	return jsonToCString(service.GetSecurityEvents(C.GoString(jsonC)))
}

//export GetSecurityEventCountFFI
func GetSecurityEventCountFFI() *C.char {
	return jsonToCString(service.GetSecurityEventCount())
}

//export ClearAllSecurityEventsFFI
func ClearAllSecurityEventsFFI() *C.char {
	return jsonToCString(service.ClearAllSecurityEvents())
}

//export ClearSecurityEventsFFI
func ClearSecurityEventsFFI(jsonC *C.char) *C.char {
	return jsonToCString(service.ClearSecurityEvents(C.GoString(jsonC)))
}

//export GetPendingSecurityEvents
func GetPendingSecurityEvents() *C.char {
	return C.CString(shepherd.GetPendingSecurityEventsInternal())
}

//export ClearSecurityEventsBuffer
func ClearSecurityEventsBuffer() *C.char {
	return C.CString(shepherd.ClearSecurityEventsBufferInternal())
}

//export GetSecurityEventsByRequestIDFFI
func GetSecurityEventsByRequestIDFFI(requestIDC *C.char) *C.char {
	return jsonToCString(service.GetSecurityEventsByRequestID(C.GoString(requestIDC)))
}

// ==================== API 指标 FFI ====================

//export SaveApiMetricsFFI
func SaveApiMetricsFFI(jsonC *C.char) *C.char {
	return jsonToCString(service.SaveApiMetrics(C.GoString(jsonC)))
}

//export GetApiStatisticsFFI
func GetApiStatisticsFFI(jsonC *C.char) *C.char {
	return jsonToCString(service.GetApiStatistics(C.GoString(jsonC)))
}

//export GetRecentApiMetricsFFI
func GetRecentApiMetricsFFI(jsonC *C.char) *C.char {
	return jsonToCString(service.GetRecentApiMetrics(C.GoString(jsonC)))
}

//export CleanOldApiMetricsFFI
func CleanOldApiMetricsFFI(jsonC *C.char) *C.char {
	return jsonToCString(service.CleanOldApiMetrics(C.GoString(jsonC)))
}

//export GetDailyTokenUsageFFI
func GetDailyTokenUsageFFI(assetNameC, assetIDC *C.char) *C.char {
	return jsonToCString(service.GetDailyTokenUsage(C.GoString(assetNameC), C.GoString(assetIDC)))
}

// ==================== 模型配置 FFI ====================

//export SaveSecurityModelConfigFFI
func SaveSecurityModelConfigFFI(configJSON *C.char) *C.char {
	return jsonToCString(service.SaveSecurityModelConfig(C.GoString(configJSON)))
}

//export GetSecurityModelConfigFFI
func GetSecurityModelConfigFFI() *C.char {
	return jsonToCString(service.GetSecurityModelConfig())
}

//export SaveBotModelConfigFFI
func SaveBotModelConfigFFI(configJSON *C.char) *C.char {
	return jsonToCString(service.SaveBotModelConfig(C.GoString(configJSON)))
}

//export GetBotModelConfigFFI
func GetBotModelConfigFFI(assetNameC, assetIDC *C.char) *C.char {
	return jsonToCString(service.GetBotModelConfig(C.GoString(assetNameC), C.GoString(assetIDC)))
}

//export DeleteBotModelConfigFFI
func DeleteBotModelConfigFFI(assetNameC, assetIDC *C.char) *C.char {
	return jsonToCString(service.DeleteBotModelConfig(C.GoString(assetNameC), C.GoString(assetIDC)))
}

// ==================== 应用设置 FFI ====================

//export SetLanguageFFI
func SetLanguageFFI(langC *C.char) *C.char {
	lang := C.GoString(langC)

	// 保存到数据库
	result := service.SetLanguage(lang)
	if success, ok := result["success"].(bool); !ok || !success {
		return jsonToCString(result)
	}

	// 更新运行时的 ShepherdGate 语言
	proxy.UpdateLanguage(lang)

	return jsonToCString(result)
}

//export GetLanguageFFI
func GetLanguageFFI() *C.char {
	return jsonToCString(service.GetLanguage())
}

//export SaveAppSettingFFI
func SaveAppSettingFFI(jsonC *C.char) *C.char {
	return jsonToCString(service.SaveAppSetting(C.GoString(jsonC)))
}

//export GetAppSettingFFI
func GetAppSettingFFI(keyC *C.char) *C.char {
	return jsonToCString(service.GetAppSetting(C.GoString(keyC)))
}

// ==================== 沙箱 FFI ====================

//export StartSandboxedGateway
func StartSandboxedGateway(configJSON *C.char) *C.char {
	jsonStr := C.GoString(configJSON)

	var req struct {
		AssetName         string                          `json:"asset_name"`
		GatewayBinaryPath string                          `json:"gateway_binary_path"`
		GatewayConfigPath string                          `json:"gateway_config_path"`
		GatewayArgs       []string                        `json:"gateway_args"`
		GatewayEnv        []string                        `json:"gateway_env"`
		PathPermission    sandbox.PathPermissionConfig    `json:"path_permission"`
		NetworkPermission sandbox.NetworkPermissionConfig `json:"network_permission"`
		ShellPermission   sandbox.ShellPermissionConfig   `json:"shell_permission"`
		PolicyDir         string                          `json:"policy_dir"`
		LogDir            string                          `json:"log_dir"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &req); err != nil {
		return C.CString(fmt.Sprintf(`{"success": false, "error": "invalid json: %v"}`, err))
	}

	if !sandbox.IsSandboxSupported() {
		return C.CString(`{"success": false, "error": "sandbox-exec not supported", "sandbox_supported": false}`)
	}

	policyDir := req.PolicyDir
	if policyDir == "" {
		policyDir = sandbox.GetDefaultPolicyDir()
	}

	manager := sandbox.GetSandboxManager(req.AssetName, policyDir)
	if req.LogDir != "" {
		manager.SetLogDir(req.LogDir)
	} else {
		pm := core.GetPathManager()
		if pm.IsInitialized() {
			manager.SetLogDir(pm.GetLogDir())
		}
	}

	config := sandbox.SandboxConfig{
		AssetName:         req.AssetName,
		GatewayBinaryPath: req.GatewayBinaryPath,
		GatewayConfigPath: req.GatewayConfigPath,
		PathPermission:    req.PathPermission,
		NetworkPermission: req.NetworkPermission,
		ShellPermission:   req.ShellPermission,
	}

	if err := manager.Configure(config, req.GatewayArgs, req.GatewayEnv); err != nil {
		return C.CString(fmt.Sprintf(`{"success": false, "error": "configuration failed: %v"}`, err))
	}

	if err := manager.Start(); err != nil {
		return C.CString(fmt.Sprintf(`{"success": false, "error": "%v"}`, err))
	}

	status := manager.GetStatus()
	return jsonToCString(map[string]interface{}{
		"success":           true,
		"managed_pid":       status.ManagedPID,
		"policy_path":       status.PolicyPath,
		"asset_name":        status.AssetName,
		"sandbox_supported": true,
	})
}

//export StopSandboxedGateway
func StopSandboxedGateway(assetName *C.char) *C.char {
	name := C.GoString(assetName)
	manager := sandbox.GetSandboxManager(name, sandbox.GetDefaultPolicyDir())
	if manager == nil {
		return C.CString(`{"success": true, "message": "no sandbox manager found"}`)
	}

	if err := manager.Stop(); err != nil {
		return C.CString(fmt.Sprintf(`{"success": false, "error": "%v"}`, err))
	}

	sandbox.RemoveProcessMonitor(name)
	return C.CString(`{"success": true}`)
}

//export GetSandboxStatus
func GetSandboxStatus(assetName *C.char) *C.char {
	name := C.GoString(assetName)
	manager := sandbox.GetSandboxManager(name, sandbox.GetDefaultPolicyDir())
	status := manager.GetStatus()
	return jsonToCString(status)
}

//export EnableProcessMonitor
func EnableProcessMonitor(configJSON *C.char) *C.char {
	jsonStr := C.GoString(configJSON)

	var req struct {
		AssetName      string `json:"asset_name"`
		GatewayPattern string `json:"gateway_pattern"`
		CheckInterval  int    `json:"check_interval_seconds"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &req); err != nil {
		return C.CString(fmt.Sprintf(`{"success": false, "error": "invalid json: %v"}`, err))
	}

	manager := sandbox.GetSandboxManager(req.AssetName, sandbox.GetDefaultPolicyDir())
	monitor := sandbox.GetProcessMonitor(req.AssetName, req.GatewayPattern)
	monitor.SetSandboxManager(manager)
	manager.SetMonitor(monitor)

	if err := monitor.Start(); err != nil {
		return C.CString(fmt.Sprintf(`{"success": false, "error": "%v"}`, err))
	}

	return C.CString(`{"success": true}`)
}

//export DisableProcessMonitor
func DisableProcessMonitor(assetName *C.char) *C.char {
	name := C.GoString(assetName)
	sandbox.RemoveProcessMonitor(name)
	return C.CString(`{"success": true}`)
}

//export GetAllSandboxStatus
func GetAllSandboxStatus() *C.char {
	statuses := sandbox.GetAllSandboxStatus()
	return jsonToCString(statuses)
}

//export KillUnmanagedGateway
func KillUnmanagedGateway(configJSON *C.char) *C.char {
	jsonStr := C.GoString(configJSON)

	var req struct {
		GatewayPattern string `json:"gateway_pattern"`
		ManagedPID     int    `json:"managed_pid"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &req); err != nil {
		return C.CString(fmt.Sprintf(`{"success": false, "error": "invalid json: %v"}`, err))
	}

	pids, err := sandbox.FindProcessesByName(req.GatewayPattern)
	if err != nil {
		return C.CString(fmt.Sprintf(`{"success": false, "error": "%v"}`, err))
	}

	var killedPIDs []int
	for _, pid := range pids {
		if pid != req.ManagedPID && pid != 0 {
			if err := sandbox.KillProcess(pid); err == nil {
				killedPIDs = append(killedPIDs, pid)
			}
		}
	}

	return jsonToCString(map[string]interface{}{
		"success":     true,
		"killed_pids": killedPIDs,
	})
}

//export GenerateSandboxPolicy
func GenerateSandboxPolicy(configJSON *C.char) *C.char {
	jsonStr := C.GoString(configJSON)

	var config sandbox.SandboxConfig
	if err := json.Unmarshal([]byte(jsonStr), &config); err != nil {
		return C.CString(fmt.Sprintf(`{"success": false, "error": "invalid json: %v"}`, err))
	}

	policy, err := sandbox.GeneratePlatformPolicy(config)
	if err != nil {
		return C.CString(fmt.Sprintf(`{"success": false, "error": "%v"}`, err))
	}

	return jsonToCString(map[string]interface{}{
		"success": true,
		"policy":  policy,
	})
}

//export CheckSandboxSupported
func CheckSandboxSupported() *C.char {
	supported := sandbox.IsSandboxSupported()
	return jsonToCString(map[string]interface{}{"supported": supported})
}

// ==================== LLM Provider FFI ====================

//export GetSupportedProviders
func GetSupportedProviders(scopeC *C.char) *C.char {
	scope := C.GoString(scopeC)

	var providers []adapter.ProviderInfo
	switch adapter.ProviderScope(scope) {
	case adapter.ScopeSecurity:
		providers = adapter.GetProviders(adapter.ScopeSecurity)
	case adapter.ScopeBot:
		providers = adapter.GetProviders(adapter.ScopeBot)
	default:
		providers = adapter.GetAllProviders()
	}

	return jsonToCString(providers)
}

// ==================== 回调桥接 FFI ====================

var (
	callbackBridge   *callback_bridge.Bridge
	callbackBridgeMu sync.Mutex
	callbackActive   bool
)

// 版本检查服务全局实例
var (
	versionCheckService   *service.VersionCheckService
	versionCheckServiceMu sync.Mutex
)

//export RegisterMessageCallback
func RegisterMessageCallback(callback C.DartCallback) *C.char {
	callbackBridgeMu.Lock()
	defer callbackBridgeMu.Unlock()

	if callbackBridge != nil && callbackBridge.IsRunning() {
		callbackActive = false
		proxy.SetCallbackBridge(nil)
		callbackBridge.Close()
		callbackBridge = nil
		C.clearDartCallback()
	}

	C.setDartCallback(callback)
	callbackActive = true

	bridge, err := callback_bridge.NewBridge(func(message string) {
		if !callbackActive {
			return
		}
		cStr := C.CString(message)
		defer C.free(unsafe.Pointer(cStr))
		C.invokeDartCallback(cStr)
	})
	if err != nil {
		callbackActive = false
		C.clearDartCallback()
		return jsonToCString(map[string]interface{}{"success": false, "error": err.Error()})
	}

	callbackBridge = bridge
	// 将回调桥接器设置到 openclaw 插件，使其能够发送日志和指标
	proxy.SetCallbackBridge(bridge)
	shepherd.GetSecurityEventBuffer().SetCallback(func(event shepherd.SecurityEvent) {
		bridge.SendSecurityEvent(map[string]interface{}{
			"id":          event.ID,
			"timestamp":   event.Timestamp,
			"event_type":  event.EventType,
			"action_desc": event.ActionDesc,
			"risk_type":   event.RiskType,
			"detail":      event.Detail,
			"source":      event.Source,
			"asset_name":  event.AssetName,
			"asset_id":    event.AssetID,
			"request_id":  event.RequestID,
		})
	})
	return jsonToCString(map[string]interface{}{"success": true, "mode": "callback"})
}

//export UnregisterMessageCallback
func UnregisterMessageCallback() *C.char {
	callbackBridgeMu.Lock()
	defer callbackBridgeMu.Unlock()

	callbackActive = false
	C.clearDartCallback()
	// 清除 openclaw 插件的回调桥接器
	proxy.SetCallbackBridge(nil)
	shepherd.GetSecurityEventBuffer().SetCallback(nil)

	if callbackBridge != nil {
		callbackBridge.Close()
		callbackBridge = nil
	}
	return jsonToCString(map[string]interface{}{"success": true})
}

//export IsCallbackBridgeRunning
func IsCallbackBridgeRunning() C.int {
	callbackBridgeMu.Lock()
	defer callbackBridgeMu.Unlock()

	if callbackBridge != nil && callbackBridge.IsRunning() {
		return 1
	}
	return 0
}

// ==================== 版本检查服务 FFI ====================

//export StartVersionCheckServiceFFI
func StartVersionCheckServiceFFI(configJSON *C.char) *C.char {
	versionCheckServiceMu.Lock()
	defer versionCheckServiceMu.Unlock()

	// 如果服务已运行，先停止
	if versionCheckService != nil {
		versionCheckService.Stop()
		versionCheckService = nil
	}

	// 解析配置
	var config service.VersionCheckConfig
	if err := json.Unmarshal([]byte(C.GoString(configJSON)), &config); err != nil {
		return jsonToCString(map[string]interface{}{"success": false, "error": fmt.Sprintf("invalid config: %v", err)})
	}

	// 获取回调桥接器
	callbackBridgeMu.Lock()
	bridge := callbackBridge
	callbackBridgeMu.Unlock()

	// 创建服务
	svc, err := service.NewVersionCheckService(&config, bridge)
	if err != nil {
		return jsonToCString(map[string]interface{}{"success": false, "error": err.Error()})
	}

	versionCheckService = svc
	versionCheckService.Start()

	return jsonToCString(map[string]interface{}{"success": true})
}

//export StopVersionCheckServiceFFI
func StopVersionCheckServiceFFI() *C.char {
	versionCheckServiceMu.Lock()
	defer versionCheckServiceMu.Unlock()

	if versionCheckService != nil {
		versionCheckService.Stop()
		versionCheckService = nil
	}

	return jsonToCString(map[string]interface{}{"success": true})
}

//export UpdateVersionCheckLanguageFFI
func UpdateVersionCheckLanguageFFI(langC *C.char) *C.char {
	versionCheckServiceMu.Lock()
	defer versionCheckServiceMu.Unlock()

	if versionCheckService == nil {
		return jsonToCString(map[string]interface{}{"success": false, "error": "service not running"})
	}

	lang := C.GoString(langC)
	versionCheckService.SetLanguage(lang)

	return jsonToCString(map[string]interface{}{"success": true})
}

// ==================== 辅助函数 FFI ====================

//export FreeString
func FreeString(str *C.char) {
	C.free(unsafe.Pointer(str))
}

// ==================== Protection Proxy FFI (core/proxy) ====================

//export StartProtectionProxy
func StartProtectionProxy(configJSON *C.char) *C.char {
	return C.CString(proxy.StartProtectionProxyInternal(C.GoString(configJSON)))
}

//export StopProtectionProxy
func StopProtectionProxy() *C.char {
	return C.CString(proxy.StopProtectionProxyInternal())
}

//export StopProtectionProxyByAsset
func StopProtectionProxyByAsset(assetNameC, assetIDC *C.char) *C.char {
	return C.CString(proxy.StopProtectionProxyByAssetInternal(C.GoString(assetNameC), C.GoString(assetIDC)))
}

//export ResetProtectionStatistics
func ResetProtectionStatistics() *C.char {
	return C.CString(proxy.ResetProtectionStatisticsInternal())
}

//export GetProtectionProxyStatus
func GetProtectionProxyStatus() *C.char {
	return C.CString(proxy.GetProtectionProxyStatusInternal())
}

//export GetProtectionProxyStatusByAsset
func GetProtectionProxyStatusByAsset(assetNameC, assetIDC *C.char) *C.char {
	return C.CString(proxy.GetProtectionProxyStatusByAssetInternal(C.GoString(assetNameC), C.GoString(assetIDC)))
}

//export UpdateProtectionConfig
func UpdateProtectionConfig(configJSON *C.char) *C.char {
	return C.CString(proxy.UpdateProtectionConfigInternal(C.GoString(configJSON)))
}

//export UpdateProtectionConfigByAsset
func UpdateProtectionConfigByAsset(assetNameC, assetIDC, configJSON *C.char) *C.char {
	return C.CString(proxy.UpdateProtectionConfigByAssetInternal(
		C.GoString(assetNameC),
		C.GoString(assetIDC),
		C.GoString(configJSON),
	))
}

//export UpdateSecurityModelConfig
func UpdateSecurityModelConfig(configJSON *C.char) *C.char {
	return C.CString(proxy.UpdateSecurityModelConfigInternal(C.GoString(configJSON)))
}

//export UpdateSecurityModelConfigByAsset
func UpdateSecurityModelConfigByAsset(assetNameC, assetIDC, configJSON *C.char) *C.char {
	return C.CString(proxy.UpdateSecurityModelConfigByAssetInternal(
		C.GoString(assetNameC),
		C.GoString(assetIDC),
		C.GoString(configJSON),
	))
}

//export UpdateBotForwardingProvider
func UpdateBotForwardingProvider(configJSON *C.char) *C.char {
	return C.CString(proxy.UpdateBotForwardingProviderInternal(C.GoString(configJSON)))
}

//export SetProtectionProxyAuditOnly
func SetProtectionProxyAuditOnly(auditOnly C.int) *C.char {
	return C.CString(proxy.SetProtectionProxyAuditOnlyInternal(auditOnly != 0))
}

//export SetProtectionProxyAuditOnlyByAsset
func SetProtectionProxyAuditOnlyByAsset(assetNameC, assetIDC *C.char, auditOnly C.int) *C.char {
	return C.CString(proxy.SetProtectionProxyAuditOnlyByAssetInternal(
		C.GoString(assetNameC),
		C.GoString(assetIDC),
		auditOnly != 0,
	))
}

//export GetProtectionProxyLogs
func GetProtectionProxyLogs(sessionID *C.char) *C.char {
	return C.CString(proxy.GetProtectionProxyLogsInternal(C.GoString(sessionID)))
}

//export WaitForProtectionLogs
func WaitForProtectionLogs(sessionID *C.char, timeoutMs C.int) *C.char {
	return C.CString(proxy.WaitForProtectionLogsInternal(C.GoString(sessionID), int(timeoutMs)))
}

// ==================== ShepherdGate FFI (core/shepherd) ====================

//export UpdateShepherdRulesFFI
func UpdateShepherdRulesFFI(assetNameC, assetIDC, rulesJSON *C.char) *C.char {
	return C.CString(proxy.UpdateShepherdRulesByAssetInternal(
		C.GoString(assetNameC),
		C.GoString(assetIDC),
		C.GoString(rulesJSON),
	))
}

//export GetShepherdRulesFFI
func GetShepherdRulesFFI(assetNameC, assetIDC *C.char) *C.char {
	return C.CString(proxy.GetShepherdRulesByAssetInternal(
		C.GoString(assetNameC),
		C.GoString(assetIDC),
	))
}

//export ListBundledReActSkillsFFI
func ListBundledReActSkillsFFI() *C.char {
	return C.CString(shepherd.ListBundledReActSkillsInternal())
}

// ==================== Skill Scan FFI (plugin capability dispatch) ====================

//export StartSkillSecurityScan
func StartSkillSecurityScan(skillPath, modelConfigJSON *C.char) *C.char {
	return C.CString(core.StartSkillSecurityScanByPlugin("", C.GoString(skillPath), C.GoString(modelConfigJSON)))
}

//export GetSkillSecurityScanLog
func GetSkillSecurityScanLog(scanID *C.char) *C.char {
	return C.CString(core.GetSkillSecurityScanLogByPlugin("", C.GoString(scanID)))
}

//export GetSkillSecurityScanResult
func GetSkillSecurityScanResult(scanID *C.char) *C.char {
	return C.CString(core.GetSkillSecurityScanResultByPlugin("", C.GoString(scanID)))
}

//export CancelSkillSecurityScan
func CancelSkillSecurityScan(scanID *C.char) *C.char {
	return C.CString(core.CancelSkillSecurityScanByPlugin("", C.GoString(scanID)))
}

//export StartBatchSkillScan
func StartBatchSkillScan() *C.char {
	return C.CString(core.StartBatchSkillScanByPlugin(""))
}

//export GetBatchSkillScanLog
func GetBatchSkillScanLog(batchID *C.char) *C.char {
	return C.CString(core.GetBatchSkillScanLogByPlugin("", C.GoString(batchID)))
}

//export GetBatchSkillScanResults
func GetBatchSkillScanResults(batchID *C.char) *C.char {
	return C.CString(core.GetBatchSkillScanResultsByPlugin("", C.GoString(batchID)))
}

//export CancelBatchSkillScan
func CancelBatchSkillScan(batchID *C.char) *C.char {
	return C.CString(core.CancelBatchSkillScanByPlugin("", C.GoString(batchID)))
}

// ==================== Model Connection FFI (plugin capability dispatch) ====================

//export TestModelConnectionFFI
func TestModelConnectionFFI(configJSON *C.char) *C.char {
	return C.CString(core.TestModelConnectionByPlugin("", C.GoString(configJSON)))
}

// ==================== Skill Management FFI (plugin capability dispatch) ====================

//export DeleteSkill
func DeleteSkill(skillPath *C.char) *C.char {
	return C.CString(core.DeleteSkillByPlugin("", C.GoString(skillPath)))
}

// ==================== TruthRecord Snapshot FFI ====================

//export GetAllTruthRecordSnapshots
func GetAllTruthRecordSnapshots() *C.char {
	return C.CString(proxy.GetAllTruthRecordSnapshotsInternal())
}

// ==================== Audit Log FFI (core/proxy — backed by TruthRecord) ====================

//export GetAuditLogs
func GetAuditLogs(limit, offset C.int, riskOnly C.int) *C.char {
	return C.CString(proxy.GetTruthRecordsInternal(int(limit), int(offset), riskOnly != 0))
}

//export GetPendingAuditLogs
func GetPendingAuditLogs() *C.char {
	return C.CString(proxy.GetPendingTruthRecordsInternal())
}

//export ClearAuditLogs
func ClearAuditLogs() *C.char {
	return C.CString(proxy.ClearTruthRecordsInternal())
}

//export ClearAuditLogsWithFilter
func ClearAuditLogsWithFilter(jsonC *C.char) *C.char {
	return C.CString(proxy.ClearTruthRecordsWithFilterInternal(C.GoString(jsonC)))
}

// ==================== Gateway Sandbox FFI (plugin capability dispatch) ====================

// SyncGatewaySandbox synchronizes gateway sandbox config
//
//export SyncGatewaySandbox
func SyncGatewaySandbox() *C.char {
	return C.CString(core.SyncGatewaySandboxByPlugin(""))
}

//export SyncGatewaySandboxByAsset
func SyncGatewaySandboxByAsset(assetIDC *C.char) *C.char {
	return C.CString(core.SyncGatewaySandboxByAssetAndPlugin("", C.GoString(assetIDC)))
}

//export SyncGatewaySandboxByAssetName
func SyncGatewaySandboxByAssetName(assetNameC, assetIDC *C.char) *C.char {
	return C.CString(core.SyncGatewaySandboxByAssetAndPlugin(C.GoString(assetNameC), C.GoString(assetIDC)))
}

//export HasInitialBackupFFI
func HasInitialBackupFFI() *C.char {
	return C.CString(core.HasInitialBackupByPlugin(""))
}

//export HasInitialBackupByAssetFFI
func HasInitialBackupByAssetFFI(assetNameC *C.char) *C.char {
	return C.CString(core.HasInitialBackupByPlugin(C.GoString(assetNameC)))
}

//export RestoreToInitialConfigFFI
func RestoreToInitialConfigFFI() *C.char {
	return C.CString(core.RestoreToInitialConfigByPlugin(""))
}

//export RestoreToInitialConfigByAssetFFI
func RestoreToInitialConfigByAssetFFI(assetNameC *C.char) *C.char {
	return C.CString(core.RestoreToInitialConfigByPlugin(C.GoString(assetNameC)))
}

//export NotifyPluginAppExitFFI
func NotifyPluginAppExitFFI(assetNameC, assetIDC *C.char) *C.char {
	return C.CString(core.NotifyAppExitByPlugin(C.GoString(assetNameC), C.GoString(assetIDC)))
}

//export RestoreBotDefaultStateFFI
func RestoreBotDefaultStateFFI(assetNameC, assetIDC *C.char) *C.char {
	return C.CString(core.RestoreBotDefaultStateByPlugin(C.GoString(assetNameC), C.GoString(assetIDC)))
}

func main() {}
