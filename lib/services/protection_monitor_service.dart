import 'dart:async';
import 'dart:convert';
import 'dart:ffi' as ffi;
import 'dart:io';
import 'package:ffi/ffi.dart';

import '../models/audit_log_model.dart';
import '../models/protection_analysis_model.dart';
import '../models/security_event_model.dart';
import 'audit_log_database_service.dart';
import 'message_bridge_service.dart' hide FreeStringC, FreeStringDart;
import 'metrics_database_service.dart';
import 'native_library_service.dart' hide FreeStringDart;
import 'protection_database_service.dart';
import 'protection_proxy_ffi.dart';
import 'security_event_database_service.dart';
import '../utils/app_logger.dart';

/// 防护监控服务：负责日志/指标/事件流、轮询、MessageBridge、统计与审计日志。
///
/// 由 [ProtectionService] 协调：通过 setProxySession / startProxyLogPolling / stopProxyLogPolling 与代理生命周期联动。
class ProtectionMonitorService {
  static const String defaultInstanceKey = '__global__';
  static final Map<String, ProtectionMonitorService> _instances = {};

  static String normalizeInstanceKey(String? instanceKey) {
    final key = instanceKey?.trim() ?? '';
    return key.isEmpty ? defaultInstanceKey : key;
  }

  factory ProtectionMonitorService([String instanceKey = defaultInstanceKey]) {
    final normalizedKey = normalizeInstanceKey(instanceKey);
    return _instances.putIfAbsent(
      normalizedKey,
      () => ProtectionMonitorService._internal(normalizedKey),
    );
  }

  ProtectionMonitorService._internal(this._instanceKey);

  final String _instanceKey;

  final StreamController<String> _logController =
      StreamController<String>.broadcast();
  final StreamController<ProtectionAnalysisResult> _resultController =
      StreamController<ProtectionAnalysisResult>.broadcast();
  final StreamController<ApiMetrics> _metricsController =
      StreamController<ApiMetrics>.broadcast();
  final StreamController<List<SecurityEvent>> _securityEventController =
      StreamController<List<SecurityEvent>>.broadcast();

  Timer? _proxyLogPollTimer;
  Timer? _dbSyncTimer; // 独立定时器：回调模式下仍周期同步审计日志
  String? _proxySessionID;
  bool _isProxyRunning = false;
  String? _assetName;
  String _assetID = '';

  MessageBridgeService? _messageBridgeService;
  bool _useCallback = false;
  StreamSubscription<String>? _logSubscription;
  StreamSubscription<Map<String, dynamic>>? _metricsSubscription;
  StreamSubscription<Map<String, dynamic>>? _securityEventBridgeSubscription;

  final bool _isAnalyzing = false;
  int _analysisCount = 0;
  int _blockedCount = 0;
  int _warningCount = 0;
  DateTime? _lastAnalysisTime;
  int _baselineAnalysisCount = 0;
  int _baselineBlockedCount = 0;
  int _baselineWarningCount = 0;
  int _baselineTotalTokens = 0;
  int _baselineTotalPromptTokens = 0;
  int _baselineTotalCompletionTokens = 0;
  int _baselineTotalToolCalls = 0;
  int _baselineRequestCount = 0;
  int _baselineAuditTokens = 0;
  int _baselineAuditPromptTokens = 0;
  int _baselineAuditCompletionTokens = 0;
  int _auditTokens = 0;
  int _auditPromptTokens = 0;
  int _auditCompletionTokens = 0;
  int _totalTokens = 0;
  int _totalPromptTokens = 0;
  int _totalCompletionTokens = 0;
  int _totalToolCalls = 0;
  int _requestCount = 0;

  Stream<String> get logStream => _logController.stream;
  Stream<ProtectionAnalysisResult> get resultStream => _resultController.stream;
  Stream<ApiMetrics> get metricsStream => _metricsController.stream;
  Stream<List<SecurityEvent>> get securityEventStream =>
      _securityEventController.stream;

  bool get isAnalyzing => _isAnalyzing;
  int get analysisCount => _analysisCount;
  int get blockedCount => _blockedCount;
  int get warningCount => _warningCount;
  DateTime? get lastAnalysisTime => _lastAnalysisTime;
  bool get useCallbackBridge => _useCallback;
  int get totalTokens => _totalTokens;
  int get totalPromptTokens => _totalPromptTokens;
  int get totalCompletionTokens => _totalCompletionTokens;
  int get totalToolCalls => _totalToolCalls;
  int get requestCount => _requestCount;
  int get auditTokens => _auditTokens;
  int get auditPromptTokens => _auditPromptTokens;
  int get auditCompletionTokens => _auditCompletionTokens;

