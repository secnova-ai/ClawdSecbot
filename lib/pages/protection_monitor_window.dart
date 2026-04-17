import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:desktop_multi_window/desktop_multi_window.dart';
import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter/services.dart';
import 'package:window_manager/window_manager.dart';

import '../l10n/app_localizations.dart';
import '../models/protection_analysis_model.dart';
import '../models/security_event_model.dart';
import '../models/truth_record_model.dart';
import '../pages/protection_monitor_page.dart';
import '../services/model_config_database_service.dart';
import '../services/protection_service.dart';
import '../utils/app_fonts.dart';
import '../utils/app_logger.dart';
import '../utils/window_animation_helper.dart';
import '../widgets/hide_window_shortcut.dart';

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
        onRequestStartDragging: () async {
          try {
            await windowManager.startDragging();
          } catch (_) {}
        },
        onRequestMinimize: () async {
          try {
            await windowManager.minimize();
          } catch (_) {}
        },
        onRequestToggleMaximize: () async {
          try {
            final maximized = await windowManager.isMaximized();
            if (maximized) {
              await windowManager.unmaximize();
            } else {
              await windowManager.maximize();
            }
          } catch (_) {}
        },
        onRequestClose: () async {
          await WindowAnimationHelper.hideWithAnimation();
        },
        initialMaximized: false,
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
