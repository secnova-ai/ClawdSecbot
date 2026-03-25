import 'dart:convert';
import 'dart:ffi' as ffi;
import 'dart:isolate';
import 'package:ffi/ffi.dart';

import '../models/llm_config_model.dart';
import '../models/audit_log_model.dart';
import '../models/protection_analysis_model.dart';
import '../models/protection_config_model.dart';
import '../models/security_event_model.dart';
import '../utils/locale_utils.dart';
import 'model_config_database_service.dart';
import 'metrics_database_service.dart';
import 'native_library_service.dart' hide FreeStringDart;
import 'protection_database_service.dart';
import 'protection_monitor_service.dart';
import 'protection_proxy_ffi.dart';
import '../utils/app_logger.dart';

/// 防护服务：统一管理代理启停、配置热更新、环境同步与监控协调。
///
/// 合并了代理生命周期管理与协调编排，内部持有 [ProtectionMonitorService]
/// 负责日志/事件/统计/审计。UI 层仅依赖本服务。
class ProtectionService {
  static const String defaultInstanceKey = '__global__';
  static final Map<String, ProtectionService> _instances = {};

  static String normalizeInstanceKey(String? instanceKey) {
    final key = instanceKey?.trim() ?? '';
    return key.isEmpty ? defaultInstanceKey : key;
  }

  static String buildAssetScopedInstanceKey(String _, [String assetID = '']) {
    final normalizedAssetID = assetID.trim();
    if (normalizedAssetID.isNotEmpty) {
      return 'asset::$normalizedAssetID';
    }
    return defaultInstanceKey;
  }

  factory ProtectionService([String instanceKey = defaultInstanceKey]) {
    final normalizedKey = normalizeInstanceKey(instanceKey);
    return _instances.putIfAbsent(
      normalizedKey,
      () => ProtectionService._internal(normalizedKey),
    );
  }

  factory ProtectionService.scoped(String instanceKey) =>
      ProtectionService(instanceKey);

  factory ProtectionService.forAsset(String assetName, [String assetID = '']) =>
      ProtectionService(buildAssetScopedInstanceKey(assetName, assetID));

  ProtectionService._internal(this._instanceKey) {
    _monitor = ProtectionMonitorService(_instanceKey);
  }

  final String _instanceKey;
  late final ProtectionMonitorService _monitor;

  // === 代理状态 ===
  String? _assetName;
  String _assetID = '';
  bool _isProxyRunning = false;
  int? _proxyPort;
  String? _proxyURL;
  String? _providerName;
  String? _proxySessionID;
  String? _originalBaseURL;

  // === 对外属性 ===
  String? get assetName => _assetName;
  String get assetID => _assetID;
  bool get isProxyRunning => _isProxyRunning;
  int? get proxyPort => _proxyPort;
  String? get proxyURL => _proxyURL;
  String? get providerName => _providerName;
  bool get useCallbackBridge => _monitor.useCallbackBridge;

  // === 监控流透传 ===
  Stream<String> get logStream => _monitor.logStream;
  Stream<ProtectionAnalysisResult> get resultStream => _monitor.resultStream;
  Stream<ApiMetrics> get metricsStream => _monitor.metricsStream;
  Stream<List<SecurityEvent>> get securityEventStream =>
      _monitor.securityEventStream;

  // === 统计透传 ===
  bool get isAnalyzing => _monitor.isAnalyzing;
  int get analysisCount => _monitor.analysisCount;
  int get blockedCount => _monitor.blockedCount;
  int get warningCount => _monitor.warningCount;
  DateTime? get lastAnalysisTime => _monitor.lastAnalysisTime;
  int get totalTokens => _monitor.totalTokens;
  int get totalPromptTokens => _monitor.totalPromptTokens;
  int get totalCompletionTokens => _monitor.totalCompletionTokens;
  int get totalToolCalls => _monitor.totalToolCalls;
  int get requestCount => _monitor.requestCount;
  int get auditTokens => _monitor.auditTokens;
  int get auditPromptTokens => _monitor.auditPromptTokens;
  int get auditCompletionTokens => _monitor.auditCompletionTokens;

  void setAssetName(String name, [String assetID = '']) {
    final normalizedName = name.trim();
    final normalizedAssetID = assetID.trim();
    _assetName = normalizedName;
    _assetID = normalizedAssetID;
    _monitor.setAssetName(normalizedName, normalizedAssetID);
  }

  bool get _hasAssetBinding => _assetID.isNotEmpty;

  // === 内部工具 ===

  ffi.DynamicLibrary _getDylib() {
    final dylib = NativeLibraryService().dylib;
    if (dylib == null) {
      throw Exception('Plugin library not loaded');
    }
    return dylib;
  }

  String? _getLibraryPath() => NativeLibraryService().libraryPath;

  // ============================================================
  // 代理生命周期
  // ============================================================