  int get baselineAnalysisCount => _baselineAnalysisCount;
  int get baselineBlockedCount => _baselineBlockedCount;
  int get baselineWarningCount => _baselineWarningCount;
  int get baselineTotalTokens => _baselineTotalTokens;
  int get baselineTotalPromptTokens => _baselineTotalPromptTokens;
  int get baselineTotalCompletionTokens => _baselineTotalCompletionTokens;
  int get baselineTotalToolCalls => _baselineTotalToolCalls;
  int get baselineRequestCount => _baselineRequestCount;
  int get baselineAuditTokens => _baselineAuditTokens;
  int get baselineAuditPromptTokens => _baselineAuditPromptTokens;
  int get baselineAuditCompletionTokens => _baselineAuditCompletionTokens;

  void setAssetName(String? name, [String assetID = '']) {
    final normalizedName = name?.trim();
    final normalizedAssetID = assetID.trim();
    final changed =
        _assetName != normalizedName || _assetID != normalizedAssetID;
    _assetName = normalizedName;
    _assetID = normalizedAssetID;
    if (changed) {
      resetMemoryState();
    }
  }

  void setProxySession(String? sessionId, bool isRunning) {
    _proxySessionID = sessionId;
    _isProxyRunning = isRunning;
  }

  bool _matchesCurrentAsset(SecurityEvent event) {
    if (_assetID.isNotEmpty) {
      return event.assetID == _assetID;
    }
    if ((_assetName ?? '').isNotEmpty) {
      return event.assetName == _assetName;
    }
    return true;
  }

  bool _matchesCurrentMetrics(Map<String, dynamic> metrics) {
    final metricAssetID = (metrics['asset_id'] ?? '').toString().trim();
    if (_assetID.isNotEmpty) {
      return metricAssetID == _assetID;
    }
    final metricAssetName = (metrics['asset_name'] ?? '').toString().trim();
    if ((_assetName ?? '').isNotEmpty) {
      return metricAssetName == _assetName;
    }
    return false;
  }

  int _deltaWithReset(int current, int previous) {
    if (current >= previous) {
      return current - previous;
    }
    // Counter reset or asset switch: treat current as fresh baseline delta.
    return current;
  }

  void addLog(String log) {
    if (!_logController.isClosed) _logController.add(log);
  }

  void startProxyLogPolling() {
    _proxyLogPollTimer?.cancel();
    int syncCounter = 25;
    _proxyLogPollTimer = Timer.periodic(const Duration(milliseconds: 200), (
      timer,
    ) {
      if (_proxySessionID != null && _isProxyRunning) {
        _pollProxyLogs(_proxySessionID!);
        syncCounter++;
        if (syncCounter >= 25) {
          syncCounter = 0;
          syncPendingAuditLogs();
          syncPendingSecurityEvents();
        }
      }
    });
  }

  void stopProxyLogPolling() {
    _proxyLogPollTimer?.cancel();
    _proxyLogPollTimer = null;
    _stopDbSyncTimer();
  }

  /// 启动独立的 DB 同步定时器（5 秒周期），用于回调模式下拉取审计日志。
  /// 安全事件已由 Go 层直接持久化并通过 Bridge 实时推送，无需定时拉取。
  void _startDbSyncTimer() {
    _dbSyncTimer?.cancel();
    _dbSyncTimer = Timer.periodic(const Duration(seconds: 5), (_) {
      if (_proxySessionID != null && _isProxyRunning) {
        syncPendingAuditLogs();
      }
    });
  }

  void _stopDbSyncTimer() {
    _dbSyncTimer?.cancel();
    _dbSyncTimer = null;
  }

  ffi.DynamicLibrary _getDylib() {
    final dylib = NativeLibraryService().dylib;
    if (dylib == null) throw Exception('Plugin library not loaded');
    return dylib;
  }

  Future<bool> enableCallbackBridge() async {
    if (Platform.environment['BOTSEC_FORCE_FFI_POLLING'] == '1') {
      appLogger.info('[ProtectionMonitor] Force FFI polling mode');
      return false;
    }
    return await _enableCallbackMode();
  }

