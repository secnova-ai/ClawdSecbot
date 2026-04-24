// ignore_for_file: avoid_web_libraries_in_flutter, deprecated_member_use

import 'dart:async';
import 'dart:html' as html;
import 'dart:math';

import 'package:flutter/material.dart';
import 'package:flutter_animate/flutter_animate.dart';
import 'package:lucide_icons/lucide_icons.dart';
import 'package:package_info_plus/package_info_plus.dart';

import '../config/build_config.dart';
import '../core_transport/http_transport_web.dart';
import '../core_transport/transport_registry.dart';
import '../l10n/app_localizations.dart';
import '../models/asset_model.dart';
import '../models/protection_config_model.dart';
import '../models/risk_model.dart';
import '../pages/audit_log_page.dart';
import '../pages/protection_monitor_page.dart';
import '../services/app_settings_database_service.dart';
import '../services/model_config_database_service.dart';
import '../services/plugin_service.dart';
import '../services/protection_database_service.dart';
import '../services/protection_service.dart';
import '../services/scan_database_service.dart';
import '../services/scanner_service.dart';
import '../services/skill_security_analyzer_service.dart';
import '../utils/app_fonts.dart';
import '../utils/app_logger.dart';
import '../utils/runtime_platform.dart';
import '../widgets/mitigation_dialog.dart';
import '../widgets/protection_config_dialog.dart';
import '../widgets/scan_result_view.dart';
import '../widgets/settings_dialog.dart';
import '../widgets/skill_scan_dialog.dart';
import '../widgets/skill_scan_results_dialog.dart';

const _configuredApiBaseUrl = String.fromEnvironment(
  'BOTSEC_WEB_API_BASE_URL',
  defaultValue: '',
);
const _defaultApiPort = String.fromEnvironment(
  'BOTSEC_WEB_API_PORT',
  defaultValue: '18080',
);
const _defaultWorkspacePrefix = String.fromEnvironment(
  'BOTSEC_WORKSPACE_DIR_PREFIX',
  defaultValue: '/tmp/botsec_web_workspace',
);
const _defaultHomeDir = String.fromEnvironment(
  'BOTSEC_HOME_DIR',
  defaultValue: '/tmp',
);
const _defaultCurrentVersion = String.fromEnvironment(
  'BOTSEC_CURRENT_VERSION',
  defaultValue: '1.0.3',
);

class WebHomePage extends StatefulWidget {
  const WebHomePage({super.key});

  @override
  State<WebHomePage> createState() => _WebHomePageState();
}

class _WebHomePageState extends State<WebHomePage> {
  static const String _webSessionClientStorageKey = 'botsec_web_client_id';

  late final TextEditingController _apiBaseCtrl;
  late final TextEditingController _workspacePrefixCtrl;
  late final TextEditingController _homeDirCtrl;
  late final TextEditingController _currentVersionCtrl;
  late final ValueNotifier<Locale> _localeNotifier;
  late final String _webSessionClientID;
  late final String _webSessionClientLabel;

  final BotScanner _scanner = BotScanner();
  StreamSubscription<String>? _scanLogSubscription;
  StreamSubscription<html.Event>? _beforeUnloadSubscription;

  HttpTransportWeb? _transport;
  Timer? _bootstrapRetryTimer;
  int _bootstrapRetryAttempts = 0;
  Timer? _sessionHeartbeatTimer;
  Timer? _sessionAcquireRetryTimer;

  ScanState _scanState = ScanState.idle;
  final List<String> _logs = <String>[];
  ScanResult? _result;

  final Set<String> _protectedAssetIDs = <String>{};
  final Map<String, String> _protectedAssetNamesByID = <String, String>{};
  final Set<String> _stoppingProtectionAssetIDs = <String>{};

  final bool _isRestoringProtection = false;
  bool _bootstrapped = false;
  bool _bootstrapping = false;
  bool _showBackendConfig = false;
  String? _bootstrapError;
  bool _uiSessionOwned = false;
  String? _uiSessionError;
  bool _postBootstrapInitialized = false;
  Future<void>? _postBootstrapInitFuture;
  bool _sessionAcquireRetryInFlight = false;
  int _scheduledScanIntervalSeconds = 0;
  Timer? _scheduledScanTimer;
  bool _launchAtStartupEnabled = false;
  bool _apiServerEnabled = false;
  RescanAction _selectedRescanAction = RescanAction.securityDiscovery;

  Locale _locale = const Locale('zh');

  bool get _isZh => _locale.languageCode == 'zh';

  String _txt(String zh, String en) => _isZh ? zh : en;

  @override
  void initState() {
    super.initState();
    final query = Uri.base.queryParameters;

    _apiBaseCtrl = TextEditingController(
      text: query['api_base_url'] ?? _resolveDefaultApiBaseUrl(),
    );
    _workspacePrefixCtrl = TextEditingController(
      text: query['workspace_dir_prefix'] ?? _defaultWorkspacePrefix,
    );
    _homeDirCtrl = TextEditingController(
      text: query['home_dir'] ?? _defaultHomeDir,
    );
    _currentVersionCtrl = TextEditingController(
      text: query['current_version'] ?? _defaultCurrentVersion,
    );

    final queryLang = query['lang']?.toLowerCase();
    if (queryLang == 'en' || queryLang == 'zh') {
      _locale = Locale(queryLang!);
    } else {
      final systemLang =
          WidgetsBinding.instance.platformDispatcher.locale.languageCode;
      _locale = Locale(systemLang.toLowerCase().startsWith('zh') ? 'zh' : 'en');
    }
    _localeNotifier = ValueNotifier<Locale>(_locale);
    _webSessionClientID = _resolveWebSessionClientID();
    _webSessionClientLabel = _resolveWebSessionClientLabel();
    _beforeUnloadSubscription = html.window.onBeforeUnload.listen((_) {
      _releaseUiSessionLock();
    });

    _scanLogSubscription = _scanner.logStream.listen((log) {
      if (!mounted) return;
      setState(() {
        _logs.add(log);
      });
    });

    _setupTransport(resetBootstrapped: true);
    unawaited(_bootstrap(auto: true));
  }

  @override
  void dispose() {
    _bootstrapRetryTimer?.cancel();
    _sessionHeartbeatTimer?.cancel();
    _sessionAcquireRetryTimer?.cancel();
    _scheduledScanTimer?.cancel();
    _beforeUnloadSubscription?.cancel();
    _releaseUiSessionLock();
    _scanLogSubscription?.cancel();
    _scanner.dispose();
    _apiBaseCtrl.dispose();
    _workspacePrefixCtrl.dispose();
    _homeDirCtrl.dispose();
    _currentVersionCtrl.dispose();
    _localeNotifier.dispose();
    super.dispose();
  }

  String _resolveDefaultApiBaseUrl() {
    final configured = _configuredApiBaseUrl.trim();
    if (configured.isNotEmpty) {
      return configured;
    }

    final scheme = Uri.base.scheme == 'https' ? 'https' : 'http';
    final host = Uri.base.host.isNotEmpty ? Uri.base.host : '127.0.0.1';
    return '$scheme://$host:$_defaultApiPort';
  }