  /// 启动防护代理,协调基线统计加载、代理启动、日志/事件推送与轮询。
  /// [securityConfig] 安全模型配置,用于 ShepherdGate 风险检测
  /// [runtimeConfig] 运行时配置
  Future<Map<String, dynamic>> startProtectionProxy(
    SecurityModelConfig securityConfig,
    ProtectionRuntimeConfig runtimeConfig, {
    int? proxyPort,
  }) async {
    appLogger.info(
      '[Protection] startProtectionProxy called: asset=$_assetName, auditOnly=${runtimeConfig.auditOnly}',
    );

    // 切换资产实例时必须重新加载该资产的基线统计，避免复用上一资产数据。
    if (_assetName != null) {
      await _monitor.loadStatisticsFromDatabase();
    }

    final statistics = ProtectionBaselineStatistics(
      analysisCount: _monitor.baselineAnalysisCount,
      blockedCount: _monitor.baselineBlockedCount,
      warningCount: _monitor.baselineWarningCount,
      totalTokens: _monitor.baselineTotalTokens,
      totalPromptTokens: _monitor.baselineTotalPromptTokens,
      totalCompletionTokens: _monitor.baselineTotalCompletionTokens,
      totalToolCalls: _monitor.baselineTotalToolCalls,
      requestCount: _monitor.baselineRequestCount,
      auditTokens: _monitor.baselineAuditTokens,
      auditPromptTokens: _monitor.baselineAuditPromptTokens,
      auditCompletionTokens: _monitor.baselineAuditCompletionTokens,
    );

    _monitor.addLog(jsonEncode({'key': 'dart_proxy_starting'}));

    Map<String, dynamic> result;
    try {
      result = await _startProxy(
        securityConfig,
        runtimeConfig,
        statistics,
        proxyPort: proxyPort,
      );
    } catch (e) {
      appLogger.error('[Protection] Start proxy error', e);
      _monitor.addLog(
        jsonEncode({
          'key': 'dart_proxy_error',
          'params': {'error': e.toString()},
        }),
      );
      return {'success': false, 'error': e.toString()};
    }

    if (result['success'] == true) {
      _monitor.addLog(
        jsonEncode({
          'key': 'dart_proxy_started',
          'params': {
            'port': result['port'],
            'provider': result['provider_name'],
          },
        }),
      );
      _monitor.setProxySession(result['session_id']?.toString(), true);
      _monitor.startProxyLogPolling();
      return result;
    } else {
      _monitor.addLog(
        jsonEncode({
          'key': 'dart_proxy_failed',
          'params': {'error': result['error']},
        }),
      );
      return result;
    }
  }

  /// 停止防护代理。
  Future<Map<String, dynamic>> stopProtectionProxy() async {
    appLogger.info('[Protection] stopProtectionProxy called');
    _monitor.addLog(jsonEncode({'key': 'dart_proxy_stopping'}));

    try {
      final result = await _stopProxy();
      _monitor.stopProxyLogPolling();
      _monitor.setProxySession(null, false);
      _monitor.addLog(jsonEncode({'key': 'dart_proxy_stopped'}));
      return result;
    } catch (e) {
      appLogger.error('[Protection] Stop proxy error', e);
      _monitor.stopProxyLogPolling();
      _monitor.setProxySession(null, false);
      return {'success': false, 'error': e.toString()};
    }
  }

  /// 完全重置服务状态。
  Future<void> fullReset() async {
    appLogger.info('[Protection] fullReset called');
    _monitor.clearAuditLogsBuffer();
    await _monitor.resetProxyStatistics();
    if (_isProxyRunning) {
      await _stopProxy();
    }
    _monitor.stopProxyLogPolling();
    _monitor.setProxySession(null, false);
    _monitor.resetMemoryState();
    appLogger.info('[Protection] Service fully reset');
  }

  // ============================================================
  // 代理内部执行（原 ProtectionProxyService 逻辑）
  // ============================================================