  Future<bool> _enableCallbackMode() async {
    try {
      _messageBridgeService = MessageBridgeService();
      _useCallback = await _messageBridgeService!.initialize();

      if (_useCallback) {
        appLogger.info('[ProtectionMonitor] FFI callback mode enabled');
        _logSubscription = _messageBridgeService!.logStream
            .handleError(_handleBridgeError)
            .listen((log) {
              scheduleMicrotask(() {
                if (!_logController.isClosed) _logController.add(log);
              });
            });
        _metricsSubscription = _messageBridgeService!.metricsStream
            .handleError(_handleBridgeError)
            .listen((metrics) {
              scheduleMicrotask(() {
                if (_matchesCurrentMetrics(metrics)) {
                  _updateMetricsFromBridge(metrics);
                }
              });
            });
        _messageBridgeService!.errorStream
            .handleError((error, stackTrace) => _handleBridgeError(error))
            .listen((error) => _handleBridgeError(error));
        // 订阅安全事件流（Go 已直接持久化到 SQLite，Flutter 仅推送 UI）
        _securityEventBridgeSubscription = _messageBridgeService!
            .securityEventStream
            .handleError(_handleBridgeError)
            .listen((payload) {
              scheduleMicrotask(() {
                try {
                  final event = SecurityEvent.fromJson(payload);
                  if (_matchesCurrentAsset(event) &&
                      !_securityEventController.isClosed) {
                    _securityEventController.add([event]);
                  }
                } catch (e) {
                  appLogger.error(
                    '[ProtectionMonitor] Parse security event error',
                    e,
                  );
                }
              });
            });
        _proxyLogPollTimer?.cancel();
        _proxyLogPollTimer = null;
        // 回调模式下日志/指标/安全事件由 MessageBridge 推送，
        // 审计日志仍需定时从 Go 内存缓冲拉取并持久化到 SQLite。
        _startDbSyncTimer();
        return true;
      }
      _messageBridgeService?.dispose();
      _messageBridgeService = null;
      return false;
    } catch (e) {
      appLogger.error('[ProtectionMonitor] Enable callback mode error', e);
      _useCallback = false;
      _messageBridgeService?.dispose();
      _messageBridgeService = null;
      return false;
    }
  }

  void disableCallbackBridge() {
    _logSubscription?.cancel();
    _metricsSubscription?.cancel();
    _securityEventBridgeSubscription?.cancel();
    _messageBridgeService?.dispose();
    _messageBridgeService = null;
    _useCallback = false;
    _stopDbSyncTimer();
    appLogger.info('[ProtectionMonitor] Message bridge disabled');

    // 如果代理正在运行，重启 FFI 轮询（轮询模式自带审计/事件同步）
    if (_isProxyRunning) {
      startProxyLogPolling();
    }
  }

  void _updateMetricsFromBridge(Map<String, dynamic> metrics) {
    // 保存之前的值用于计算增量
    final prevPromptTokens = _totalPromptTokens;
    final prevCompletionTokens = _totalCompletionTokens;
    final prevToolCalls = _totalToolCalls;
    final prevAuditTokens = _auditTokens;
    final prevAnalysisCount = _analysisCount;
    final prevBlockedCount = _blockedCount;
    final prevWarningCount = _warningCount;

    // 更新统计数据（Go 代理现在管理包含基线的总计数）
    if (metrics['analysis_count'] != null) {
      _analysisCount = metrics['analysis_count'] as int;
    }
    if (metrics['blocked_count'] != null) {
      _blockedCount = metrics['blocked_count'] as int;
    }
    if (metrics['warning_count'] != null) {
      _warningCount = metrics['warning_count'] as int;
    }
    if (metrics['total_tokens'] != null) {
      _totalTokens = metrics['total_tokens'] as int;
    }
    if (metrics['total_prompt_tokens'] != null) {
      _totalPromptTokens = metrics['total_prompt_tokens'] as int;
    }
    if (metrics['total_completion_tokens'] != null) {
      _totalCompletionTokens = metrics['total_completion_tokens'] as int;
    }
    if (metrics['total_tool_calls'] != null) {
      _totalToolCalls = metrics['total_tool_calls'] as int;
    }
    if (metrics['request_count'] != null) {
      _requestCount = metrics['request_count'] as int;
    }
    if (metrics['audit_tokens'] != null) {
      _auditTokens = metrics['audit_tokens'] as int;
    }
    if (metrics['audit_prompt_tokens'] != null) {
      _auditPromptTokens = metrics['audit_prompt_tokens'] as int;
    }
    if (metrics['audit_completion_tokens'] != null) {
      _auditCompletionTokens = metrics['audit_completion_tokens'] as int;
    }

    // 计算增量用于数据库存储（支持计数器重置/资产切换）
    final deltaPromptTokens = _deltaWithReset(
      _totalPromptTokens,
      prevPromptTokens,
    );
    final deltaCompletionTokens = _deltaWithReset(
      _totalCompletionTokens,
      prevCompletionTokens,
    );
    final deltaToolCalls = _deltaWithReset(_totalToolCalls, prevToolCalls);
    final deltaAuditTokens = _deltaWithReset(_auditTokens, prevAuditTokens);
    final deltaAnalysisCount = _deltaWithReset(
      _analysisCount,
      prevAnalysisCount,
    );
    final deltaBlockedCount = _deltaWithReset(_blockedCount, prevBlockedCount);
    final deltaWarningCount = _deltaWithReset(_warningCount, prevWarningCount);

    // 只有在有变化时才保存到数据库并通知 UI
    final hasTokenChanges =
        deltaPromptTokens > 0 ||
        deltaCompletionTokens > 0 ||
        deltaToolCalls > 0 ||
        deltaAuditTokens > 0;
    final hasStatChanges =
        deltaAnalysisCount > 0 ||
        deltaBlockedCount > 0 ||
        deltaWarningCount > 0;

    if (hasTokenChanges || hasStatChanges) {
      final apiMetrics = ApiMetrics(
        timestamp: DateTime.now(),
        promptTokens: deltaPromptTokens,
        completionTokens: deltaCompletionTokens,
        totalTokens: deltaPromptTokens + deltaCompletionTokens,
        toolCallCount: deltaToolCalls,
        model: 'unknown',
        isBlocked: deltaBlockedCount > 0,
      );
      _metricsController.add(apiMetrics);
      // 同时保存到 api_metrics 表用于趋势图
      if (hasTokenChanges) {
        unawaited(
          MetricsDatabaseService()
              .saveApiMetrics(
                apiMetrics,
                assetName: _assetName,
                assetID: _assetID,
              )
              .catchError((Object e) {
                appLogger.error(
                  '[ProtectionMonitor] Failed to save API metrics',
                  e,
                );
              }),
        );
      }
      unawaited(_saveStatisticsToDatabase());
    }
  }

