import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'package:desktop_multi_window/desktop_multi_window.dart';
import 'package:flutter/material.dart';
import 'package:window_manager/window_manager.dart';
import '../../models/protection_analysis_model.dart';
import '../../models/truth_record_model.dart';
import '../../services/protection_database_service.dart';
import '../../models/security_event_model.dart';
import '../../services/protection_service.dart';
import '../../utils/app_logger.dart';
import '../../utils/window_animation_helper.dart';
import '../main_page.dart';

/// 窗口管理 Mixin
/// 负责主窗口、审计日志窗口、防护监控窗口的管理
mixin MainPageWindowMixin on State<MainPage>, WindowListener {
  // ============ 状态变量 ============

  /// 审计日志窗口缓存
  WindowController? auditLogWindow;

  /// 正在打开的审计日志窗口 Future
  Future<WindowController>? openingAuditLogWindow;

  /// 防护监控窗口缓存（按资产实例索引：优先 assetID）
  final Map<String, WindowController> protectionMonitorWindows = {};

  /// 正在打开的防护监控窗口 Future（按资产实例索引：优先 assetID）
  final Map<String, Future<WindowController>> openingProtectionMonitorWindows =
      {};

  // ============ Linux 子窗口日志中继（仅多进程时使用）============

  static const _relayInterval = Duration(milliseconds: 300);
  static const _relayStatsInterval = Duration(milliseconds: 500);

  final Map<String, List<String>> _relayLogBuffers = {};
  final Map<String, StreamSubscription<String>> _relayLogSubscriptions = {};
  final Map<String, StreamSubscription<ProtectionAnalysisResult>>
  _relayResultSubscriptions = {};
  final Map<String, ProtectionService> _relayServices = {};
  final Map<String, StreamSubscription<List<SecurityEvent>>>
  _relaySecurityEventSubscriptions = {};
  final Map<String, StreamSubscription<TruthRecordModel>>
  _relayTruthRecordSubscriptions = {};
  final Map<String, Timer> _relayTimers = {};
  final Map<String, Timer> _relayStatsTimers = {};
  final Map<String, bool> _relayStatsInFlight = {};

  // ============ 主窗口方法 ============

  @override
  Future<void> onWindowClose() async {
    await hideMainWindow();
  }

  /// 显示主窗口
  Future<void> showWindow() async {
    try {
      if (await windowManager.isMinimized()) {
        await windowManager.restore();
        await windowManager.focus();
        return;
      }
    } catch (_) {}
    try {
      if (!await windowManager.isVisible()) {
        await WindowAnimationHelper.showWithAnimation();
        return;
      }
    } catch (_) {}
    await WindowAnimationHelper.showWithAnimation();
  }

  Future<void> closeMainWindow() async {
    await hideMainWindow();
  }

  Future<void> hideMainWindow() async {
    await WindowAnimationHelper.hideWithAnimation();
  }

  Future<void> minimizeMainWindow() async {
    await WindowAnimationHelper.minimizeWithAnimation();
  }

  // ============ 审计日志窗口方法 ============

  /// 显示审计日志窗口
  void showAuditLogWindow() async {
    await openAuditLogWindow();
  }

  /// 创建审计日志窗口实例
  Future<WindowController> createAuditLogWindow([
    String assetName = '',
    String assetID = '',
  ]) async {
    final currentLocale = Localizations.localeOf(context).languageCode;
    return WindowController.create(
      WindowConfiguration(
        hiddenAtLaunch: true,
        arguments: jsonEncode({
          'windowType': 'audit_log',
          'locale': currentLocale,
          'assetName': assetName,
          'assetID': assetID,
        }),
      ),
    );
  }

  /// 将已有的审计日志窗口带到前台
  Future<void> bringAuditLogToFront(WindowController controller) async {
    try {
      await controller.show();
    } catch (e) {
      appLogger.warning(
        '[MainPage] Audit log window unavailable, recreate: $e',
      );
      auditLogWindow = null;
      await openAuditLogWindow();
    }
  }

  /// 打开审计日志窗口（单例行为）
  Future<void> openAuditLogWindow([
    String assetName = '',
    String assetID = '',
  ]) async {
    final existing = auditLogWindow;
    if (existing != null) {
      await bringAuditLogToFront(existing);
      return;
    }

    final opening = openingAuditLogWindow;
    if (opening != null) {
      final controller = await opening;
      await bringAuditLogToFront(controller);
      return;
    }

    final createFuture = createAuditLogWindow(assetName, assetID);
    openingAuditLogWindow = createFuture;
    try {
      final controller = await createFuture;
      auditLogWindow = controller;
    } catch (e) {
      appLogger.error('[MainPage] Open audit log window failed', e);
    } finally {
      openingAuditLogWindow = null;
    }
  }

  // ============ 防护监控窗口方法 ============

  /// 显示防护监控窗口
  void showProtectionMonitor(String assetName, [String assetID = '']) {
    openProtectionMonitorWindow(assetName, assetID);
  }

  String _monitorWindowKey(String assetName, [String assetID = '']) {
    if (assetID.isNotEmpty) return assetID;
    return assetName;
  }

  /// 创建防护监控窗口实例
  Future<WindowController> createProtectionMonitorWindow(
    String assetName,
    String assetID,
  ) async {
    final currentLocale = Localizations.localeOf(context).languageCode;
    return WindowController.create(
      WindowConfiguration(
        hiddenAtLaunch: true,
        arguments: jsonEncode({
          'assetName': assetName,
          'assetID': assetID,
          'locale': currentLocale,
        }),
      ),
    );
  }

  /// 将已有的防护监控窗口带到前台
  Future<void> bringProtectionMonitorToFront(
    String assetName,
    String assetID,
    WindowController controller,
  ) async {
    final windowKey = _monitorWindowKey(assetName, assetID);
    try {
      await controller.show();
    } catch (e) {
      appLogger.warning(
        '[MainPage] Protection monitor window unavailable, recreate: $e',
      );
      _stopRelayForWindowKey(windowKey);
      protectionMonitorWindows.remove(windowKey);
      await openProtectionMonitorWindow(assetName, assetID);
    }
  }

  /// 打开防护监控窗口（按资产单例行为）
  Future<void> openProtectionMonitorWindow(
    String assetName, [
    String assetID = '',
  ]) async {
    final windowKey = _monitorWindowKey(assetName, assetID);
    final existing = protectionMonitorWindows[windowKey];
    if (existing != null) {
      await bringProtectionMonitorToFront(assetName, assetID, existing);
      return;
    }

    final opening = openingProtectionMonitorWindows[windowKey];
    if (opening != null) {
      final controller = await opening;
      await bringProtectionMonitorToFront(assetName, assetID, controller);
      return;
    }

    final createFuture = createProtectionMonitorWindow(assetName, assetID);
    openingProtectionMonitorWindows[windowKey] = createFuture;
    try {
      final controller = await createFuture;
      protectionMonitorWindows[windowKey] = controller;
      if (Platform.isLinux) {
        _startRelayForWindowKey(windowKey, assetName, assetID, controller);
      }
    } catch (e) {
      appLogger.error('[MainPage] Open protection monitor failed', e);
    } finally {
      openingProtectionMonitorWindows.remove(windowKey);
    }
  }

  /// 启动 Linux 子窗口数据中继。
  void _startRelayForWindowKey(
    String windowKey,
    String assetName,
    String assetID,
    WindowController controller,
  ) {
    if (!mounted) return;
    final service = _relayServices.putIfAbsent(
      windowKey,
      () => ProtectionService.forAsset(assetName, assetID),
    );

    _relayLogBuffers[windowKey] = [];

    _relayLogSubscriptions[windowKey] = service.logStream.listen((log) {
      _relayLogBuffers[windowKey]?.add(log);
    });

    _relayResultSubscriptions[windowKey] = service.resultStream.listen((
      result,
    ) {
      controller
          .invokeMethod('relayResult', jsonEncode(result.toJson()))
          .catchError((e) {
            if (e.toString().contains('disposed') ||
                e.toString().contains('closed')) {
              _stopRelayForWindowKey(windowKey);
            }
          });
    });

    _relaySecurityEventSubscriptions[windowKey] = service.securityEventStream
        .listen((events) {
          if (events.isEmpty) return;
          final payload = events.map((e) => e.toJson()).toList();
          controller
              .invokeMethod('relaySecurityEvents', jsonEncode(payload))
              .catchError((e) {
                if (e.toString().contains('disposed') ||
                    e.toString().contains('closed')) {
                  _stopRelayForWindowKey(windowKey);
                }
              });
        });

    _relayTruthRecordSubscriptions[windowKey] = service.truthRecordStream
        .listen((record) {
          controller
              .invokeMethod('relayTruthRecords', jsonEncode([record.toJson()]))
              .catchError((e) {
                if (e.toString().contains('disposed') ||
                    e.toString().contains('closed')) {
                  _stopRelayForWindowKey(windowKey);
                }
              });
        });

    _relayTimers[windowKey] = Timer.periodic(_relayInterval, (_) {
      if (!mounted) return;
      final ctrl = protectionMonitorWindows[windowKey];
      if (ctrl == null) return;

      final logBuf = _relayLogBuffers[windowKey];
      if (logBuf != null && logBuf.isNotEmpty) {
        final batch = List<String>.from(logBuf);
        logBuf.clear();
        ctrl.invokeMethod('relayLogs', jsonEncode(batch)).catchError((_) {});
      }
    });

    _relayStatsTimers[windowKey] = Timer.periodic(_relayStatsInterval, (_) {
      if (!mounted) return;
      final ctrl = protectionMonitorWindows[windowKey];
      if (ctrl == null) return;
      if (_relayStatsInFlight[windowKey] == true) return;
      _relayStatsInFlight[windowKey] = true;
      ProtectionDatabaseService()
          .getProtectionStatistics(assetName, assetID)
          .then((stats) {
            if (!mounted) return;
            final currentCtrl = protectionMonitorWindows[windowKey];
            if (currentCtrl == null) return;
            final payload = {
              'analysisCount': stats?.analysisCount ?? 0,
              'blockedCount': stats?.blockedCount ?? 0,
              'warningCount': stats?.warningCount ?? 0,
              'totalPromptTokens': stats?.totalPromptTokens ?? 0,
              'totalCompletionTokens': stats?.totalCompletionTokens ?? 0,
              'totalToolCalls': stats?.totalToolCalls ?? 0,
              'auditPromptTokens': stats?.auditPromptTokens ?? 0,
              'auditCompletionTokens': stats?.auditCompletionTokens ?? 0,
              'requestCount': stats?.requestCount ?? 0,
            };
            currentCtrl
                .invokeMethod('relayStats', jsonEncode(payload))
                .catchError((_) {});
          })
          .catchError((e) {
            appLogger.warning(
              '[MainPage] Relay stats query failed for $windowKey: $e',
            );
          })
          .whenComplete(() {
            _relayStatsInFlight[windowKey] = false;
          });
    });
  }

  /// 停止指定窗口的数据中继（仅取消订阅，不 dispose 共享的 ProtectionService）
  void _stopRelayForWindowKey(String windowKey) {
    _relayLogSubscriptions.remove(windowKey)?.cancel();
    _relayResultSubscriptions.remove(windowKey)?.cancel();
    _relaySecurityEventSubscriptions.remove(windowKey)?.cancel();
    _relayTruthRecordSubscriptions.remove(windowKey)?.cancel();
    _relayTimers.remove(windowKey)?.cancel();
    _relayStatsTimers.remove(windowKey)?.cancel();
    _relayStatsInFlight.remove(windowKey);
    _relayLogBuffers.remove(windowKey);
    _relayServices.remove(windowKey);
  }

  /// 关闭所有监控窗口
  Future<void> closeAllMonitorWindows() async {
    for (final windowKey in protectionMonitorWindows.keys.toList()) {
      _stopRelayForWindowKey(windowKey);
    }
    for (final entry in protectionMonitorWindows.entries.toList()) {
      try {
        await entry.value.invokeMethod('window_close');
      } catch (_) {}
    }
    protectionMonitorWindows.clear();
    openingProtectionMonitorWindows.clear();
  }

  /// 向所有监控窗口发送语言更新
  Future<void> notifyMonitorWindowsLanguageUpdate(String language) async {
    for (final controller in protectionMonitorWindows.values) {
      try {
        await controller.invokeMethod('updateLanguage', language);
        appLogger.info(
          '[MainPage] Sent updateLanguage to window ${controller.windowId}',
        );
      } catch (e) {
        appLogger.warning(
          '[MainPage] Failed to update language for window ${controller.windowId}: $e',
        );
      }
    }
  }

  Future<void> notifyMonitorWindowsProtectionConfigReload() async {
    for (final controller in protectionMonitorWindows.values) {
      try {
        await controller.invokeMethod('reloadProtectionConfig');
        appLogger.info(
          '[MainPage] Sent reloadProtectionConfig to window ${controller.windowId}',
        );
      } catch (e) {
        appLogger.warning(
          '[MainPage] Failed to reload protection config for window ${controller.windowId}: $e',
        );
      }
    }
  }
}