  /// 实际执行代理启动的内部方法。
  Future<Map<String, dynamic>> _startProxy(
    SecurityModelConfig securityConfig,
    ProtectionRuntimeConfig runtimeConfig,
    ProtectionBaselineStatistics statistics, {
    int? proxyPort,
  }) async {
    bool effectiveAuditOnly = runtimeConfig.auditOnly;
    int effectiveSingleSessionTokenLimit =
        runtimeConfig.singleSessionTokenLimit;
    int effectiveDailyTokenLimit = runtimeConfig.dailyTokenLimit;
    if (_assetName != null) {
      try {
        final protConfig = await ProtectionDatabaseService()
            .getProtectionConfig(_assetName!, _assetID);
        if (protConfig != null) {
          effectiveAuditOnly = protConfig.auditOnly;
          effectiveSingleSessionTokenLimit = protConfig.singleSessionTokenLimit;
          effectiveDailyTokenLimit = protConfig.dailyTokenLimit;
        }
      } catch (e) {
        appLogger.warning(
          '[Protection] Failed to read audit-only from protection config: $e',
        );
      }
    }

    int initialDailyTokenUsage = runtimeConfig.initialDailyTokenUsage;
    if (_assetName != null && effectiveDailyTokenLimit > 0) {
      initialDailyTokenUsage = await MetricsDatabaseService()
          .getDailyTokenUsage(_assetName!, _assetID);
    }

    final ProtectionRuntimeConfig finalRuntimeConfig = ProtectionRuntimeConfig(
      auditOnly: effectiveAuditOnly,
      singleSessionTokenLimit: effectiveSingleSessionTokenLimit,
      dailyTokenLimit: effectiveDailyTokenLimit,
      initialDailyTokenUsage: initialDailyTokenUsage,
    );

    final proxyStatus = await getProtectionProxyStatus();
    if (proxyStatus['running'] == true) {
      appLogger.info(
        '[Protection] Proxy already running, performing hot config update',
      );
      final updated = await updateRuntimeConfig(
        securityConfig,
        finalRuntimeConfig,
      );
      return {
        'success': true,
        'already_running': true,
        'config_updated': updated,
        'port': _proxyPort,
        'proxy_url': _proxyURL,
        'provider_name': _providerName,
      };
    }

    final effectiveLanguage = LocaleUtils.resolveLanguageCode();

    // 构建新的 ProtectionConfig 格式（与 Go 端结构对应）
    // 包含 security_model、bot_model、runtime 三个独立部分
    final botModelCfg = await _loadBotModelConfig();

    final configPayload = <String, dynamic>{
      // Asset identification for plugin lifecycle hooks
      'asset_name': _assetName,
      'asset_id': _assetID,

      // SecurityModel: 安全模型配置（用于 ShepherdGate 风险检测）
      'security_model': {
        'provider': securityConfig.provider,
        'endpoint': securityConfig.endpoint,
        'api_key': securityConfig.apiKey,
        'model': securityConfig.model,
        'secret_key': securityConfig.secretKey,
        'language': effectiveLanguage,
      },
      // BotModel: Bot 模型配置（用于代理转发目标）
      if (botModelCfg.isNotEmpty)
        'bot_model': {
          if (botModelCfg['provider']?.isNotEmpty == true)
            'provider': botModelCfg['provider'],
          if (botModelCfg['base_url']?.isNotEmpty == true)
            'base_url': botModelCfg['base_url'],
          if (botModelCfg['api_key']?.isNotEmpty == true)
            'api_key': botModelCfg['api_key'],
          if (botModelCfg['model']?.isNotEmpty == true)
            'model': botModelCfg['model'],
        },
      // Runtime: 防护运行时配置
      'runtime': {
        'proxy_port': proxyPort,
        'audit_only': finalRuntimeConfig.auditOnly,
        'single_session_token_limit':
            finalRuntimeConfig.singleSessionTokenLimit,
        'daily_token_limit': finalRuntimeConfig.dailyTokenLimit,
        'initial_daily_token_usage': finalRuntimeConfig.initialDailyTokenUsage,
      },
      // 基线统计（从数据库恢复的历史数据）
      'baseline_analysis_count': statistics.analysisCount,
      'baseline_blocked_count': statistics.blockedCount,
      'baseline_warning_count': statistics.warningCount,
      'baseline_total_tokens': statistics.totalTokens,
      'baseline_total_prompt_tokens': statistics.totalPromptTokens,
      'baseline_total_completion_tokens': statistics.totalCompletionTokens,
      'baseline_total_tool_calls': statistics.totalToolCalls,
      'baseline_request_count': statistics.requestCount,
      'baseline_audit_tokens': statistics.auditTokens,
      'baseline_audit_prompt_tokens': statistics.auditPromptTokens,
      'baseline_audit_completion_tokens': statistics.auditCompletionTokens,
    };

    final configJSON = jsonEncode(configPayload);

    final libPath = _getLibraryPath();
    if (libPath == null) {
      throw Exception('Plugin library not found');
    }

    final resultStr = await Isolate.run(() {
      return ProtectionProxyFFI.startProtectionProxyInIsolate(
        libPath,
        configJSON,
      );
    });

    final result = jsonDecode(resultStr);
    appLogger.info(
      '[Protection] Proxy start result: success=${result['success']}',
    );

    if (result['success'] == true) {
      _isProxyRunning = true;
      _proxyPort = result['port'];
      _proxyURL = result['proxy_url'];
      _providerName = result['provider_name'];
      _proxySessionID = result['session_id'];
      _originalBaseURL = result['original_base_url'];

      // 【架构变更】网关启动逻辑已内聚到 Go 层的 StartProtectionProxy 中
      // 不再需要显式调用 _applyOpenclawConfig()
      // Go 层在代理启动成功后会自动：
      // 1. 更新 openclaw.json（将 Bot 模型配置指向本地代理）
      // 2. 重启 openclaw gateway
      // 3. 同步沙箱配置
      appLogger.info(
        '[Protection] Proxy started, gateway will be started automatically by Go layer',
      );

      try {
        await updateRuntimeConfig(securityConfig, finalRuntimeConfig);
      } catch (_) {}

      return {
        'success': true,
        'port': _proxyPort,
        'proxy_url': _proxyURL,
        'provider_name': _providerName,
        'target_url': result['target_url'],
        'original_base_url': _originalBaseURL,
        'session_id': _proxySessionID,
      };
    } else {
      return {'success': false, 'error': result['error']};
    }
  }

