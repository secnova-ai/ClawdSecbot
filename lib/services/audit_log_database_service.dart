import 'dart:convert';
import '../core_transport/transport_registry.dart';
import '../models/audit_log_model.dart';
import '../utils/app_logger.dart';

/// 审计日志 FFI 持久化门面：通过 FFI 委托 Go 层进行数据持久化，Flutter 不直接操作 DB。
class AuditLogDatabaseService {
  static final AuditLogDatabaseService _instance =
      AuditLogDatabaseService._internal();

  factory AuditLogDatabaseService() => _instance;

  AuditLogDatabaseService._internal();

  /// Save an audit log entry
  Future<void> saveAuditLog(AuditLog log) async {
    final result = _callFFI(
      'SaveAuditLogFFI',
      jsonEncode({
        'id': log.id,
        'timestamp': log.timestamp.toIso8601String(),
        'request_id': log.requestId,
        'asset_name': log.assetName,
        'asset_id': log.assetID,
        'model': log.model,
        'request_content': log.requestContent,
        'tool_calls': jsonEncode(log.toolCalls.map((e) => e.toJson()).toList()),
        'output_content': log.outputContent,
        'has_risk': log.hasRisk,
        'risk_level': log.riskLevel,
        'risk_reason': log.riskReason,
        'confidence': log.confidence ?? 0,
        'action': log.action,
        'prompt_tokens': log.promptTokens ?? 0,
        'completion_tokens': log.completionTokens ?? 0,
        'total_tokens': log.totalTokens ?? 0,
        'duration_ms': log.durationMs,
        'messages': jsonEncode(log.messages.map((m) => m.toJson()).toList()),
        'message_count': log.messageCount,
      }),
    );

    if (result['success'] != true) {
      throw Exception('Failed to save audit log: ${result['error']}');
    }
  }

  /// Save multiple audit logs in a batch
  Future<void> saveAuditLogsBatch(List<AuditLog> logs) async {
    if (logs.isEmpty) return;

    final logsList = logs
        .map(
          (log) => {
            'id': log.id,
            'timestamp': log.timestamp.toIso8601String(),
            'request_id': log.requestId,
            'asset_name': log.assetName,
            'asset_id': log.assetID,
            'model': log.model,
            'request_content': log.requestContent,
            'tool_calls': jsonEncode(
              log.toolCalls.map((e) => e.toJson()).toList(),
            ),
            'output_content': log.outputContent,
            'has_risk': log.hasRisk,
            'risk_level': log.riskLevel,
            'risk_reason': log.riskReason,
            'confidence': log.confidence ?? 0,
            'action': log.action,
            'prompt_tokens': log.promptTokens ?? 0,
            'completion_tokens': log.completionTokens ?? 0,
            'total_tokens': log.totalTokens ?? 0,
            'duration_ms': log.durationMs,
            'messages': jsonEncode(
              log.messages.map((m) => m.toJson()).toList(),
            ),
            'message_count': log.messageCount,
          },
        )
        .toList();

    final result = _callFFI('SaveAuditLogsBatchFFI', jsonEncode(logsList));
    if (result['success'] != true) {
      appLogger.warning('[AuditDB] Batch save failed: ${result['error']}');
    }
  }

  /// Get audit logs with optional filtering
  Future<List<AuditLog>> getAuditLogs({
    int limit = 100,
    int offset = 0,
    bool riskOnly = false,
    String? assetName,
    String? assetID,
    DateTime? startTime,
    DateTime? endTime,
    String? searchQuery,
  }) async {
    final filter = <String, dynamic>{
      'limit': limit,
      'offset': offset,
      'risk_only': riskOnly,
    };
    if (startTime != null) filter['start_time'] = startTime.toIso8601String();
    if (endTime != null) filter['end_time'] = endTime.toIso8601String();
    if (searchQuery != null && searchQuery.isNotEmpty) {
      filter['search_query'] = searchQuery;
    }
    if (assetName != null && assetName.isNotEmpty) {
      filter['asset_name'] = assetName;
    }
    if (assetID != null && assetID.isNotEmpty) {
      filter['asset_id'] = assetID;
    }

    final result = _callFFI('GetAuditLogsFFI', jsonEncode(filter));
    if (result['success'] != true) return [];

    final data = result['data'];
    if (data == null || data is! List) return [];

    return data.map((item) {
      final row = item as Map<String, dynamic>;
      List<AuditToolCall> toolCalls = [];
      final toolCallsStr = row['tool_calls'] as String?;
      if (toolCallsStr != null && toolCallsStr.isNotEmpty) {
        try {
          final decoded = jsonDecode(toolCallsStr) as List<dynamic>;
          toolCalls = decoded
              .map((e) => AuditToolCall.fromJson(e as Map<String, dynamic>))
              .toList();
        } catch (e) {
          // Failed to parse tool calls
        }
      }

      List<AuditMessage> messages = [];
      final messagesStr = row['messages'] as String?;
      if (messagesStr != null && messagesStr.isNotEmpty) {
        try {
          final decoded = jsonDecode(messagesStr) as List<dynamic>;
          messages = decoded
              .whereType<Map>()
              .map((e) => AuditMessage.fromJson(Map<String, dynamic>.from(e)))
              .toList();
        } catch (_) {}
      }

      return AuditLog(
        id: row['id'] as String? ?? '',
        timestamp: DateTime.parse(row['timestamp'] as String),
        requestId: row['request_id'] as String? ?? '',
        assetName: row['asset_name'] as String? ?? '',
        assetID: row['asset_id'] as String? ?? '',
        model: row['model'] as String?,
        requestContent: row['request_content'] as String? ?? '',
        toolCalls: toolCalls,
        outputContent: row['output_content'] as String?,
        hasRisk: row['has_risk'] as bool? ?? false,
        riskLevel: row['risk_level'] as String?,
        riskReason: row['risk_reason'] as String?,
        confidence: row['confidence'] as int?,
        action: row['action'] as String? ?? 'ALLOW',
        promptTokens: row['prompt_tokens'] as int?,
        completionTokens: row['completion_tokens'] as int?,
        totalTokens: row['total_tokens'] as int?,
        durationMs: row['duration_ms'] as int? ?? 0,
        messages: messages,
        messageCount: row['message_count'] as int? ?? 0,
      );
    }).toList();
  }