  void _handleBridgeError(dynamic error) {
    scheduleMicrotask(() {
      appLogger.warning('[ProtectionMonitor] Message bridge error: $error');
      if (error.toString().contains('Exception') && _useCallback) {
        disableCallbackBridge();
      }
    });
  }

  Future<void> loadStatisticsFromDatabase() async {
    if (_assetName == null) return;
    try {
      final stats = await ProtectionDatabaseService().getProtectionStatistics(
        _assetName!,
        _assetID,
      );
      if (stats != null) {
        _baselineAnalysisCount = stats.analysisCount;
        _baselineBlockedCount = stats.blockedCount;
        _baselineWarningCount = stats.warningCount;
        _baselineTotalTokens = stats.totalTokens;
        _baselineTotalPromptTokens = stats.totalPromptTokens;
        _baselineTotalCompletionTokens = stats.totalCompletionTokens;
        _baselineTotalToolCalls = stats.totalToolCalls;
        _baselineRequestCount = stats.requestCount;
        _baselineAuditTokens = stats.auditTokens;
        _baselineAuditPromptTokens = stats.auditPromptTokens;
        _baselineAuditCompletionTokens = stats.auditCompletionTokens;
        _analysisCount = _baselineAnalysisCount;
        _blockedCount = _baselineBlockedCount;
        _warningCount = _baselineWarningCount;
        _totalTokens = _baselineTotalTokens;
        _totalPromptTokens = _baselineTotalPromptTokens;
        _totalCompletionTokens = _baselineTotalCompletionTokens;
        _totalToolCalls = _baselineTotalToolCalls;
        _requestCount = _baselineRequestCount;
        _auditTokens = _baselineAuditTokens;
        _auditPromptTokens = _baselineAuditPromptTokens;
        _auditCompletionTokens = _baselineAuditCompletionTokens;
      } else {
        _baselineAnalysisCount = _baselineBlockedCount = _baselineWarningCount =
            0;
        _baselineTotalTokens = _baselineTotalPromptTokens =
            _baselineTotalCompletionTokens = 0;
        _baselineTotalToolCalls = _baselineRequestCount = 0;
        _baselineAuditTokens = _baselineAuditPromptTokens =
            _baselineAuditCompletionTokens = 0;
        _analysisCount = _blockedCount = _warningCount = 0;
        _totalTokens = _totalPromptTokens = _totalCompletionTokens = 0;
        _totalToolCalls = _requestCount = 0;
        _auditTokens = _auditPromptTokens = _auditCompletionTokens = 0;
      }
    } catch (e) {
      appLogger.error('[ProtectionMonitor] Failed to load statistics', e);
    }
  }