  /// 实际执行代理停止的内部方法。
  Future<Map<String, dynamic>> _stopProxy() async {
    if (!_isProxyRunning) {
      appLogger.info('[Protection] stopProxy skipped: proxy not running');
      return {'success': true, 'message': 'Proxy not running'};
    }

    appLogger.info('[Protection] Stopping proxy...');
    try {
      final dylib = _getDylib();
      final freeString = dylib.lookupFunction<FreeStringC, FreeStringDart>(
        'FreeString',
      );
      ffi.Pointer<Utf8> resultPtr;
      if (_hasAssetBinding) {
        final stopProxy = dylib
            .lookupFunction<
              StopProtectionProxyByAssetC,
              StopProtectionProxyByAssetDart
            >('StopProtectionProxyByAsset');
        final assetNamePtr = _assetName!.toNativeUtf8();
        final assetIDPtr = _assetID.toNativeUtf8();
        resultPtr = stopProxy(assetNamePtr, assetIDPtr);
        malloc.free(assetNamePtr);
        malloc.free(assetIDPtr);
      } else {
        final stopProxy = dylib
            .lookupFunction<StopProtectionProxyC, StopProtectionProxyDart>(
              'StopProtectionProxy',
            );
        resultPtr = stopProxy();
      }
      final resultStr = resultPtr.toDartString();
      freeString(resultPtr);

      final result = jsonDecode(resultStr);
      appLogger.info('[Protection] Proxy stopped successfully');
      return result;
    } catch (e) {
      appLogger.error('[Protection] Stop proxy error', e);
      return {'success': false, 'error': e.toString()};
    } finally {
      _isProxyRunning = false;
      _proxyPort = null;
      _proxyURL = null;
      _providerName = null;
      _proxySessionID = null;
    }
  }

  Future<Map<String, String>> _loadBotModelConfig() async {
    try {
      final assetName = _assetName ?? 'openclaw';
      var botConfig = await ModelConfigDatabaseService().getBotModelConfig(
        assetName,
        _assetID,
      );
      if ((botConfig == null || botConfig.baseUrl.isEmpty) &&
          _assetID.isNotEmpty) {
        // Backward-compatible fallback: use legacy asset-level config when
        // instance-specific bot model has not been configured yet.
        botConfig = await ModelConfigDatabaseService().getBotModelConfig(
          assetName,
          '',
        );
      }
      if (botConfig != null && botConfig.baseUrl.isNotEmpty) {
        appLogger.info(
          '[Protection] Loaded bot model config: provider=${botConfig.provider}, baseUrl=${botConfig.baseUrl}, model=${botConfig.model}',
        );
        return {
          'provider': botConfig.provider,
          'base_url': botConfig.baseUrl,
          if (botConfig.apiKey.isNotEmpty) 'api_key': botConfig.apiKey,
          if (botConfig.model.isNotEmpty) 'model': botConfig.model,
        };
      }
      return {};
    } catch (e) {
      appLogger.error('[Protection] Load bot_model_config failed', e);
      return {};
    }
  }

  // 【已移除】_applyOpenclawConfig() 方法
  // 网关启动逻辑已内聚到 Go 层的 StartProtectionProxy 中
  // 当代理启动时，Go 层会自动完成：
  // 1. 更新 openclaw.json（将 Bot 模型配置指向本地代理）
  // 2. 重启 openclaw gateway
  // 3. 同步沙箱配置

  // ============================================================
  // 配置热更新
  // ============================================================

  Future<bool> setAuditOnly(bool auditOnly) async {
    try {
      final dylib = _getDylib();
      final freeString = dylib.lookupFunction<FreeStringC, FreeStringDart>(
        'FreeString',
      );
      ffi.Pointer<Utf8> resultPtr;
      if (_hasAssetBinding) {
        final setAuditOnlyFunc = dylib
            .lookupFunction<
              SetProtectionProxyAuditOnlyByAssetC,
              SetProtectionProxyAuditOnlyByAssetDart
            >('SetProtectionProxyAuditOnlyByAsset');
        final assetNamePtr = _assetName!.toNativeUtf8();
        final assetIDPtr = _assetID.toNativeUtf8();
        resultPtr = setAuditOnlyFunc(
          assetNamePtr,
          assetIDPtr,
          auditOnly ? 1 : 0,
        );
        malloc.free(assetNamePtr);
        malloc.free(assetIDPtr);
      } else {
        final setAuditOnlyFunc = dylib
            .lookupFunction<
              SetProtectionProxyAuditOnlyC,
              SetProtectionProxyAuditOnlyDart
            >('SetProtectionProxyAuditOnly');
        resultPtr = setAuditOnlyFunc(auditOnly ? 1 : 0);
      }
      final resultStr = resultPtr.toDartString();
      freeString(resultPtr);

      final result = jsonDecode(resultStr);
      if (result['success'] == true) {
        appLogger.info('[Protection] Audit-only mode set to: $auditOnly');
        return true;
      }
      appLogger.error(
        '[Protection] Failed to set audit-only: ${result['error']}',
      );
      return false;
    } catch (e) {
      appLogger.error('[Protection] Set audit-only error', e);
      return false;
    }
  }

