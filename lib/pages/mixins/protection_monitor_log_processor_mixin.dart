import 'dart:async';
import 'dart:convert';
import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import '../../models/protection_analysis_model.dart';
import '../../models/truth_record_model.dart';
import '../../services/protection_service.dart';
import '../../utils/app_logger.dart';
import '../protection_monitor_window.dart';

mixin ProtectionMonitorLogProcessorMixin on State<ProtectionMonitorPage> {
  // ============ 需要主 State 提供的状态 ============
  List<LogEntry> get logsList;
  int get maxLogCount;
  Map<String, TruthRecordModel> get requestGroups;
  List<String> get requestOrder;
  List<LogEntry> get pendingLogs;
  Timer? get logUpdateTimer;
  set logUpdateTimer(Timer? value);
  ScrollController get logScrollController;
  bool get useGroupedView;
  bool get autoScrollEnabled;
  bool get userScrolledAway;
  set userScrolledAway(bool value);

  ProtectionAnalysisResult? get pendingResult;
  set pendingResult(ProtectionAnalysisResult? value);
  Timer? get resultUpdateTimer;
  set resultUpdateTimer(Timer? value);
  set latestResult(ProtectionAnalysisResult? value);
  set currentRiskLevel(RiskLevel value);

  ProtectionService get protectionService;
  void updateCountersFromService();

  void flushPendingLogs() {
    if (!mounted || pendingLogs.isEmpty) {
      logUpdateTimer = null;
      return;
    }

    final logsToAdd = List<LogEntry>.from(pendingLogs);
    pendingLogs.clear();

    setState(() {
      logsList.addAll(logsToAdd);
      if (logsList.length > maxLogCount) {
        logsList.removeRange(0, logsList.length - maxLogCount);
      }
    });

    if (autoScrollEnabled && !userScrolledAway) {
      scrollToBottom();
    }

    logUpdateTimer = null;
  }

  void onLogScroll() {
    if (!logScrollController.hasClients || useGroupedView) return;

    final position = logScrollController.position;
    final maxScroll = position.maxScrollExtent;
    final currentScroll = position.pixels;
    final isAtBottom = (maxScroll - currentScroll).abs() < 50;

    if (isAtBottom && userScrolledAway) {
      setState(() {
        userScrolledAway = false;
      });
    } else if (!isAtBottom && !userScrolledAway) {
      setState(() {
        userScrolledAway = true;
      });
    }
  }

  void scrollToBottom() {
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (logScrollController.hasClients) {
        logScrollController.animateTo(
          logScrollController.position.maxScrollExtent,
          duration: const Duration(milliseconds: 200),
          curve: Curves.easeOut,
        );
      }
    });
  }

  void flushPendingResult() {
    if (!mounted || pendingResult == null) {
      resultUpdateTimer = null;
      return;
    }

    final result = pendingResult!;
    pendingResult = null;
    resultUpdateTimer = null;

    setState(() {
      latestResult = result;
      currentRiskLevel = result.riskLevel;
      updateCountersFromService();
    });
  }

  /// 更新分组映射与顺序列表(不排序); 须在 [setState] 内与 [_finalizeTruthRecordOrder] 成对使用.
  void _applyTruthRecordMutation(TruthRecordModel record) {
    final requestId = record.requestId.trim();
    if (requestId.isEmpty) {
      return;
    }

    var existed = requestGroups.containsKey(requestId);
    String? mergedKey;

    if (!existed && record.isComplete) {
      for (final entry in requestGroups.entries) {
        final existing = entry.value;
        final sameAsset = existing.assetID == record.assetID;
        final sameModel = existing.model == record.model;
        final sameContent =
            existing.primaryContent.isNotEmpty &&
            existing.primaryContent == record.primaryContent;
        final closeInTime =
            existing.startedAt.difference(record.startedAt).abs().inSeconds <=
            10;
        final inFlight = !existing.isComplete;
        if (sameAsset && sameModel && sameContent && closeInTime && inFlight) {
          mergedKey = entry.key;
          break;
        }
      }
    }

    final targetKey = mergedKey ?? requestId;
    if (kDebugMode) {
      appLogger.debug(
        '[TruthRecord] process request_id=$requestId target_key=$targetKey existed=$existed phase=${record.phase} type=${record.primaryContentType} complete=${record.isComplete}',
      );
    }
    requestGroups[targetKey] = record;
    if (!requestOrder.contains(targetKey)) {
      requestOrder.add(targetKey);
    }
  }

  /// 去重并排序请求卡片顺序.
  void _finalizeTruthRecordOrder() {
    final seen = <String>{};
    requestOrder.retainWhere(seen.add);
    requestOrder.sort((a, b) {
      final left = requestGroups[a];
      final right = requestGroups[b];
      if (left == null || right == null) {
        return 0;
      }
      final cmp = left.startedAt.compareTo(right.startedAt);
      if (cmp != 0) {
        return cmp;
      }
      return a.compareTo(b);
    });
  }

  /// 处理 TruthRecord 快照，每次收到完整快照直接替换（不做 merge）。
  void processProtectionRecord(TruthRecordModel record) {
    final requestId = record.requestId.trim();
    if (!mounted || requestId.isEmpty) {
      return;
    }
    setState(() {
      _applyTruthRecordMutation(record);
      _finalizeTruthRecordOrder();
    });
  }

  /// 批量应用 TruthRecord: 单次 setState、单次排序, 避免 Go 侧快照突发时 Windows UI 未响应.
  void processProtectionRecordBatch(Iterable<TruthRecordModel> records) {
    if (!mounted) {
      return;
    }
    final list = records.where((r) => r.requestId.trim().isNotEmpty).toList();
    if (list.isEmpty) {
      return;
    }
    setState(() {
      for (final r in list) {
        _applyTruthRecordMutation(r);
      }
      _finalizeTruthRecordOrder();
    });
  }

  /// 解析结构化 JSON 日志; 分组视图以 [processProtectionRecord] 为主, 此处仅校验格式避免上层异常。
  void processStructuredLog(String logJson) {
    try {
      jsonDecode(logJson);
    } catch (_) {}
  }
}