  Future<void> _saveStatisticsToDatabase() async {
    if (_assetName == null) return;
    try {
      await ProtectionDatabaseService().saveProtectionStatistics(
        assetName: _assetName!,
        assetID: _assetID,
        analysisCount: _analysisCount,
        messageCount: 0,
        warningCount: _warningCount,
        blockedCount: _blockedCount,
        totalTokens: _totalTokens,
        totalPromptTokens: _totalPromptTokens,
        totalCompletionTokens: _totalCompletionTokens,
        totalToolCalls: _totalToolCalls,
        requestCount: _requestCount,
        auditTokens: _auditTokens,
        auditPromptTokens: _auditPromptTokens,
        auditCompletionTokens: _auditCompletionTokens,
      );
    } catch (e) {
      appLogger.error('[ProtectionMonitor] Failed to save statistics', e);
    }
  }

  void resetMemoryState() {
    _analysisCount = _blockedCount = _warningCount = 0;
    _lastAnalysisTime = null;
    _baselineAnalysisCount = _baselineBlockedCount = _baselineWarningCount = 0;
    _baselineTotalTokens = _baselineTotalPromptTokens =
        _baselineTotalCompletionTokens = 0;
    _baselineTotalToolCalls = _baselineRequestCount = 0;
    _baselineAuditTokens = _baselineAuditPromptTokens =
        _baselineAuditCompletionTokens = 0;
    _totalTokens = _totalPromptTokens = _totalCompletionTokens = 0;
    _totalToolCalls = _requestCount = 0;
    _auditTokens = _auditPromptTokens = _auditCompletionTokens = 0;
    _proxySessionID = null;
  }

  Future<void> resetProxyStatistics() async {
    try {
      final dylib = _getDylib();
      try {
        final resetStats = dylib
            .lookupFunction<
              ResetProtectionStatisticsC,
              ResetProtectionStatisticsDart
            >('ResetProtectionStatistics');
        final resultPtr = resetStats();
        final freeString = dylib.lookupFunction<FreeStringC, FreeStringDart>(
          'FreeString',
        );
        final resultStr = resultPtr.toDartString();
        freeString(resultPtr);
        appLogger.info(
          '[ProtectionMonitor] Proxy statistics reset: $resultStr',
        );
      } catch (e) {
        appLogger.warning(
          '[ProtectionMonitor] ResetProtectionStatistics not available: $e',
        );
      }
    } catch (e) {
      appLogger.error(
        '[ProtectionMonitor] Failed to reset proxy statistics',
        e,
      );
    }
  }

  void resetStatistics() {
    _analysisCount = _blockedCount = _warningCount = 0;
    _lastAnalysisTime = null;
  }

  Future<ApiStatistics> getApiStatistics({Duration? duration}) async {
    try {
      return await MetricsDatabaseService().getApiStatistics(
        duration: duration,
        assetName: _assetName,
        assetID: _assetID,
      );
    } catch (e) {
      appLogger.error('[ProtectionMonitor] Get API statistics failed', e);
      return ApiStatistics.empty();
    }
  }

  void recordApiMetricsManually({
    required int promptTokens,
    required int completionTokens,
    required int toolCallCount,
    required String model,
    bool isBlocked = false,
    String? riskLevel,
  }) {
    final metrics = ApiMetrics(
      timestamp: DateTime.now(),
      promptTokens: promptTokens,
      completionTokens: completionTokens,
      totalTokens: promptTokens + completionTokens,
      toolCallCount: toolCallCount,
      model: model,
      isBlocked: isBlocked,
      riskLevel: riskLevel,
    );
    _totalTokens += metrics.totalTokens;
    _totalPromptTokens += metrics.promptTokens;
    _totalCompletionTokens += metrics.completionTokens;
    _totalToolCalls += metrics.toolCallCount;
    _requestCount += 1;
    _processApiMetric(metrics.toJson());
  }

  void _processApiMetric(Map<String, dynamic> metricData) {
    try {
      final metrics = ApiMetrics(
        timestamp: metricData['timestamp'] != null
            ? DateTime.parse(metricData['timestamp'])
            : DateTime.now(),
        promptTokens: metricData['prompt_tokens'] ?? 0,
        completionTokens: metricData['completion_tokens'] ?? 0,
        totalTokens:
            metricData['total_tokens'] ??
            ((metricData['prompt_tokens'] ?? 0) +
                (metricData['completion_tokens'] ?? 0)),
        toolCallCount: metricData['tool_call_count'] ?? 0,
        model: metricData['model'] ?? 'unknown',
        isBlocked: metricData['is_blocked'] ?? false,
        riskLevel: metricData['risk_level'],
      );
      _metricsController.add(metrics);
      unawaited(
        MetricsDatabaseService()
            .saveApiMetrics(metrics, assetName: _assetName, assetID: _assetID)
            .catchError((Object e) {
              appLogger.error(
                '[ProtectionMonitor] Failed to save API metrics',
                e,
              );
            }),
      );
      unawaited(_saveStatisticsToDatabase());
    } catch (e) {
      appLogger.error('[ProtectionMonitor] Failed to save API metrics', e);
    }
  }