  Future<bool> updateAuditOnlyMode(
    String assetName,
    bool auditOnly, [
    String assetID = '',
  ]) async {
    appLogger.info(
      '[Protection] updateAuditOnlyMode: asset=$assetName, auditOnly=$auditOnly',
    );
    try {
      final existing = await ProtectionDatabaseService().getProtectionConfig(
        assetName,
        assetID,
      );
      final resolvedAssetID = assetID.isNotEmpty
          ? assetID
          : (existing?.assetID ?? '');
      final config =
          (existing ??
                  ProtectionConfig.defaultConfig(
                    assetName,
                  ).copyWith(assetID: resolvedAssetID))
              .copyWith(assetID: resolvedAssetID, auditOnly: auditOnly);
      await ProtectionDatabaseService().saveProtectionConfig(config);
      setAssetName(assetName, resolvedAssetID);
      final result = await setAuditOnly(auditOnly);
      if (!result) {
        appLogger.warning(
          '[Protection] Saved to DB but failed to notify Go proxy (proxy may not be running)',
        );
      }
      return true;
    } catch (e) {
      appLogger.error('[Protection] updateAuditOnlyMode failed', e);
      return false;
    }
  }

  Future<bool> pushTokenLimitsToProxy({
    required String assetName,
    String assetID = '',
    required int singleSessionTokenLimit,
    required int dailyTokenLimit,
  }) async {
    try {
      final dylib = _getDylib();
      final freeString = dylib.lookupFunction<FreeStringC, FreeStringDart>(
        'FreeString',
      );

      int initialDailyTokenUsage = 0;
      if (dailyTokenLimit > 0 && assetName.isNotEmpty) {
        initialDailyTokenUsage = await MetricsDatabaseService()
            .getDailyTokenUsage(assetName, assetID);
      }

      final configJson = jsonEncode({
        'single_session_token_limit': singleSessionTokenLimit,
        'daily_token_limit': dailyTokenLimit,
        'initial_daily_token_usage': initialDailyTokenUsage,
      });
      final configPtr = configJson.toNativeUtf8();
      ffi.Pointer<Utf8> resultPtr;
      if (assetID.isNotEmpty) {
        final updateConfig = dylib
            .lookupFunction<
              UpdateProtectionConfigByAssetC,
              UpdateProtectionConfigByAssetDart
            >('UpdateProtectionConfigByAsset');
        final assetNamePtr = assetName.toNativeUtf8();
        final assetIDPtr = assetID.toNativeUtf8();
        resultPtr = updateConfig(assetNamePtr, assetIDPtr, configPtr);
        malloc.free(assetNamePtr);
        malloc.free(assetIDPtr);
      } else if (_hasAssetBinding) {
        final updateConfig = dylib
            .lookupFunction<
              UpdateProtectionConfigByAssetC,
              UpdateProtectionConfigByAssetDart
            >('UpdateProtectionConfigByAsset');
        final assetNamePtr = _assetName!.toNativeUtf8();
        final assetIDPtr = _assetID.toNativeUtf8();
        resultPtr = updateConfig(assetNamePtr, assetIDPtr, configPtr);
        malloc.free(assetNamePtr);
        malloc.free(assetIDPtr);
      } else {
        final updateConfig = dylib
            .lookupFunction<
              UpdateProtectionConfigC,
              UpdateProtectionConfigDart
            >('UpdateProtectionConfig');
        resultPtr = updateConfig(configPtr);
      }
      malloc.free(configPtr);

      final resultStr = resultPtr.toDartString();
      freeString(resultPtr);

      final result = jsonDecode(resultStr);
      if (result['success'] == true) {
        appLogger.info(
          '[Protection] Token limits pushed: singleSession=$singleSessionTokenLimit, daily=$dailyTokenLimit',
        );
        return true;
      }
      appLogger.warning(
        '[Protection] Failed to push token limits: ${result['error']}',
      );
      return false;
    } catch (e) {
      appLogger.error('[Protection] pushTokenLimitsToProxy error', e);
      return false;
    }
  }

