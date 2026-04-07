import 'dart:async';
import 'dart:convert';
import 'dart:ffi' as ffi;
import 'dart:io';
import 'dart:ui' show AppExitResponse;
import 'package:ffi/ffi.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_animate/flutter_animate.dart';
import '../utils/app_fonts.dart';
import 'package:launch_at_startup/launch_at_startup.dart';
import 'package:lucide_icons/lucide_icons.dart';
import 'package:provider/provider.dart';
import 'package:tray_manager/tray_manager.dart';
import 'package:window_manager/window_manager.dart';
import '../constants.dart';
import '../models/llm_config_model.dart';
import '../config/build_config.dart';
import '../l10n/app_localizations.dart';
import '../models/asset_model.dart';
import '../models/protection_config_model.dart';
import '../models/risk_model.dart';
import '../providers/locale_provider.dart';
import '../services/app_settings_database_service.dart';
import '../services/scanner_service.dart';
import '../services/protection_service.dart';
import '../services/audit_log_database_service.dart';
import '../services/database_service.dart';
import '../services/metrics_database_service.dart';
import '../services/model_config_database_service.dart';
import '../services/protection_database_service.dart';
import '../services/scan_database_service.dart';
import '../services/bookmark_service.dart';
import '../services/native_library_service.dart';
import '../services/plugin_service.dart';
import '../services/api_service.dart';
import '../services/api_shutdown_service.dart';
import '../utils/app_logger.dart';
import '../widgets/mitigation_dialog.dart';
import '../widgets/settings_dialog.dart';
import '../widgets/onboarding_dialog.dart';
import '../widgets/skill_scan_dialog.dart';
import '../widgets/skill_scan_results_dialog.dart';
import '../widgets/protection_config_dialog.dart';
import '../widgets/scan_result_view.dart';
import '../widgets/welcome_overlay.dart';
import '../widgets/onboarding_completion_overlay.dart';
import '../utils/locale_utils.dart';
import 'mixins/main_page_tray_mixin.dart';
import 'mixins/main_page_version_mixin.dart';
import 'mixins/main_page_window_mixin.dart';
import 'mixins/main_page_data_mixin.dart';

class MainPage extends StatefulWidget {
  const MainPage({super.key});

  @override
  State<MainPage> createState() => _MainPageState();
}