  void _pollProxyLogs(String sessionID) {
    if (sessionID != _proxySessionID || !_isProxyRunning) return;
    try {
      final dylib = _getDylib();
      final getLogs = dylib
          .lookupFunction<GetProtectionProxyLogsC, GetProtectionProxyLogsDart>(
            'GetProtectionProxyLogs',
          );
      final freeString = dylib.lookupFunction<FreeStringC, FreeStringDart>(
        'FreeString',
      );

      final sessionIDPtr = sessionID.toNativeUtf8();
      final resultPtr = getLogs(sessionIDPtr);
      malloc.free(sessionIDPtr);

      final resultStr = resultPtr.toDartString();
      freeString(resultPtr);

      final result = jsonDecode(resultStr);
      if (sessionID != _proxySessionID || !_isProxyRunning) return;

      final logs = result['logs'] as List?;
      if (logs != null) {
        for (var log in logs) {
          if (!_logController.isClosed) _logController.add(log.toString());
        }
      }

      final prevPromptTokens = _totalPromptTokens;
      final prevCompletionTokens = _totalCompletionTokens;
      final prevToolCalls = _totalToolCalls;
      final prevAuditTokens = _auditTokens;
      final prevAnalysisCount = _analysisCount;
      final prevBlockedCount = _blockedCount;
      final prevWarningCount = _warningCount;

      if (result['analysis_count'] != null) {
        _analysisCount = result['analysis_count'] as int;
      }
      if (result['blocked_count'] != null) {
        _blockedCount = result['blocked_count'] as int;
      }
      if (result['warning_count'] != null) {
        _warningCount = result['warning_count'] as int;
      }
      if (result['total_tokens'] != null) {
        _totalTokens = result['total_tokens'] as int;
      }
      if (result['total_prompt_tokens'] != null) {
        _totalPromptTokens = result['total_prompt_tokens'] as int;
      }
      if (result['total_completion_tokens'] != null) {
        _totalCompletionTokens = result['total_completion_tokens'] as int;
      }
      if (result['total_tool_calls'] != null) {
        _totalToolCalls = result['total_tool_calls'] as int;
      }
      if (result['request_count'] != null) {
        _requestCount = result['request_count'] as int;
      }
      if (result['audit_tokens'] != null) {
        _auditTokens = result['audit_tokens'] as int;
      }
      if (result['audit_prompt_tokens'] != null) {
        _auditPromptTokens = result['audit_prompt_tokens'] as int;
      }
      if (result['audit_completion_tokens'] != null) {
        _auditCompletionTokens = result['audit_completion_tokens'] as int;
      }

      final deltaPromptTokens = _deltaWithReset(
        _totalPromptTokens,
        prevPromptTokens,
      );
      final deltaCompletionTokens = _deltaWithReset(
        _totalCompletionTokens,
        prevCompletionTokens,
      );
      final deltaToolCalls = _deltaWithReset(_totalToolCalls, prevToolCalls);
      final deltaAuditTokens = _deltaWithReset(_auditTokens, prevAuditTokens);
      final deltaAnalysisCount = _deltaWithReset(
        _analysisCount,
        prevAnalysisCount,
      );
      final deltaBlockedCount = _deltaWithReset(
        _blockedCount,
        prevBlockedCount,
      );
      final deltaWarningCount = _deltaWithReset(
        _warningCount,
        prevWarningCount,
      );

      final hasTokenChanges =
          deltaPromptTokens > 0 ||
          deltaCompletionTokens > 0 ||
          deltaToolCalls > 0 ||
          deltaAuditTokens > 0;
      final hasStatChanges =
          deltaAnalysisCount > 0 ||
          deltaBlockedCount > 0 ||
          deltaWarningCount > 0;

      if (hasTokenChanges || hasStatChanges) {
        final metrics = ApiMetrics(
          timestamp: DateTime.now(),
          promptTokens: deltaPromptTokens,
          completionTokens: deltaCompletionTokens,
          totalTokens: deltaPromptTokens + deltaCompletionTokens,
          toolCallCount: deltaToolCalls,
          model: 'unknown',
          isBlocked: deltaBlockedCount > 0,
        );
        _metricsController.add(metrics);
        if (hasTokenChanges) {
          unawaited(
            MetricsDatabaseService()
                .saveApiMetrics(
                  metrics,
                  assetName: _assetName,
                  assetID: _assetID,
                )
                .catchError((Object e) {
                  appLogger.error(
                    '[ProtectionMonitor] Failed to save API metrics',
                    e,
                  );
                }),
          );
        }
        unawaited(_saveStatisticsToDatabase());
      }
    } catch (e) {
      appLogger.error('[ProtectionMonitor] Poll proxy logs error', e);
    }
  }