  /// 同步网关沙箱配置（从数据库读取最新配置，完整重启网关以应用新策略）。
  /// 当用户在防护运行中修改沙箱开关或权限设置时调用。
  /// 使用后台 Isolate 执行，避免阻塞 UI 线程。
  Future<bool> syncGatewaySandbox() async {
    try {
      final libPath = _getLibraryPath();
      if (libPath == null) {
        appLogger.error(
          '[Protection] syncGatewaySandbox: library path not available',
        );
        return false;
      }

      final resultStr = await Isolate.run(() {
        if (_hasAssetBinding) {
          return ProtectionProxyFFI.syncGatewaySandboxByAssetInIsolate(
            libPath,
            _assetName!,
            _assetID,
          );
        }
        return ProtectionProxyFFI.syncGatewaySandboxInIsolate(libPath);
      });

      final result = jsonDecode(resultStr);
      if (result['success'] == true) {
        appLogger.info(
          '[Protection] Gateway sandbox synced: modified=${result['modified']}',
        );
        return true;
      }
      appLogger.warning(
        '[Protection] Failed to sync gateway sandbox: ${result['error']}',
      );
      return false;
    } catch (e) {
      appLogger.error('[Protection] syncGatewaySandbox error', e);
      return false;
    }
  }

  /// Bot 模型配置变更后，完整重启防护代理。
  ///
  /// Bot 模型变更需要完整重启链：停止代理 → 重新读取 Bot 配置 → 启动代理
  /// （Go 层 StartProtectionProxy 会自动完成 openclaw.json 更新和 gateway 重启）。
  /// 安全模型变更无需此方法，仅需 [updateSecurityModelConfig] 热更新即可。
  Future<Map<String, dynamic>> restartProtectionProxyForBotModelUpdate(
    SecurityModelConfig securityConfig,
    ProtectionRuntimeConfig runtimeConfig,
  ) async {
    appLogger.info(
      '[Protection] restartProtectionProxyForBotModelUpdate called: asset=$_assetName',
    );

    if (!_isProxyRunning) {
      appLogger.info(
        '[Protection] Proxy not running, bot model will apply on next start',
      );
      return {'success': true, 'message': 'proxy_not_running'};
    }

    // 停止当前代理
    final stopResult = await stopProtectionProxy();
    if (stopResult['success'] != true) {
      appLogger.error(
        '[Protection] Failed to stop proxy for bot model update: ${stopResult['error']}',
      );
      return {
        'success': false,
        'error': 'failed to stop proxy: ${stopResult['error']}',
      };
    }

    // 重新启动代理（会自动从 DB 读取最新 Bot 配置，并触发完整的 gateway 重启链）
    return await startProtectionProxy(securityConfig, runtimeConfig);
  }

  /// 更新运行时配置（热更新）
  /// 仅更新运行时参数，不涉及模型配置变更
  Future<bool> updateRuntimeConfig(
    SecurityModelConfig securityConfig,
    ProtectionRuntimeConfig runtimeConfig,
  ) async {
    try {
      final effectiveLanguage = LocaleUtils.resolveLanguageCode();

      final dylib = _getDylib();
      final freeString = dylib.lookupFunction<FreeStringC, FreeStringDart>(
        'FreeString',
      );

      // 只传递运行时相关配置
      final configJSON = jsonEncode({
        'language': effectiveLanguage,
        'audit_only': runtimeConfig.auditOnly,
        'single_session_token_limit': runtimeConfig.singleSessionTokenLimit,
        'daily_token_limit': runtimeConfig.dailyTokenLimit,
        'initial_daily_token_usage': runtimeConfig.initialDailyTokenUsage,
      });
      final configPtr = configJSON.toNativeUtf8();
      ffi.Pointer<Utf8> resultPtr;
      if (_hasAssetBinding) {
        final updateConfig = dylib
            .lookupFunction<
              UpdateProtectionConfigByAssetC,
              UpdateProtectionConfigByAssetDart
            >('UpdateProtectionConfigByAsset');
        final assetNamePtr = _assetName!.toNativeUtf8();
        final assetIDPtr = _assetID.toNativeUtf8();
        resultPtr = updateConfig(assetNamePtr, assetIDPtr, configPtr);
        malloc.free(assetNamePtr);
        malloc.free(assetIDPtr);
      } else {
        final updateConfig = dylib
            .lookupFunction<
              UpdateProtectionConfigC,
              UpdateProtectionConfigDart
            >('UpdateProtectionConfig');
        resultPtr = updateConfig(configPtr);
      }
      malloc.free(configPtr);

      final resultStr = resultPtr.toDartString();
      freeString(resultPtr);

      final result = jsonDecode(resultStr);
      if (result['success'] == true) {
        appLogger.info('[Protection] Runtime config updated successfully');
        return true;
      }
      appLogger.error(
        '[Protection] Failed to update runtime config: ${result['error']}',
      );
      return false;
    } catch (e) {
      appLogger.error('[Protection] Update runtime config error', e);
      return false;
    }
  }