  /// 获取审计日志条数（可选资产、风险、与列表一致的全文搜索条件）.
  Future<int> getAuditLogCount({
    bool riskOnly = false,
    String? assetName,
    String? assetID,
    String? searchQuery,
  }) async {
    final result = _callFFI(
      'GetAuditLogCountFFI',
      jsonEncode({
        'risk_only': riskOnly,
        if (assetName != null && assetName.isNotEmpty) 'asset_name': assetName,
        if (assetID != null && assetID.isNotEmpty) 'asset_id': assetID,
        if (searchQuery != null && searchQuery.isNotEmpty)
          'search_query': searchQuery,
      }),
    );

    if (result['success'] != true) return 0;
    return result['data'] as int? ?? 0;
  }

  /// Get audit log statistics
  Future<Map<String, dynamic>> getAuditLogStatistics({
    String? assetName,
    String? assetID,
  }) async {
    final normalizedAssetID = (assetID ?? '').trim();
    final hasAssetFilter = normalizedAssetID.isNotEmpty;
    final result = hasAssetFilter
        ? _callFFI(
            'GetAuditLogStatisticsWithFilterFFI',
            jsonEncode({'asset_id': normalizedAssetID}),
          )
        : _callFFINoArg('GetAuditLogStatisticsFFI');
    if (result['success'] != true) {
      return {
        'total': 0,
        'risk_count': 0,
        'blocked_count': 0,
        'allowed_count': 0,
      };
    }

    final data = result['data'] as Map<String, dynamic>?;
    if (data == null) {
      return {
        'total': 0,
        'risk_count': 0,
        'blocked_count': 0,
        'allowed_count': 0,
      };
    }

    return {
      'total': data['total'] as int? ?? 0,
      'risk_count': data['risk_count'] as int? ?? 0,
      'blocked_count': data['blocked_count'] as int? ?? 0,
      'allowed_count': data['allowed_count'] as int? ?? 0,
    };
  }

  /// Get all asset tabs that still have audit log history
  Future<List<Map<String, String>>> getAuditLogAssets() async {
    final result = _callFFINoArg('GetAuditLogAssetsFFI');
    if (result['success'] != true) return [];

    final data = result['data'];
    if (data == null || data is! List) return [];

    return data
        .map((item) {
          final row = Map<String, dynamic>.from(item as Map);
          return {
            'asset_name': row['asset_name'] as String? ?? '',
            'asset_id': row['asset_id'] as String? ?? '',
          };
        })
        .where((item) => (item['asset_name'] ?? '').isNotEmpty)
        .toList();
  }

  /// Clear old audit logs (keep last N days)
  Future<void> cleanOldAuditLogs({int keepDays = 30}) async {
    _callFFI('CleanOldAuditLogsFFI', jsonEncode({'keep_days': keepDays}));
  }

  /// Clear all audit logs
  Future<void> clearAllAuditLogs() async =>
      _callFFINoArg('ClearAllAuditLogsFFI');

  /// Clear audit logs for the current asset tab. Falls back to clearing all
  /// logs when no asset filter is provided.
  Future<void> clearAuditLogs({String? assetName, String? assetID}) async {
    final normalizedAssetID = (assetID ?? '').trim();
    final normalizedAssetName = (assetName ?? '').trim();
    if (normalizedAssetID.isEmpty && normalizedAssetName.isNotEmpty) {
      throw Exception('asset_id is required for filtered clear');
    }
    if (normalizedAssetID.isEmpty) {
      await clearAllAuditLogs();
      return;
    }

    _callFFI(
      'ClearAuditLogsWithFilterFFI',
      jsonEncode({'asset_id': normalizedAssetID}),
    );
  }

  // --- Helper methods ---

  Map<String, dynamic> _callFFINoArg(String funcName) {
    final transport = TransportRegistry.transport;
    if (!transport.isReady) {
      return {'success': false, 'error': 'Native library not initialized'};
    }

    try {
      return transport.callNoArg(funcName);
    } catch (e) {
      appLogger.error('[AuditDB] $funcName failed: $e');
      return {'success': false, 'error': '$funcName failed: $e'};
    }
  }

  Map<String, dynamic> _callFFI(String funcName, String jsonStr) {
    final transport = TransportRegistry.transport;
    if (!transport.isReady) {
      return {'success': false, 'error': 'Native library not initialized'};
    }

    try {
      return transport.callOneArg(funcName, jsonStr);
    } catch (e) {
      appLogger.error('[AuditDB] $funcName failed: $e');
      return {'success': false, 'error': '$funcName failed: $e'};
    }
  }
}
