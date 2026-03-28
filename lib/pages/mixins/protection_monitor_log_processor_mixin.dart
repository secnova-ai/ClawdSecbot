import 'dart:async';
import 'dart:convert';
import 'package:flutter/material.dart';
import '../../models/protection_analysis_model.dart';
import '../../models/request_log_group_model.dart';
import '../../services/protection_service.dart';
import '../protection_monitor_window.dart';

/// 日志处理 Mixin
/// 负责结构化日志解析、请求分组、批量刷新、滚动控制
mixin ProtectionMonitorLogProcessorMixin on State<ProtectionMonitorPage> {
  // ============ 需要主 State 提供的状态 ============
  List<LogEntry> get logsList;
  int get maxLogCount;
  Map<String, RequestLogGroup> get requestGroups;
  List<String> get requestOrder;
  List<LogEntry> get pendingLogs;
  Timer? get logUpdateTimer;
  set logUpdateTimer(Timer? value);
  ScrollController get logScrollController;
  bool get useGroupedView;
  bool get autoScrollEnabled;
  bool get userScrolledAway;
  set userScrolledAway(bool value);

  // 结果批处理状态
  ProtectionAnalysisResult? get pendingResult;
  set pendingResult(ProtectionAnalysisResult? value);
  Timer? get resultUpdateTimer;
  set resultUpdateTimer(Timer? value);
  set latestResult(ProtectionAnalysisResult? value);
  set currentRiskLevel(RiskLevel value);

  // 统计更新回调
  ProtectionService get protectionService;
  void updateCountersFromService();

  // ============ 日志批量刷新 ============

  void flushPendingLogs() {
    if (!mounted || pendingLogs.isEmpty) {
      logUpdateTimer = null;
      return;
    }

    // 快照待处理日志,避免在 setState 期间持有列表
    final logsToAdd = List<LogEntry>.from(pendingLogs);
    pendingLogs.clear();

    setState(() {
      logsList.addAll(logsToAdd);
      if (logsList.length > maxLogCount) {
        logsList.removeRange(0, logsList.length - maxLogCount);
      }
    });

    // 如果启用自动滚动,滚动到底部
    if (autoScrollEnabled && !userScrolledAway) {
      scrollToBottom();
    }

    logUpdateTimer = null;
  }

  // ============ 滚动控制 ============

  /// 检测用户滚动行为
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

  /// 动画滚动到日志底部
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

  // ============ 结果批量刷新 ============

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

  // ============ Token 估算 ============

  int estimateTokenCount(String text) {
    if (text.isEmpty) return 0;
    double count = 0;
    for (var i = 0; i < text.length; i++) {
      final codeUnit = text.codeUnitAt(i);
      if (codeUnit < 128) {
        count += 0.25;
      } else {
        count += 1.5;
      }
    }
    return count.ceil();
  }

  // ============ 工具结果摘要 ============

  /// 将工具结果的原始 JSON 内容展平为单行摘要,去除换行符,
  /// 截取前 120 字符,避免多行 JSON 在卡片中只显示 `{`。
  String _flattenToolResultContent(String raw) {
    final flat = raw.replaceAll(RegExp(r'\s+'), ' ').trim();
    if (flat.length <= 120) return flat;
    return '${flat.substring(0, 120)}...';
  }

  // ============ 结构化日志处理 ============

  /// 解析结构化日志并更新请求分组
  void processStructuredLog(String logJson) {
    try {
      final data = jsonDecode(logJson);
      final key = data['key'] as String?;
      final params = data['params'] as Map<String, dynamic>?;
      if (key == null) return;
      var reqId = params?['request_id']?.toString() ?? '';
      if (reqId.isEmpty) {
        if (requestOrder.isNotEmpty) {
          reqId = requestOrder.last;
        } else {
          return;
        }
      }
      var group = requestGroups[reqId];
      if (group == null) {
        group = RequestLogGroup(reqId, DateTime.now());
        requestGroups[reqId] = group;
        requestOrder.add(reqId);
      }
      switch (key) {
        case 'proxy_new_request':
        case 'proxy_session_quota_exceeded':
        case 'proxy_quota_exceeded':
          final m = params?['model']?.toString() ?? '';
          if (m.isNotEmpty) group.model = m;
          break;
        case 'proxy_request_info':
          group.model = params?['model']?.toString() ?? group.model;
          group.messageCount =
              params?['messageCount'] as int? ?? group.messageCount;
          group.stream = params?['stream']?.toString() ?? group.stream;
          break;
        case 'proxy_response_info':
          final rm = params?['model']?.toString() ?? '';
          if (rm.isNotEmpty && group.model.isEmpty) group.model = rm;
          break;
        case 'proxy_message_info':
          final idx = params?['index'] as int? ?? 0;
          final role = params?['role']?.toString() ?? '';
          final content = params?['content']?.toString() ?? '';
          group.messages.add(RequestMessageSummary(idx, role, content));
          break;
        case 'proxy_stream_finished':
          group.finishReason =
              params?['reason']?.toString() ?? group.finishReason;
          break;
        case 'proxy_stream_content_no_tools':
        case 'proxy_stream_content_with_tools':
          final c = params?['content']?.toString() ?? '';
          if (c.isNotEmpty) {
            group.streamContent = c;
            group.originalStreamContent = c;
          }
          break;
        case 'proxy_response_content':
          final c2 = params?['content']?.toString() ?? '';
          if (c2.isNotEmpty) group.responseContent = c2;
          break;
        case 'proxy_tool_result_content':
          {
            final c3 = params?['content']?.toString() ?? '';
            if (c3.isNotEmpty) {
              // 工具结果是请求体中的 role=tool 内容,不是模型回复,
              // 存入 toolResultSummaries 供卡片展示摘要,不污染 responseContent。
              final summary = _flattenToolResultContent(c3);
              group.toolResultSummaries.add(summary);
            }
            break;
          }
        case 'proxy_tool_call_count':
          group.toolCallCount = params?['count'] as int? ?? group.toolCallCount;
          break;
        case 'proxy_tool_call_name':
          final name = params?['name']?.toString() ?? '';
          if (name.isNotEmpty && !group.toolNames.contains(name)) {
            group.toolNames.add(name);
          }
          break;
        case 'tool_validator_passed':
          {
            final name = params?['toolName']?.toString() ?? '';
            if (name.isNotEmpty && !group.toolNames.contains(name)) {
              group.toolNames.add(name);
            }
            break;
          }
        case 'proxy_token_usage':
          group.promptTokens =
              params?['promptTokens'] as int? ?? group.promptTokens;
          group.completionTokens =
              params?['completionTokens'] as int? ?? group.completionTokens;
          group.totalTokens =
              params?['totalTokens'] as int? ?? group.totalTokens;
          break;
        case 'proxy_token_usage_estimated':
          group.promptTokens =
              params?['promptTokens'] as int? ?? group.promptTokens;
          if (group.originalStreamContent.isNotEmpty) {
            final estimatedCompletion = estimateTokenCount(
              group.originalStreamContent,
            );
            group.completionTokens = estimatedCompletion;
            group.totalTokens = group.promptTokens + estimatedCompletion;
          } else {
            group.completionTokens =
                params?['completionTokens'] as int? ?? group.completionTokens;
            group.totalTokens =
                params?['totalTokens'] as int? ?? group.totalTokens;
          }
          break;
        case 'proxy_tool_result_decision':
          group.decisionStatus =
              params?['status']?.toString() ?? group.decisionStatus;
          group.decisionReason =
              params?['reason']?.toString() ?? group.decisionReason;
          group.decisionBlocked =
              params?['blocked'] as bool? ?? group.decisionBlocked;
          break;
        case 'proxy_stream_security_message':
          {
            final sm = params?['content']?.toString() ?? '';
            if (sm.isNotEmpty) group.securityMessage = sm;
            if (group.decisionStatus.isEmpty) {
              final zhStatus = RegExp(r'状态:\s*([A-Z_]+)');
              final enStatus = RegExp(r'Status:\s*([A-Z_]+)');
              final zhReason = RegExp(r'原因:\s*(.+)');
              final enReason = RegExp(r'Reason:\s*(.+)');
              String? st;
              String? rs;
              final m1 = zhStatus.firstMatch(sm);
              final m2 = enStatus.firstMatch(sm);
              if (m1 != null && m1.groupCount >= 1) {
                st = m1.group(1);
              } else if (m2 != null && m2.groupCount >= 1) {
                st = m2.group(1);
              }
              final r1 = zhReason.firstMatch(sm);
              final r2 = enReason.firstMatch(sm);
              if (r1 != null && r1.groupCount >= 1) {
                rs = r1.group(1);
              } else if (r2 != null && r2.groupCount >= 1) {
                rs = r2.group(1);
              }
              if (st != null && st.isNotEmpty) {
                group.decisionStatus = st;
              }
              if (rs != null && rs.isNotEmpty) {
                group.decisionReason = rs;
              }
            }
            break;
          }
        case 'proxy_stream_intercepted_content':
          {
            final ic = params?['content']?.toString() ?? '';
            if (ic.isNotEmpty) group.streamContent = ic;
            break;
          }
      }
      // 不调用 setState 以避免过度重建,UI 由 flushPendingLogs 定时器刷新
    } catch (_) {}
  }
}
