import 'dart:convert';
import 'dart:ffi' as ffi;
import 'package:ffi/ffi.dart';
import '../models/audit_log_model.dart';
import '../utils/app_logger.dart';
import 'native_library_service.dart';

// FFI type definitions
typedef _OneArgC = ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);
typedef _OneArgDart = ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);

/// 审计日志 FFI 持久化门面：通过 FFI 委托 Go 层进行数据持久化，Flutter 不直接操作 DB。
class AuditLogDatabaseService {
  static final AuditLogDatabaseService _instance =
      AuditLogDatabaseService._internal();

  factory AuditLogDatabaseService() => _instance;

  AuditLogDatabaseService._internal();

  ffi.DynamicLibrary? get _dylib => NativeLibraryService().dylib;
  FreeStringDart? get _freeString => NativeLibraryService().freeString;

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
    String assetName = '',
    String assetID = '',
    DateTime? startTime,
    DateTime? endTime,
    String? searchQuery,
  }) async {
    final filter = <String, dynamic>{
      'limit': limit,
      'offset': offset,
      'risk_only': riskOnly,
      'asset_name': assetName,
      'asset_id': assetID,
    };
    if (startTime != null) filter['start_time'] = startTime.toIso8601String();
    if (endTime != null) filter['end_time'] = endTime.toIso8601String();
    if (searchQuery != null && searchQuery.isNotEmpty) {
      filter['search_query'] = searchQuery;
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
      );
    }).toList();
  }

  /// Get audit log count with optional filtering
  Future<int> getAuditLogCount({
    bool riskOnly = false,
    String assetName = '',
    String assetID = '',
  }) async {
    final result = _callFFI(
      'GetAuditLogCountFFI',
      jsonEncode({
        'risk_only': riskOnly,
        'asset_name': assetName,
        'asset_id': assetID,
      }),
    );

    if (result['success'] != true) return 0;
    return result['data'] as int? ?? 0;
  }

  /// Get audit log statistics
  Future<Map<String, dynamic>> getAuditLogStatistics({
    String assetName = '',
    String assetID = '',
  }) async {
    final result = _callFFI(
      'GetAuditLogStatisticsByFilterFFI',
      jsonEncode({'asset_name': assetName, 'asset_id': assetID}),
    );
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

  /// Clear old audit logs (keep last N days)
  Future<void> cleanOldAuditLogs({int keepDays = 30}) async {
    _callFFI('CleanOldAuditLogsFFI', jsonEncode({'keep_days': keepDays}));
  }

  /// Clear all audit logs
  Future<void> clearAllAuditLogs({
    String assetName = '',
    String assetID = '',
  }) async {
    _callFFI(
      'ClearAllAuditLogsByFilterFFI',
      jsonEncode({'asset_name': assetName, 'asset_id': assetID}),
    );
  }

  // --- Helper methods ---

  Map<String, dynamic> _callFFI(String funcName, String jsonStr) {
    final dylib = _dylib;
    if (dylib == null || _freeString == null) {
      return {'success': false, 'error': 'Native library not initialized'};
    }

    try {
      final func = dylib.lookupFunction<_OneArgC, _OneArgDart>(funcName);
      final argPtr = jsonStr.toNativeUtf8();
      final resultPtr = func(argPtr);
      final result = resultPtr.toDartString();
      _freeString!(resultPtr);
      malloc.free(argPtr);
      return jsonDecode(result) as Map<String, dynamic>;
    } catch (e) {
      appLogger.error('[AuditDB] $funcName failed: $e');
      return {'success': false, 'error': '$funcName failed: $e'};
    }
  }
}