  AuditLogQueryResult getAuditLogsFromBuffer({
    int limit = 100,
    int offset = 0,
    bool riskOnly = false,
  }) {
    try {
      final dylib = _getDylib();
      final getAuditLogs = dylib
          .lookupFunction<GetAuditLogsC, GetAuditLogsDart>('GetAuditLogs');
      final resultPtr = getAuditLogs(limit, offset, riskOnly ? 1 : 0);
      final resultJson = resultPtr.toDartString();
      final freeString = dylib.lookupFunction<FreeStringC, FreeStringDart>(
        'FreeString',
      );
      freeString(resultPtr);
      final result = jsonDecode(resultJson) as Map<String, dynamic>;
      return AuditLogQueryResult.fromJson(result);
    } catch (e) {
      appLogger.error('[ProtectionMonitor] Get audit logs failed', e);
      return AuditLogQueryResult(logs: [], total: 0);
    }
  }

  Future<int> syncPendingAuditLogs() async {
    try {
      final dylib = _getDylib();
      final getPendingLogs = dylib
          .lookupFunction<GetPendingAuditLogsC, GetPendingAuditLogsDart>(
            'GetPendingAuditLogs',
          );
      final resultPtr = getPendingLogs();
      final resultJson = resultPtr.toDartString();
      final freeString = dylib.lookupFunction<FreeStringC, FreeStringDart>(
        'FreeString',
      );
      freeString(resultPtr);

      final logsJson = jsonDecode(resultJson) as List<dynamic>;
      if (logsJson.isEmpty) return 0;
      final logs = logsJson
          .map((e) => AuditLog.fromJson(e as Map<String, dynamic>))
          .toList();
      await AuditLogDatabaseService().saveAuditLogsBatch(logs);
      return logs.length;
    } catch (e) {
      appLogger.error('[ProtectionMonitor] Sync audit logs failed', e);
      return 0;
    }
  }

  void clearAuditLogsBuffer() {
    try {
      final dylib = _getDylib();
      final clearLogs = dylib
          .lookupFunction<ClearAuditLogsC, ClearAuditLogsDart>(
            'ClearAuditLogs',
          );
      final resultPtr = clearLogs();
      final freeString = dylib.lookupFunction<FreeStringC, FreeStringDart>(
        'FreeString',
      );
      freeString(resultPtr);
    } catch (e) {
      appLogger.error('[ProtectionMonitor] Clear audit logs buffer failed', e);
    }
  }

  void clearAuditLogsBufferWithFilter({
    String? assetName,
    String? assetID,
  }) {
    final hasAssetFilter =
        (assetName != null && assetName.isNotEmpty) ||
        (assetID != null && assetID.isNotEmpty);
    if (!hasAssetFilter) {
      clearAuditLogsBuffer();
      return;
    }

    try {
      final dylib = _getDylib();
      final clearLogs = dylib.lookupFunction<
        ClearAuditLogsWithFilterC,
        ClearAuditLogsWithFilterDart
      >('ClearAuditLogsWithFilter');
      final freeString = dylib.lookupFunction<FreeStringC, FreeStringDart>(
        'FreeString',
      );
      final argPtr = jsonEncode({
        if (assetName != null && assetName.isNotEmpty) 'asset_name': assetName,
        if (assetID != null && assetID.isNotEmpty) 'asset_id': assetID,
      }).toNativeUtf8();
      final resultPtr = clearLogs(argPtr);
      freeString(resultPtr);
      malloc.free(argPtr);
    } catch (e) {
      appLogger.error(
        '[ProtectionMonitor] Clear filtered audit logs buffer failed',
        e,
      );
    }
  }

  Future<List<AuditLog>> getAuditLogs({
    int limit = 100,
    int offset = 0,
    bool riskOnly = false,
    String? assetName,
    String? assetID,
    DateTime? startTime,
    DateTime? endTime,
    String? searchQuery,
  }) async {
    final effectiveAssetID =
        (assetID != null && assetID.trim().isNotEmpty)
            ? assetID.trim()
            : (_assetID.isNotEmpty ? _assetID : null);
    final effectiveAssetName =
        (assetName != null && assetName.trim().isNotEmpty)
            ? assetName.trim()
            : (((_assetName ?? '').isNotEmpty) ? _assetName : null);

    return await AuditLogDatabaseService().getAuditLogs(
      limit: limit,
      offset: offset,
      riskOnly: riskOnly,
      assetName: effectiveAssetName,
      assetID: effectiveAssetID,
      startTime: startTime,
      endTime: endTime,
      searchQuery: searchQuery,
    );
  }

