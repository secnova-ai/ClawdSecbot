import 'dart:convert';
import '../core_transport/transport_registry.dart';
import '../models/protection_analysis_model.dart';
import '../utils/app_logger.dart';

/// API 指标 FFI 持久化门面：通过 FFI 委托 Go 层进行数据持久化，Flutter 不直接操作 DB。
class MetricsDatabaseService {
  static final MetricsDatabaseService _instance =
      MetricsDatabaseService._internal();

  factory MetricsDatabaseService() => _instance;

  MetricsDatabaseService._internal();

  /// Save API metrics
  Future<void> saveApiMetrics(
    ApiMetrics metrics, {
    String? assetName,
    String? assetID,
  }) async {
    final result = _callFFI(
      'SaveApiMetricsFFI',
      jsonEncode({
        'timestamp': metrics.timestamp.toIso8601String(),
        'prompt_tokens': metrics.promptTokens,
        'completion_tokens': metrics.completionTokens,
        'total_tokens': metrics.totalTokens,
        'tool_call_count': metrics.toolCallCount,
        'model': metrics.model,
        'is_blocked': metrics.isBlocked,
        'risk_level': metrics.riskLevel,
        'asset_name': assetName ?? '',
        'asset_id': assetID ?? '',
      }),
    );

    if (result['success'] != true) {
      throw Exception('Failed to save API metrics: ${result['error']}');
    }
  }

  /// Get API statistics for the current session
  Future<ApiStatistics> getApiStatistics({
    Duration? duration,
    String? assetName,
    String? assetID,
  }) async {
    final durationSeconds = (duration ?? const Duration(hours: 24)).inSeconds;

    final result = _callFFI(
      'GetApiStatisticsFFI',
      jsonEncode({
        'duration_seconds': durationSeconds,
        'asset_name': assetName ?? '',
        'asset_id': assetID ?? '',
      }),
    );

    if (result['success'] != true) return ApiStatistics.empty();

    final data = result['data'] as Map<String, dynamic>?;
    if (data == null) return ApiStatistics.empty();

    final tokenTrend = <TokenTrendPoint>[];
    if (data['token_trend'] != null) {
      for (final item in data['token_trend'] as List) {
        final point = item as Map<String, dynamic>;
        tokenTrend.add(
          TokenTrendPoint(
            timestamp: DateTime.parse(point['timestamp'] as String),
            tokens: point['tokens'] as int? ?? 0,
            promptTokens: point['prompt_tokens'] as int? ?? 0,
            completionTokens: point['completion_tokens'] as int? ?? 0,
          ),
        );
      }
    }

    final toolCallTrend = <ToolCallTrendPoint>[];
    if (data['tool_call_trend'] != null) {
      for (final item in data['tool_call_trend'] as List) {
        final point = item as Map<String, dynamic>;
        toolCallTrend.add(
          ToolCallTrendPoint(
            timestamp: DateTime.parse(point['timestamp'] as String),
            count: point['count'] as int? ?? 0,
          ),
        );
      }
    }

    return ApiStatistics(
      totalTokens: data['total_tokens'] as int? ?? 0,
      totalPromptTokens: data['total_prompt_tokens'] as int? ?? 0,
      totalCompletionTokens: data['total_completion_tokens'] as int? ?? 0,
      totalToolCalls: data['total_tool_calls'] as int? ?? 0,
      requestCount: data['request_count'] as int? ?? 0,
      blockedCount: data['blocked_count'] as int? ?? 0,
      tokenTrend: tokenTrend,
      toolCallTrend: toolCallTrend,
    );
  }

  /// Get recent API metrics
  Future<List<ApiMetrics>> getRecentApiMetrics({int limit = 100}) async {
    final result = _callFFI(
      'GetRecentApiMetricsFFI',
      jsonEncode({'limit': limit}),
    );

    if (result['success'] != true) return [];

    final data = result['data'];
    if (data == null || data is! List) return [];

    return data.map((item) {
      final row = item as Map<String, dynamic>;
      return ApiMetrics(
        id: row['id'] as int? ?? 0,
        timestamp: DateTime.parse(row['timestamp'] as String),
        promptTokens: row['prompt_tokens'] as int? ?? 0,
        completionTokens: row['completion_tokens'] as int? ?? 0,
        totalTokens: row['total_tokens'] as int? ?? 0,
        toolCallCount: row['tool_call_count'] as int? ?? 0,
        model: row['model'] as String? ?? '',
        isBlocked: row['is_blocked'] as bool? ?? false,
        riskLevel: row['risk_level'] as String?,
      );
    }).toList();
  }

  /// Clear old API metrics (keep last N days)
  Future<void> cleanOldApiMetrics({int keepDays = 7}) async {
    _callFFI('CleanOldApiMetricsFFI', jsonEncode({'keep_days': keepDays}));
  }

  /// Get daily token usage for an asset
  Future<int> getDailyTokenUsage(String assetName, String assetID) async {
    final transport = TransportRegistry.transport;
    if (!transport.isReady) return 0;

    try {
      final result = transport.callOneArg('GetDailyTokenUsageFFI', assetID);
      if (result['success'] == true) {
        return result['data'] as int? ?? 0;
      }
      return 0;
    } catch (e) {
      appLogger.error('[MetricsDB] GetDailyTokenUsageFFI failed: $e');
      return 0;
    }
  }

  // --- Helper methods ---

  Map<String, dynamic> _callFFI(String funcName, String jsonStr) {
    final transport = TransportRegistry.transport;
    if (!transport.isReady) {
      return {'success': false, 'error': 'Native library not initialized'};
    }

    try {
      return transport.callOneArg(funcName, jsonStr);
    } catch (e) {
      appLogger.error('[MetricsDB] $funcName failed: $e');
      return {'success': false, 'error': '$funcName failed: $e'};
    }
  }
}