  /// 更新安全模型配置（热更新）
  /// 用于更新 ShepherdGate 风险检测使用的 LLM 配置
  Future<bool> updateSecurityModelConfig(SecurityModelConfig config) async {
    try {
      final dylib = _getDylib();
      final freeString = dylib.lookupFunction<FreeStringC, FreeStringDart>(
        'FreeString',
      );

      final configJSON = jsonEncode(config.toJson());
      final configPtr = configJSON.toNativeUtf8();
      ffi.Pointer<Utf8> resultPtr;
      if (_hasAssetBinding) {
        final updateSecModel = dylib
            .lookupFunction<
              UpdateSecurityModelConfigByAssetC,
              UpdateSecurityModelConfigByAssetDart
            >('UpdateSecurityModelConfigByAsset');
        final assetNamePtr = _assetName!.toNativeUtf8();
        final assetIDPtr = _assetID.toNativeUtf8();
        resultPtr = updateSecModel(assetNamePtr, assetIDPtr, configPtr);
        malloc.free(assetNamePtr);
        malloc.free(assetIDPtr);
      } else {
        final updateSecModel = dylib
            .lookupFunction<
              UpdateSecurityModelConfigC,
              UpdateSecurityModelConfigDart
            >('UpdateSecurityModelConfig');
        resultPtr = updateSecModel(configPtr);
      }
      malloc.free(configPtr);

      final resultStr = resultPtr.toDartString();
      freeString(resultPtr);

      final result = jsonDecode(resultStr);
      if (result['success'] == true) {
        appLogger.info('[Protection] Security model config updated');
        return true;
      }
      appLogger.error(
        '[Protection] Failed to update security model config: ${result['error']}',
      );
      return false;
    } catch (e) {
      appLogger.error('[Protection] Update security model config error', e);
      return false;
    }
  }

  Future<Map<String, dynamic>> getProtectionProxyStatus() async {
    try {
      final dylib = _getDylib();
      final freeString = dylib.lookupFunction<FreeStringC, FreeStringDart>(
        'FreeString',
      );
      ffi.Pointer<Utf8> resultPtr;
      if (_hasAssetBinding) {
        final getStatus = dylib
            .lookupFunction<
              GetProtectionProxyStatusByAssetC,
              GetProtectionProxyStatusByAssetDart
            >('GetProtectionProxyStatusByAsset');
        final assetNamePtr = _assetName!.toNativeUtf8();
        final assetIDPtr = _assetID.toNativeUtf8();
        resultPtr = getStatus(assetNamePtr, assetIDPtr);
        malloc.free(assetNamePtr);
        malloc.free(assetIDPtr);
      } else {
        final getStatus = dylib
            .lookupFunction<
              GetProtectionProxyStatusC,
              GetProtectionProxyStatusDart
            >('GetProtectionProxyStatus');
        resultPtr = getStatus();
      }
      final resultStr = resultPtr.toDartString();
      freeString(resultPtr);

      final result = jsonDecode(resultStr);
      _isProxyRunning = result['running'] == true;
      if (_isProxyRunning) {
        _proxyPort = result['port'];
        _proxyURL = result['proxy_url'];
        _providerName = result['provider_name'];
        _originalBaseURL = result['original_base_url'];
      }
      return result;
    } catch (e) {
      appLogger.error('[Protection] Get status error', e);
      return {'running': false, 'error': e.toString()};
    }
  }

  // ============================================================
  // 环境同步（已移除）
  // ============================================================

  // 【已移除】syncProtectionEnvironment() 方法
  // 网关环境同步逻辑已内聚到 Go 层的 StartProtectionProxy 中
  // 当代理启动时，Go 层会自动从数据库读取防护配置并同步到网关环境

  // ============================================================
  // 监控委托
  // ============================================================

  Future<bool> enableCallbackBridge() async => _monitor.enableCallbackBridge();

  void disableCallbackBridge() => _monitor.disableCallbackBridge();

  Future<void> loadStatisticsFromDatabase() async =>
      _monitor.loadStatisticsFromDatabase();

  void resetMemoryState() => _monitor.resetMemoryState();

  Future<void> resetProxyStatistics() async => _monitor.resetProxyStatistics();

  void resetStatistics() => _monitor.resetStatistics();

  Future<ApiStatistics> getApiStatistics({Duration? duration}) async =>
      _monitor.getApiStatistics(duration: duration);

  void recordApiMetricsManually({
    required int promptTokens,
    required int completionTokens,
    required int toolCallCount,
    required String model,
    bool isBlocked = false,
    String? riskLevel,
  }) => _monitor.recordApiMetricsManually(
    promptTokens: promptTokens,
    completionTokens: completionTokens,
    toolCallCount: toolCallCount,
    model: model,
    isBlocked: isBlocked,
    riskLevel: riskLevel,
  );

  void dispose({bool removeInstance = true}) {
    _monitor.dispose(removeInstance: removeInstance);
    if (removeInstance) {
      _instances.remove(_instanceKey);
    }
  }

