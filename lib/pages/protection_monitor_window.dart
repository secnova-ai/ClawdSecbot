import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'package:desktop_multi_window/desktop_multi_window.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import '../utils/app_fonts.dart';
import 'package:lucide_icons/lucide_icons.dart';
import 'package:flutter_animate/flutter_animate.dart';
import 'package:window_manager/window_manager.dart';
import 'package:flutter/services.dart';
import '../l10n/app_localizations.dart';
import '../models/protection_analysis_model.dart';
import '../models/llm_config_model.dart';
import '../models/protection_config_model.dart';
import '../models/truth_record_model.dart';
import '../models/security_event_model.dart';
import '../services/protection_service.dart';
import '../services/database_service.dart';
import '../services/model_config_database_service.dart';
import '../services/protection_database_service.dart';
import '../utils/app_logger.dart';
import '../utils/window_animation_helper.dart';
import '../widgets/hide_window_shortcut.dart';
import '../widgets/protection_monitor_charts.dart';
import '../widgets/protection_monitor_loading_screen.dart';
import '../widgets/protection_monitor_log_panel.dart';
import '../widgets/protection_monitor_event_panel.dart';
import 'mixins/protection_monitor_log_processor_mixin.dart';
import 'mixins/protection_monitor_translation_mixin.dart';

const _windowBackground = Color(0xFF0F0F23);

class ProtectionMonitorWindowApp extends StatefulWidget {
  final String windowId;
  final String assetName;
  final String assetID;
  final String locale;

  const ProtectionMonitorWindowApp({
    super.key,
    required this.windowId,
    required this.assetName,
    this.assetID = '',
    this.locale = 'en',
  });

  @override
  State<ProtectionMonitorWindowApp> createState() =>
      _ProtectionMonitorWindowAppState();
}

class _ProtectionMonitorWindowAppState
    extends State<ProtectionMonitorWindowApp> {
  late String _locale;
  bool _isWindowShown = false;
  late final ProtectionService _protectionService;

  /// Linux 子进程：主进程通过 WindowMethodChannel 中继的日志/结果/统计流
  final StreamController<List<String>> _relayLogController =
      StreamController<List<String>>.broadcast();
  final StreamController<ProtectionAnalysisResult> _relayResultController =
      StreamController<ProtectionAnalysisResult>.broadcast();
  final StreamController<Map<String, dynamic>> _relayStatsController =
      StreamController<Map<String, dynamic>>.broadcast();
  final StreamController<List<SecurityEvent>> _relaySecurityEventController =
      StreamController<List<SecurityEvent>>.broadcast();
  final StreamController<TruthRecordModel> _relayTruthRecordController =
      StreamController<TruthRecordModel>.broadcast();

  Stream<List<String>> get relayedLogBatches => _relayLogController.stream;
  Stream<ProtectionAnalysisResult> get relayedResultStream =>
      _relayResultController.stream;
  Stream<Map<String, dynamic>> get relayedStatsStream =>
      _relayStatsController.stream;
  Stream<List<SecurityEvent>> get relayedSecurityEventStream =>
      _relaySecurityEventController.stream;
  Stream<TruthRecordModel> get relayedTruthRecordStream =>
      _relayTruthRecordController.stream;

  @override
  void initState() {
    super.initState();
    _protectionService = ProtectionService.forAsset(
      widget.assetName,
      widget.assetID,
    );
    _locale = widget.locale;
    _showWindowAfterFirstFrame();

    WindowController.fromCurrentEngine().then((controller) {
      controller.setWindowMethodHandler((call) async {
        if (call.method == 'updateLanguage') {
          final language = call.arguments as String;
          try {
            appLogger.info(
              '[Protection Monitor] Received updateLanguage: $language',
            );
            setState(() {
              _locale = language;
            });
            final dbService = ModelConfigDatabaseService();
            // 安全模型配置作为顶层参数传递给 Go，bot 模型由 Go 内部独立加载
            final securityModelConfig = await dbService
                .getSecurityModelConfig();
            if (securityModelConfig != null) {
              // 语言由 Go 层 app_settings 管理，直接更新安全模型配置
              await _protectionService.updateSecurityModelConfig(
                securityModelConfig,
              );
              appLogger.info('[Protection Monitor] Proxy config updated');
            }
          } catch (e) {
            appLogger.error(
              '[Protection Monitor] Failed to update language',
              e,
            );
          }
        } else if (call.method == 'window_close') {
          try {
            await windowManager.close();
          } catch (_) {
            final ctrl = await WindowController.fromCurrentEngine();
            await ctrl.hide();
          }
        } else if (call.method == 'relayLogs') {
          try {
            final args = call.arguments;
            final List<dynamic> list = args is String
                ? jsonDecode(args) as List<dynamic>
                : args as List<dynamic>;
            final batch = list.map((e) => e.toString()).toList();
            if (!_relayLogController.isClosed) {
              _relayLogController.add(batch);
            }
          } catch (_) {}
        } else if (call.method == 'relayResult') {
          try {
            final args = call.arguments;
            final Map<String, dynamic> map = args is String
                ? jsonDecode(args) as Map<String, dynamic>
                : args as Map<String, dynamic>;
            final result = ProtectionAnalysisResult.fromJson(map);
            if (!_relayResultController.isClosed) {
              _relayResultController.add(result);
            }
          } catch (_) {}
        } else if (call.method == 'relayStats') {
          try {
            final args = call.arguments;
            final Map<String, dynamic> map = args is String
                ? jsonDecode(args) as Map<String, dynamic>
                : args as Map<String, dynamic>;
            if (!_relayStatsController.isClosed) {
              _relayStatsController.add(map);
            }
          } catch (_) {}
        } else if (call.method == 'relaySecurityEvents') {
          try {
            final args = call.arguments;
            final List<dynamic> list = args is String
                ? jsonDecode(args) as List<dynamic>
                : args as List<dynamic>;
            final events = list
                .map((e) => SecurityEvent.fromJson(e as Map<String, dynamic>))
                .toList();
            if (!_relaySecurityEventController.isClosed) {
              _relaySecurityEventController.add(events);
            }
          } catch (_) {}
        } else if (call.method == 'relayTruthRecords') {
          try {
            final args = call.arguments;
            final List<dynamic> list = args is String
                ? jsonDecode(args) as List<dynamic>
                : args as List<dynamic>;
            for (final item in list) {
              final record = TruthRecordModel.fromJson(
                Map<String, dynamic>.from(item as Map),
              );
              if (!_relayTruthRecordController.isClosed) {
                _relayTruthRecordController.add(record);
              }
            }
          } catch (_) {}
        }
        return null;
      });
    });
  }

  @override
  void dispose() {
    _relayLogController.close();
    _relayResultController.close();
    _relayStatsController.close();
    _relaySecurityEventController.close();
    _relayTruthRecordController.close();
    super.dispose();
  }

  /// 在首帧完成栅格化后显示防护监控窗口，减少启动闪烁。
  void _showWindowAfterFirstFrame() {
    Future<void>(() async {
      await WidgetsBinding.instance.waitUntilFirstFrameRasterized;
      if (!mounted || _isWindowShown) return;
      _isWindowShown = true;
      await WindowAnimationHelper.showWithAnimation();
    });
  }

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Protection Monitor',
      debugShowCheckedModeBanner: false,
      locale: Locale(_locale),
      localizationsDelegates: const [
        AppLocalizations.delegate,
        GlobalMaterialLocalizations.delegate,
        GlobalWidgetsLocalizations.delegate,
        GlobalCupertinoLocalizations.delegate,
      ],
      supportedLocales: const [Locale('zh'), Locale('en')],
      theme: ThemeData(
        useMaterial3: true,
        colorScheme: ColorScheme.fromSeed(
          seedColor: const Color(0xFF6366F1),
          brightness: Brightness.dark,
        ),
        scaffoldBackgroundColor: _windowBackground,
        textTheme: AppFonts.interTextTheme(ThemeData.dark().textTheme),
      ),
      // 在 MaterialApp 级别定义快捷键，确保全局生效
      shortcuts: {
        LogicalKeySet(LogicalKeyboardKey.meta, LogicalKeyboardKey.keyW):
            const HideWindowIntent(),
      },
      actions: {
        HideWindowIntent: CallbackAction<HideWindowIntent>(
          onInvoke: (_) {
            WindowAnimationHelper.hideWithAnimation();
            return null;
          },
        ),
      },
      home: ProtectionMonitorPage(
        windowId: widget.windowId,
        assetName: widget.assetName,
        assetID: widget.assetID,
        relayedLogBatches: Platform.isLinux ? relayedLogBatches : null,
        relayedResultStream: Platform.isLinux ? relayedResultStream : null,
        relayedStatsStream: Platform.isLinux ? relayedStatsStream : null,
        relayedSecurityEventStream: Platform.isLinux
            ? relayedSecurityEventStream
            : null,
        relayedTruthRecordStream: Platform.isLinux
            ? relayedTruthRecordStream
            : null,
      ),
    );
  }
}