class _MainPageState extends State<MainPage>
    with
        TrayListener,
        WindowListener,
        MainPageTrayMixin,
        MainPageVersionMixin,
        MainPageWindowMixin,
        MainPageDataMixin {
  // ============ 扫描状态 ============
  ScanState _scanState = ScanState.idle;
  final List<String> _logs = [];
  ScanResult? _result;
  final BotScanner _scanner = BotScanner();
  StreamSubscription<String>? _logSubscription;

  // ============ 防护状态 ============
  final Set<String> _protectedAssetIDs = {};
  final Map<String, String> _protectedAssetNamesByID = {};

  /// 启动时后台恢复防护中，期间防护监控按钮显示 loading，防护配置按钮禁用
  bool _isRestoringProtection = false;

  // ============ 引导和欢迎状态 ============
  bool _hasConfigAccess = false;
  bool _showOnboarding = false;
  bool _showOnboardingCompletionOverlay = false;
  bool _showWelcome = true;
  bool _startupFlowStarted = false;
  Timer? _onboardingCompletionTimer;
  AppLifecycleListener? _appExitListener;
  bool _isExitFlowInProgress = false;
  static const MethodChannel _appExitChannel = MethodChannel(
    'com.clawdbot.guard/app_exit',
  );

  // ============ 开机启动 ============
  bool _launchAtStartupEnabled = false;
  int _scheduledScanIntervalSeconds = 0;
  Timer? _scheduledScanTimer;

  // ============ API Server 状态 ============
  bool _apiServerEnabled = false;
  bool _isApiServerToggling = false;
  Timer? _externalStateRefreshTimer;
  bool _isExternalStateRefreshing = false;
  StreamSubscription<ApiShutdownRequest>? _apiShutdownSubscription;

  /// Future tracking heavy initialization (DB, plugins, etc.)
  Future<void>? _initFuture;

  /// 待应用的扫描结果（在 welcome sequence 结束后应用）
  ScanResult? _pendingScanResult;

  // ============ Mixin 接口实现 ============

  // MainPageTrayMixin 需要的接口
  @override
  bool get launchAtStartupEnabled => _launchAtStartupEnabled;
  @override
  set launchAtStartupEnabled(bool value) =>
      setState(() => _launchAtStartupEnabled = value);

  @override
  void handleExitFromTray() async {
    await _requestAppExit();
  }

  // MainPageDataMixin 需要的接口
  @override
  Set<String> get protectedAssets => _protectedAssetIDs;
  @override
  bool get isRestoringProtection => _isRestoringProtection;
  @override
  set isRestoringProtection(bool value) =>
      setState(() => _isRestoringProtection = value);

  @override
  void resetUIStateAfterClear() {
    setState(() {
      _scanState = ScanState.idle;
      _logs.clear();
      _result = null;
      _protectedAssetIDs.clear();
      _protectedAssetNamesByID.clear();
    });
  }

  @override
  void clearProtectedAssetMappings() {
    _protectedAssetNamesByID.clear();
  }

  @override
  void onConfigAccessChanged(bool hasAccess) {
    setState(() {
      _hasConfigAccess = hasAccess;
    });
  }

  @override
  Future<bool> showConfigAccessDialogForReauthorize() {
    return _showConfigAccessDialog();
  }

  // ============ 生命周期方法 ============

  @override
  void initState() {
    super.initState();
    trayManager.addListener(this);
    windowManager.addListener(this);
    _appExitChannel.setMethodCallHandler(_handleNativeAppExitCall);
    _appExitListener = AppLifecycleListener(
      onExitRequested: _handleAppExitRequest,
    );
    _init();
    _startExternalStateRefresh();
  }

  void _init() async {
    appLogger.info('[MainPage] Init start');
    await windowManager.setPreventClose(true);
    await initTray();

    // 初始化开机启动
    await _initLaunchAtStartup();

    // 默认访问标志（实际检查在欢迎屏幕后进行）
    _hasConfigAccess = !Platform.isMacOS || !BuildConfig.requiresDirectoryAuth;
    appLogger.info('[MainPage] Initial config access: $_hasConfigAccess');

    // 启动重量级初始化（与欢迎覆盖层并发运行）
    _initFuture = _performHeavyInit();

    // 帧构建后启动欢迎屏幕序列
    WidgetsBinding.instance.addPostFrameCallback((_) {
      _startWelcomeSequence();
      subscribeVersionUpdates();
    });
  }

  /// 执行重量级初始化任务（数据库、插件、扫描结果恢复）
  Future<void> _performHeavyInit() async {
    // 1. 计算数据库路径
    await DatabaseService().init();

    // 2. 加载 dylib + 初始化 Go 日志 + 初始化 Go 数据库
    try {
      await NativeLibraryService().initialize();
      await _syncProtectionLanguage();
      await _registerApiShutdownHandler();
    } catch (e) {
      appLogger.error('[MainPage] Failed to initialize native library', e);
    }

    // 3. 初始化插件专有配置
    try {
      await PluginService().initializePlugin();
    } catch (e) {
      appLogger.error('[MainPage] Failed to initialize plugin', e);
    }

    // 4. 启动维护任务（清理旧日志和指标）
    try {
      await MetricsDatabaseService().cleanOldApiMetrics();
      await AuditLogDatabaseService().cleanOldAuditLogs();
      appLogger.info('[MainPage] Maintenance completed (cleaned old logs)');
    } catch (e) {
      appLogger.error('[MainPage] Maintenance failed', e);
    }

    // 初始化 Shepherd 规则
    try {
      final enabledConfigs = await ProtectionDatabaseService()
          .getEnabledProtectionConfigs();
      if (enabledConfigs.isNotEmpty) {
        final config = enabledConfigs.first;
        await PluginService().loadAndSyncShepherdRules(
          config.assetName,
          config.assetID,
        );
      }
    } catch (e) {
      appLogger.error('[MainPage] Failed to init Shepherd rules', e);
    }

    // 检查现有扫描结果
    final savedResult = await ScanDatabaseService().getLatestScanResult();
    if (savedResult != null) {
      _pendingScanResult = savedResult;
      appLogger.info(
        '[MainPage] Loaded saved scan result with ${savedResult.risks.length} risks',
      );
    } else {
      appLogger.info('[MainPage] No saved scan result found');
    }

    await _restoreScheduledScanSettings();

    // 从数据库恢复已启用的防护状态
    await _restoreProtectionStates();

    // 启动版本检查服务
    await startVersionCheckService();

    // 初始化 API Server 状态
    await _initApiServerStatus();
  }

  Future<void> _syncProtectionLanguage() async {
    final localeProvider = context.read<LocaleProvider>();
    final savedLanguage = await AppSettingsDatabaseService().getLanguage();
    final effectiveLanguage = LocaleUtils.resolveLanguageCode(
      explicitLanguage: savedLanguage,
      savedLanguage: localeProvider.locale.languageCode,
    );

    if (localeProvider.locale.languageCode != effectiveLanguage) {
      await localeProvider.setLocale(Locale(effectiveLanguage));
    }

    await applyProtectionLanguage(effectiveLanguage);
    appLogger.info(
      '[MainPage] Protection language synchronized: $effectiveLanguage',
    );
  }

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    updateTray();
  }

  @override
  void dispose() {
    _appExitChannel.setMethodCallHandler(null);
    _appExitListener?.dispose();
    trayManager.removeListener(this);
    windowManager.removeListener(this);
    _logSubscription?.cancel();
    _onboardingCompletionTimer?.cancel();
    _scheduledScanTimer?.cancel();
    _externalStateRefreshTimer?.cancel();
    _apiShutdownSubscription?.cancel();
    unawaited(ApiShutdownService().dispose());
    disposeVersionMixin();
    stopVersionCheckService();
    _scanner.dispose();
    // 关闭 Go 插件
    PluginService().closePlugin();
    // 关闭 Go DB
    NativeLibraryService().close();
    super.dispose();
  }

  Future<dynamic> _handleNativeAppExitCall(MethodCall call) async {
    if (call.method == 'requestAppExit') {
      await _requestAppExit();
      return true;
    }
    return null;
  }

  Future<AppExitResponse> _handleAppExitRequest() async {
    await _requestAppExit();
    return AppExitResponse.cancel;
  }

  // ============ 初始化开机启动 ============

  Future<void> _initLaunchAtStartup() async {
    if (BuildConfig.isAppStore) {
      _launchAtStartupEnabled = false;
      appLogger.info('[MainPage] Launch at startup disabled for App Store');
      return;
    }

    try {
      launchAtStartup.setup(
        appName: 'ClawdSecbot',
        appPath: Platform.resolvedExecutable,
        packageName: 'com.ClawdSecbot.clawbot.guard',
      );
      _launchAtStartupEnabled = await launchAtStartup.isEnabled();
      appLogger.info('[MainPage] Launch at startup: $_launchAtStartupEnabled');
      if (mounted) {
        updateTray();
      }
    } catch (e) {
      appLogger.error('[MainPage] Failed to init launch at startup', e);
    }
  }

  // ============ 初始化 API Server 状态 ============

  Future<void> _initApiServerStatus() async {
    try {
      final shouldEnable = await AppSettingsDatabaseService()
          .getApiServerEnabled(defaultValue: true);
      await _applyApiServerState(
        shouldEnable,
        persistSetting: false,
        showFeedback: false,
      );
      appLogger.info('[MainPage] API Server preference loaded: $shouldEnable');
    } catch (e) {
      appLogger.error('[MainPage] Failed to init API Server status', e);
    }
  }

  // ============ 切换 API Server 状态 ============

  @override
  void onWindowFocus() {
    unawaited(_refreshUiStateFromDatabase(force: true));
  }

  @override
  void onWindowRestore() {
    unawaited(_refreshUiStateFromDatabase(force: true));
  }

  Future<void> _toggleApiServer(bool enable) async {
    await _applyApiServerState(
      enable,
      persistSetting: true,
      showFeedback: true,
    );
    if (DateTime.now().microsecond >= 0) {
      return;
    }
    try {
      final result = await ApiService().toggleServer(enable);

      if (result['success'] == true) {
        if (mounted) {
          setState(() {
            _apiServerEnabled = enable;
          });
          updateTray();
        }

        if (enable) {
          final port = result['port'] ?? 'unknown';
          final url = result['url'] ?? '';
          appLogger.info(
            '[MainPage] API Server started (port: $port, url: $url)',
          );

          if (mounted) {
            ScaffoldMessenger.of(context).showSnackBar(
              SnackBar(
                content: Text('API 服务已启动 - 端口：$port'),
                backgroundColor: const Color(0xFF22C55E),
                duration: const Duration(seconds: 2),
              ),
            );
          }
        } else {
          appLogger.info('[MainPage] API Server stopped');
          if (mounted) {
            ScaffoldMessenger.of(context).showSnackBar(
              SnackBar(
                content: const Text('API 服务已停止'),
                backgroundColor: const Color(0xFFEF4444),
                duration: const Duration(seconds: 2),
              ),
            );
          }
        }
      } else {
        final error = result['error'] ?? 'Unknown error';
        appLogger.error('[MainPage] Failed to toggle API Server: $error');
        if (mounted) {
          ScaffoldMessenger.of(context).showSnackBar(
            SnackBar(content: Text('操作失败：$error'), backgroundColor: Colors.red),
          );
        }
      }
    } catch (e) {
      appLogger.error('[MainPage] Failed to toggle API Server', e);
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('操作失败：$e'), backgroundColor: Colors.red),
        );
      }
    }
  }

  Future<void> _applyApiServerState(
    bool enable, {
    required bool persistSetting,
    required bool showFeedback,
  }) async {
    if (_isApiServerToggling) {
      appLogger.warning('[MainPage] API Server toggle is already in progress');
      return;
    }

    _isApiServerToggling = true;
    try {
      final status = await ApiService().checkStatus();
      Map<String, dynamic> result = {'success': true};
      if (enable != status.isRunning) {
        result = await ApiService().toggleServer(enable);
      }

      if (result['success'] != true) {
        final error = result['error'] ?? 'Unknown error';
        appLogger.error('[MainPage] Failed to toggle API Server: $error');
        if (showFeedback && mounted) {
          ScaffoldMessenger.of(context).showSnackBar(
            SnackBar(
              content: Text('Failed to toggle API service: $error'),
              backgroundColor: Colors.red,
            ),
          );
        }
        return;
      }

      if (persistSetting) {
        await AppSettingsDatabaseService().setApiServerEnabled(enable);
      }

      if (mounted) {
        setState(() {
          _apiServerEnabled = enable;
        });
        updateTray();
      } else {
        _apiServerEnabled = enable;
      }

      if (showFeedback && mounted) {
        if (enable) {
          final port = result['port'] ?? 'unknown';
          ScaffoldMessenger.of(context).showSnackBar(
            SnackBar(
              content: Text('API service started on port: $port'),
              backgroundColor: const Color(0xFF22C55E),
              duration: const Duration(seconds: 2),
            ),
          );
        } else {
          ScaffoldMessenger.of(context).showSnackBar(
            const SnackBar(
              content: Text('API service stopped'),
              backgroundColor: Color(0xFFEF4444),
              duration: Duration(seconds: 2),
            ),
          );
        }
      }
    } catch (e) {
      appLogger.error('[MainPage] Failed to apply API Server state', e);
      if (showFeedback && mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text('Failed to toggle API service: $e'),
            backgroundColor: Colors.red,
          ),
        );
      }
    } finally {
      _isApiServerToggling = false;
    }
  }

  @override
  Future<void> toggleLaunchAtStartup() async {
    try {
      if (_launchAtStartupEnabled) {
        await launchAtStartup.disable();
      } else {
        await launchAtStartup.enable();
      }
      _launchAtStartupEnabled = await launchAtStartup.isEnabled();
      if (mounted) {
        setState(() {});
        updateTray();
      }
      appLogger.info(
        '[MainPage] Launch at startup toggled: $_launchAtStartupEnabled',
      );
    } catch (e) {
      appLogger.error('[MainPage] Failed to toggle launch at startup', e);
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text('Failed to toggle launch at startup: $e'),
            backgroundColor: Colors.red,
          ),
        );
      }
    }
  }

  // ============ 防护状态恢复 ============

  Future<void> _restoreScheduledScanSettings() async {
    try {
      final seconds = await AppSettingsDatabaseService()
          .getScheduledScanIntervalSeconds();
      if (mounted) {
        setState(() {
          _scheduledScanIntervalSeconds = seconds;
        });
      } else {
        _scheduledScanIntervalSeconds = seconds;
      }
      _configureScheduledScanTimer();
      appLogger.info(
        '[MainPage] Restored scheduled scan interval: $_scheduledScanIntervalSeconds seconds',
      );
    } catch (e) {
      appLogger.error(
        '[MainPage] Failed to restore scheduled scan settings',
        e,
      );
    }
  }

  Future<void> _updateScheduledScanInterval(int seconds) async {
    final success = await AppSettingsDatabaseService()
        .setScheduledScanIntervalSeconds(seconds);
    if (!success) {
      appLogger.error(
        '[MainPage] Failed to save scheduled scan interval: $seconds',
      );
      return;
    }

    if (mounted) {
      setState(() {
        _scheduledScanIntervalSeconds = seconds;
      });
    } else {
      _scheduledScanIntervalSeconds = seconds;
    }

    _configureScheduledScanTimer();
    appLogger.info(
      '[MainPage] Scheduled scan interval updated: $_scheduledScanIntervalSeconds seconds',
    );
  }

  Future<void> _saveGeneralSettings({
    required bool launchAtStartupEnabled,
    required int scheduledScanIntervalSeconds,
  }) async {
    if (launchAtStartupEnabled != _launchAtStartupEnabled) {
      await toggleLaunchAtStartup();
    }
    if (scheduledScanIntervalSeconds != _scheduledScanIntervalSeconds) {
      await _updateScheduledScanInterval(scheduledScanIntervalSeconds);
    }
  }

  void _configureScheduledScanTimer() {
    _scheduledScanTimer?.cancel();
    _scheduledScanTimer = null;

    if (_scheduledScanIntervalSeconds <= 0) {
      appLogger.info('[MainPage] Scheduled scan disabled');
      return;
    }

    final interval = Duration(seconds: _scheduledScanIntervalSeconds);
    _scheduledScanTimer = Timer.periodic(interval, (_) {
      _startScan(triggeredByScheduler: true);
    });
    appLogger.info('[MainPage] Scheduled scan enabled: every $interval');
  }

  Future<void> _restoreProtectionStates() async {
    try {
      appLogger.info('[MainPage] Restoring protection states');
      final enabledConfigs = await ProtectionDatabaseService()
          .getEnabledProtectionConfigs();
      final stateChanged = await _syncProtectedAssetsFromConfigs(
        enabledConfigs,
        scanResult: _pendingScanResult ?? _result,
      );
      if (enabledConfigs.isEmpty) {
        appLogger.info('[MainPage] No enabled protection configs');
        return;
      }

      appLogger.info(
        '[MainPage] Restored ${enabledConfigs.length} protected assets, changed=$stateChanged',
      );

      if (mounted) {
        setState(() {
          _isRestoringProtection = true;
        });
      }

      // 后台启动代理
      _startProxyInBackground();
    } catch (e) {
      appLogger.error('[MainPage] Failed to restore protection states', e);
      if (mounted) {
        setState(() {
          _isRestoringProtection = false;
        });
      }
    }
  }

  String _resolveAssetIDFromScan(String assetName, [ScanResult? scanResult]) {
    final assets =
        scanResult?.assets ??
        _pendingScanResult?.assets ??
        _result?.assets ??
        const <Asset>[];
    final matches = assets
        .where((asset) => asset.name == assetName && asset.id.isNotEmpty)
        .toList();
    if (matches.isEmpty) {
      return '';
    }
    if (matches.length > 1) {
      appLogger.warning(
        '[MainPage] Multiple assets matched for empty assetID restore: $assetName, using first',
      );
    }
    return matches.first.id;
  }

  void _startExternalStateRefresh() {
    _externalStateRefreshTimer?.cancel();
    _externalStateRefreshTimer = Timer.periodic(const Duration(seconds: 3), (
      _,
    ) {
      unawaited(_refreshUiStateFromDatabase());
    });
  }

  String _scanResultSignature(ScanResult? result) {
    if (result == null) {
      return '';
    }
    return jsonEncode(result.toJson());
  }

  Future<bool> _syncProtectedAssetsFromConfigs(
    List<ProtectionConfig> enabledConfigs, {
    required ScanResult? scanResult,
  }) async {
    final nextProtectedAssetIDs = <String>{};
    final nextProtectedAssetNamesByID = <String, String>{};

    for (final config in enabledConfigs) {
      var resolvedAssetID = config.assetID;
      if (resolvedAssetID.isEmpty) {
        resolvedAssetID = _resolveAssetIDFromScan(config.assetName, scanResult);
        if (resolvedAssetID.isNotEmpty) {
          try {
            final modelService = ModelConfigDatabaseService();
            final oldBotModelConfig = await modelService.getBotModelConfig(
              config.assetName,
              config.assetID,
            );
            if (oldBotModelConfig != null) {
              await modelService.saveBotModelConfig(
                oldBotModelConfig.copyWith(assetID: resolvedAssetID),
              );
              await modelService.deleteBotModelConfig(
                config.assetName,
                config.assetID,
              );
            }

            await ProtectionDatabaseService().saveProtectionConfig(
              config.copyWith(assetID: resolvedAssetID),
            );
            await ProtectionDatabaseService().deleteProtectionConfig(
              config.assetName,
              config.assetID,
            );
            appLogger.info(
              '[MainPage] Migrated protection config assetID: ${config.assetName} -> $resolvedAssetID',
            );
          } catch (e) {
            appLogger.warning(
              '[MainPage] Failed to migrate empty assetID for ${config.assetName}: $e',
            );
          }
        }
      }

      if (resolvedAssetID.isEmpty) {
        appLogger.warning(
          '[MainPage] Skip restoring protection without assetID: ${config.assetName}',
        );
        continue;
      }

      nextProtectedAssetIDs.add(resolvedAssetID);
      nextProtectedAssetNamesByID[resolvedAssetID] = config.assetName;
    }

    final idsChanged = !setEquals(_protectedAssetIDs, nextProtectedAssetIDs);
    final namesChanged = !mapEquals(
      _protectedAssetNamesByID,
      nextProtectedAssetNamesByID,
    );
    if (!idsChanged && !namesChanged) {
      return false;
    }

    _protectedAssetIDs
      ..clear()
      ..addAll(nextProtectedAssetIDs);
    _protectedAssetNamesByID
      ..clear()
      ..addAll(nextProtectedAssetNamesByID);

    appLogger.info(
      '[MainPage] Enabled assets synced: ${_protectedAssetIDs.map((id) => '${_protectedAssetNamesByID[id]}/$id').join(', ')}',
    );
    return true;
  }

  Future<void> _refreshUiStateFromDatabase({bool force = false}) async {
    if (_isExternalStateRefreshing) {
      return;
    }

    if (!force && _scanState == ScanState.scanning) {
      return;
    }

    _isExternalStateRefreshing = true;
    try {
      final latestScanResult = await ScanDatabaseService()
          .getLatestScanResult();
      final currentScanResult = _showWelcome ? _pendingScanResult : _result;
      final scanChanged =
          latestScanResult != null &&
          _scanResultSignature(latestScanResult) !=
              _scanResultSignature(currentScanResult);

      final enabledConfigs = await ProtectionDatabaseService()
          .getEnabledProtectionConfigs();
      final protectedStateChanged = await _syncProtectedAssetsFromConfigs(
        enabledConfigs,
        scanResult: latestScanResult ?? _pendingScanResult ?? _result,
      );

      if (!mounted) {
        return;
      }

      if (scanChanged || protectedStateChanged) {
        setState(() {
          if (scanChanged) {
            if (_showWelcome) {
              _pendingScanResult = latestScanResult;
            } else {
              _result = latestScanResult;
              _scanState = ScanState.completed;
            }
          }
        });

        if (protectedStateChanged) {
          await notifyMonitorWindowsProtectionConfigReload();
        }

        appLogger.info(
          '[MainPage] UI state refreshed from database: scanChanged=$scanChanged, protectionChanged=$protectedStateChanged',
        );
      }
    } catch (e) {
      appLogger.warning('[MainPage] Failed to refresh UI state from DB: $e');
    } finally {
      _isExternalStateRefreshing = false;
    }
  }

  /// 按启用资产逐个恢复 proxy,每个资产幂等启动(已运行则跳过)。
  Future<void> _startProxyInBackground() async {
    try {
      final securityModelConfig = await ModelConfigDatabaseService()
          .getSecurityModelConfig();
      if (securityModelConfig == null) {
        appLogger.warning(
          '[MainPage] No security model config, skip background proxy start',
        );
        if (mounted) {
          setState(() {
            _isRestoringProtection = false;
          });
        }
        return;
      }

      for (final assetID in _protectedAssetIDs.toList()) {
        final assetName = _protectedAssetNamesByID[assetID] ?? '';
        if (assetName.isEmpty) continue;

        try {
          final service = ProtectionService.forAsset(assetName, assetID);
          service.setAssetName(assetName, assetID);

          // 始终调用 startProtectionProxy，即使代理已在 Go 层运行。
          // Go 层会为已运行的代理创建新 session 并返回 session_id，
          // 确保 Dart 侧能正确建立日志轮询。
          final result = await service.startProtectionProxy(
            securityModelConfig,
            ProtectionRuntimeConfig(),
          );
          appLogger.info(
            '[MainPage] Background proxy start for $assetName/$assetID: success=${result['success']}, already_running=${result['already_running']}',
          );
        } catch (e) {
          appLogger.error(
            '[MainPage] Background proxy start failed for $assetName/$assetID',
            e,
          );
        }
      }

      if (mounted) {
        setState(() {
          _isRestoringProtection = false;
        });
      }
    } catch (e) {
      appLogger.error('[MainPage] Background proxy start failed', e);
      if (mounted) {
        setState(() {
          _isRestoringProtection = false;
        });
      }
    }
  }

  // ============ 欢迎和引导流程 ============

  Future<void> _startWelcomeSequence() async {
    if (_startupFlowStarted) return;
    _startupFlowStarted = true;
    if (mounted) {
      setState(() {
        _showWelcome = true;
      });
    }

    final futures = <Future>[
      Future.delayed(const Duration(milliseconds: 1500)),
    ];
    if (_initFuture != null) {
      futures.add(_initFuture!);
    }
    await Future.wait(futures);
    if (!mounted) return;
    setState(() {
      _showWelcome = false;
      if (_pendingScanResult != null) {
        _result = _pendingScanResult;
        _scanState = ScanState.completed;
        _pendingScanResult = null;
        appLogger.info('[MainPage] Applied pending scan result');
      }
    });
    await _runStartupFlowAfterWelcome();
  }

  Future<void> _runStartupFlowAfterWelcome() async {
    final authorized = await _ensureHomeDirectoryAuthorization();
    if (!authorized) return;
    // 首次启动不再强制模型引导，模型配置在“一键防护”保存时按需触发。
  }

  Future<bool> _ensureHomeDirectoryAuthorization() async {
    if (!Platform.isMacOS || !BuildConfig.requiresDirectoryAuth) {
      return true;
    }

    try {
      final bookmarkService = BookmarkService();
      var authorized = await bookmarkService.initialize();
      if (!authorized) {
        authorized = await _requestHomeDirectoryAuthorization();
      }

      _hasConfigAccess = authorized;
      await _recordHomeAuthorization(
        authorized: authorized,
        authorizedPath: bookmarkService.authorizedPath,
      );

      if (!authorized) {
        await _exitDueToAuthorizationDenied();
        return false;
      }
      if (mounted) {
        setState(() {});
      }
      return true;
    } catch (e) {
      appLogger.error('[MainPage] Auth check failed', e);
      await _recordHomeAuthorization(authorized: false);
      await _exitDueToAuthorizationDenied();
      return false;
    }
  }

  Future<bool> _requestHomeDirectoryAuthorization() async {
    final path = await BookmarkService().selectAndStoreDirectory();
    if (path == null || path.isEmpty) {
      return false;
    }
    return await BookmarkService().startAccessingDirectory();
  }

  Future<void> _recordHomeAuthorization({
    required bool authorized,
    String? authorizedPath,
  }) async {
    try {
      final dylib = NativeLibraryService().dylib;
      final freeStr = NativeLibraryService().freeString;
      if (dylib != null && freeStr != null) {
        final func = dylib
            .lookupFunction<
              ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>),
              ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>)
            >('SaveHomeDirectoryPermissionFFI');
        final jsonStr = jsonEncode({
          'authorized': authorized,
          'authorized_path': authorizedPath ?? '',
        });
        final argPtr = jsonStr.toNativeUtf8();
        final resultPtr = func(argPtr);
        freeStr(resultPtr);
        malloc.free(argPtr);
      }
    } catch (e) {
      appLogger.error('[MainPage] Save auth result failed', e);
    }
  }

  Future<void> _exitDueToAuthorizationDenied() async {
    if (!mounted) return;
    final l10n = AppLocalizations.of(context)!;
    ScaffoldMessenger.of(
      context,
    ).showSnackBar(SnackBar(content: Text(l10n.authDeniedExit)));
    await Future.delayed(const Duration(seconds: 1));
    await windowManager.close();
    exit(0);
  }

  Future<void> _requestAppExit() async {
    await _requestAppExitWithOptions(interactive: true, restoreConfig: null);
  }

  Future<void> _requestAppExitWithOptions({
    required bool interactive,
    required bool? restoreConfig,
  }) async {
    if (_isExitFlowInProgress) {
      return;
    }
    _isExitFlowInProgress = true;

    try {
      final enabledConfigs = await _loadExitTargets();
      var shouldRestore = restoreConfig ?? false;
      if (enabledConfigs.isNotEmpty) {
        if (interactive && mounted) {
          await showWindow();
        }
        if (interactive) {
          final confirmed = await _showExitRestoreDialog(enabledConfigs.length);
          if (confirmed != true) {
            return;
          }
          shouldRestore = true;
        }
        if (shouldRestore) {
          final failures = await _runExitCleanupWithProgress(
            () => _restoreTargetsForExit(enabledConfigs),
          );
          if (failures.isNotEmpty) {
            if (interactive) {
              await _showExitFailureDialog(failures);
            } else {
              appLogger.error(
                '[MainPage] API-triggered exit restore failed: ${failures.join('; ')}',
              );
            }
            return;
          }
        }
      }

      await _finalizeAppExit();
    } finally {
      _isExitFlowInProgress = false;
    }
  }

  Future<void> _registerApiShutdownHandler() async {
    final registered = await ApiShutdownService().register();
    if (!registered) {
      return;
    }

    await _apiShutdownSubscription?.cancel();
    _apiShutdownSubscription = ApiShutdownService().requests.listen((request) {
      appLogger.info(
        '[MainPage] Received API shutdown request: restoreConfig=${request.restoreConfig}',
      );
      unawaited(
        _requestAppExitWithOptions(
          interactive: false,
          restoreConfig: request.restoreConfig,
        ),
      );
    });
  }

  Future<List<ProtectionConfig>> _loadExitTargets() async {
    final configs = await ProtectionDatabaseService()
        .getEnabledProtectionConfigs();
    if (configs.isNotEmpty) {
      return configs;
    }

    final fallbacks = <ProtectionConfig>[];
    for (final assetID in _protectedAssetIDs) {
      final assetName = _protectedAssetNamesByID[assetID];
      if (assetName == null || assetName.trim().isEmpty) {
        continue;
      }
      fallbacks.add(
        ProtectionConfig.defaultConfig(
          assetName,
        ).copyWith(assetID: assetID, enabled: true),
      );
    }
    return fallbacks;
  }

  Future<List<String>> _restoreTargetsForExit(
    List<ProtectionConfig> targets,
  ) async {
    final pluginService = PluginService();
    final failures = <String>[];

    for (final target in targets) {
      final assetName = target.assetName.trim();
      final assetID = target.assetID.trim();
      final assetLabel = assetID.isEmpty ? assetName : '$assetName ($assetID)';

      try {
        final exitResult = await pluginService.notifyPluginAppExit(
          assetName,
          assetID,
        );
        if (exitResult['success'] != true) {
          appLogger.warning(
            '[MainPage] Plugin exit callback failed for $assetLabel: ${exitResult['error']}',
          );
        }

        final protectionService = ProtectionService.forAsset(
          assetName,
          assetID,
        );
        protectionService.setAssetName(assetName, assetID);

        final proxyStatus = await protectionService.getProtectionProxyStatus();
        final wasRunning = proxyStatus['running'] == true;
        String? stopError;
        if (wasRunning) {
          final stopResult = await protectionService.stopProtectionProxy();
          if (stopResult['success'] != true) {
            stopError = '${stopResult['error'] ?? 'stop proxy failed'}';
          }
        }

        final requiresExplicitRestore =
            assetName.toLowerCase() == 'openclaw' || !wasRunning;
        if (requiresExplicitRestore) {
          final restoreResult = await pluginService.restoreBotDefaultState(
            assetName,
            assetID,
          );
          if (restoreResult['success'] != true) {
            failures.add(
              '$assetLabel: ${restoreResult['error'] ?? 'restore bot default state failed'}',
            );
            continue;
          }
        }
        if (stopError != null && !requiresExplicitRestore) {
          failures.add('$assetLabel: $stopError');
          continue;
        }

        await ProtectionDatabaseService().setProtectionEnabled(
          assetName,
          false,
          assetID,
        );
      } catch (e) {
        failures.add('$assetLabel: $e');
      }
    }

    return failures;
  }

  Future<T> _runExitCleanupWithProgress<T>(Future<T> Function() action) async {
    if (mounted) {
      unawaited(
        showDialog<void>(
          context: context,
          barrierDismissible: false,
          builder: (context) => AlertDialog(
            backgroundColor: const Color(0xFF1E1E2E),
            shape: RoundedRectangleBorder(
              borderRadius: BorderRadius.circular(12),
            ),
            content: Row(
              children: [
                const SizedBox(
                  width: 24,
                  height: 24,
                  child: CircularProgressIndicator(strokeWidth: 2.5),
                ),
                const SizedBox(width: 16),
                Expanded(
                  child: Text(
                    AppLocalizations.of(context)!.exitRestoreInProgress,
                    style: AppFonts.inter(fontSize: 14, color: Colors.white70),
                  ),
                ),
              ],
            ),
          ),
        ),
      );
      await Future.delayed(const Duration(milliseconds: 50));
    }

    try {
      return await action();
    } finally {
      if (mounted) {
        final navigator = Navigator.of(context, rootNavigator: true);
        if (navigator.canPop()) {
          navigator.pop();
        }
      }
    }
  }

  Future<bool?> _showExitRestoreDialog(int protectedCount) {
    final l10n = AppLocalizations.of(context)!;
    return showDialog<bool>(
      context: context,
      barrierDismissible: false,
      builder: (context) => AlertDialog(
        backgroundColor: const Color(0xFF1E1E2E),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
        title: Row(
          children: [
            Container(
              padding: const EdgeInsets.all(8),
              decoration: BoxDecoration(
                color: const Color(0xFFF59E0B).withValues(alpha: 0.2),
                borderRadius: BorderRadius.circular(8),
              ),
              child: const Icon(
                LucideIcons.shieldAlert,
                color: Color(0xFFF59E0B),
                size: 20,
              ),
            ),
            const SizedBox(width: 12),
            Expanded(
              child: Text(
                l10n.exitRestoreTitle,
                style: AppFonts.inter(
                  fontSize: 16,
                  fontWeight: FontWeight.w600,
                  color: Colors.white,
                ),
              ),
            ),
          ],
        ),
        content: Text(
          l10n.exitRestoreMessage(protectedCount),
          style: AppFonts.inter(fontSize: 14, color: Colors.white70),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(false),
            child: Text(
              l10n.cancel,
              style: AppFonts.inter(color: Colors.white54),
            ),
          ),
          ElevatedButton(
            onPressed: () => Navigator.of(context).pop(true),
            style: ElevatedButton.styleFrom(
              backgroundColor: const Color(0xFFF59E0B),
              shape: RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(8),
              ),
            ),
            child: Text(
              l10n.exitRestoreConfirm,
              style: AppFonts.inter(
                fontWeight: FontWeight.w600,
                color: Colors.white,
              ),
            ),
          ),
        ],
      ),
    );
  }

  Future<void> _showExitFailureDialog(List<String> failures) async {
    if (!mounted) {
      return;
    }
    final l10n = AppLocalizations.of(context)!;
    final message = failures.join('\n');
    await showDialog<void>(
      context: context,
      barrierDismissible: false,
      builder: (context) => AlertDialog(
        backgroundColor: const Color(0xFF1E1E2E),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
        title: Text(
          l10n.exitRestoreFailedTitle,
          style: AppFonts.inter(
            fontSize: 16,
            fontWeight: FontWeight.w600,
            color: Colors.white,
          ),
        ),
        content: Text(
          l10n.exitRestoreFailedMessage(message),
          style: AppFonts.inter(fontSize: 13, color: Colors.white70),
        ),
        actions: [
          ElevatedButton(
            onPressed: () => Navigator.of(context).pop(),
            child: Text(
              l10n.continueButton,
              style: AppFonts.inter(
                fontWeight: FontWeight.w600,
                color: Colors.white,
              ),
            ),
          ),
        ],
      ),
    );
  }

  Future<void> _finalizeAppExit() async {
    try {
      await PluginService().closePlugin();
    } catch (e) {
      appLogger.warning('[MainPage] Failed to close plugin during exit: $e');
    }

    try {
      await NativeLibraryService().close();
    } catch (e) {
      appLogger.warning(
        '[MainPage] Failed to close native library during exit: $e',
      );
    }

    try {
      await windowManager.setPreventClose(false);
    } catch (_) {}

    try {
      await windowManager.destroy();
    } catch (e) {
      appLogger.warning('[MainPage] Window destroy failed during exit: $e');
    }

    exit(0);
  }

  Future<void> _hideOnboarding() async {
    if (!mounted) return;
    setState(() {
      _showOnboarding = false;
    });
    _showOnboardingCompletion();

    if (!BuildConfig.isAppStore) {
      _launchAtStartupEnabled = await launchAtStartup.isEnabled();
      appLogger.info(
        '[MainPage] Launch at startup after onboarding: $_launchAtStartupEnabled',
      );
      updateTray();
    }
  }

  void _showOnboardingCompletion() {
    _onboardingCompletionTimer?.cancel();
    if (!mounted) return;
    setState(() {
      _showOnboardingCompletionOverlay = true;
    });
    _onboardingCompletionTimer = Timer(const Duration(milliseconds: 1800), () {
      if (!mounted) return;
      setState(() {
        _showOnboardingCompletionOverlay = false;
      });
    });
  }

  // ============ 设置和语言 ============

  Future<void> _handleSettingsTap() async {
    try {
      await _showSettingsDialog();
    } catch (e) {
      appLogger.error('[MainPage] Failed to handle settings tap', e);
    }
  }

  @override
  Future<void> applyProtectionLanguage(String language) async {
    final normalizedLanguage = LocaleUtils.normalizeLanguageCode(language);
    try {
      final updated = await AppSettingsDatabaseService().setLanguage(
        normalizedLanguage,
      );
      if (!updated) {
        appLogger.warning(
          '[MainPage] Failed to sync protection language: $normalizedLanguage',
        );
      }

      await updateVersionCheckLanguage(normalizedLanguage);

      if (_protectedAssetIDs.isNotEmpty) {
        final securityModelConfig = await ModelConfigDatabaseService()
            .getSecurityModelConfig();
        if (securityModelConfig != null) {
          final service = ProtectionService();
          await service.updateSecurityModelConfig(securityModelConfig);
        }
      }

      await notifyMonitorWindowsLanguageUpdate(normalizedLanguage);
    } catch (e) {
      appLogger.error('[MainPage] _applyProtectionLanguage error', e);
    }
  }

  // ============ 扫描功能 ============

  Future<void> _startScan({
    bool triggeredByScheduler = false,
    bool skipReconcile = false,
  }) async {
    if (_scanState == ScanState.scanning) {
      if (triggeredByScheduler) {
        appLogger.info(
          '[MainPage] Scheduled scan skipped because another scan is running',
        );
      }
      return;
    }

    if (Platform.isMacOS &&
        BuildConfig.requiresDirectoryAuth &&
        !_hasConfigAccess) {
      final authorized = await _showConfigAccessDialog();
      if (!authorized) return;
    }

    if (!mounted) {
      return;
    }

    final l10n = AppLocalizations.of(context)!;

    final beforeAssetIDs =
        _result?.assets.map((a) => a.id).toSet() ?? <String>{};

    setState(() {
      _scanState = ScanState.scanning;
      _logs.clear();
      _result = null;
    });

    await windowManager.setSize(AppConstants.windowSize);

    await _logSubscription?.cancel();
    _logSubscription = _scanner.logStream.listen((log) {
      if (!mounted) {
        return;
      }
      setState(() {
        _logs.add(log);
      });
    });

    if (!mounted) return;
    _scanner.setLocalization(l10n);

    try {
      final result = await _scanner.scan();

      try {
        await ScanDatabaseService().saveScanResult(result);
      } catch (e) {
        appLogger.error('[MainPage] Failed to save scan result', e);
      }

      if (!mounted) {
        return;
      }

      setState(() {
        _scanState = ScanState.completed;
        _result = result;
      });

      if (!skipReconcile) {
        try {
          await _reconcileAssetsAfterScan(beforeAssetIDs, result.assets);
        } catch (e) {
          appLogger.error('[MainPage] Reconcile failed unexpectedly: $e');
        }
      }

      try {
        await _syncProtectedAssetsWithScanResult(result.assets);
      } catch (e) {
        appLogger.error('[MainPage] Sync protected assets failed: $e');
      }
    } catch (e) {
      appLogger.error('[MainPage] Scan failed', e);
      if (mounted) {
        setState(() {
          _scanState = ScanState.idle;
        });
      }
    } finally {
      await _logSubscription?.cancel();
      _logSubscription = null;
      await windowManager.setSize(AppConstants.windowSize);
    }
  }

  /// 扫描后对账：回收真正消失的资产（名字也不存在于新结果中）的代理与防护状态。
  /// asset_id 变化但名字仍在的资产视为"重启中"，不回收。
  /// 每个资产独立 try/catch，单个失败不影响其余资产。
  Future<void> _reconcileAssetsAfterScan(
    Set<String> beforeAssetIDs,
    List<Asset> afterAssets,
  ) async {
    if (_protectedAssetIDs.isEmpty) return;

    final afterAssetIDs = afterAssets.map((a) => a.id).toSet();
    final afterAssetNames = afterAssets.map((a) => a.name).toSet();

    final idDisappeared = _protectedAssetIDs
        .where(
          (id) => beforeAssetIDs.contains(id) && !afterAssetIDs.contains(id),
        )
        .toList();

    if (idDisappeared.isEmpty) {
      appLogger.info(
        '[MainPage] Reconcile: all protected assets still present',
      );
      return;
    }

    final trulyMissing = idDisappeared.where((id) {
      final name = _protectedAssetNamesByID[id] ?? '';
      return name.isEmpty || !afterAssetNames.contains(name);
    }).toList();

    if (trulyMissing.isEmpty) {
      appLogger.info(
        '[MainPage] Reconcile: ${idDisappeared.length} asset ID(s) changed but names still present, skip teardown',
      );
      return;
    }

    appLogger.info(
      '[MainPage] Reconcile: ${trulyMissing.length} protected asset(s) truly disappeared: '
      '${trulyMissing.join(', ')}',
    );

    final removed = <String>[];

    for (final assetID in trulyMissing) {
      final assetName = _protectedAssetNamesByID[assetID] ?? '';
      final label = assetID.isEmpty ? assetName : '$assetName/$assetID';

      try {
        await PluginService().notifyPluginAppExit(assetName, assetID);
      } catch (e) {
        appLogger.warning(
          '[MainPage] Reconcile: plugin exit callback failed for $label: $e',
        );
      }

      try {
        final service = ProtectionService.forAsset(assetName, assetID);
        service.setAssetName(assetName, assetID);
        final stopResult = await service.stopProtectionProxy();
        appLogger.info(
          '[MainPage] Reconcile: stopped proxy for $label, success=${stopResult['success']}',
        );
      } catch (e) {
        appLogger.error(
          '[MainPage] Reconcile: failed to stop proxy for $label: $e',
        );
      }

      try {
        await ProtectionDatabaseService().setProtectionEnabled(
          assetName,
          false,
          assetID,
        );
      } catch (e) {
        appLogger.error(
          '[MainPage] Reconcile: failed to disable protection config for $label: $e',
        );
      }

      removed.add(assetID);
    }

    if (mounted && removed.isNotEmpty) {
      setState(() {
        for (final assetID in removed) {
          _protectedAssetIDs.remove(assetID);
          _protectedAssetNamesByID.remove(assetID);
        }
      });
    }

    appLogger.info(
      '[MainPage] Reconcile completed: removed ${removed.length}/${trulyMissing.length} asset(s)',
    );
  }

  /// 从 DB 重建 _protectedAssetIDs，按名字映射到扫描结果的最新 asset_id。
  /// 解决 bot 重启导致 asset_id 临时变化时 UI 匹配不上的问题。
  Future<void> _syncProtectedAssetsWithScanResult(List<Asset> assets) async {
    final enabledConfigs = await ProtectionDatabaseService()
        .getEnabledProtectionConfigs();

    if (enabledConfigs.isEmpty) {
      if (_protectedAssetIDs.isNotEmpty && mounted) {
        setState(() {
          _protectedAssetIDs.clear();
          _protectedAssetNamesByID.clear();
        });
      }
      return;
    }

    final scanIdByName = <String, String>{};
    final scanIdSet = <String>{};
    for (final asset in assets) {
      scanIdByName[asset.name] = asset.id;
      scanIdSet.add(asset.id);
    }

    final newIDs = <String>{};
    final newNames = <String, String>{};

    for (final config in enabledConfigs) {
      if (scanIdSet.contains(config.assetID)) {
        newIDs.add(config.assetID);
        newNames[config.assetID] = config.assetName;
      } else {
        final scanID = scanIdByName[config.assetName];
        if (scanID != null) {
          appLogger.info(
            '[MainPage] Sync: asset_id changed for ${config.assetName}: '
            '${config.assetID} -> $scanID',
          );
          newIDs.add(scanID);
          newNames[scanID] = config.assetName;
        } else {
          newIDs.add(config.assetID);
          newNames[config.assetID] = config.assetName;
        }
      }
    }

    if (mounted) {
      setState(() {
        _protectedAssetIDs.clear();
        _protectedAssetIDs.addAll(newIDs);
        _protectedAssetNamesByID.clear();
        _protectedAssetNamesByID.addAll(newNames);
      });
    }
  }

  Future<bool> _showConfigAccessDialog() async {
    final l10n = AppLocalizations.of(context)!;

    final result = await showDialog<bool>(
      context: context,
      barrierDismissible: false,
      builder: (context) => AlertDialog(
        backgroundColor: const Color(0xFF1A1A2E),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
        title: Row(
          children: [
            Container(
              padding: const EdgeInsets.all(8),
              decoration: BoxDecoration(
                color: const Color(0xFF6366F1).withValues(alpha: 0.2),
                borderRadius: BorderRadius.circular(8),
              ),
              child: const Icon(
                LucideIcons.folderOpen,
                color: Color(0xFF6366F1),
                size: 20,
              ),
            ),
            const SizedBox(width: 12),
            Text(
              l10n.configAccessTitle,
              style: AppFonts.inter(
                fontSize: 16,
                fontWeight: FontWeight.w600,
                color: Colors.white,
              ),
            ),
          ],
        ),
        content: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              l10n.configAccessMessage,
              style: AppFonts.inter(fontSize: 13, color: Colors.white70),
            ),
            const SizedBox(height: 16),
            Container(
              padding: const EdgeInsets.all(12),
              decoration: BoxDecoration(
                color: Colors.white.withValues(alpha: 0.05),
                borderRadius: BorderRadius.circular(8),
                border: Border.all(color: Colors.white.withValues(alpha: 0.1)),
              ),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    l10n.configAccessPaths,
                    style: AppFonts.inter(fontSize: 11, color: Colors.white54),
                  ),
                  const SizedBox(height: 8),
                  Text(
                    '~/.openclaw\n~/.moltbot\n~/.clawdbot',
                    style: AppFonts.firaCode(
                      fontSize: 12,
                      color: Colors.white70,
                    ),
                  ),
                  const SizedBox(height: 8),
                  Text(
                    '提示：您可以选择具体的配置目录,也可以选择整个用户主目录 (~)',
                    style: AppFonts.inter(
                      fontSize: 11,
                      color: Colors.amber.withValues(alpha: 0.7),
                      fontStyle: FontStyle.italic,
                    ),
                  ),
                ],
              ),
            ),
          ],
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(false),
            child: Text(
              l10n.cancel,
              style: AppFonts.inter(color: Colors.white54),
            ),
          ),
          ElevatedButton(
            onPressed: () async {
              final path = await BookmarkService().selectAndStoreDirectory();
              if (path != null) {
                await BookmarkService().startAccessingDirectory();
                if (context.mounted) {
                  Navigator.of(context).pop(true);
                }
              }
            },
            style: ElevatedButton.styleFrom(
              backgroundColor: const Color(0xFF6366F1),
              shape: RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(8),
              ),
            ),
            child: Text(
              l10n.selectDirectory,
              style: AppFonts.inter(
                fontWeight: FontWeight.w600,
                color: Colors.white,
              ),
            ),
          ),
        ],
      ),
    );

    if (result == true) {
      setState(() {
        _hasConfigAccess = true;
      });
      return true;
    }
    return false;
  }

  void _resetScan() async {
    final l10n = AppLocalizations.of(context)!;

    showDialog(
      context: context,
      barrierDismissible: false,
      builder: (context) => Center(
        child: Container(
          padding: const EdgeInsets.all(24),
          decoration: BoxDecoration(
            color: const Color(0xFF1E1E2E),
            borderRadius: BorderRadius.circular(12),
          ),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const SizedBox(
                width: 32,
                height: 32,
                child: CircularProgressIndicator(
                  strokeWidth: 3,
                  valueColor: AlwaysStoppedAnimation<Color>(Color(0xFF6366F1)),
                ),
              ),
              const SizedBox(height: 16),
              Text(
                l10n.checkingProtectionStatus,
                style: AppFonts.inter(fontSize: 14, color: Colors.white70),
              ),
            ],
          ),
        ),
      ),
    );

    try {
      final activeCount = await ProtectionDatabaseService()
          .getActiveProtectionCount();

      if (mounted) {
        Navigator.of(context).pop();
      }

      if (activeCount > 0) {
        if (!mounted) return;

        final confirmed = await showDialog<bool>(
          context: context,
          barrierDismissible: false,
          builder: (context) => AlertDialog(
            backgroundColor: const Color(0xFF1E1E2E),
            shape: RoundedRectangleBorder(
              borderRadius: BorderRadius.circular(12),
            ),
            title: Row(
              children: [
                Container(
                  padding: const EdgeInsets.all(8),
                  decoration: BoxDecoration(
                    color: const Color(0xFFEF4444).withValues(alpha: 0.2),
                    borderRadius: BorderRadius.circular(8),
                  ),
                  child: const Icon(
                    LucideIcons.alertTriangle,
                    color: Color(0xFFEF4444),
                    size: 20,
                  ),
                ),
                const SizedBox(width: 12),
                Text(
                  l10n.rescanConfirmTitle,
                  style: AppFonts.inter(
                    fontSize: 16,
                    fontWeight: FontWeight.w600,
                    color: Colors.white,
                  ),
                ),
              ],
            ),
            content: Text(
              l10n.rescanConfirmMessage(activeCount),
              style: AppFonts.inter(fontSize: 14, color: Colors.white70),
            ),
            actions: [
              TextButton(
                onPressed: () => Navigator.of(context).pop(false),
                child: Text(
                  l10n.cancel,
                  style: AppFonts.inter(color: Colors.white54),
                ),
              ),
              ElevatedButton(
                onPressed: () => Navigator.of(context).pop(true),
                style: ElevatedButton.styleFrom(
                  backgroundColor: const Color(0xFFEF4444),
                  shape: RoundedRectangleBorder(
                    borderRadius: BorderRadius.circular(8),
                  ),
                ),
                child: Text(
                  l10n.continueButton,
                  style: AppFonts.inter(
                    fontWeight: FontWeight.w600,
                    color: Colors.white,
                  ),
                ),
              ),
            ],
          ),
        );

        if (confirmed != true) {
          return;
        }

        try {
          final enabledConfigs = await ProtectionDatabaseService()
              .getEnabledProtectionConfigs();
          for (final config in enabledConfigs) {
            await ProtectionDatabaseService().setProtectionEnabled(
              config.assetName,
              false,
              config.assetID,
            );
          }
          _protectedAssetIDs.clear();
          _protectedAssetNamesByID.clear();
          appLogger.info('[MainPage] Stopped all protection for rescan');
        } catch (e) {
          appLogger.error('[MainPage] Failed to stop protections', e);
        }
      }

      if (mounted) {
        setState(() {
          _scanState = ScanState.idle;
          _logs.clear();
          _result = null;
        });
        await windowManager.setSize(AppConstants.windowSize);
      }
    } catch (e) {
      appLogger.error('[MainPage] Failed to check protection status', e);
      if (mounted) {
        Navigator.of(context).pop();
      }
    }
  }

  // ============ 防护监控窗口 ============

  /// 解析已启用防护配置中的有效 asset_id 后再打开监控窗口，避免用重扫后的临时 ID 绑定到错误实例。
  Future<void> _showProtectionMonitorResolved(Asset asset) async {
    final config = await ProtectionDatabaseService().getProtectionConfig(
      asset.name,
      asset.id,
    );
    final resolvedID = (config != null && config.assetID.isNotEmpty)
        ? config.assetID
        : asset.id;
    if (resolvedID != asset.id) {
      appLogger.info(
        '[MainPage] Resolved monitor asset_id: scan=${asset.id} -> config=$resolvedID',
      );
    }
    showProtectionMonitor(asset.name, resolvedID);
  }

  // ============ 防护配置 ============

  void _showProtectionConfigDialog(
    Asset asset, {
    required bool isEditMode,
  }) async {
    final result = await showDialog<ProtectionConfig>(
      context: context,
      builder: (context) => ProtectionConfigDialog(
        assetName: asset.name,
        assetID: asset.id,
        isEditMode: isEditMode,
      ),
    );

    if (result != null && !isEditMode) {
      setState(() {
        _protectedAssetIDs.add(asset.id);
        _protectedAssetNamesByID[asset.id] = asset.name;
      });
      _showProtectionMonitorResolved(asset);

      // Bot restarts after protection enable; delay-rescan to refresh card info.
      // skipReconcile: asset_id may change during restart, must not tear down proxy.
      Future.delayed(const Duration(seconds: 5), () {
        if (mounted && _scanState != ScanState.scanning) {
          _startScan(skipReconcile: true);
        }
      });
    } else if (result != null && isEditMode) {
      if (_protectedAssetIDs.contains(asset.id)) {
        if (!mounted) return;
        final l10n = AppLocalizations.of(context)!;

        if (Platform.isMacOS && BuildConfig.isPersonal) {
          if (mounted) {
            ScaffoldMessenger.of(context).showSnackBar(
              SnackBar(
                content: Text(result.sandboxEnabled ? '沙箱保护已应用' : '已切换为普通模式'),
                backgroundColor: Colors.green,
                duration: const Duration(seconds: 2),
              ),
            );
          }
          return;
        }

        try {
          final securityModelConfig = await ModelConfigDatabaseService()
              .getSecurityModelConfig();
          if (securityModelConfig != null) {
            final service = ProtectionService.forAsset(asset.name, asset.id);
            final updateResult = await service.updateSecurityModelConfig(
              securityModelConfig,
            );

            if (updateResult) {
              if (mounted) {
                ScaffoldMessenger.of(context).showSnackBar(
                  SnackBar(
                    content: Text(l10n.configUpdated),
                    backgroundColor: Colors.green,
                    duration: const Duration(seconds: 2),
                  ),
                );
              }
              return;
            }
          }
        } catch (e) {
          appLogger.error('[MainPage] Hot update failed', e);
        }
      }
    }
  }

  void _showMitigationDialog(BuildContext context, RiskInfo risk) async {
    final result = await showDialog(
      context: context,
      builder: (context) => MitigationDialog(risk: risk),
    );

    if (result == true) {
      if (mounted) {
        _startScan();
      }
    } else if (result is Map && result['action'] == 'skill_scan') {
      if (mounted) {
        _showSkillScanDialog();
      }
    }
  }

  void _showSkillScanDialog() async {
    await showDialog(
      context: context,
      barrierDismissible: false,
      builder: (context) => const SkillScanDialog(),
    );

    if (mounted) {
      _startScan();
    }
  }

  void _showSkillScanResultsDialog() {
    showDialog(
      context: context,
      builder: (context) => const SkillScanResultsDialog(),
    );
  }

  Future<void> _showSettingsDialog() async {
    await showDialog(
      context: context,
      builder: (dialogContext) => SettingsDialog(
        launchAtStartupEnabled: _launchAtStartupEnabled,
        onSaveGeneralSettings: _saveGeneralSettings,
        scheduledScanIntervalSeconds: _scheduledScanIntervalSeconds,
        onClearData: () {
          Navigator.of(dialogContext).pop();
          showClearDataConfirmDialog();
        },
        onRestoreConfig: () {
          Navigator.of(dialogContext).pop();
          showRestoreConfigConfirmDialog();
        },
        onShowAbout: showAppAboutDialog,
        onReauthorizeDirectory: () {
          Navigator.of(dialogContext).pop();
          reauthorizeDirectory();
        },
        apiServerEnabled: _apiServerEnabled,
        onToggleApiServer: _toggleApiServer,
      ),
    );
  }

  // ============ UI 构建 ============

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: const Color(0xFF0F0F23),
      body: Container(
        decoration: BoxDecoration(
          gradient: LinearGradient(
            begin: Alignment.topLeft,
            end: Alignment.bottomRight,
            colors: [
              const Color(0xFF0F0F23),
              const Color(0xFF1A1A2E),
              const Color(0xFF16213E),
            ],
          ),
        ),
        child: Column(
          children: [
            _buildTitleBar(),
            Expanded(
              child: Stack(
                children: [
                  AnimatedSwitcher(
                    duration: const Duration(milliseconds: 520),
                    reverseDuration: const Duration(milliseconds: 360),
                    switchInCurve: Curves.easeOutCubic,
                    switchOutCurve: Curves.easeInCubic,
                    layoutBuilder: (currentChild, previousChildren) {
                      return Stack(
                        alignment: Alignment.topCenter,
                        children: [...previousChildren, ?currentChild],
                      );
                    },
                    transitionBuilder: _buildContentTransition,
                    child: _buildContent(),
                  ),
                  if (_showOnboarding)
                    Positioned.fill(
                      child: Container(
                        color: Colors.black.withValues(alpha: 0.35),
                      ),
                    ),
                  if (_showOnboarding)
                    Center(child: OnboardingDialog(onFinish: _hideOnboarding)),
                  if (_showWelcome)
                    const Positioned.fill(child: WelcomeOverlay()),
                  Positioned.fill(
                    child: OnboardingCompletionOverlay(
                      visible: _showOnboardingCompletionOverlay,
                    ),
                  ),
                  if (isRestoringConfig)
                    Positioned.fill(
                      child: Container(
                        color: Colors.black.withValues(alpha: 0.6),
                        child: Center(
                          child: Column(
                            mainAxisSize: MainAxisSize.min,
                            children: [
                              const CircularProgressIndicator(
                                color: Color(0xFFEAB308),
                              ),
                              const SizedBox(height: 16),
                              Text(
                                AppLocalizations.of(context)?.restoringConfig ??
                                    '',
                                style: const TextStyle(
                                  color: Colors.white70,
                                  fontSize: 14,
                                ),
                              ),
                            ],
                          ),
                        ),
                      ),
                    ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildTitleBar() {
    final l10n = AppLocalizations.of(context)!;

    // Linux 使用原生 GTK 标题栏处理窗口拖拽/最小化/关闭，Flutter 层只提供
    // 功能性工具栏（设置、语言切换），不重复渲染窗口控制按钮。
    if (Platform.isLinux) {
      return Container(
        height: 40,
        padding: const EdgeInsets.symmetric(horizontal: 16),
        decoration: BoxDecoration(
          color: Colors.black.withValues(alpha: 0.3),
          border: Border(
            bottom: BorderSide(color: Colors.white.withValues(alpha: 0.1)),
          ),
        ),
        child: Row(
          children: [
            Container(
              padding: const EdgeInsets.all(5),
              decoration: BoxDecoration(
                gradient: const LinearGradient(
                  colors: [Color(0xFF6366F1), Color(0xFF8B5CF6)],
                ),
                borderRadius: BorderRadius.circular(6),
              ),
              child: const Icon(
                LucideIcons.shield,
                color: Colors.white,
                size: 14,
              ),
            ),
            const SizedBox(width: 8),
            Text(
              l10n.appTitle,
              style: AppFonts.inter(
                fontSize: 13,
                fontWeight: FontWeight.w600,
                color: Colors.white,
              ),
            ),
            const Spacer(),
            IconButton(
              icon: const Icon(
                LucideIcons.fileSearch,
                size: 16,
                color: Colors.white70,
              ),
              tooltip: l10n.auditLog,
              onPressed: showAuditLogWindow,
            ),
            IconButton(
              icon: const Icon(
                LucideIcons.cpu,
                size: 16,
                color: Colors.white70,
              ),
              tooltip: l10n.settings,
              onPressed: _handleSettingsTap,
            ),
            PopupMenuButton<String>(
              icon: const Icon(
                LucideIcons.languages,
                size: 16,
                color: Colors.white70,
              ),
              color: const Color(0xFF1A1A2E),
              tooltip: l10n.switchLanguage,
              itemBuilder: (context) => [
                const PopupMenuItem(
                  value: 'zh',
                  child: Text('中文', style: TextStyle(color: Colors.white)),
                ),
                const PopupMenuItem(
                  value: 'en',
                  child: Text('English', style: TextStyle(color: Colors.white)),
                ),
              ],
              onSelected: (value) async {
                await context.read<LocaleProvider>().setLocale(Locale(value));
                await applyProtectionLanguage(value);
              },
            ),
          ],
        ),
      );
    }

    return Container(
      height: 48,
      decoration: BoxDecoration(
        color: Colors.black.withValues(alpha: 0.3),
        border: Border(
          bottom: BorderSide(color: Colors.white.withValues(alpha: 0.1)),
        ),
      ),
      child: Row(
        children: [
          Expanded(
            child: GestureDetector(
              onPanStart: (_) => windowManager.startDragging(),
              behavior: HitTestBehavior.translucent,
              child: Padding(
                padding: const EdgeInsets.only(left: 16),
                child: Row(
                  children: [
                    Container(
                      padding: const EdgeInsets.all(6),
                      decoration: BoxDecoration(
                        gradient: const LinearGradient(
                          colors: [Color(0xFF6366F1), Color(0xFF8B5CF6)],
                        ),
                        borderRadius: BorderRadius.circular(8),
                      ),
                      child: const Icon(
                        LucideIcons.shield,
                        color: Colors.white,
                        size: 16,
                      ),
                    ),
                    const SizedBox(width: 10),
                    Text(
                      l10n.appTitle,
                      style: AppFonts.inter(
                        fontSize: 14,
                        fontWeight: FontWeight.w600,
                        color: Colors.white,
                      ),
                    ),
                  ],
                ),
              ),
            ),
          ),
          IconButton(
            icon: const Icon(
              LucideIcons.fileSearch,
              size: 16,
              color: Colors.white70,
            ),
            tooltip: l10n.auditLog,
            onPressed: showAuditLogWindow,
          ),
          IconButton(
            icon: const Icon(LucideIcons.cpu, size: 16, color: Colors.white70),
            tooltip: l10n.settings,
            onPressed: _handleSettingsTap,
          ),
          PopupMenuButton<String>(
            icon: const Icon(
              LucideIcons.languages,
              size: 16,
              color: Colors.white70,
            ),
            color: const Color(0xFF1A1A2E),
            tooltip: l10n.switchLanguage,
            itemBuilder: (context) => [
              const PopupMenuItem(
                value: 'zh',
                child: Text('中文', style: TextStyle(color: Colors.white)),
              ),
              const PopupMenuItem(
                value: 'en',
                child: Text('English', style: TextStyle(color: Colors.white)),
              ),
            ],
            onSelected: (value) async {
              await context.read<LocaleProvider>().setLocale(Locale(value));
              await applyProtectionLanguage(value);
            },
          ),
          const SizedBox(width: 8),
          _buildWindowButton(
            icon: LucideIcons.minus,
            onTap: () => minimizeMainWindow(),
          ),
          const SizedBox(width: 8),
          _buildWindowButton(
            icon: LucideIcons.x,
            onTap: () => hideMainWindow(),
            isClose: true,
            isCloseBtn: true,
          ),
          const SizedBox(width: 16),
        ],
      ),
    );
  }

  Widget _buildWindowButton({
    required IconData icon,
    required VoidCallback onTap,
    bool isClose = false,
    bool isCloseBtn = false,
  }) {
    return MouseRegion(
      cursor: SystemMouseCursors.click,
      child: GestureDetector(
        onTap: onTap,
        child: Container(
          width: 28,
          height: 28,
          decoration: BoxDecoration(
            color: isCloseBtn
                ? Colors.red.withValues(alpha: 0.2)
                : Colors.white.withValues(alpha: 0.1),
            borderRadius: BorderRadius.circular(6),
          ),
          child: Icon(
            icon,
            size: 14,
            color: isCloseBtn ? Colors.red.shade300 : Colors.white70,
          ),
        ),
      ),
    );
  }

  Widget _buildContent() {
    switch (_scanState) {
      case ScanState.idle:
        return _buildIdleState();
      case ScanState.scanning:
        return _buildScanningState();
      case ScanState.completed:
        return _buildCompletedState();
    }
  }

  Widget _buildContentTransition(Widget child, Animation<double> animation) {
    final fadeAnimation = CurvedAnimation(
      parent: animation,
      curve: Curves.easeOutCubic,
      reverseCurve: Curves.easeInCubic,
    );
    final slideAnimation = Tween<Offset>(
      begin: const Offset(0, 0.03),
      end: Offset.zero,
    ).animate(fadeAnimation);
    final scaleAnimation = Tween<double>(
      begin: 0.985,
      end: 1,
    ).animate(fadeAnimation);

    return FadeTransition(
      opacity: fadeAnimation,
      child: SlideTransition(
        position: slideAnimation,
        child: ScaleTransition(scale: scaleAnimation, child: child),
      ),
    );
  }

  Widget _buildIdleState() {
    final l10n = AppLocalizations.of(context)!;
    return Center(
      key: const ValueKey('idle'),
      child: Column(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          Container(
                width: 100,
                height: 100,
                decoration: BoxDecoration(
                  gradient: LinearGradient(
                    begin: Alignment.topLeft,
                    end: Alignment.bottomRight,
                    colors: [
                      const Color(0xFF6366F1).withValues(alpha: 0.2),
                      const Color(0xFF8B5CF6).withValues(alpha: 0.2),
                    ],
                  ),
                  shape: BoxShape.circle,
                  border: Border.all(
                    color: const Color(0xFF6366F1).withValues(alpha: 0.3),
                    width: 2,
                  ),
                ),
                child: const Icon(
                  LucideIcons.shieldCheck,
                  size: 48,
                  color: Color(0xFF6366F1),
                ),
              )
              .animate(onPlay: (controller) => controller.repeat())
              .shimmer(duration: 2000.ms, color: Colors.white24)
              .then()
              .shake(hz: 0.5, rotation: 0.02),
          const SizedBox(height: 32),
          Text(
            l10n.idleTitle,
            style: AppFonts.inter(
              fontSize: 20,
              fontWeight: FontWeight.bold,
              color: Colors.white,
            ),
          ),
          const SizedBox(height: 8),
          Text(
            l10n.idleSubtitle,
            style: AppFonts.inter(fontSize: 13, color: Colors.white54),
          ),
          const SizedBox(height: 40),
          MouseRegion(
            cursor: SystemMouseCursors.click,
            child: GestureDetector(
              onTap: _startScan,
              child: Container(
                padding: const EdgeInsets.symmetric(
                  horizontal: 32,
                  vertical: 16,
                ),
                decoration: BoxDecoration(
                  gradient: const LinearGradient(
                    colors: [Color(0xFF6366F1), Color(0xFF8B5CF6)],
                  ),
                  borderRadius: BorderRadius.circular(12),
                  boxShadow: [
                    BoxShadow(
                      color: const Color(0xFF6366F1).withValues(alpha: 0.4),
                      blurRadius: 20,
                      offset: const Offset(0, 8),
                    ),
                  ],
                ),
                child: Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    const Icon(LucideIcons.scan, color: Colors.white, size: 20),
                    const SizedBox(width: 10),
                    Text(
                      l10n.startScan,
                      style: AppFonts.inter(
                        fontSize: 15,
                        fontWeight: FontWeight.w600,
                        color: Colors.white,
                      ),
                    ),
                  ],
                ),
              ),
            ),
          ).animate().fadeIn(duration: 600.ms).slideY(begin: 0.3, end: 0),
          const SizedBox(height: 16),
          MouseRegion(
            cursor: SystemMouseCursors.click,
            child: GestureDetector(
              onTap: _showSkillScanResultsDialog,
              child: Row(
                mainAxisSize: MainAxisSize.min,
                children: [
                  const Icon(
                    LucideIcons.fileSearch,
                    color: Colors.white54,
                    size: 14,
                  ),
                  const SizedBox(width: 6),
                  Text(
                    l10n.viewSkillScanResults,
                    style: AppFonts.inter(fontSize: 12, color: Colors.white54),
                  ),
                ],
              ),
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildScanningState() {
    final l10n = AppLocalizations.of(context)!;
    return Padding(
      key: const ValueKey('scanning'),
      padding: const EdgeInsets.all(24),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Container(
                    padding: const EdgeInsets.all(8),
                    decoration: BoxDecoration(
                      color: const Color(0xFF6366F1).withValues(alpha: 0.2),
                      borderRadius: BorderRadius.circular(8),
                    ),
                    child: const Icon(
                      LucideIcons.loader2,
                      color: Color(0xFF6366F1),
                      size: 20,
                    ),
                  )
                  .animate(onPlay: (controller) => controller.repeat())
                  .rotate(duration: 1000.ms),
              const SizedBox(width: 12),
              Text(
                l10n.scanning,
                style: AppFonts.inter(
                  fontSize: 18,
                  fontWeight: FontWeight.w600,
                  color: Colors.white,
                ),
              ),
            ],
          ),
          const SizedBox(height: 24),
          Expanded(
            child: Container(
              padding: const EdgeInsets.all(16),
              decoration: BoxDecoration(
                color: Colors.black.withValues(alpha: 0.3),
                borderRadius: BorderRadius.circular(12),
                border: Border.all(color: Colors.white.withValues(alpha: 0.1)),
              ),
              child: ListView.builder(
                itemCount: _logs.length,
                itemBuilder: (context, index) {
                  return Padding(
                        padding: const EdgeInsets.symmetric(vertical: 4),
                        child: Row(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Text(
                              '>',
                              style: AppFonts.firaCode(
                                fontSize: 12,
                                color: const Color(0xFF6366F1),
                              ),
                            ),
                            const SizedBox(width: 8),
                            Expanded(
                              child: Text(
                                _logs[index],
                                style: AppFonts.firaCode(
                                  fontSize: 12,
                                  color: Colors.white70,
                                ),
                              ),
                            ),
                          ],
                        ),
                      )
                      .animate()
                      .fadeIn(duration: 300.ms)
                      .slideX(begin: -0.1, end: 0);
                },
              ),
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildCompletedState() {
    final result = _result;
    if (result == null) return const SizedBox();

    return ScanResultView(
      result: result,
      protectedAssets: _protectedAssetIDs,
      isRestoringProtection: _isRestoringProtection,
      onRescan: _resetScan,
      onViewSkillScanResults: _showSkillScanResultsDialog,
      onShowProtectionConfig: _showProtectionConfigDialog,
      onShowProtectionMonitor: (asset) => _showProtectionMonitorResolved(asset),
      onShowMitigation: (risk) => _showMitigationDialog(context, risk),
    );
  }
}