  Future<int> getAuditLogCount({
    bool riskOnly = false,
    String? assetName,
    String? assetID,
  }) async {
    final effectiveAssetID =
        (assetID != null && assetID.trim().isNotEmpty)
            ? assetID.trim()
            : (_assetID.isNotEmpty ? _assetID : null);
    final effectiveAssetName =
        (assetName != null && assetName.trim().isNotEmpty)
            ? assetName.trim()
            : (((_assetName ?? '').isNotEmpty) ? _assetName : null);
    return await AuditLogDatabaseService().getAuditLogCount(
      riskOnly: riskOnly,
      assetName: effectiveAssetName,
      assetID: effectiveAssetID,
    );
  }

  Future<Map<String, dynamic>> getAuditLogStatistics({
    String? assetName,
    String? assetID,
  }) async {
    final effectiveAssetID =
        (assetID != null && assetID.trim().isNotEmpty)
            ? assetID.trim()
            : (_assetID.isNotEmpty ? _assetID : null);
    final effectiveAssetName =
        (assetName != null && assetName.trim().isNotEmpty)
            ? assetName.trim()
            : (((_assetName ?? '').isNotEmpty) ? _assetName : null);
    return await AuditLogDatabaseService().getAuditLogStatistics(
      assetName: effectiveAssetName,
      assetID: effectiveAssetID,
    );
  }

  Future<void> clearAllAuditLogs() async {
    await AuditLogDatabaseService().clearAllAuditLogs();
  }

  Future<void> clearAuditLogs({
    String? assetName,
    String? assetID,
  }) async {
    final effectiveAssetID =
        (assetID != null && assetID.trim().isNotEmpty)
            ? assetID.trim()
            : (_assetID.isNotEmpty ? _assetID : null);
    final effectiveAssetName =
        (assetName != null && assetName.trim().isNotEmpty)
            ? assetName.trim()
            : (((_assetName ?? '').isNotEmpty) ? _assetName : null);
    await AuditLogDatabaseService().clearAuditLogs(
      assetName: effectiveAssetName,
      assetID: effectiveAssetID,
    );
  }

  /// 从 Go 缓冲拉取待持久化的安全事件，写入 SQLite 并通知 UI 流。
  Future<int> syncPendingSecurityEvents() async {
    try {
      final dylib = _getDylib();
      final getPendingEvents = dylib
          .lookupFunction<
            GetPendingSecurityEventsC,
            GetPendingSecurityEventsDart
          >('GetPendingSecurityEvents');
      final resultPtr = getPendingEvents();
      final resultJson = resultPtr.toDartString();
      final freeString = dylib.lookupFunction<FreeStringC, FreeStringDart>(
        'FreeString',
      );
      freeString(resultPtr);

      final eventsJson = jsonDecode(resultJson) as List<dynamic>;
      if (eventsJson.isEmpty) return 0;
      final events = eventsJson
          .map((e) => SecurityEvent.fromJson(e as Map<String, dynamic>))
          .toList();
      await SecurityEventDatabaseService().saveSecurityEventsBatch(events);
      final matchedEvents = events.where(_matchesCurrentAsset).toList();
      if (matchedEvents.isNotEmpty && !_securityEventController.isClosed) {
        _securityEventController.add(matchedEvents);
      }
      return events.length;
    } catch (e) {
      appLogger.error('[ProtectionMonitor] Sync security events failed', e);
      return 0;
    }
  }

  /// 从数据库加载安全事件
  Future<List<SecurityEvent>> getSecurityEvents({
    int limit = 100,
    int offset = 0,
  }) async {
    return await SecurityEventDatabaseService().getSecurityEvents(
      limit: limit,
      offset: offset,
      assetName: _assetName ?? '',
      assetID: _assetID,
    );
  }

  /// 获取安全事件总数
  Future<int> getSecurityEventCount() async {
    return await SecurityEventDatabaseService().getSecurityEventCount();
  }

  /// 清空所有安全事件
  Future<void> clearAllSecurityEvents() async {
    await SecurityEventDatabaseService().clearSecurityEvents(
      assetName: _assetName ?? '',
      assetID: _assetID,
    );
  }

  void dispose({bool removeInstance = true}) {
    stopProxyLogPolling();
    _logSubscription?.cancel();
    _metricsSubscription?.cancel();
    _securityEventBridgeSubscription?.cancel();
    _messageBridgeService?.dispose();
    _saveStatisticsToDatabase();
    _logController.close();
    _resultController.close();
    _metricsController.close();
    _securityEventController.close();
    if (removeInstance) {
      _instances.remove(_instanceKey);
    }
  }
}