class ProtectionMonitorPage extends StatefulWidget {
  final String windowId;
  final String assetName;
  final String assetID;

  /// Linux 子进程：主进程中继的日志批次流，非 null 时优先于 logStream
  final Stream<List<String>>? relayedLogBatches;
  final Stream<ProtectionAnalysisResult>? relayedResultStream;
  final Stream<Map<String, dynamic>>? relayedStatsStream;
  final Stream<List<SecurityEvent>>? relayedSecurityEventStream;
  final Stream<TruthRecordModel>? relayedTruthRecordStream;

  const ProtectionMonitorPage({
    super.key,
    required this.windowId,
    required this.assetName,
    this.assetID = '',
    this.relayedLogBatches,
    this.relayedResultStream,
    this.relayedStatsStream,
    this.relayedSecurityEventStream,
    this.relayedTruthRecordStream,
  });

  @override
  State<ProtectionMonitorPage> createState() => _ProtectionMonitorPageState();
}

class _ProtectionMonitorPageState extends State<ProtectionMonitorPage>
    with
        ProtectionMonitorTranslationMixin,
        ProtectionMonitorLogProcessorMixin,
        WindowListener {
  late final ProtectionService _protectionService;
  final ScrollController _logScrollController = ScrollController();
  final ScrollController _horizontalScrollController = ScrollController();
  bool _useGroupedView = true;
  bool _isLogPanelExpanded = false;
  final Map<String, TruthRecordModel> _requestGroups = {};
  final List<String> _requestOrder = [];

  // 原始视图自动滚动状态
  final bool _autoScrollEnabled = true;
  bool _userScrolledAway = false;

  // State
  bool _isProtectionActive = false;
  bool _isInitializing = true;
  bool _isStartingProxy = false;
  String? _proxyError;
  RiskLevel _currentRiskLevel = RiskLevel.safe;
  final List<LogEntry> _logsList = [];
  static const int _maxLogCount = 5000;
  ProtectionAnalysisResult? _latestResult;

  // Proxy info
  int? _proxyPort;
  String? _providerName;
  String _currentBotModelName = '';

  // Statistics
  int _messageCount = 0;
  int _analysisCount = 0;
  int _blockedCount = 0;
  int _warningCount = 0;

  // API Metrics
  int _totalPromptTokens = 0;
  int _totalCompletionTokens = 0;
  int _totalToolCalls = 0;
  int _auditPromptTokens = 0;
  int _auditCompletionTokens = 0;
  ApiStatistics? _apiStatistics;

  // 日志批量更新
  final List<LogEntry> _pendingLogs = [];
  Timer? _logUpdateTimer;
  static const _logUpdateInterval = Duration(milliseconds: 200);

  // 结果批量更新
  ProtectionAnalysisResult? _pendingResult;
  Timer? _resultUpdateTimer;
  static const _resultUpdateInterval = Duration(milliseconds: 200);

  // 指标节流
  DateTime? _lastMetricsUpdate;
  static const _metricsUpdateInterval = Duration(seconds: 2);
  bool _metricsUpdatePending = false;

  // 初始化守护
  bool _initInProgress = false;
  DateTime? _lastLogReceivedAt;
  DateTime? _lastMetricsReceivedAt;
  DateTime? _lastResultReceivedAt;
  bool _resumeReconnectInProgress = false;
  DateTime? _lastBridgeReconnectTime;
  Timer? _truthRecordCatchUpTimer;
  Timer? _securityEventCatchUpTimer;

  /// 合并同一帧内多条 TruthRecord(同一 request_id 保留最新), 再一次性 setState.
  final Map<String, TruthRecordModel> _pendingTruthByRequestId = {};
  bool _truthRecordFrameFlushScheduled = false;

  // 防护配置
  ProtectionConfig? _protectionConfig;
  bool _auditOnly = false;
  final bool _isAnalyzingInProgress = false;

  // Stream subscriptions
  StreamSubscription<String>? _logSubscription;
  StreamSubscription<ProtectionAnalysisResult>? _resultSubscription;
  StreamSubscription<dynamic>? _metricsSubscription;
  StreamSubscription<List<SecurityEvent>>? _securityEventSubscription;
  StreamSubscription<TruthRecordModel>? _truthRecordSubscription;
  bool _isRelayMode = false;

  // 安全事件
  final List<SecurityEvent> _securityEvents = [];
  bool _isMaximized = false;

  // ============ Mixin @override getters/setters ============

  @override
  List<LogEntry> get logsList => _logsList;
  @override
  int get maxLogCount => _maxLogCount;
  @override
  Map<String, TruthRecordModel> get requestGroups => _requestGroups;
  @override
  List<String> get requestOrder => _requestOrder;
  @override
  List<LogEntry> get pendingLogs => _pendingLogs;
  @override
  Timer? get logUpdateTimer => _logUpdateTimer;
  @override
  set logUpdateTimer(Timer? value) => _logUpdateTimer = value;
  @override
  ScrollController get logScrollController => _logScrollController;
  @override
  bool get useGroupedView => _useGroupedView;
  @override
  bool get autoScrollEnabled => _autoScrollEnabled;
  @override
  bool get userScrolledAway => _userScrolledAway;
  @override
  set userScrolledAway(bool value) => _userScrolledAway = value;
  @override
  ProtectionAnalysisResult? get pendingResult => _pendingResult;
  @override
  set pendingResult(ProtectionAnalysisResult? value) => _pendingResult = value;
  @override
  Timer? get resultUpdateTimer => _resultUpdateTimer;
  @override
  set resultUpdateTimer(Timer? value) => _resultUpdateTimer = value;
  @override
  set latestResult(ProtectionAnalysisResult? value) => _latestResult = value;
  @override
  set currentRiskLevel(RiskLevel value) => _currentRiskLevel = value;
  @override
  ProtectionService get protectionService => _protectionService;

  @override
  void updateCountersFromService() {
    _analysisCount = _protectionService.analysisCount;
    _blockedCount = _protectionService.blockedCount;
    _warningCount = _protectionService.warningCount;
    _messageCount = _protectionService.requestCount;
  }

  // ============ 生命周期 ============

  @override
  void initState() {
    super.initState();
    _protectionService = ProtectionService.forAsset(
      widget.assetName,
      widget.assetID,
    );
    try {
      windowManager.addListener(this);
      if (Platform.isLinux || Platform.isWindows) {
        windowManager.setPreventClose(true).catchError((_) {});
      }
      if (Platform.isWindows) {
        _syncMaximizedState();
      }
    } catch (_) {
      // 子窗口 window_manager 可能未注册，焦点跟踪降级
    }

    _logScrollController.addListener(onLogScroll);

    if (kDebugMode) {
      debugPrint(
        '[Protection Monitor] initState called, _isInitializing=$_isInitializing',
      );
    }
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (kDebugMode) {
        debugPrint(
          '[Protection Monitor] PostFrameCallback: starting initialization',
        );
      }
      _initializeService();
    });
  }

  @override
  void onWindowFocus() {
    if (Platform.isWindows) {
      _syncMaximizedState();
    }
    _reconnectBridgeOnResume();
    _reloadProtectionConfig();
  }

  @override
  void onWindowMaximize() {
    if (!Platform.isWindows || !mounted) return;
    setState(() => _isMaximized = true);
  }

  @override
  void onWindowUnmaximize() {
    if (!Platform.isWindows || !mounted) return;
    setState(() => _isMaximized = false);
  }

  @override
  Future<void> onWindowClose() async {
    if (Platform.isLinux || Platform.isWindows) {
      await WindowAnimationHelper.hideWithAnimation();
      return;
    }
  }

  DateTime _recordStreamActivity(String name, DateTime? lastTime) {
    final now = DateTime.now();
    if (kDebugMode && lastTime != null) {
      final gap = now.difference(lastTime);
      if (gap.inSeconds >= 5) {
        debugPrint('[Monitor] $name resumed after ${gap.inSeconds}s');
      }
    }
    return now;
  }

  Future<void> _syncMaximizedState() async {
    try {
      final maximized = await windowManager.isMaximized();
      if (!mounted || _isMaximized == maximized) return;
      setState(() => _isMaximized = maximized);
    } catch (_) {}
  }

  Future<void> _toggleMaximize() async {
    try {
      final maximized = await windowManager.isMaximized();
      if (maximized) {
        await windowManager.unmaximize();
      } else {
        await windowManager.maximize();
      }
      if (mounted) {
        setState(() => _isMaximized = !maximized);
      }
    } catch (_) {}
  }

  /// 窗口恢复焦点时尝试重连回调桥。
  /// 仅在桥实际断开且冷却期（30秒）已过时才执行，避免频繁拆桥导致快照丢失。
  Future<void> _reconnectBridgeOnResume() async {
    if (_resumeReconnectInProgress) return;
    if (_protectionService.useCallbackBridge) return;
    final now = DateTime.now();
    if (_lastBridgeReconnectTime != null &&
        now.difference(_lastBridgeReconnectTime!).inSeconds < 30) {
      return;
    }
    _resumeReconnectInProgress = true;
    _lastBridgeReconnectTime = now;
    try {
      if (!_protectionService.isProxyRunning) {
        return;
      }
      _protectionService.disableCallbackBridge();
      final enabled = await _protectionService.enableCallbackBridge();
      if (kDebugMode) {
        debugPrint('[Monitor] resume reconnect: callbackEnabled=$enabled');
      }
    } catch (e) {
      if (kDebugMode) {
        debugPrint('[Monitor] resume reconnect failed: $e');
      }
    } finally {
      _resumeReconnectInProgress = false;
    }
  }

  /// 启动 TruthRecord catch-up 定时器，每 3 秒非破坏性地从 Go RecordStore 拉取全量快照。
  /// 用于弥补回调桥偶发丢失快照的情况（如桥重建间隙）。
  void _startTruthRecordCatchUp() {
    _truthRecordCatchUpTimer?.cancel();
    _truthRecordCatchUpTimer = Timer.periodic(
      const Duration(seconds: 3),
      (_) => _runTruthRecordCatchUp(),
    );
  }

  void _runTruthRecordCatchUp() {
    if (!mounted || !_protectionService.isProxyRunning) return;
    try {
      final snapshots = _protectionService.fetchAllTruthRecordSnapshots();
      if (snapshots.isEmpty) return;
      final toApply = <TruthRecordModel>[];
      for (final record in snapshots) {
        final existing = requestGroups[record.requestId];
        if (existing == null ||
            record.updatedAt.compareTo(existing.updatedAt) > 0) {
          toApply.add(record);
        }
      }
      if (toApply.isNotEmpty) {
        processProtectionRecordBatch(toApply);
        appLogger.debug(
          '[TruthRecord] catch_up fetched=${snapshots.length} updated=${toApply.length}',
        );
      }
    } catch (e) {
      appLogger.error('[TruthRecord] catch_up error', e);
    }
  }

  /// 将流式 TruthRecord 先入队, 在下一帧合并后批量刷新 UI, 避免同步文件日志与连续 setState 卡住主线程.
  void _enqueueTruthRecordForUi(TruthRecordModel record) {
    if (!mounted) return;
    _pendingTruthByRequestId[record.requestId] = record;
    if (_truthRecordFrameFlushScheduled) return;
    _truthRecordFrameFlushScheduled = true;
    WidgetsBinding.instance.addPostFrameCallback((_) {
      _truthRecordFrameFlushScheduled = false;
      if (!mounted) return;
      if (_pendingTruthByRequestId.isEmpty) return;
      final batch = _pendingTruthByRequestId.values.toList();
      _pendingTruthByRequestId.clear();
      processProtectionRecordBatch(batch);
    });
  }

  /// 启动安全事件 catch-up 定时器，每 5 秒从数据库增量拉取新事件。
  /// 用于弥补回调桥或 FFI 轮询偶发丢失事件的情况。
  void _startSecurityEventCatchUp() {
    _securityEventCatchUpTimer?.cancel();
    _securityEventCatchUpTimer = Timer.periodic(
      const Duration(seconds: 5),
      (_) => _runSecurityEventCatchUp(),
    );
  }

  void _runSecurityEventCatchUp() {
    if (!mounted) return;
    _protectionService
        .getSecurityEvents(limit: 200)
        .then((dbEvents) {
          if (!mounted || dbEvents.isEmpty) return;
          final currentIds = _securityEvents.map((e) => e.id).toSet();
          final newEvents =
              dbEvents.where((e) => !currentIds.contains(e.id)).toList();
          if (newEvents.isEmpty) return;
          setState(() {
            _securityEvents
              ..clear()
              ..addAll(dbEvents);
          });
        })
        .catchError((e) {
          appLogger.error('[Monitor] Security event catch-up error', e);
        });
  }

  // ============ 初始化和准备 ============

  Future<void> _prepareForInitialization() async {
    _logSubscription?.cancel();
    _resultSubscription?.cancel();
    _metricsSubscription?.cancel();
    _securityEventSubscription?.cancel();
    _truthRecordSubscription?.cancel();
    _truthRecordCatchUpTimer?.cancel();
    _securityEventCatchUpTimer?.cancel();
    _logSubscription = null;
    _resultSubscription = null;
    _metricsSubscription = null;
    _securityEventSubscription = null;
    _truthRecordSubscription = null;
    _truthRecordCatchUpTimer = null;
    _securityEventCatchUpTimer = null;

    logUpdateTimer?.cancel();
    resultUpdateTimer?.cancel();
    logUpdateTimer = null;
    resultUpdateTimer = null;

    _pendingLogs.clear();
    _pendingResult = null;
    _logsList.clear();
    _latestResult = null;

    _lastMetricsUpdate = null;
    _metricsUpdatePending = false;
    _apiStatistics = null;
    _lastLogReceivedAt = null;
    _lastMetricsReceivedAt = null;
    _lastResultReceivedAt = null;
    _lastResultReceivedAt = null;

    // 仅重置窗口本地 UI 状态,不停止共享的 proxy 实例
    // (proxy 生命周期由 main_page 或窗口内显式操作管理)
    _isProtectionActive = false;
    _proxyPort = null;
    _providerName = null;
  }

  /// 带节流的 API 统计更新
  void _updateApiMetrics() {
    if (!mounted) return;

    if (_metricsUpdatePending) return;
    _metricsUpdatePending = true;

    Future(() async {
      try {
        if (!mounted) return;
        final stats = await _protectionService.getApiStatistics(
          duration: const Duration(hours: 1),
        );
        if (!mounted) return;

        // Linux 中继模式下，顶部统计只接受主窗口 relayStats，避免双源覆盖导致计数跳变
        if (_isRelayMode) {
          setState(() {
            _apiStatistics = stats;
          });
        } else {
          setState(() {
            _totalPromptTokens = _protectionService.totalPromptTokens;
            _totalCompletionTokens = _protectionService.totalCompletionTokens;
            _totalToolCalls = _protectionService.totalToolCalls;
            _auditPromptTokens = _protectionService.auditPromptTokens;
            _auditCompletionTokens = _protectionService.auditCompletionTokens;
            _messageCount = _protectionService.requestCount;
            _apiStatistics = stats;
            _analysisCount = _protectionService.analysisCount;
            _blockedCount = _protectionService.blockedCount;
            _warningCount = _protectionService.warningCount;
          });
        }

        if (kDebugMode &&
            (stats.tokenTrend.isNotEmpty || stats.toolCallTrend.isNotEmpty)) {
          debugPrint(
            '[Monitor] API stats: tokenTrend=${stats.tokenTrend.length} entries, toolCallTrend=${stats.toolCallTrend.length} entries',
          );
        }
      } catch (e) {
        if (kDebugMode) {
          debugPrint('[Monitor] API stats update failed: $e');
        }
      } finally {
        _metricsUpdatePending = false;
      }
    });
  }

  /// 初始化服务并订阅流
  Future<void> _initializeService() async {
    if (_initInProgress) return;
    _initInProgress = true;
    final l10n = AppLocalizations.of(context);
    try {
      if (mounted) {
        setState(() {
          _isInitializing = true;
          _isStartingProxy = false;
          _proxyError = null;
        });
      }

      await _prepareForInitialization();

      // Step 1: 初始化数据库
      try {
        await DatabaseService().init();
      } catch (e) {
        if (kDebugMode) {
          debugPrint('[Protection Monitor] Database init failed: $e');
        }
      }

      await Future.delayed(const Duration(milliseconds: 50));

      // Step 2: 设置资产名称（临时值，后续加载配置后可能校正）
      _protectionService.setAssetName(widget.assetName, widget.assetID);

      // Step 3: 加载防护配置并校正 asset_id 绑定（必须在启用回调桥之前完成）
      await _loadProtectionConfig();

      // Step 4: 启用 Callback Bridge（此时 asset_id 已校正，过滤不会漏掉数据）
      final callbackEnabled = await _protectionService.enableCallbackBridge();
      if (kDebugMode) {
        debugPrint(
          callbackEnabled
              ? '[Monitor] Callback bridge mode enabled'
              : '[Monitor] Using FFI polling mode',
        );
      }

      await Future.delayed(const Duration(milliseconds: 50));

      // Step 5: 加载历史统计数据
      await _protectionService.loadStatisticsFromDatabase();

      if (mounted) {
        setState(() {
          _analysisCount = _protectionService.analysisCount;
          _blockedCount = _protectionService.blockedCount;
          _warningCount = _protectionService.warningCount;
          _totalPromptTokens = _protectionService.totalPromptTokens;
          _totalCompletionTokens = _protectionService.totalCompletionTokens;
          _totalToolCalls = _protectionService.totalToolCalls;
          _auditPromptTokens = _protectionService.auditPromptTokens;
          _auditCompletionTokens = _protectionService.auditCompletionTokens;
          _messageCount = _protectionService.requestCount;
        });
      }

      _updateApiMetrics();

      // 加载历史安全事件
      _loadSecurityEvents();

      await Future.delayed(const Duration(milliseconds: 50));

      final useRelay = widget.relayedLogBatches != null;
      _isRelayMode = useRelay;

      if (useRelay) {
        // Linux 子进程：从主进程中继流接收日志/事件/结果/统计
        _logSubscription = widget.relayedLogBatches!
            .expand((batch) => batch)
            .listen((log) {
              if (!mounted) return;
              final l10n = AppLocalizations.of(context);
              _lastLogReceivedAt = _recordStreamActivity(
                'relayLogs',
                _lastLogReceivedAt,
              );
              processStructuredLog(log);
              if (shouldHideFromRawView(log)) {
                return;
              }
              final translatedLog = l10n != null
                  ? translateLog(log, l10n)
                  : log;
              final entry = LogEntry(
                '[${_formatTime(DateTime.now())}] $translatedLog',
              );
              _pendingLogs.add(entry);
              logUpdateTimer ??= Timer(_logUpdateInterval, flushPendingLogs);
            });

        _resultSubscription = widget.relayedResultStream!.listen((result) {
          if (!mounted) return;
          _lastResultReceivedAt = _recordStreamActivity(
            'relayResult',
            _lastResultReceivedAt,
          );
          pendingResult = result;
          resultUpdateTimer ??= Timer(
            _resultUpdateInterval,
            flushPendingResult,
          );
        });

        _metricsSubscription = widget.relayedStatsStream!.listen((stats) {
          if (!mounted) return;
          _lastMetricsReceivedAt = _recordStreamActivity(
            'relayStats',
            _lastMetricsReceivedAt,
          );
          final nextAnalysisCount =
              stats['analysisCount'] as int? ?? _analysisCount;
          final nextBlockedCount =
              stats['blockedCount'] as int? ?? _blockedCount;
          final nextWarningCount =
              stats['warningCount'] as int? ?? _warningCount;
          final nextTotalPromptTokens =
              stats['totalPromptTokens'] as int? ?? _totalPromptTokens;
          final nextTotalCompletionTokens =
              stats['totalCompletionTokens'] as int? ?? _totalCompletionTokens;
          final nextTotalToolCalls =
              stats['totalToolCalls'] as int? ?? _totalToolCalls;
          final nextAuditPromptTokens =
              stats['auditPromptTokens'] as int? ?? _auditPromptTokens;
          final nextAuditCompletionTokens =
              stats['auditCompletionTokens'] as int? ?? _auditCompletionTokens;
          final nextMessageCount =
              stats['requestCount'] as int? ?? _messageCount;

          final hasChanged =
              nextAnalysisCount != _analysisCount ||
              nextBlockedCount != _blockedCount ||
              nextWarningCount != _warningCount ||
              nextTotalPromptTokens != _totalPromptTokens ||
              nextTotalCompletionTokens != _totalCompletionTokens ||
              nextTotalToolCalls != _totalToolCalls ||
              nextAuditPromptTokens != _auditPromptTokens ||
              nextAuditCompletionTokens != _auditCompletionTokens ||
              nextMessageCount != _messageCount;

          if (hasChanged) {
            setState(() {
              _analysisCount = nextAnalysisCount;
              _blockedCount = nextBlockedCount;
              _warningCount = nextWarningCount;
              _totalPromptTokens = nextTotalPromptTokens;
              _totalCompletionTokens = nextTotalCompletionTokens;
              _totalToolCalls = nextTotalToolCalls;
              _auditPromptTokens = nextAuditPromptTokens;
              _auditCompletionTokens = nextAuditCompletionTokens;
              _messageCount = nextMessageCount;
            });
          }
          // 中继模式下也按节流刷新趋势图（从 DB 拉取 tokenTrend / toolCallTrend）
          final now = DateTime.now();
          if (_lastMetricsUpdate == null ||
              now.difference(_lastMetricsUpdate!) >= _metricsUpdateInterval) {
            _lastMetricsUpdate = now;
            _updateApiMetrics();
          }
        });

        if (widget.relayedTruthRecordStream != null) {
          _truthRecordSubscription = widget.relayedTruthRecordStream!.listen((
            record,
          ) {
            if (!mounted) return;
            _enqueueTruthRecordForUi(record);
          });
        }

        if (widget.relayedSecurityEventStream != null) {
          _securityEventSubscription = widget.relayedSecurityEventStream!
              .listen((events) {
                if (!mounted || events.isEmpty) return;
                setState(() {
                  _securityEvents.insertAll(0, events);
                });
              });
        }
      } else {
        // 本进程流（主窗口或 macOS 同进程子窗口）
        _logSubscription = _protectionService.logStream.listen((log) {
          if (!mounted) return;
          final l10n = AppLocalizations.of(context);
          if (l10n == null) {
            if (kDebugMode) {
              debugPrint('[Monitor] Warning: l10n is null, using raw log');
            }
          }
          _lastLogReceivedAt = _recordStreamActivity(
            'logStream',
            _lastLogReceivedAt,
          );
          processStructuredLog(log);
          if (shouldHideFromRawView(log)) {
            return;
          }
          final translatedLog = l10n != null ? translateLog(log, l10n) : log;
          final entry = LogEntry(
            '[${_formatTime(DateTime.now())}] $translatedLog',
          );
          _pendingLogs.add(entry);
          logUpdateTimer ??= Timer(_logUpdateInterval, flushPendingLogs);
        });

        _resultSubscription = _protectionService.resultStream.listen((result) {
          if (!mounted) return;
          _lastResultReceivedAt = _recordStreamActivity(
            'resultStream',
            _lastResultReceivedAt,
          );
          pendingResult = result;
          resultUpdateTimer ??= Timer(
            _resultUpdateInterval,
            flushPendingResult,
          );
        });

        _metricsSubscription = _protectionService.metricsStream.listen((
          metrics,
        ) {
          if (!mounted) return;

          final now = DateTime.now();
          if (_lastMetricsUpdate == null ||
              now.difference(_lastMetricsUpdate!) >= _metricsUpdateInterval) {
            _lastMetricsUpdate = now;
            _lastMetricsReceivedAt = _recordStreamActivity(
              'metricsStream',
              _lastMetricsReceivedAt,
            );
            _updateApiMetrics();
          }
        });
        _truthRecordSubscription = _protectionService.truthRecordStream.listen((
          record,
        ) {
          if (!mounted) return;
          _enqueueTruthRecordForUi(record);
        });
        _startTruthRecordCatchUp();
      }

      // 订阅安全事件流（非中继模式）
      if (!useRelay) {
        _securityEventSubscription = _protectionService.securityEventStream
            .listen((events) {
              if (!mounted) return;
              setState(() {
                _securityEvents.insertAll(0, events);
              });
            });
        _startSecurityEventCatchUp();
      }

      if (mounted) {
        setState(() {
          _isInitializing = false;
        });
      }

      _startProxyAsync();
    } catch (e) {
      if (kDebugMode) {
        debugPrint('[Protection Monitor] Initialization error: $e');
      }
      if (mounted) {
        setState(() {
          _isInitializing = false;
          _proxyError =
              l10n?.initFailed(e.toString()) ?? 'Initialization failed: $e';
        });
      }
    } finally {
      _initInProgress = false;
    }
  }

  Future<void> _startProxyAsync() async {
    if (!mounted) return;
    final l10n = AppLocalizations.of(context);

    // 查询 Go 端代理真实运行状态
    final proxyStatus = await _protectionService.getProtectionProxyStatus();
    if (proxyStatus['running'] == true) {
      if (mounted) {
        setState(() {
          _isProtectionActive = true;
          _isStartingProxy = false;
          _proxyPort = proxyStatus['port'] ?? _proxyPort;
          _providerName = proxyStatus['provider_name'] ?? _providerName;
        });
        _updateApiMetrics();
      }
      return;
    }

    setState(() {
      _isStartingProxy = true;
    });

    try {
      final dbService = ModelConfigDatabaseService();
      final securityModelConfig = await dbService.getSecurityModelConfig();

      if (securityModelConfig == null || !securityModelConfig.isValid) {
        if (mounted) {
          setState(() {
            _isStartingProxy = false;
            _proxyError =
                l10n?.configureAiModelFirst ??
                'Please configure AI model first';
          });
        }
        return;
      }

      final result = await _protectionService.startProtectionProxy(
        securityModelConfig,
        ProtectionRuntimeConfig(),
      );

      if (!mounted) return;

      if (result['success'] == true) {
        setState(() {
          _isProtectionActive = true;
          _isStartingProxy = false;
          _proxyPort = result['port'];
          _providerName = result['provider_name'];
        });

        try {
          await ProtectionDatabaseService().saveProtectionState(
            enabled: true,
            providerName: result['provider_name'],
            proxyPort: result['port'],
            originalBaseUrl: result['original_base_url'],
          );
        } catch (e) {
          if (kDebugMode) {
            debugPrint(
              '[Protection Monitor] Failed to save protection state: $e',
            );
          }
        }
      } else {
        setState(() {
          _isStartingProxy = false;
          _proxyError = result['error'] ?? 'Unknown error';
        });
      }
    } catch (e) {
      if (!mounted) return;
      setState(() {
        _isStartingProxy = false;
        _proxyError = e.toString();
      });
    }
  }

  // ============ 防护配置 ============

  Future<void> _loadProtectionConfig() async {
    try {
      _protectionConfig = await ProtectionDatabaseService().getProtectionConfig(
        widget.assetName,
        widget.assetID,
      );
      await _loadCurrentBotModelName();
      if (_protectionConfig != null) {
        _auditOnly = _protectionConfig!.auditOnly;
        _protectionService.setAssetName(
          widget.assetName,
          _protectionConfig!.assetID,
        );
      }
    } catch (e) {
      if (kDebugMode) {
        debugPrint('[Protection Monitor] Failed to load protection config: $e');
      }
    }
  }

  Future<void> _reloadProtectionConfig() async {
    try {
      final newConfig = await ProtectionDatabaseService().getProtectionConfig(
        widget.assetName,
        widget.assetID,
      );
      await _loadCurrentBotModelName();
      if (newConfig != null) {
        final auditOnlyChanged = newConfig.auditOnly != _auditOnly;
        _protectionConfig = newConfig;
        _protectionService.setAssetName(widget.assetName, newConfig.assetID);
        if (auditOnlyChanged && mounted) {
          setState(() {
            _auditOnly = newConfig.auditOnly;
          });
        }
      }
    } catch (e) {
      if (kDebugMode) {
        debugPrint(
          '[Protection Monitor] Failed to reload protection config: $e',
        );
      }
    }
  }

  Future<void> _loadCurrentBotModelName() async {
    try {
      final assetID = widget.assetID.isNotEmpty
          ? widget.assetID
          : (_protectionConfig?.assetID ?? '');
      final config = await ModelConfigDatabaseService().getBotModelConfig(
        widget.assetName,
        assetID,
      );
      final modelName = config?.model.trim() ?? '';
      if (!mounted || modelName == _currentBotModelName) return;
      setState(() {
        _currentBotModelName = modelName;
      });
    } catch (e) {
      appLogger.error(
        '[Protection Monitor] Failed to load bot model config',
        e,
      );
    }
  }

  Future<void> _toggleAuditOnlyMode(bool value) async {
    if (_isAnalyzingInProgress) {
      if (mounted) {
        final l10n = AppLocalizations.of(context);
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text(
              l10n?.auditOnlyModePendingHint ??
                  'Change will take effect after analysis completes',
            ),
            duration: const Duration(seconds: 2),
          ),
        );
      }
      return;
    }

    setState(() {
      _auditOnly = value;
    });

    try {
      final success = await _protectionService.updateAuditOnlyMode(
        widget.assetName,
        value,
        widget.assetID.isNotEmpty
            ? widget.assetID
            : (_protectionConfig?.assetID ?? ''),
      );
      if (success) {
        _protectionConfig = await ProtectionDatabaseService()
            .getProtectionConfig(
              widget.assetName,
              widget.assetID.isNotEmpty
                  ? widget.assetID
                  : (_protectionConfig?.assetID ?? ''),
            );
      }
    } catch (e) {
      if (kDebugMode) {
        debugPrint('[Protection Monitor] Failed to save audit-only mode: $e');
      }
    }
  }

  // ============ 工具方法 ============

  String _formatTime(DateTime time) {
    final y = time.year.toString();
    final m = time.month.toString().padLeft(2, '0');
    final d = time.day.toString().padLeft(2, '0');
    final hh = time.hour.toString().padLeft(2, '0');
    final mm = time.minute.toString().padLeft(2, '0');
    final ss = time.second.toString().padLeft(2, '0');
    return '$y-$m-$d $hh:$mm:$ss';
  }

  void _copyText(String text, AppLocalizations l10n) {
    Clipboard.setData(ClipboardData(text: text));
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        content: Text(l10n.appStoreGuideCopied),
        duration: const Duration(seconds: 2),
      ),
    );
  }

  @override
  void dispose() {
    _logSubscription?.cancel();
    _resultSubscription?.cancel();
    _metricsSubscription?.cancel();
    _securityEventSubscription?.cancel();
    _truthRecordSubscription?.cancel();
    _truthRecordCatchUpTimer?.cancel();
    _securityEventCatchUpTimer?.cancel();
    _pendingTruthByRequestId.clear();
    _truthRecordFrameFlushScheduled = false;
    logUpdateTimer?.cancel();
    resultUpdateTimer?.cancel();
    _logScrollController.dispose();
    _horizontalScrollController.dispose();
    try {
      windowManager.removeListener(this);
    } catch (_) {}

    // 关闭监控窗口不停止代理 — 代理生命周期由主窗口管理（启动时自动恢复、退出时统一清理）
    _protectionService.dispose(removeInstance: false);
    super.dispose();
  }

  // ============ 辅助方法 ============

  Color _getRiskColor(RiskLevel level) {
    switch (level) {
      case RiskLevel.safe:
        return const Color(0xFF22C55E);
      case RiskLevel.suspicious:
        return const Color(0xFFF59E0B);
      case RiskLevel.dangerous:
        return const Color(0xFFEF4444);
      case RiskLevel.critical:
        return const Color(0xFFDC2626);
    }
  }

  IconData _getRiskIcon(RiskLevel level) {
    switch (level) {
      case RiskLevel.safe:
        return LucideIcons.shieldCheck;
      case RiskLevel.suspicious:
        return LucideIcons.alertTriangle;
      case RiskLevel.dangerous:
        return LucideIcons.shieldAlert;
      case RiskLevel.critical:
        return LucideIcons.shieldOff;
    }
  }

  // ============ UI 构建 ============

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context)!;

    if (kDebugMode) {
      debugPrint(
        '[Protection Monitor] build called, _isInitializing=$_isInitializing',
      );
    }

    if (_isInitializing) {
      if (kDebugMode) {
        debugPrint('[Protection Monitor] Showing loading screen');
      }
      return Scaffold(
        backgroundColor: _windowBackground,
        body: ProtectionMonitorLoadingScreen(
          l10n: l10n,
          assetName: widget.assetName,
        ),
      );
    }

    if (kDebugMode) {
      debugPrint('[Protection Monitor] Showing main UI');
    }

    final charts = ProtectionMonitorCharts(
      analysisCount: _analysisCount,
      messageCount: _messageCount,
      warningCount: _warningCount,
      blockedCount: _blockedCount,
      totalPromptTokens: _totalPromptTokens,
      totalCompletionTokens: _totalCompletionTokens,
      totalToolCalls: _totalToolCalls,
      auditPromptTokens: _auditPromptTokens,
      auditCompletionTokens: _auditCompletionTokens,
      statistics: _apiStatistics,
    );

    return Scaffold(
      backgroundColor: _windowBackground,
      body: LayoutBuilder(
        builder: (context, constraints) {
          if (constraints.maxHeight < 100 || constraints.maxWidth < 100) {
            return const SizedBox.shrink();
          }
          return Container(
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
                _buildTitleBar(l10n),
                Expanded(
                  child: Container(
                    padding: const EdgeInsets.all(16),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        _buildStatusCard(l10n),
                        const SizedBox(height: 12),
                        charts.buildStatisticsRow(l10n),
                        const SizedBox(height: 12),
                        charts.buildApiMetricsRow(l10n),
                        charts.buildAnalysisMetricsRow(l10n),
                        const SizedBox(height: 12),
                        Expanded(
                          child: Row(
                            children: [
                              if (!_isLogPanelExpanded) ...[
                                Expanded(
                                  flex: 2,
                                  child: Column(
                                    children: [
                                      Expanded(
                                        child: charts.buildTokenTrendChart(l10n),
                                      ),
                                      const SizedBox(height: 12),
                                      Expanded(
                                        child: charts.buildToolCallChart(l10n),
                                      ),
                                    ],
                                  ),
                                ),
                                const SizedBox(width: 12),
                              ],
                              Expanded(
                                flex: _isLogPanelExpanded ? 5 : 3,
                                child: ProtectionMonitorLogPanel(
                                  logs: _logsList,
                                  useGroupedView: _useGroupedView,
                                  requestGroups: _requestGroups,
                                  requestOrder: _requestOrder,
                                  currentBotModelName: _currentBotModelName,
                                  logScrollController: _logScrollController,
                                  horizontalScrollController:
                                      _horizontalScrollController,
                                  defaultModelName:
                                      _protectionConfig
                                          ?.botModelConfig
                                          ?.model ??
                                      '',
                                  isExpanded: _isLogPanelExpanded,
                                  onToggleExpand: () {
                                    setState(() {
                                      _isLogPanelExpanded = !_isLogPanelExpanded;
                                    });
                                  },
                                  onViewModeChanged: (grouped) {
                                    setState(() => _useGroupedView = grouped);
                                  },
                                  onClearLogs: () {
                                    setState(() {
                                      _logsList.clear();
                                      _requestGroups.clear();
                                      _requestOrder.clear();
                                    });
                                  },
                                  onCopyText: _copyText,
                                  onScrollToBottom: scrollToBottom,
                                ),
                              ),
                              const SizedBox(width: 12),
                              Expanded(
                                flex: 2,
                                child: ProtectionMonitorEventPanel(
                                  events: _securityEvents,
                                  onClearEvents: _clearSecurityEvents,
                                  onRefresh: _loadSecurityEvents,
                                ),
                              ),
                            ],
                          ),
                        ),
                      ],
                    ),
                  ),
                ),
              ],
            ),
          );
        },
      ),
    );
  }

  /// 从数据库加载安全事件
  void _loadSecurityEvents() {
    _protectionService
        .getSecurityEvents(limit: 200)
        .then((events) {
          if (mounted) {
            setState(() {
              _securityEvents.clear();
              _securityEvents.addAll(events);
            });
          }
        })
        .catchError((e) {
          if (kDebugMode) {
            debugPrint('[Monitor] Load security events failed: $e');
          }
        });
  }

  /// 清空安全事件
  void _clearSecurityEvents() {
    _protectionService
        .clearAllSecurityEvents()
        .then((_) {
          if (mounted) {
            setState(() {
              _securityEvents.clear();
            });
          }
        })
        .catchError((e) {
          if (kDebugMode) {
            debugPrint('[Monitor] Clear security events failed: $e');
          }
        });
  }

  Widget _buildTitleBar(AppLocalizations l10n) {
    if (Platform.isLinux) {
      return const SizedBox.shrink();
    }

    final riskColor = _getRiskColor(_currentRiskLevel);
    final isWindows = Platform.isWindows;

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
              onPanStart: (_) {
                try {
                  windowManager.startDragging();
                } catch (_) {}
              },
              behavior: HitTestBehavior.translucent,
              child: Padding(
                padding: isWindows
                    ? const EdgeInsets.only(left: 16)
                    : const EdgeInsets.only(left: 78),
                child: Row(
                  children: [
                    Container(
                          padding: const EdgeInsets.all(6),
                          decoration: BoxDecoration(
                            color: riskColor.withValues(alpha: 0.2),
                            borderRadius: BorderRadius.circular(8),
                          ),
                          child: Icon(
                            _getRiskIcon(_currentRiskLevel),
                            color: riskColor,
                            size: 16,
                          ),
                        )
                        .animate(onPlay: (controller) => controller.repeat())
                        .shimmer(duration: 2000.ms, color: Colors.white24),
                    const SizedBox(width: 10),
                    Expanded(
                      child: Column(
                        mainAxisAlignment: MainAxisAlignment.center,
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Text(
                            l10n.protectionMonitorTitle,
                            style: AppFonts.inter(
                              fontSize: 14,
                              fontWeight: FontWeight.w600,
                              color: Colors.white,
                            ),
                          ),
                          Text(
                            widget.assetName,
                            style: AppFonts.inter(
                              fontSize: 11,
                              color: Colors.white54,
                            ),
                          ),
                        ],
                      ),
                    ),
                  ],
                ),
              ),
            ),
          ),
          if (Platform.isWindows) ...[
            const SizedBox(width: 8),
            _buildWindowButton(
              icon: LucideIcons.minus,
              onTap: () => windowManager.minimize(),
            ),
            const SizedBox(width: 8),
            _buildWindowButton(
              icon: _isMaximized ? Icons.filter_none : Icons.crop_square,
              onTap: _toggleMaximize,
            ),
            const SizedBox(width: 8),
            _buildWindowButton(
              icon: LucideIcons.x,
              onTap: () => WindowAnimationHelper.hideWithAnimation(),
              isClose: true,
            ),
            const SizedBox(width: 16),
          ],
          // macOS only: custom -/x
          if (Platform.isMacOS) ...[
            const SizedBox(width: 8),
            _buildWindowButton(
              icon: LucideIcons.minus,
              onTap: () => windowManager.minimize(),
            ),
            const SizedBox(width: 8),
            _buildWindowButton(
              icon: LucideIcons.x,
              onTap: () => WindowAnimationHelper.hideWithAnimation(),
              isClose: true,
            ),
            const SizedBox(width: 16),
          ],
        ],
      ),
    );
  }

  Widget _buildWindowButton({
    required IconData icon,
    required VoidCallback onTap,
    bool isClose = false,
  }) {
    return MouseRegion(
      cursor: SystemMouseCursors.click,
      child: GestureDetector(
        onTap: onTap,
        child: Container(
          width: 28,
          height: 28,
          decoration: BoxDecoration(
            color: isClose
                ? Colors.red.withValues(alpha: 0.2)
                : Colors.white.withValues(alpha: 0.1),
            borderRadius: BorderRadius.circular(6),
          ),
          child: Icon(
            icon,
            size: 14,
            color: isClose ? Colors.red.shade300 : Colors.white70,
          ),
        ),
      ),
    );
  }

  Widget _buildStatusCard(AppLocalizations l10n) {
    // 启动中状态
    if (_isStartingProxy) {
      return Container(
        padding: const EdgeInsets.all(16),
        decoration: BoxDecoration(
          gradient: LinearGradient(
            colors: [
              const Color(0xFF6366F1).withValues(alpha: 0.2),
              const Color(0xFF6366F1).withValues(alpha: 0.1),
            ],
          ),
          borderRadius: BorderRadius.circular(12),
          border: Border.all(
            color: const Color(0xFF6366F1).withValues(alpha: 0.3),
          ),
        ),
        child: Row(
          children: [
            const SizedBox(
              width: 20,
              height: 20,
              child: CircularProgressIndicator(
                strokeWidth: 2,
                valueColor: AlwaysStoppedAnimation<Color>(Color(0xFF6366F1)),
              ),
            ),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    l10n.proxyStarting,
                    style: AppFonts.inter(
                      fontSize: 14,
                      fontWeight: FontWeight.w600,
                      color: Colors.white,
                    ),
                  ),
                  Text(
                    l10n.proxyStartingDesc,
                    style: AppFonts.inter(fontSize: 12, color: Colors.white54),
                  ),
                ],
              ),
            ),
          ],
        ),
      );
    }

    // 错误状态
    if (_proxyError != null) {
      return Container(
        padding: const EdgeInsets.all(16),
        decoration: BoxDecoration(
          gradient: LinearGradient(
            colors: [
              const Color(0xFFEF4444).withValues(alpha: 0.2),
              const Color(0xFFEF4444).withValues(alpha: 0.1),
            ],
          ),
          borderRadius: BorderRadius.circular(12),
          border: Border.all(
            color: const Color(0xFFEF4444).withValues(alpha: 0.3),
          ),
        ),
        child: Row(
          children: [
            const Icon(
              LucideIcons.alertCircle,
              color: Color(0xFFEF4444),
              size: 20,
            ),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    l10n.proxyStartFailed,
                    style: AppFonts.inter(
                      fontSize: 14,
                      fontWeight: FontWeight.w600,
                      color: const Color(0xFFEF4444),
                    ),
                  ),
                  Text(
                    _proxyError!,
                    style: AppFonts.inter(fontSize: 12, color: Colors.white70),
                    maxLines: 2,
                    overflow: TextOverflow.ellipsis,
                  ),
                ],
              ),
            ),
            const SizedBox(width: 8),
            ElevatedButton(
              onPressed: _initializeService,
              style: ElevatedButton.styleFrom(
                backgroundColor: const Color(0xFFEF4444),
                padding: const EdgeInsets.symmetric(
                  horizontal: 12,
                  vertical: 8,
                ),
              ),
              child: Text(
                l10n.retry,
                style: AppFonts.inter(fontSize: 12, color: Colors.white),
              ),
            ),
          ],
        ),
      );
    }

    final riskColor = _getRiskColor(_currentRiskLevel);
    final isAnalyzing = _protectionService.isAnalyzing;

    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        gradient: LinearGradient(
          colors: [
            riskColor.withValues(alpha: 0.2),
            riskColor.withValues(alpha: 0.1),
          ],
        ),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: riskColor.withValues(alpha: 0.3)),
      ),
      child: Row(
        children: [
          Container(
                width: 12,
                height: 12,
                decoration: BoxDecoration(
                  color: riskColor,
                  shape: BoxShape.circle,
                  boxShadow: [
                    BoxShadow(
                      color: riskColor.withValues(alpha: 0.5),
                      blurRadius: 8,
                      spreadRadius: 2,
                    ),
                  ],
                ),
              )
              .animate(onPlay: (controller) => controller.repeat())
              .fadeIn(duration: 800.ms)
              .then()
              .fadeOut(duration: 800.ms),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  l10n.protectionStatus,
                  style: AppFonts.inter(fontSize: 12, color: Colors.white54),
                ),
                Row(
                  children: [
                    Text(
                      _isProtectionActive
                          ? l10n.protectionActive
                          : l10n.protectionInactive,
                      style: AppFonts.inter(
                        fontSize: 16,
                        fontWeight: FontWeight.w600,
                        color: riskColor,
                      ),
                    ),
                    if (_proxyPort != null) ...[
                      const SizedBox(width: 8),
                      Container(
                        padding: const EdgeInsets.symmetric(
                          horizontal: 6,
                          vertical: 2,
                        ),
                        decoration: BoxDecoration(
                          color: Colors.white.withValues(alpha: 0.1),
                          borderRadius: BorderRadius.circular(4),
                        ),
                        child: Text(
                          'Port: $_proxyPort',
                          style: AppFonts.firaCode(
                            fontSize: 10,
                            color: Colors.white54,
                          ),
                        ),
                      ),
                    ],
                    if (_providerName != null) ...[
                      const SizedBox(width: 6),
                      Container(
                        padding: const EdgeInsets.symmetric(
                          horizontal: 6,
                          vertical: 2,
                        ),
                        decoration: BoxDecoration(
                          color: const Color(0xFF6366F1).withValues(alpha: 0.2),
                          borderRadius: BorderRadius.circular(4),
                        ),
                        child: Text(
                          _providerName!,
                          style: AppFonts.firaCode(
                            fontSize: 10,
                            color: const Color(0xFF6366F1),
                          ),
                        ),
                      ),
                    ],
                    if (_latestResult != null) ...[
                      const SizedBox(width: 12),
                      Container(
                        padding: const EdgeInsets.symmetric(
                          horizontal: 8,
                          vertical: 2,
                        ),
                        decoration: BoxDecoration(
                          color: riskColor.withValues(alpha: 0.2),
                          borderRadius: BorderRadius.circular(4),
                        ),
                        child: Text(
                          _currentRiskLevel.displayName,
                          style: AppFonts.firaCode(
                            fontSize: 11,
                            color: riskColor,
                          ),
                        ),
                      ),
                    ],
                  ],
                ),
              ],
            ),
          ),
          if (isAnalyzing)
            Container(
              padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
              decoration: BoxDecoration(
                color: const Color(0xFF6366F1).withValues(alpha: 0.2),
                borderRadius: BorderRadius.circular(20),
              ),
              child: Row(
                children: [
                  SizedBox(
                    width: 14,
                    height: 14,
                    child: CircularProgressIndicator(
                      strokeWidth: 2,
                      valueColor: AlwaysStoppedAnimation<Color>(
                        const Color(0xFF6366F1),
                      ),
                    ),
                  ),
                  const SizedBox(width: 6),
                  Text(
                    l10n.analyzing,
                    style: AppFonts.inter(
                      fontSize: 12,
                      color: const Color(0xFF6366F1),
                    ),
                  ),
                ],
              ),
            )
          else
            Container(
              padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
              decoration: BoxDecoration(
                color: riskColor.withValues(alpha: 0.2),
                borderRadius: BorderRadius.circular(20),
              ),
              child: Row(
                children: [
                  Icon(LucideIcons.activity, color: riskColor, size: 14),
                  const SizedBox(width: 6),
                  Text(
                    l10n.realTimeMonitor,
                    style: AppFonts.inter(fontSize: 12, color: riskColor),
                  ),
                ],
              ),
            ),
          const SizedBox(width: 12),
          _buildAuditOnlySwitch(l10n),
        ],
      ),
    ).animate().fadeIn().slideY(begin: -0.1, end: 0);
  }

  Widget _buildAuditOnlySwitch(AppLocalizations l10n) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
      decoration: BoxDecoration(
        color: _auditOnly
            ? const Color(0xFFF59E0B).withValues(alpha: 0.2)
            : Colors.white.withValues(alpha: 0.1),
        borderRadius: BorderRadius.circular(20),
        border: Border.all(
          color: _auditOnly
              ? const Color(0xFFF59E0B).withValues(alpha: 0.3)
              : Colors.white.withValues(alpha: 0.1),
        ),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(
            _auditOnly ? LucideIcons.eye : LucideIcons.shieldCheck,
            color: _auditOnly ? const Color(0xFFF59E0B) : Colors.white54,
            size: 14,
          ),
          const SizedBox(width: 6),
          Text(
            l10n.auditOnlyModeShort,
            style: AppFonts.inter(
              fontSize: 11,
              color: _auditOnly ? const Color(0xFFF59E0B) : Colors.white54,
            ),
          ),
          const SizedBox(width: 4),
          SizedBox(
            height: 20,
            child: Transform.scale(
              scale: 0.7,
              child: Switch(
                value: _auditOnly,
                onChanged: _toggleAuditOnlyMode,
                activeThumbColor: const Color(0xFFF59E0B),
                activeTrackColor: const Color(
                  0xFFF59E0B,
                ).withValues(alpha: 0.3),
                materialTapTargetSize: MaterialTapTargetSize.shrinkWrap,
              ),
            ),
          ),
        ],
      ),
    );
  }
}