  String _resolveWebSessionClientID() {
    try {
      final storage = html.window.sessionStorage;
      final cached = storage[_webSessionClientStorageKey];
      if (cached != null && cached.trim().isNotEmpty) {
        return cached.trim();
      }
      final generated =
          'web-${DateTime.now().microsecondsSinceEpoch}-${Random().nextInt(1 << 30)}';
      storage[_webSessionClientStorageKey] = generated;
      return generated;
    } catch (_) {
      return 'web-${DateTime.now().microsecondsSinceEpoch}';
    }
  }

  String _resolveWebSessionClientLabel() {
    final host = Uri.base.host.trim().isEmpty ? 'unknown-host' : Uri.base.host;
    return '$host:${DateTime.now().millisecondsSinceEpoch}';
  }

  void _setupTransport({required bool resetBootstrapped}) {
    final apiBase = _apiBaseCtrl.text.trim();
    _transport = HttpTransportWeb(
      apiBaseUrl: apiBase,
      isBootstrapped: resetBootstrapped ? false : _bootstrapped,
    );
    TransportRegistry.setTransport(_transport!);
    if (resetBootstrapped) {
      _bootstrapped = false;
      _uiSessionOwned = false;
      _postBootstrapInitialized = false;
      _postBootstrapInitFuture = null;
      _sessionAcquireRetryInFlight = false;
      _sessionAcquireRetryTimer?.cancel();
      _sessionAcquireRetryTimer = null;
    }
  }

