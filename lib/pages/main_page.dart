import 'dart:async';
import 'dart:convert';
import 'dart:ffi' as ffi;
import 'dart:io';
import 'dart:ui' show AppExitResponse;
import 'package:ffi/ffi.dart';
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
import '../utils/window_animation_helper.dart';
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

    // 从数据库恢复已启用的防护状态
    await _restoreProtectionStates();

    // 启动版本检查服务
    await startVersionCheckService();
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

  Future<void> _restoreProtectionStates() async {
    try {
      appLogger.info('[MainPage] Restoring protection states');
      final enabledConfigs = await ProtectionDatabaseService()
          .getEnabledProtectionConfigs();
      if (enabledConfigs.isEmpty) {
        appLogger.info('[MainPage] No enabled protection configs');
        return;
      }

      appLogger.info(
        '[MainPage] Restoring ${enabledConfigs.length} protected assets',
      );

      for (final config in enabledConfigs) {
        if (config.assetID.isEmpty) {
          appLogger.warning(
            '[MainPage] Skip restoring protection without assetID: ${config.assetName}',
          );
          continue;
        }

        _protectedAssetIDs.add(config.assetID);
        _protectedAssetNamesByID[config.assetID] = config.assetName;
      }
      appLogger.info(
        '[MainPage] Enabled assets: ${_protectedAssetIDs.map((id) => '${_protectedAssetNamesByID[id]}/$id').join(', ')}',
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

      final assetID = _protectedAssetIDs.isNotEmpty
          ? _protectedAssetIDs.first
          : '';
      final assetName = _protectedAssetNamesByID[assetID] ?? 'Openclaw';

      final service = ProtectionService();
      service.setAssetName(assetName, assetID);

      final result = await service.startProtectionProxy(
        securityModelConfig,
        ProtectionRuntimeConfig(),
      );
      appLogger.info(
        '[MainPage] Background proxy start result: success=${result['success']}, already_running=${result['already_running']}',
      );

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
    if (_isExitFlowInProgress) {
      return;
    }
    _isExitFlowInProgress = true;

    try {
      final enabledConfigs = await _loadExitTargets();
      if (enabledConfigs.isNotEmpty) {
        if (mounted) {
          await showWindow();
        }
        final confirmed = await _showExitRestoreDialog(enabledConfigs.length);
        if (confirmed != true) {
          return;
        }

        final failures = await _runExitCleanupWithProgress(
          () => _restoreTargetsForExit(enabledConfigs),
        );
        if (failures.isNotEmpty) {
          await _showExitFailureDialog(failures);
          return;
        }
      }

      await _finalizeAppExit();
    } finally {
      _isExitFlowInProgress = false;
    }
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

  Future<void> _startScan() async {
    if (Platform.isMacOS &&
        BuildConfig.requiresDirectoryAuth &&
        !_hasConfigAccess) {
      final authorized = await _showConfigAccessDialog();
      if (!authorized) return;
    }

    setState(() {
      _scanState = ScanState.scanning;
      _logs.clear();
      _result = null;
    });

    await windowManager.setSize(AppConstants.windowSize);

    _logSubscription = _scanner.logStream.listen((log) {
      setState(() {
        _logs.add(log);
      });
    });

    // 设置本地化，确保风险标题显示中文
    if (!mounted) return;
    _scanner.setLocalization(AppLocalizations.of(context)!);

    final result = await _scanner.scan();

    try {
      await ScanDatabaseService().saveScanResult(result);
    } catch (e) {
      appLogger.error('[MainPage] Failed to save scan result', e);
    }

    _logSubscription?.cancel();

    setState(() {
      _scanState = ScanState.completed;
      _result = result;
    });

    await windowManager.setSize(AppConstants.windowSize);
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
      showProtectionMonitor(asset.name, asset.id);
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
            final service = ProtectionService();
            service.setAssetName(asset.name, asset.id);
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
        onToggleLaunchAtStartup: () => toggleLaunchAtStartup(),
        onClearData: () {
          Navigator.of(dialogContext).pop();
          showClearDataConfirmDialog();
        },
        onRestoreConfig: () {
          Navigator.of(dialogContext).pop();
          showRestoreConfigConfirmDialog();
        },
        onReauthorizeDirectory: () {
          Navigator.of(dialogContext).pop();
          reauthorizeDirectory();
        },
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
                    duration: const Duration(milliseconds: 400),
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
            onTap: () => windowManager.minimize(),
          ),
          const SizedBox(width: 8),
          _buildWindowButton(
            icon: LucideIcons.x,
            onTap: () => WindowAnimationHelper.hideWithAnimation(),
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
      onShowProtectionMonitor: (asset) =>
          showProtectionMonitor(asset.name, asset.id),
      onShowMitigation: (risk) => _showMitigationDialog(context, risk),
    );
  }
}
