import 'dart:convert';
import 'package:flutter/material.dart';
import '../../l10n/app_localizations.dart';
import '../protection_monitor_page.dart';

/// 日志翻译 Mixin
/// 将 Go 层结构化日志 JSON 翻译为本地化字符串
mixin ProtectionMonitorTranslationMixin on State<ProtectionMonitorPage> {
  String normalizeVisibleMessageContent(String content) {
    final raw = content.trim();
    if (!raw.startsWith('Sender (untrusted metadata):')) {
      return content;
    }

    final fencedJson = RegExp(
      r'^Sender \(untrusted metadata\):\s*```json[\s\S]*?```\s*([\s\S]*)$',
      multiLine: true,
    );
    final match = fencedJson.firstMatch(raw);
    if (match != null) {
      return match.group(1)?.trim() ?? '';
    }
    return raw;
  }

  /// 从消息内容中提取 `<summary>...</summary>` 标签中的摘要文本
  String? _extractSummaryTag(String content) {
    final pattern = RegExp(r'<summary>\s*([\s\S]*?)\s*</summary>');
    final match = pattern.firstMatch(content);
    return match?.group(1)?.trim();
  }

  static final _inlineToolUseRe = RegExp(
    r'<tool_use>\s*([\s\S]*?)\s*</tool_use>',
    caseSensitive: false,
  );
  static final _inlineToolResultRe = RegExp(
    r'<tool_result>\s*([\s\S]*?)\s*</tool_result>',
    caseSensitive: false,
  );
  static final _thinkingTagRe = RegExp(
    r'<thinking>[\s\S]*?</thinking>',
    caseSensitive: false,
  );

  /// 将 DinTalClaw 文本嵌入式工具标签转换为可读格式（用于原始视图）
  String _formatInlineToolTags(String content) {
    var result = content;
    result = result.replaceAll(_thinkingTagRe, '');
    result = result.replaceAllMapped(_inlineToolUseRe, (m) {
      final body = m.group(1)?.trim() ?? '';
      try {
        final json = jsonDecode(body) as Map<String, dynamic>;
        final name = json['name']?.toString() ?? 'tool';
        final args = json['arguments'];
        final argsStr = args is Map ? jsonEncode(args) : args?.toString() ?? '';
        final preview = argsStr.length > 80
            ? '${argsStr.substring(0, 80)}...'
            : argsStr;
        return '🔧 [$name] $preview';
      } catch (_) {
        final preview = body.length > 80 ? '${body.substring(0, 80)}...' : body;
        return '🔧 [tool] $preview';
      }
    });
    result = result.replaceAllMapped(_inlineToolResultRe, (m) {
      final body = m.group(1)?.trim() ?? '';
      final preview = body.length > 120
          ? '${body.substring(0, 120)}...'
          : body;
      return '📋 [result] $preview';
    });
    return result.trim();
  }

  bool shouldHideFromRawView(String logJson) {
    try {
      final data = jsonDecode(logJson) as Map<String, dynamic>;
      final key = data['key'];
      if (key == 'protection_record_snapshot') return false;
      return key == 'proxy_stream_delta' ||
          key == 'monitor_upstream_stream_delta';
    } catch (_) {
      return false;
    }
  }

  /// 将结构化日志 JSON 翻译为本地化字符串
  String translateLog(String logJson, AppLocalizations l10n) {
    try {
      final data = jsonDecode(logJson);
      final key = data['key'] as String?;
      final params = data['params'] as Map<String, dynamic>?;

      if (key == null) {
        return logJson;
      }

      switch (key) {
        case 'protection_record_snapshot':
          final reqId = params?['request_id']?.toString() ?? '';
          final phase = params?['phase']?.toString() ?? '';
          final model = params?['model']?.toString() ?? '';
          final toolCount = (params?['tool_calls'] as List?)?.length ?? 0;
          return '[ProtectionRecord] $reqId phase=$phase model=$model tools=$toolCount';
        case 'monitor_request_created':
          return '[Monitor] Request created';
        case 'monitor_client_message_received':
          return '[Monitor] Client message received';
        case 'monitor_upstream_request_built':
          return '[Monitor] Upstream request built';
        case 'monitor_upstream_request_sent':
          return '[Monitor] Upstream request sent';
        case 'monitor_upstream_stream_started':
          return '[Monitor] Upstream stream started';
        case 'monitor_upstream_stream_delta':
          return params?['content']?.toString() ?? '';
        case 'monitor_upstream_tool_call':
          return '[Monitor] Upstream tool call: ${params?['name']?.toString() ?? ''}';
        case 'monitor_upstream_tool_result':
          return '[Monitor] Upstream tool result: ${params?['summary']?.toString() ?? ''}';
        case 'monitor_upstream_completed':
          return '[Monitor] Upstream completed';
        case 'monitor_security_decision':
          return '[Monitor] Security decision: ${params?['status']?.toString() ?? ''}';
        case 'monitor_response_returned':
          return '[Monitor] Response returned: ${params?['status']?.toString() ?? ''}';
        case 'monitor_request_failed':
          return '[Monitor] Request failed: ${params?['error']?.toString() ?? ''}';
        // Proxy logs
        case 'proxy_new_request':
          return l10n.proxyNewRequest;
        case 'proxy_request_info':
          return l10n.proxyRequestInfo(
            params?['model']?.toString() ?? '',
            params?['messageCount'] as int? ?? 0,
            params?['stream']?.toString() ?? '',
          );
        case 'proxy_message_info':
          {
            final role = (params?['role']?.toString() ?? '').toLowerCase();
            final rawContent = params?['content']?.toString() ?? '';
            final content = normalizeVisibleMessageContent(rawContent);
            final formatted = _formatInlineToolTags(content);
            final summary = _extractSummaryTag(content);
            if ((role == 'user' || role == 'assistant') &&
                summary != null &&
                summary.isNotEmpty) {
              return l10n.proxyMessageInfo(
                params?['index'] as int? ?? 0,
                params?['role']?.toString() ?? '',
                summary,
              );
            }
            return l10n.proxyMessageInfo(
              params?['index'] as int? ?? 0,
              params?['role']?.toString() ?? '',
              formatted,
            );
          }
        case 'proxy_tool_activity_detected':
          return l10n.proxyToolActivityDetected;
        case 'proxy_tool_calls_found':
          return l10n.proxyToolCallsFound(
            params?['toolCount'] as int? ?? 0,
            params?['resultCount'] as int? ?? 0,
          );
        case 'proxy_response_non_stream':
          return l10n.proxyResponseNonStream;
        case 'proxy_response_info':
          return l10n.proxyResponseInfo(
            params?['model']?.toString() ?? '',
            params?['choiceCount'] as int? ?? 0,
          );
        case 'proxy_response_content':
          return l10n.proxyResponseContent(
            params?['content']?.toString() ?? '',
          );
        case 'proxy_tool_calls_detected':
          return l10n.proxyToolCallsDetected;
        case 'proxy_tool_call_count':
          return l10n.proxyToolCallCount(params?['count'] as int? ?? 0);
        case 'proxy_tool_call_name':
          return l10n.proxyToolCallName(
            params?['index'] as int? ?? 0,
            params?['name']?.toString() ?? '',
          );
        case 'proxy_tool_call_args':
          return l10n.proxyToolCallArgs(
            params?['index'] as int? ?? 0,
            params?['args']?.toString() ?? '',
          );
        case 'proxy_tool_result_decision':
          {
            final status = params?['status']?.toString() ?? '';
            final reason = params?['reason']?.toString() ?? '';
            final isZh = l10n.localeName.startsWith('zh');
            final statusLabel = isZh ? '状态' : 'Status';
            final reasonLabel = isZh ? '原因' : 'Reason';
            return '[ShepherdGate] $statusLabel: $status | $reasonLabel: $reason';
          }
        case 'proxy_tool_result_content':
          {
            final content = params?['content']?.toString() ?? '';
            final isZh = l10n.localeName.startsWith('zh');
            final label = isZh ? '工具响应内容' : 'Tool Response Content';
            return '[Proxy] $label: $content';
          }
        case 'proxy_stream_security_message':
          {
            final content = params?['content']?.toString() ?? '';
            final isZh = l10n.localeName.startsWith('zh');
            final label = isZh ? '安全提示' : 'Security Message';
            return '[Proxy] $label: $content';
          }
        case 'proxy_stream_intercepted_content':
          {
            final content = params?['content']?.toString() ?? '';
            final isZh = l10n.localeName.startsWith('zh');
            final label = isZh ? '被拦截内容' : 'Intercepted Content';
            return '[Proxy] $label: $content';
          }
        case 'proxy_starting_analysis':
          return l10n.proxyStartingAnalysis;
        case 'proxy_stream_finished':
          return l10n.proxyStreamFinished(params?['reason']?.toString() ?? '');
        case 'proxy_tool_calls_in_stream':
          return l10n.proxyToolCallsInStream;
        case 'proxy_stream_content_no_tools':
        case 'proxy_stream_content_with_tools':
          return l10n.proxyStreamContentNoTools(
            params?['content']?.toString() ?? '',
          );
        case 'proxy_stream_delta':
          return params?['content']?.toString() ?? '';
        case 'proxy_agent_not_available':
          return l10n.proxyAgentNotAvailable;
        case 'proxy_sending_analysis':
          return l10n.proxySendingAnalysis;
        case 'proxy_original_task':
          return l10n.proxyOriginalTask(params?['task']?.toString() ?? '');
        case 'proxy_message_count_log':
          return l10n.proxyMessageCountLog(params?['count'] as int? ?? 0);
        case 'proxy_analyze_message':
          {
            final role = (params?['role']?.toString() ?? '').toLowerCase();
            final rawContent = params?['content']?.toString() ?? '';
            final content = normalizeVisibleMessageContent(rawContent);
            final formatted = _formatInlineToolTags(content);
            final summary = _extractSummaryTag(content);
            if ((role == 'user' || role == 'assistant') &&
                summary != null &&
                summary.isNotEmpty) {
              return l10n.proxyAnalyzeMessage(
                params?['index'] as int? ?? 0,
                params?['role']?.toString() ?? '',
                summary,
              );
            }
            return l10n.proxyAnalyzeMessage(
              params?['index'] as int? ?? 0,
              params?['role']?.toString() ?? '',
              formatted,
            );
          }
        case 'proxy_analysis_error':
          return l10n.proxyAnalysisError(params?['error']?.toString() ?? '');
        case 'proxy_analysis_result':
          return l10n.proxyAnalysisResult;
        case 'proxy_risk_level':
          return l10n.proxyRiskLevel(params?['level']?.toString() ?? '');
        case 'proxy_confidence':
          return l10n.proxyConfidence(params?['confidence'] as int? ?? 0);
        case 'proxy_suggested_action':
          return l10n.proxySuggestedAction(params?['action']?.toString() ?? '');
        case 'proxy_reason':
          return l10n.proxyReason(params?['reason']?.toString() ?? '');
        case 'proxy_malicious_instruction':
          return l10n.proxyMaliciousInstruction(
            params?['instruction']?.toString() ?? '',
          );
        case 'proxy_traceable_quote':
          return l10n.proxyTraceableQuote(params?['quote']?.toString() ?? '');
        case 'proxy_blocking':
          return l10n.proxyBlocking;
        case 'proxy_warning':
          return l10n.proxyWarning;
        case 'proxy_allowed':
          return l10n.proxyAllowed;
        case 'proxy_restarting_gateway':
          return l10n.proxyRestartingGateway;
        case 'proxy_gateway_restart_error':
          return l10n.proxyGatewayRestartError(
            params?['error']?.toString() ?? '',
          );
        case 'proxy_gateway_restart_success':
          return l10n.proxyGatewayRestartSuccess;
        case 'proxy_gateway_restart_skipped_appstore':
          return l10n.proxyGatewayRestartSkippedAppstore;
        case 'proxy_server_error':
          return l10n.proxyServerError(params?['error']?.toString() ?? '');
        case 'proxy_started':
          return l10n.proxyStarted(
            params?['port'] as int? ?? 0,
            params?['target']?.toString() ?? '',
            params?['provider']?.toString() ?? '',
          );
        case 'proxy_config_update_failed':
          return l10n.proxyConfigUpdateFailed(
            params?['error']?.toString() ?? '',
          );
        case 'proxy_config_updated':
          return l10n.proxyConfigUpdated(
            params?['provider']?.toString() ?? '',
            params?['url']?.toString() ?? '',
          );
        case 'proxy_gateway_restart_failed':
          return l10n.proxyGatewayRestartFailed(
            params?['error']?.toString() ?? '',
          );
        case 'proxy_stopping':
          return l10n.proxyStopping;
        case 'proxy_config_restore_failed':
          return l10n.proxyConfigRestoreFailed(
            params?['error']?.toString() ?? '',
          );
        case 'proxy_config_restored':
          return l10n.proxyConfigRestored(
            params?['provider']?.toString() ?? '',
            params?['url']?.toString() ?? '',
          );
        case 'proxy_stopped':
          return l10n.proxyStopped;

        // Token usage log
        case 'proxy_token_usage':
          final promptTokens = params?['promptTokens'] as int? ?? 0;
          final completionTokens = params?['completionTokens'] as int? ?? 0;
          final totalTokens = params?['totalTokens'] as int? ?? 0;
          return l10n.proxyTokenUsage(
            promptTokens,
            completionTokens,
            totalTokens,
          );

        // Protection agent logs
        case 'protection_agent_analyzing':
          return l10n.protectionAgentAnalyzing(params?['count'] as int? ?? 0);
        case 'protection_agent_sending_llm':
          return l10n.protectionAgentSendingLLM;
        case 'protection_agent_error':
          return l10n.protectionAgentError(params?['error']?.toString() ?? '');
        case 'protection_agent_raw_response':
          return l10n.protectionAgentRawResponse(
            params?['response']?.toString() ?? '',
          );
        case 'protection_agent_warning':
          return l10n.protectionAgentWarning(
            params?['warning']?.toString() ?? '',
          );
        case 'protection_agent_result':
          return l10n.protectionAgentResult(
            params?['level']?.toString() ?? '',
            params?['confidence'] as int? ?? 0,
          );
        case 'protection_agent_reason':
          return l10n.protectionAgentReason(
            params?['reason']?.toString() ?? '',
          );
        case 'protection_agent_suggested_action':
          return l10n.protectionAgentSuggestedAction(
            params?['action']?.toString() ?? '',
          );

        // Tool validator logs
        case 'tool_validator_blocked':
          return l10n.toolValidatorBlocked(params?['reason']?.toString() ?? '');
        case 'tool_validator_passed':
          return l10n.toolValidatorPassed(
            params?['toolName']?.toString() ?? '',
          );

        // Dart-side proxy logs
        case 'dart_proxy_starting':
          return l10n.dartProxyStarting;
        case 'dart_proxy_started':
          return l10n.dartProxyStarted(
            params?['port'] as int? ?? 0,
            params?['provider']?.toString() ?? '',
          );
        case 'dart_proxy_failed':
          return l10n.dartProxyFailed(params?['error']?.toString() ?? '');
        case 'dart_proxy_error':
          return l10n.dartProxyError(params?['error']?.toString() ?? '');
        case 'dart_proxy_stopping':
          return l10n.dartProxyStopping;
        case 'dart_proxy_stopped':
          return l10n.dartProxyStopped;
        case 'dart_analysis_error':
          return l10n.dartAnalysisError(params?['error']?.toString() ?? '');

        default:
          return logJson;
      }
    } catch (e) {
      // 非 JSON 格式,返回原始字符串(兼容旧格式)
      return logJson;
    }
  }

  /// 将技能 ID 翻译为本地化的可读名称
  String translateSkillName(String skillId, AppLocalizations l10n) {
    switch (skillId) {
      case 'data_exfiltration_guard':
        return l10n.skillNameDataExfiltrationGuard;
      case 'file_access_guard':
        return l10n.skillNameFileAccessGuard;
      case 'email_delete_guard':
        return l10n.skillNameEmailDeleteGuard;
      case 'email_read_guard':
        return l10n.skillNameEmailDeleteGuard;
      case 'email_operation_guard':
        return l10n.skillNameEmailDeleteGuard;
      case 'browser_web_access_guard':
        return l10n.skillNameGeneralToolRiskGuard;
      case 'prompt_injection_guard':
        return l10n.skillNamePromptInjectionGuard;
      case 'script_execution_guard':
        return l10n.skillNameScriptExecutionGuard;
      case 'general_tool_risk_guard':
        return l10n.skillNameGeneralToolRiskGuard;
      default:
        return skillId;
    }
  }

  /// 数据安全类技能 ID 集合
  static const dataSecuritySkills = {
    'data_exfiltration_guard',
    'file_access_guard',
    'email_delete_guard',
    'email_read_guard',
    'email_operation_guard',
    'browser_web_access_guard',
    'prompt_injection_guard',
    'script_execution_guard',
    'general_tool_risk_guard',
  };

  /// 判断技能是否属于数据安全类
  static bool isDataSecuritySkill(String skillId) {
    return dataSecuritySkills.contains(skillId);
  }

  /// 根据数据安全技能 ID 返回对应的风险标签文案
  static String getDataSecurityBadgeLabel(
    String skillId,
    AppLocalizations l10n,
  ) {
    switch (skillId) {
      case 'data_exfiltration_guard':
        return l10n.dataExfiltrationRisk;
      case 'file_access_guard':
        return l10n.sensitiveAccessRisk;
      case 'email_delete_guard':
      case 'email_read_guard':
      case 'email_operation_guard':
        return l10n.emailDeleteRisk;
      case 'browser_web_access_guard':
        return l10n.generalToolRisk;
      case 'prompt_injection_guard':
        return l10n.promptInjectionRisk;
      case 'script_execution_guard':
        return l10n.scriptExecutionRisk;
      case 'general_tool_risk_guard':
        return l10n.generalToolRisk;
      default:
        return l10n.dataSecurity;
    }
  }

  /// 根据数据安全技能 ID 返回徽章颜色
  static Color getDataSecurityBadgeColor(String skillId) {
    switch (skillId) {
      case 'data_exfiltration_guard':
        return const Color(0xFFEF4444);
      case 'file_access_guard':
        return const Color(0xFFF59E0B);
      case 'email_delete_guard':
      case 'email_read_guard':
      case 'email_operation_guard':
        return const Color(0xFF8B5CF6);
      case 'browser_web_access_guard':
        return const Color(0xFF14B8A6);
      case 'prompt_injection_guard':
        return const Color(0xFF06B6D4);
      case 'script_execution_guard':
        return const Color(0xFFEC4899);
      case 'general_tool_risk_guard':
        return const Color(0xFF14B8A6);
      default:
        return const Color(0xFF3B82F6);
    }
  }

  /// 根据数据安全技能 ID 返回徽章图标
  static IconData getDataSecurityBadgeIcon(String skillId) {
    switch (skillId) {
      case 'data_exfiltration_guard':
        return Icons.cloud_upload_outlined;
      case 'file_access_guard':
        return Icons.folder_open_outlined;
      case 'email_delete_guard':
      case 'email_read_guard':
      case 'email_operation_guard':
        return Icons.email_outlined;
      case 'browser_web_access_guard':
        return Icons.language_outlined;
      case 'prompt_injection_guard':
        return Icons.psychology_outlined;
      case 'script_execution_guard':
        return Icons.terminal_outlined;
      case 'general_tool_risk_guard':
        return Icons.build_outlined;
      default:
        return Icons.security_outlined;
    }
  }
}