  AuditLogQueryResult getAuditLogsFromBuffer({
    int limit = 100,
    int offset = 0,
    bool riskOnly = false,
  }) => _monitor.getAuditLogsFromBuffer(
    limit: limit,
    offset: offset,
    riskOnly: riskOnly,
  );

  Future<int> syncPendingAuditLogs() async => _monitor.syncPendingAuditLogs();

  void clearAuditLogsBuffer() => _monitor.clearAuditLogsBuffer();

  Future<List<AuditLog>> getAuditLogs({
    int limit = 100,
    int offset = 0,
    bool riskOnly = false,
    DateTime? startTime,
    DateTime? endTime,
    String? searchQuery,
  }) async => _monitor.getAuditLogs(
    limit: limit,
    offset: offset,
    riskOnly: riskOnly,
    startTime: startTime,
    endTime: endTime,
    searchQuery: searchQuery,
  );

  Future<int> getAuditLogCount({bool riskOnly = false}) async =>
      _monitor.getAuditLogCount(riskOnly: riskOnly);

  Future<Map<String, dynamic>> getAuditLogStatistics() async =>
      _monitor.getAuditLogStatistics();

  Future<void> clearAllAuditLogs() async => _monitor.clearAllAuditLogs();

  // === 安全事件委托 ===

  Future<int> syncPendingSecurityEvents() async =>
      _monitor.syncPendingSecurityEvents();

  Future<List<SecurityEvent>> getSecurityEvents({
    int limit = 100,
    int offset = 0,
  }) async => _monitor.getSecurityEvents(limit: limit, offset: offset);

  Future<int> getSecurityEventCount() async => _monitor.getSecurityEventCount();

  Future<void> clearAllSecurityEvents() async =>
      _monitor.clearAllSecurityEvents();

  // ============================================================
  // 配置恢复
  // ============================================================

  /// 检查是否存在初始备份文件
  Future<bool> hasInitialBackup() async {
    try {
      final dylib = _getDylib();
      late final ffi.Pointer<Utf8> Function() hasBackupFunc;
      ffi.Pointer<Utf8>? assetNamePtr;
      ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>)? hasBackupByAssetFunc;
      if (_hasAssetBinding) {
        hasBackupByAssetFunc = dylib
            .lookupFunction<
              HasInitialBackupByAssetFFIC,
              HasInitialBackupByAssetFFIDart
            >('HasInitialBackupByAssetFFI');
      } else {
        hasBackupFunc = dylib
            .lookupFunction<HasInitialBackupFFIC, HasInitialBackupFFIDart>(
              'HasInitialBackupFFI',
            );
      }
      final freeString = dylib.lookupFunction<FreeStringC, FreeStringDart>(
        'FreeString',
      );

      final ffi.Pointer<Utf8> resultPtr;
      if (_hasAssetBinding) {
        assetNamePtr = _assetName!.toNativeUtf8();
        resultPtr = hasBackupByAssetFunc!(assetNamePtr);
      } else {
        resultPtr = hasBackupFunc();
      }
      if (assetNamePtr != null) {
        malloc.free(assetNamePtr);
      }
      final resultStr = resultPtr.toDartString();
      freeString(resultPtr);

      final result = jsonDecode(resultStr);
      if (result['success'] == true) {
        return result['exists'] == true;
      }
      return false;
    } catch (e) {
      appLogger.error('[Protection] hasInitialBackup error', e);
      return false;
    }
  }

  /// 恢复 openclaw.json 到初始配置状态并重启 gateway
  Future<Map<String, dynamic>> restoreToInitialConfig() async {
    appLogger.info('[Protection] restoreToInitialConfig called');

    try {
      // 先停止代理
      if (_isProxyRunning) {
        await _stopProxy();
        _monitor.stopProxyLogPolling();
        _monitor.setProxySession(null, false);
      }

      final libPath = _getLibraryPath();
      if (libPath == null) {
        throw Exception('Plugin library not found');
      }

      // 通过 Isolate 执行恢复操作，避免阻塞 UI 线程
      final resultStr = await Isolate.run(() {
        if (_hasAssetBinding) {
          return ProtectionProxyFFI.restoreToInitialConfigByAssetInIsolate(
            libPath,
            _assetName!,
          );
        }
        return ProtectionProxyFFI.restoreToInitialConfigInIsolate(libPath);
      });

      final result = jsonDecode(resultStr) as Map<String, dynamic>;
      appLogger.info(
        '[Protection] restoreToInitialConfig result: success=${result['success']}',
      );

      if (result['success'] == true) {
        // 清除保护状态
        if (_assetName != null) {
          try {
            await ProtectionDatabaseService().setProtectionEnabled(
              _assetName!,
              false,
              _assetID,
            );
          } catch (_) {}
        }
      }

      return result;
    } catch (e) {
      appLogger.error('[Protection] restoreToInitialConfig error', e);
      return {'success': false, 'error': e.toString()};
    }
  }
}