  Future<void> _bootstrap({bool auto = false}) async {
    if (_bootstrapping) return;

    _setupTransport(resetBootstrapped: true);

    if (mounted) {
      setState(() {
        _bootstrapping = true;
        _bootstrapError = null;
      });
    }

    final result = _transport!.bootstrapInit(
      workspaceDirPrefix: _workspacePrefixCtrl.text.trim(),
      homeDir: _homeDirCtrl.text.trim(),
      currentVersion: _currentVersionCtrl.text.trim(),
    );

    if (result['success'] != true) {
      final rawError = result['error']?.toString() ?? 'bootstrap failed';
      final retryable = _isRetryableBootstrapError(rawError);
      final autoRepairedApi = retryable && _tryRepairApiBaseForRetryableError();

      if (autoRepairedApi) {
        if (mounted) {
          setState(() {
            _bootstrapping = false;
            _bootstrapped = false;
            _bootstrapError = _txt(
              '已自动修正 API 地址，正在重试连接…',
              'API endpoint auto-corrected, retrying connection...',
            );
            _showBackendConfig = false;
          });
        }
        _scheduleBootstrapRetry();
        return;
      }

      if (mounted) {
        setState(() {
          _bootstrapping = false;
          _bootstrapped = false;
          _bootstrapError = _friendlyBootstrapError(rawError);
          _showBackendConfig = !auto || !retryable;
        });
      }
      if (retryable) {
        _scheduleBootstrapRetry();
      } else {
        _bootstrapRetryTimer?.cancel();
      }
      return;
    }

    final requestedWorkspace = _workspacePrefixCtrl.text.trim();
    final initPaths = result['init_paths'];
    final dbPath = initPaths is Map<String, dynamic>
        ? (initPaths['db_path']?.toString() ?? '')
        : '';
    if (requestedWorkspace.isNotEmpty &&
        dbPath.isNotEmpty &&
        !dbPath.startsWith(requestedWorkspace)) {
      if (mounted) {
        setState(() {
          _bootstrapping = false;
          _bootstrapped = false;
          _bootstrapError = _txt(
            '后端已绑定到其他工作目录，请重启 web bridge 后重试。',
            'Backend is bound to a different workspace. Restart web bridge and retry.',
          );
          _showBackendConfig = true;
        });
      }
      return;
    }

    _bootstrapRetryTimer?.cancel();
    _bootstrapRetryAttempts = 0;

    final sessionAcquired = _tryAcquireUiSessionLock(silent: auto);
    if (!sessionAcquired) {
      if (mounted) {
        setState(() {
          _bootstrapping = false;
          _bootstrapped = true;
          _bootstrapError = null;
        });
      } else {
        _bootstrapping = false;
        _bootstrapped = true;
        _bootstrapError = null;
      }
      _scheduleSessionAcquireRetry();
      return;
    }

    await _runPostBootstrapInitIfNeeded();

    if (mounted) {
      setState(() {
        _bootstrapping = false;
        _bootstrapped = true;
        _bootstrapError = null;
      });
      if (!auto) {
        if (!mounted) return;
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text(_txt('初始化成功', 'Initialized')),
            duration: const Duration(seconds: 2),
          ),
        );
      }
    }
  }

  bool _tryAcquireUiSessionLock({bool silent = false}) {
    if (_transport == null) {
      return false;
    }
    final result = _transport!.claimUiSession(
      clientID: _webSessionClientID,
      clientLabel: _webSessionClientLabel,
    );
    if (result['success'] == true) {
      _sessionAcquireRetryTimer?.cancel();
      _sessionAcquireRetryTimer = null;
      _uiSessionOwned = true;
      _uiSessionError = null;
      _startSessionHeartbeat();
      return true;
    }

    _uiSessionOwned = false;
    _stopSessionHeartbeat();

    final errorCode = result['error_code']?.toString() ?? '';
    if (errorCode == 'ui_session_occupied') {
      _uiSessionError = _buildOccupiedSessionMessage(result);
      if (!silent) {
        appLogger.warning('[WebMain] ui session occupied by another client');
      }
      return false;
    }

    _uiSessionError =
        result['error']?.toString() ??
        _txt('页面会话获取失败，请稍后重试。', 'Failed to acquire web session.');
    return false;
  }

  Future<void> _runPostBootstrapInitIfNeeded() async {
    if (_postBootstrapInitialized) {
      return;
    }
    if (_postBootstrapInitFuture != null) {
      await _postBootstrapInitFuture;
      return;
    }

    _postBootstrapInitFuture = _runPostBootstrapInitInternal();
    await _postBootstrapInitFuture;
  }

  Future<void> _runPostBootstrapInitInternal() async {
    try {
      await PluginService().initializePlugin();
      await _syncLanguageToBackend();
      await _restoreScheduledScanSettings();
      await _restoreSavedScanResult();
      _postBootstrapInitialized = true;
    } catch (e) {
      appLogger.warning('[WebMain] post-bootstrap init failed: $e');
    } finally {
      _postBootstrapInitFuture = null;
    }
  }

  void _startSessionHeartbeat() {
    _sessionHeartbeatTimer?.cancel();
    _sessionHeartbeatTimer = Timer.periodic(const Duration(seconds: 5), (_) {
      if (!_uiSessionOwned || _transport == null) {
        return;
      }

      final result = _transport!.heartbeatUiSession(
        clientID: _webSessionClientID,
        clientLabel: _webSessionClientLabel,
      );
      if (result['success'] == true) {
        return;
      }

      _uiSessionOwned = false;
      _uiSessionError = _buildOccupiedSessionMessage(result);
      _stopSessionHeartbeat();
      _scheduleSessionAcquireRetry();
      if (mounted) {
        setState(() {});
      }
    });
  }

  void _stopSessionHeartbeat() {
    _sessionHeartbeatTimer?.cancel();
    _sessionHeartbeatTimer = null;
  }

  void _scheduleSessionAcquireRetry() {
    if (_sessionAcquireRetryTimer != null) {
      return;
    }
    _sessionAcquireRetryTimer = Timer.periodic(const Duration(seconds: 3), (_) {
      unawaited(_handleSessionAcquireRetryTick());
    });
  }

  Future<void> _handleSessionAcquireRetryTick() async {
    if (_sessionAcquireRetryInFlight) {
      return;
    }
    _sessionAcquireRetryInFlight = true;
    try {
      if (_transport == null || !_bootstrapped || _uiSessionOwned) {
        return;
      }
      final acquired = _tryAcquireUiSessionLock(silent: true);
      if (!acquired) {
        if (mounted) {
          setState(() {});
        }
        return;
      }
      await _runPostBootstrapInitIfNeeded();
      _sessionAcquireRetryTimer?.cancel();
      _sessionAcquireRetryTimer = null;
      if (mounted) {
        setState(() {});
      }
    } finally {
      _sessionAcquireRetryInFlight = false;
    }
  }

  void _releaseUiSessionLock() {
    _stopSessionHeartbeat();
    _sessionAcquireRetryTimer?.cancel();
    _sessionAcquireRetryTimer = null;
    _sessionAcquireRetryInFlight = false;
    if (_transport == null) {
      return;
    }
    _transport!.releaseUiSession(
      clientID: _webSessionClientID,
      clientLabel: _webSessionClientLabel,
    );
  }

  String _buildOccupiedSessionMessage(Map<String, dynamic> result) {
    final remainingMs = (result['remaining_ms'] as num?)?.toInt() ?? 0;
    final seconds = remainingMs > 0 ? (remainingMs / 1000).ceil() : null;
    final owner = result['owner_client_label']?.toString().trim() ?? '';
    final ownerText = owner.isEmpty
        ? ''
        : _txt('当前占用: $owner。', 'Current owner: $owner. ');
    final waitText = seconds == null || seconds <= 0
        ? _txt(
            '检测到另一个页面正在使用系统，当前页面会自动重试接管。',
            'Another page is using the system. This page will retry automatically.',
          )
        : _txt(
            '检测到另一个页面正在使用系统，约 ${seconds}s 后自动重试接管。',
            'Another page is using the system. Retrying in about ${seconds}s.',
          );
    return '$ownerText$waitText';
  }

  void _scheduleBootstrapRetry() {
    if (_bootstrapped || _bootstrapping) {
      return;
    }
    _bootstrapRetryTimer?.cancel();
    _bootstrapRetryAttempts += 1;
    final seconds = _retryDelaySeconds(_bootstrapRetryAttempts);
    _bootstrapRetryTimer = Timer(Duration(seconds: seconds), () {
      if (!mounted || _bootstrapped) return;
      unawaited(_bootstrap(auto: true));
    });
  }

  int _retryDelaySeconds(int attempts) {
    if (attempts <= 1) return 1;
    if (attempts == 2) return 2;
    if (attempts == 3) return 4;
    if (attempts == 4) return 8;
    return 15;
  }

  String _friendlyBootstrapError(String error) {
    if (_isRetryableBootstrapError(error)) {
      final api = _apiBaseCtrl.text.trim();
      return _txt(
        '后端服务连接失败，系统会自动重试。请确认 API 地址可访问：$api',
        'Backend connection failed. Auto-retrying. Please verify API endpoint: $api',
      );
    }
    return error;
  }

  bool _isRetryableBootstrapError(String error) {
    final lower = error.toLowerCase();
    return lower.contains('http 0') ||
        lower.contains('failed to connect') ||
        lower.contains('xmlhttprequest') ||
        lower.contains('networkerror') ||
        lower.contains('connection refused');
  }

  Future<void> _retryBootstrapNow() async {
    _bootstrapRetryTimer?.cancel();
    _bootstrapRetryAttempts = 0;
    await _bootstrap(auto: true);
  }

  bool _tryRepairApiBaseForRetryableError() {
    final current = _apiBaseCtrl.text.trim();
    final repaired = _repairApiBaseForCurrentPage(current);
    if (repaired == null || repaired == current) {
      return false;
    }
    _apiBaseCtrl.text = repaired;
    return true;
  }

  String? _repairApiBaseForCurrentPage(String current) {
    final base = Uri.tryParse(current);
    if (base == null || base.host.isEmpty) {
      return null;
    }

    final page = Uri.base;
    final pageHost = page.host.trim();
    if (pageHost.isEmpty) {
      return null;
    }

    final pageIsLocal = _isLocalHost(pageHost);
    final apiIsLocal = _isLocalHost(base.host);

    if (!pageIsLocal && apiIsLocal) {
      final scheme = page.scheme == 'https' ? 'https' : 'http';
      final port = base.hasPort
          ? base.port
          : (int.tryParse(_defaultApiPort) ?? 18080);
      return Uri(scheme: scheme, host: pageHost, port: port).toString();
    }

    if (page.scheme == 'https' &&
        base.scheme == 'http' &&
        base.host == pageHost) {
      final port = base.hasPort ? base.port : null;
      return Uri(scheme: 'https', host: pageHost, port: port).toString();
    }

    return null;
  }

  bool _isLocalHost(String host) {
    final h = host.toLowerCase();
    return h == 'localhost' || h == '127.0.0.1' || h == '::1';
  }

  Future<void> _restoreSavedScanResult() async {
    try {
      final savedResult = await ScanDatabaseService().getLatestScanResult();
      if (!mounted || savedResult == null) return;

      setState(() {
        _result = savedResult;
        _scanState = ScanState.completed;
      });
      await _syncProtectedAssetsWithScanResult(savedResult.assets);
    } catch (e) {
      appLogger.warning('[WebMain] restore saved scan result failed: $e');
    }
  }

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
    } catch (e) {
      appLogger.warning('[WebMain] restore scheduled scan settings failed: $e');
    }
  }

  Future<void> _updateScheduledScanInterval(int seconds) async {
    final success = await AppSettingsDatabaseService()
        .setScheduledScanIntervalSeconds(seconds);
    if (!success) {
      appLogger.warning('[WebMain] failed to save scheduled scan interval');
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
  }

  void _configureScheduledScanTimer() {
    _scheduledScanTimer?.cancel();
    _scheduledScanTimer = null;

    if (_scheduledScanIntervalSeconds <= 0) {
      return;
    }

    final interval = Duration(seconds: _scheduledScanIntervalSeconds);
    _scheduledScanTimer = Timer.periodic(interval, (_) {
      _startScan();
    });
  }

  Future<void> _saveGeneralSettings({
    required bool launchAtStartupEnabled,
    required int scheduledScanIntervalSeconds,
  }) async {
    if (launchAtStartupEnabled != _launchAtStartupEnabled) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text(
              _txt(
                'Web 版本暂不支持开机启动设置',
                'Launch at startup is not supported on Web',
              ),
            ),
            duration: const Duration(seconds: 2),
          ),
        );
      }
      _launchAtStartupEnabled = false;
    }
    if (scheduledScanIntervalSeconds != _scheduledScanIntervalSeconds) {
      await _updateScheduledScanInterval(scheduledScanIntervalSeconds);
    }
  }

  Future<void> _syncLanguageToBackend() async {
    if (!_bootstrapped && _transport != null) {
      // bootstrap flow calls this before _bootstrapped is flipped.
    }

    try {
      _transport!.callOneArg('SetLanguageFFI', _locale.languageCode);
      await AppSettingsDatabaseService().setLanguage(_locale.languageCode);
    } catch (e) {
      appLogger.warning('[WebMain] set language failed: $e');
    }
  }

  Future<void> _changeLocale(String code) async {
    final nextLocale = Locale(code == 'zh' ? 'zh' : 'en');
    setState(() {
      _locale = nextLocale;
    });
    _localeNotifier.value = nextLocale;
    if (_transport != null) {
      await _syncLanguageToBackend();
    }
  }

  AppLocalizations _currentL10n() {
    final l10n = AppLocalizations.of(context);
    if (l10n != null) return l10n;
    final code = _locale.languageCode.toLowerCase();
    return code == 'zh'
        ? lookupAppLocalizations(const Locale('zh'))
        : lookupAppLocalizations(const Locale('en'));
  }

  Future<T?> _showLocalizedDialog<T>({
    bool barrierDismissible = true,
    required WidgetBuilder builder,
  }) {
    return showDialog<T>(
      context: context,
      barrierDismissible: barrierDismissible,
      builder: (dialogContext) {
        return ValueListenableBuilder<Locale>(
          valueListenable: _localeNotifier,
          builder: (dialogCtx, locale, child) {
            return Localizations.override(
              context: dialogCtx,
              locale: locale,
              child: Builder(builder: builder),
            );
          },
        );
      },
    );
  }

  Future<void> _handleSettingsTap() async {
    await _showLocalizedDialog<void>(
      builder: (dialogContext) => SettingsDialog(
        launchAtStartupEnabled: _launchAtStartupEnabled,
        onSaveGeneralSettings: _saveGeneralSettings,
        scheduledScanIntervalSeconds: _scheduledScanIntervalSeconds,
        onClearData: () {
          Navigator.of(dialogContext).pop();
          _showClearDataConfirmDialog();
        },
        onRestoreConfig: () {
          Navigator.of(dialogContext).pop();
          _showRestoreConfigConfirmDialog();
        },
        onShowAbout: () {
          Navigator.of(dialogContext).pop();
          _showWebAboutDialog();
        },
        onReauthorizeDirectory: () {
          Navigator.of(dialogContext).pop();
          ScaffoldMessenger.of(context).showSnackBar(
            SnackBar(
              content: Text(
                _txt(
                  'Web 版本不需要目录重新授权',
                  'Web build does not require directory reauthorization',
                ),
              ),
              duration: const Duration(seconds: 2),
            ),
          );
        },
        apiServerEnabled: _apiServerEnabled,
        onToggleApiServer: (enabled) {
          setState(() {
            _apiServerEnabled = enabled;
          });
        },
      ),
    );
  }

  Future<void> _showWebAboutDialog() async {
    final packageInfo = await PackageInfo.fromPlatform();
    if (!mounted) return;

    final l10n = _currentL10n();
    await _showLocalizedDialog<void>(
      builder: (context) => AlertDialog(
        backgroundColor: const Color(0xFF1A1A2E),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
        title: Text(
          l10n.aboutApp(l10n.appTitle),
          style: AppFonts.inter(
            color: Colors.white,
            fontWeight: FontWeight.w600,
          ),
        ),
        content: Text(
          l10n.aboutVersionWithBuild(
            packageInfo.version,
            packageInfo.buildNumber,
          ),
          style: AppFonts.inter(color: Colors.white70),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(),
            child: Text(l10n.close),
          ),
        ],
      ),
    );
  }

  Future<void> _showClearDataConfirmDialog() async {
    final l10n = _currentL10n();
    final confirmed = await _showLocalizedDialog<bool>(
      builder: (context) => AlertDialog(
        backgroundColor: const Color(0xFF1A1A2E),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
        title: Text(
          l10n.clearDataConfirmTitle,
          style: AppFonts.inter(
            color: Colors.white,
            fontWeight: FontWeight.w600,
          ),
        ),
        content: Text(
          l10n.clearDataConfirmMessage,
          style: AppFonts.inter(color: Colors.white70),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(false),
            child: Text(l10n.cancel),
          ),
          ElevatedButton(
            onPressed: () => Navigator.of(context).pop(true),
            style: ElevatedButton.styleFrom(
              backgroundColor: const Color(0xFFEF4444),
            ),
            child: Text(l10n.clear, style: AppFonts.inter(color: Colors.white)),
          ),
        ],
      ),
    );

    if (confirmed != true) return;
    await _clearAllData();
  }

  Future<void> _clearAllData() async {
    final l10n = _currentL10n();
    try {
      final service = ProtectionService();
      await service.fullReset();
      if (_transport != null && _transport!.isReady) {
        _transport!.callNoArg('ClearAllDataFFI');
      }

      if (!mounted) return;
      setState(() {
        _scanState = ScanState.idle;
        _logs.clear();
        _result = null;
        _protectedAssetIDs.clear();
        _protectedAssetNamesByID.clear();
        _stoppingProtectionAssetIDs.clear();
      });
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(l10n.clearDataSuccess),
          backgroundColor: Colors.green,
          duration: const Duration(seconds: 2),
        ),
      );
    } catch (e) {
      appLogger.error('[WebMain] clear all data failed', e);
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(l10n.clearDataFailed),
          backgroundColor: Colors.red,
          duration: const Duration(seconds: 2),
        ),
      );
    }
  }

  Future<void> _showRestoreConfigConfirmDialog() async {
    final l10n = _currentL10n();
    final service = ProtectionService();
    final hasBackup = await service.hasInitialBackup();
    if (!mounted) return;

    if (!hasBackup) {
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(l10n.restoreConfigNoBackup),
          backgroundColor: Colors.orange,
          duration: const Duration(seconds: 2),
        ),
      );
      return;
    }

    final confirmed = await _showLocalizedDialog<bool>(
      builder: (context) => AlertDialog(
        backgroundColor: const Color(0xFF1A1A2E),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
        title: Text(
          l10n.restoreConfigConfirmTitle,
          style: AppFonts.inter(
            color: Colors.white,
            fontWeight: FontWeight.w600,
          ),
        ),
        content: Text(
          l10n.restoreConfigConfirmMessage,
          style: AppFonts.inter(color: Colors.white70),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(false),
            child: Text(l10n.cancel),
          ),
          ElevatedButton(
            onPressed: () => Navigator.of(context).pop(true),
            style: ElevatedButton.styleFrom(
              backgroundColor: const Color(0xFFEAB308),
            ),
            child: Text(
              l10n.continueButton,
              style: AppFonts.inter(color: Colors.white),
            ),
          ),
        ],
      ),
    );

    if (confirmed != true) return;
    await _restoreConfig();
  }

  Future<void> _restoreConfig() async {
    final l10n = _currentL10n();
    try {
      final service = ProtectionService();
      final result = await service.restoreToInitialConfig();
      if (result['success'] == true) {
        if (!mounted) return;
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text(l10n.restoreConfigSuccess),
            backgroundColor: Colors.green,
            duration: const Duration(seconds: 2),
          ),
        );
        await _startScan(skipReconcile: true);
      } else {
        throw Exception(result['error'] ?? 'restore config failed');
      }
    } catch (e) {
      appLogger.error('[WebMain] restore config failed', e);
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(l10n.restoreConfigFailed(e.toString())),
          backgroundColor: Colors.red,
          duration: const Duration(seconds: 2),
        ),
      );
    }
  }

  Future<void> _startScan({bool skipReconcile = false}) async {
    if (_scanState == ScanState.scanning) return;

    if (!_bootstrapped) {
      await _bootstrap();
      if (!_bootstrapped) {
        return;
      }
    }
    if (!_uiSessionOwned) {
      _scheduleSessionAcquireRetry();
      if (mounted) {
        setState(() {});
      }
      return;
    }

    if (!mounted) return;
    final l10n = _currentL10n();
    final beforeAssetIDs =
        _result?.assets.map((a) => a.id).toSet() ?? <String>{};

    setState(() {
      _scanState = ScanState.scanning;
      _logs.clear();
      _result = null;
    });

    _scanner.setLocalization(l10n);

    try {
      final result = await _scanner.scan();

      try {
        await ScanDatabaseService().saveScanResult(result);
      } catch (e) {
        appLogger.error('[WebMain] save scan result failed', e);
      }

      if (!mounted) return;

      setState(() {
        _scanState = ScanState.completed;
        _result = result;
      });

      if (!skipReconcile) {
        try {
          await _reconcileAssetsAfterScan(beforeAssetIDs, result.assets);
        } catch (e) {
          appLogger.error('[WebMain] reconcile failed', e);
        }
      }

      await _syncProtectedAssetsWithScanResult(result.assets);
    } catch (e) {
      appLogger.error('[WebMain] scan failed', e);
      if (!mounted) return;
      setState(() {
        _scanState = ScanState.idle;
      });
    }
  }

  Future<void> _resetScan() async {
    if (!mounted) return;
    final l10n = _currentL10n();

    try {
      final activeCount = await ProtectionDatabaseService()
          .getActiveProtectionCount();

      if (activeCount > 0 && mounted) {
        final confirmed = await _showLocalizedDialog<bool>(
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
                Expanded(
                  child: Text(
                    l10n.rescanConfirmTitle,
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
        _stoppingProtectionAssetIDs.clear();
      }

      if (!mounted) return;
      setState(() {
        _scanState = ScanState.idle;
        _logs.clear();
        _result = null;
      });
    } catch (e) {
      appLogger.error('[WebMain] reset scan failed', e);
    }
  }

  Future<void> _showProtectionMonitorResolved(Asset asset) async {
    final config = await ProtectionDatabaseService().getProtectionConfig(
      asset.name,
      asset.id,
    );
    final resolvedID = (config != null && config.assetID.isNotEmpty)
        ? config.assetID
        : asset.id;

    if (!mounted) return;
    await _showLocalizedDialog<void>(
      builder: (context) => _WebProtectionMonitorDialogShell(
        assetName: asset.name,
        assetID: resolvedID,
      ),
    );
  }

  Future<void> _stopProtectionForAsset(Asset asset) async {
    final scanAssetID = asset.id.trim();
    if (_stoppingProtectionAssetIDs.contains(scanAssetID)) {
      return;
    }

    setState(() {
      _stoppingProtectionAssetIDs.add(scanAssetID);
    });

    String resolvedAssetID = scanAssetID;
    String resolvedAssetName = asset.name;

    try {
      final config = await ProtectionDatabaseService().getProtectionConfig(
        asset.name,
        scanAssetID,
      );
      if (config != null) {
        resolvedAssetID = config.assetID.trim().isNotEmpty
            ? config.assetID.trim()
            : resolvedAssetID;
        resolvedAssetName = config.assetName.trim().isNotEmpty
            ? config.assetName.trim()
            : resolvedAssetName;
      }

      if (resolvedAssetID != scanAssetID && mounted) {
        setState(() {
          _stoppingProtectionAssetIDs.add(resolvedAssetID);
        });
        appLogger.info(
          '[WebMain] Resolved stop protection asset_id: scan=$scanAssetID -> config=$resolvedAssetID',
        );
      }

      final service = ProtectionService.forAsset(
        resolvedAssetName,
        resolvedAssetID,
      );
      service.setAssetName(resolvedAssetName, resolvedAssetID);
      final stopResult = await service.stopProtectionProxy();
      if (stopResult['success'] != true) {
        throw Exception(stopResult['error'] ?? 'stop protection failed');
      }

      await ProtectionDatabaseService().setProtectionEnabled(
        resolvedAssetName,
        false,
        resolvedAssetID,
      );

      if (!mounted) return;
      setState(() {
        _protectedAssetIDs
          ..remove(scanAssetID)
          ..remove(resolvedAssetID);
        _protectedAssetNamesByID
          ..remove(scanAssetID)
          ..remove(resolvedAssetID);
      });

      final l10n = _currentL10n();
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(l10n.stopProtectionSuccess),
          backgroundColor: Colors.green,
          duration: const Duration(seconds: 2),
        ),
      );
      appLogger.info(
        '[WebMain] Stopped protection for $resolvedAssetName/$resolvedAssetID',
      );
    } catch (e) {
      appLogger.error(
        '[WebMain] Failed to stop protection for $resolvedAssetName/$resolvedAssetID',
        e,
      );
      if (mounted) {
        final l10n = _currentL10n();
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text(l10n.stopProtectionFailed('$e')),
            backgroundColor: Colors.red,
            duration: const Duration(seconds: 3),
          ),
        );
      }
    } finally {
      if (mounted) {
        setState(() {
          _stoppingProtectionAssetIDs
            ..remove(scanAssetID)
            ..remove(resolvedAssetID);
        });
      }
    }
  }

  Future<void> _showProtectionConfigDialog(
    Asset asset, {
    required bool isEditMode,
  }) async {
    final result = await _showLocalizedDialog<ProtectionConfig>(
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
      await _showProtectionMonitorResolved(asset);

      Future.delayed(const Duration(seconds: 5), () {
        if (mounted && _scanState != ScanState.scanning) {
          _startScan(skipReconcile: true);
        }
      });
      return;
    }

    if (result != null && isEditMode && _protectedAssetIDs.contains(asset.id)) {
      if (isRuntimeMacOS && BuildConfig.isPersonal) {
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

          if (updateResult && mounted) {
            final l10n = _currentL10n();
            ScaffoldMessenger.of(context).showSnackBar(
              SnackBar(
                content: Text(l10n.configUpdated),
                backgroundColor: Colors.green,
                duration: const Duration(seconds: 2),
              ),
            );
          }
        }
      } catch (e) {
        appLogger.error('[WebMain] hot update failed', e);
      }
    }
  }

  void _showMitigationDialog(RiskInfo risk) async {
    final result = await _showLocalizedDialog(
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

  void _handleRescanActionChanged(RescanAction action) {
    if (_selectedRescanAction == action) return;
    setState(() {
      _selectedRescanAction = action;
    });
  }

  Future<void> _deleteRiskSkill(RiskInfo risk) async {
    final l10n = _currentL10n();
    final args = risk.args ?? const <String, Object>{};
    final skillName = (args['skillName'] ?? args['skill_name'] ?? '')
        .toString()
        .trim();
    final skillPath = (args['skillPath'] ?? args['skill_path'] ?? '')
        .toString()
        .trim();
    final skillHash = (args['skillHash'] ?? args['skill_hash'] ?? '')
        .toString()
        .trim();

    if (skillPath.isEmpty || skillHash.isEmpty) {
      if (!mounted) return;
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(SnackBar(content: Text(l10n.deleteRiskSkillUnavailable)));
      return;
    }

    final displayName = skillName.isNotEmpty ? skillName : skillPath;
    final confirmed = await _showLocalizedDialog<bool>(
      builder: (dialogContext) => AlertDialog(
        title: Text(l10n.deleteRiskSkill),
        content: Text(l10n.deleteRiskSkillConfirm(displayName)),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(dialogContext).pop(false),
            child: Text(l10n.cancel),
          ),
          TextButton(
            onPressed: () => Navigator.of(dialogContext).pop(true),
            child: Text(_txt('确认', 'Confirm')),
          ),
        ],
      ),
    );

    if (confirmed != true) return;

    final deleteResult = await SkillSecurityAnalyzerService().deleteSkill(
      skillPath: skillPath,
      skillHash: skillHash,
    );
    if (!mounted) return;

    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        content: Text(
          deleteResult.success
              ? (deleteResult.alreadyMissing
                    ? l10n.deleteRiskSkillAlreadyMissing
                    : l10n.deleteRiskSkillSuccess)
              : l10n.deleteRiskSkillFailed,
        ),
      ),
    );

    if (deleteResult.success) {
      await _startScan();
    }
  }

  void _showSkillScanDialog() async {
    await _showLocalizedDialog(
      barrierDismissible: false,
      builder: (context) => const SkillScanDialog(),
    );

    if (mounted) {
      _startScan();
    }
  }

  void _showSkillScanResultsDialog() {
    unawaited(
      _showLocalizedDialog(
        builder: (context) => const SkillScanResultsDialog(),
      ),
    );
  }

  Future<void> _showAuditLogDialog() async {
    if (!mounted) return;
    await _showLocalizedDialog<void>(
      builder: (context) => const _WebAuditLogDialogShell(),
    );
  }

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

    if (idDisappeared.isEmpty) return;

    final trulyMissing = idDisappeared.where((id) {
      final name = _protectedAssetNamesByID[id] ?? '';
      return name.isEmpty || !afterAssetNames.contains(name);
    }).toList();

    if (trulyMissing.isEmpty) return;

    final removed = <String>[];

    for (final assetID in trulyMissing) {
      final assetName = _protectedAssetNamesByID[assetID] ?? '';

      try {
        await PluginService().notifyPluginAppExit(assetName, assetID);
      } catch (_) {}

      try {
        final service = ProtectionService.forAsset(assetName, assetID);
        service.setAssetName(assetName, assetID);
        await service.stopProtectionProxy();
      } catch (_) {}

      try {
        await ProtectionDatabaseService().setProtectionEnabled(
          assetName,
          false,
          assetID,
        );
      } catch (_) {}

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
  }

  Future<void> _syncProtectedAssetsWithScanResult(List<Asset> assets) async {
    final enabledConfigs = await ProtectionDatabaseService()
        .getEnabledProtectionConfigs();

    if (enabledConfigs.isEmpty) {
      if (_protectedAssetIDs.isNotEmpty && mounted) {
        setState(() {
          _protectedAssetIDs.clear();
          _protectedAssetNamesByID.clear();
          _stoppingProtectionAssetIDs.clear();
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
          newIDs.add(scanID);
          newNames[scanID] = config.assetName;
        } else {
          newIDs.add(config.assetID);
          newNames[config.assetID] = config.assetName;
        }
      }
    }

    if (!mounted) return;
    setState(() {
      _protectedAssetIDs
        ..clear()
        ..addAll(newIDs);
      _protectedAssetNamesByID
        ..clear()
        ..addAll(newNames);
    });
  }

  @override
  Widget build(BuildContext context) {
    return Localizations.override(
      context: context,
      locale: _locale,
      child: Builder(
        builder: (context) {
          final l10n = AppLocalizations.of(context)!;
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
                  _buildTitleBar(l10n),
                  if (_showBackendConfig) _buildBackendConfigPanel(l10n),
                  if (!_bootstrapped) _buildBootstrapStatusBanner(),
                  Expanded(
                    child: AnimatedSwitcher(
                      duration: const Duration(milliseconds: 520),
                      reverseDuration: const Duration(milliseconds: 360),
                      switchInCurve: Curves.easeOutCubic,
                      switchOutCurve: Curves.easeInCubic,
                      transitionBuilder: _buildContentTransition,
                      child: _buildContent(l10n),
                    ),
                  ),
                ],
              ),
            ),
          );
        },
      ),
    );
  }

  Widget _buildTitleBar(AppLocalizations l10n) {
    final isOccupied = _bootstrapped && !_uiSessionOwned;
    final canScan =
        _bootstrapped && _uiSessionOwned && _scanState != ScanState.scanning;
    final statusText = isOccupied
        ? _txt('已被占用', 'Occupied')
        : _bootstrapped
        ? _txt('运行中', 'Running')
        : _bootstrapping
        ? _txt('连接中', 'Connecting')
        : _txt('重连中', 'Reconnecting');
    final statusColor = isOccupied
        ? const Color(0xFFF59E0B)
        : _bootstrapped
        ? const Color(0xFF22C55E)
        : _bootstrapping
        ? const Color(0xFF60A5FA)
        : const Color(0xFFF59E0B);

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
          const SizedBox(width: 12),
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
          const SizedBox(width: 12),
          Text(
            statusText,
            style: AppFonts.inter(fontSize: 11, color: statusColor),
          ),
          const Spacer(),
          IconButton(
            icon: const Icon(
              LucideIcons.fileSearch,
              size: 16,
              color: Colors.white70,
            ),
            tooltip: l10n.auditLog,
            onPressed: isOccupied ? null : _showAuditLogDialog,
          ),
          IconButton(
            icon: const Icon(LucideIcons.cpu, size: 16, color: Colors.white70),
            tooltip: l10n.settings,
            onPressed: isOccupied ? null : _handleSettingsTap,
          ),
          TextButton.icon(
            onPressed: () {
              setState(() {
                _showBackendConfig = !_showBackendConfig;
              });
            },
            icon: const Icon(LucideIcons.server, size: 14),
            label: Text(_txt('连接配置', 'Connection')),
            style: TextButton.styleFrom(foregroundColor: Colors.white70),
          ),
          const SizedBox(width: 6),
          PopupMenuButton<String>(
            icon: const Icon(
              LucideIcons.languages,
              size: 16,
              color: Colors.white70,
            ),
            color: const Color(0xFF1A1A2E),
            tooltip: l10n.switchLanguage,
            itemBuilder: (context) => const [
              PopupMenuItem(
                value: 'zh',
                child: Text('中文', style: TextStyle(color: Colors.white)),
              ),
              PopupMenuItem(
                value: 'en',
                child: Text('English', style: TextStyle(color: Colors.white)),
              ),
            ],
            onSelected: _changeLocale,
          ),
          const SizedBox(width: 8),
          FilledButton.icon(
            onPressed: canScan ? _startScan : null,
            icon: _scanState == ScanState.scanning
                ? const SizedBox(
                    width: 14,
                    height: 14,
                    child: CircularProgressIndicator(
                      strokeWidth: 2,
                      color: Colors.white,
                    ),
                  )
                : Icon(
                    _scanState == ScanState.completed
                        ? LucideIcons.refreshCw
                        : LucideIcons.scan,
                    size: 14,
                  ),
            label: Text(
              _scanState == ScanState.completed ? l10n.rescan : l10n.startScan,
            ),
            style: FilledButton.styleFrom(
              backgroundColor: const Color(0xFF6366F1),
            ),
          ),
          const SizedBox(width: 12),
        ],
      ),
    );
  }

  Widget _buildBackendConfigPanel(AppLocalizations l10n) {
    return Container(
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: Colors.black.withValues(alpha: 0.18),
        border: Border(
          bottom: BorderSide(color: Colors.white.withValues(alpha: 0.08)),
        ),
      ),
      child: Column(
        children: [
          Wrap(
            spacing: 10,
            runSpacing: 10,
            children: [
              _buildConfigField(
                _txt('API 地址', 'API Endpoint'),
                _apiBaseCtrl,
                width: 320,
              ),
              _buildConfigField(
                _txt('工作目录前缀', 'Workspace Prefix'),
                _workspacePrefixCtrl,
                width: 320,
              ),
              _buildConfigField(
                _txt('Home 目录', 'Home Directory'),
                _homeDirCtrl,
                width: 220,
              ),
              _buildConfigField(
                _txt('当前版本', 'Current Version'),
                _currentVersionCtrl,
                width: 140,
              ),
            ],
          ),
          const SizedBox(height: 10),
          Row(
            children: [
              FilledButton.icon(
                onPressed: _bootstrapping ? null : _retryBootstrapNow,
                icon: _bootstrapping
                    ? const SizedBox(
                        width: 12,
                        height: 12,
                        child: CircularProgressIndicator(
                          strokeWidth: 2,
                          color: Colors.white,
                        ),
                      )
                    : const Icon(LucideIcons.refreshCw, size: 14),
                label: Text(_txt('重新连接后端', 'Reconnect Backend')),
              ),
              const SizedBox(width: 8),
              OutlinedButton(
                onPressed: _bootstrapping
                    ? null
                    : () {
                        setState(() {
                          _showBackendConfig = true;
                        });
                        _retryBootstrapNow();
                      },
                child: Text(_txt('应用配置并重连', 'Apply Config and Reconnect')),
              ),
            ],
          ),
        ],
      ),
    );
  }

  Widget _buildBootstrapStatusBanner() {
    final message = _bootstrapError?.trim().isNotEmpty == true
        ? _bootstrapError!.trim()
        : _txt('正在自动连接后端服务…', 'Connecting to backend automatically...');
    final isError = _bootstrapError?.trim().isNotEmpty == true;

    return Container(
      width: double.infinity,
      margin: const EdgeInsets.fromLTRB(12, 8, 12, 0),
      padding: const EdgeInsets.all(10),
      decoration: BoxDecoration(
        color: isError
            ? const Color(0xFF7F1D1D).withValues(alpha: 0.35)
            : const Color(0xFF0B3A6E).withValues(alpha: 0.35),
        borderRadius: BorderRadius.circular(8),
        border: Border.all(
          color: isError
              ? const Color(0xFFEF4444).withValues(alpha: 0.5)
              : const Color(0xFF60A5FA).withValues(alpha: 0.5),
        ),
      ),
      child: Row(
        children: [
          if (_bootstrapping)
            const SizedBox(
              width: 14,
              height: 14,
              child: CircularProgressIndicator(
                strokeWidth: 2,
                color: Color(0xFF93C5FD),
              ),
            )
          else
            Icon(
              isError ? LucideIcons.alertCircle : LucideIcons.server,
              size: 14,
              color: isError
                  ? const Color(0xFFFCA5A5)
                  : const Color(0xFF93C5FD),
            ),
          const SizedBox(width: 8),
          Expanded(
            child: Text(
              message,
              style: AppFonts.inter(
                color: isError
                    ? const Color(0xFFFEE2E2)
                    : const Color(0xFFDBEAFE),
                fontSize: 12,
              ),
            ),
          ),
          const SizedBox(width: 8),
          TextButton(
            onPressed: _bootstrapping ? null : _retryBootstrapNow,
            child: Text(_txt('立即重试', 'Retry Now')),
          ),
        ],
      ),
    );
  }

  Widget _buildConfigField(
    String label,
    TextEditingController controller, {
    required double width,
  }) {
    return SizedBox(
      width: width,
      child: TextField(
        controller: controller,
        style: AppFonts.inter(color: Colors.white, fontSize: 13),
        decoration: InputDecoration(
          isDense: true,
          labelText: label,
          labelStyle: AppFonts.inter(color: Colors.white70, fontSize: 12),
          filled: true,
          fillColor: const Color(0xFF142039),
          border: OutlineInputBorder(
            borderRadius: BorderRadius.circular(8),
            borderSide: const BorderSide(color: Color(0xFF33476B)),
          ),
          enabledBorder: OutlineInputBorder(
            borderRadius: BorderRadius.circular(8),
            borderSide: const BorderSide(color: Color(0xFF33476B)),
          ),
          focusedBorder: OutlineInputBorder(
            borderRadius: BorderRadius.circular(8),
            borderSide: const BorderSide(color: Color(0xFF4F7FD9), width: 1.5),
          ),
        ),
      ),
    );
  }

  Widget _buildContent(AppLocalizations l10n) {
    if (_bootstrapped && !_uiSessionOwned) {
      return _buildSessionOccupiedState(l10n);
    }
    switch (_scanState) {
      case ScanState.idle:
        return _buildIdleState(l10n);
      case ScanState.scanning:
        return _buildScanningState(l10n);
      case ScanState.completed:
        return _buildCompletedState(l10n);
    }
  }

  Widget _buildSessionOccupiedState(AppLocalizations l10n) {
    final message = (_uiSessionError?.trim().isNotEmpty == true)
        ? _uiSessionError!.trim()
        : _txt(
            '检测到另一个浏览器页面正在使用系统，当前页面会自动重试。',
            'Another browser page is using the system. This page will retry automatically.',
          );

    return Center(
      key: const ValueKey('session-occupied'),
      child: Container(
        constraints: const BoxConstraints(maxWidth: 760),
        margin: const EdgeInsets.all(24),
        padding: const EdgeInsets.all(24),
        decoration: BoxDecoration(
          color: Colors.black.withValues(alpha: 0.35),
          borderRadius: BorderRadius.circular(16),
          border: Border.all(
            color: const Color(0xFFF59E0B).withValues(alpha: 0.45),
          ),
        ),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Container(
                  padding: const EdgeInsets.all(8),
                  decoration: BoxDecoration(
                    color: const Color(0xFFF59E0B).withValues(alpha: 0.18),
                    borderRadius: BorderRadius.circular(10),
                  ),
                  child: const Icon(
                    LucideIcons.monitorStop,
                    color: Color(0xFFF59E0B),
                    size: 18,
                  ),
                ),
                const SizedBox(width: 10),
                Text(
                  _txt('页面已被占用', 'Session Occupied'),
                  style: AppFonts.inter(
                    fontSize: 18,
                    fontWeight: FontWeight.w700,
                    color: Colors.white,
                  ),
                ),
              ],
            ),
            const SizedBox(height: 14),
            Text(
              message,
              style: AppFonts.inter(
                fontSize: 13,
                color: Colors.white70,
                height: 1.45,
              ),
            ),
            const SizedBox(height: 16),
            Wrap(
              spacing: 10,
              runSpacing: 10,
              children: [
                FilledButton.icon(
                  onPressed: () async {
                    final acquired = _tryAcquireUiSessionLock();
                    if (acquired && mounted) {
                      await _runPostBootstrapInitIfNeeded();
                      setState(() {});
                      return;
                    }
                    _scheduleSessionAcquireRetry();
                    if (mounted) {
                      setState(() {});
                    }
                  },
                  icon: const Icon(LucideIcons.refreshCw, size: 14),
                  label: Text(_txt('立即重试接管', 'Retry Takeover')),
                ),
                OutlinedButton(
                  onPressed: () {
                    setState(() {
                      _showBackendConfig = true;
                    });
                  },
                  child: Text(_txt('查看连接配置', 'Show Connection Settings')),
                ),
              ],
            ),
            const SizedBox(height: 6),
            Text(
              _txt(
                '提示：关闭占用页面后，本页面会自动恢复可操作状态。',
                'Tip: once the occupying page closes, this page will recover automatically.',
              ),
              style: AppFonts.inter(fontSize: 11, color: Colors.white54),
            ),
          ],
        ),
      ),
    );
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

    return FadeTransition(
      opacity: fadeAnimation,
      child: SlideTransition(position: slideAnimation, child: child),
    );
  }

  Widget _buildIdleState(AppLocalizations l10n) {
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
          const SizedBox(height: 28),
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
          const SizedBox(height: 34),
          FilledButton.icon(
            onPressed: (_bootstrapped && _uiSessionOwned) ? _startScan : null,
            icon: const Icon(LucideIcons.scan, size: 16),
            label: Text(l10n.startScan),
            style: FilledButton.styleFrom(
              backgroundColor: const Color(0xFF6366F1),
              padding: const EdgeInsets.symmetric(horizontal: 28, vertical: 16),
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

  Widget _buildScanningState(AppLocalizations l10n) {
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

  Widget _buildCompletedState(AppLocalizations l10n) {
    final result = _result;
    if (result == null) {
      return const SizedBox.shrink();
    }

    return ScanResultView(
      result: result,
      protectedAssets: _protectedAssetIDs,
      isRestoringProtection: _isRestoringProtection,
      stoppingProtectionAssets: _stoppingProtectionAssetIDs,
      selectedRescanAction: _selectedRescanAction,
      onRescanActionChanged: _handleRescanActionChanged,
      onRescan: _resetScan,
      onViewSkillScanResults: _showSkillScanResultsDialog,
      onShowProtectionConfig: _showProtectionConfigDialog,
      onShowProtectionMonitor: (asset) => _showProtectionMonitorResolved(asset),
      onStopProtection: _stopProtectionForAsset,
      onShowMitigation: _showMitigationDialog,
      onDeleteRiskSkill: _deleteRiskSkill,
    );
  }
}

class _WebProtectionMonitorDialogShell extends StatelessWidget {
  const _WebProtectionMonitorDialogShell({
    required this.assetName,
    required this.assetID,
  });

  final String assetName;
  final String assetID;

  @override
  Widget build(BuildContext context) {
    final size = MediaQuery.of(context).size;
    final width = (size.width - 64).clamp(960.0, 1560.0);
    final height = (size.height - 48).clamp(640.0, 980.0);

    return Dialog(
      insetPadding: const EdgeInsets.symmetric(horizontal: 24, vertical: 20),
      backgroundColor: Colors.transparent,
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
      child: ClipRRect(
        borderRadius: BorderRadius.circular(16),
        child: SizedBox(
          width: width,
          height: height,
          child: ProtectionMonitorPage(
            windowId: 'web-dialog',
            assetName: assetName,
            assetID: assetID,
            onRequestClose: () async {
              if (context.mounted && Navigator.of(context).canPop()) {
                Navigator.of(context).pop();
              }
            },
          ),
        ),
      ),
    );
  }
}

class _WebAuditLogDialogShell extends StatelessWidget {
  const _WebAuditLogDialogShell();

  @override
  Widget build(BuildContext context) {
    final size = MediaQuery.of(context).size;
    final width = (size.width - 64).clamp(1080.0, 1680.0);
    final height = (size.height - 48).clamp(680.0, 1020.0);

    return Dialog(
      insetPadding: const EdgeInsets.symmetric(horizontal: 24, vertical: 20),
      backgroundColor: Colors.transparent,
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
      child: ClipRRect(
        borderRadius: BorderRadius.circular(16),
        child: SizedBox(
          width: width,
          height: height,
          child: AuditLogPage(
            windowId: 'web-dialog-audit',
            onRequestClose: () async {
              if (context.mounted && Navigator.of(context).canPop()) {
                Navigator.of(context).pop();
              }
            },
          ),
        ),
      ),
    );
  }
}
