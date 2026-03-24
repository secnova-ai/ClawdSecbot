/// Audit log model for protection proxy
class AuditLog {
  final String id;
  final DateTime timestamp;
  final String requestId;
  final String assetName;
  final String assetID;
  final String? model;
  final String requestContent;
  final List<AuditToolCall> toolCalls;
  final String? outputContent;
  final bool hasRisk;
  final String? riskLevel;
  final String? riskReason;
  final int? confidence;
  final String action;
  final int? promptTokens;
  final int? completionTokens;
  final int? totalTokens;
  final int durationMs;

  AuditLog({
    required this.id,
    required this.timestamp,
    required this.requestId,
    this.assetName = '',
    this.assetID = '',
    this.model,
    required this.requestContent,
    required this.toolCalls,
    this.outputContent,
    required this.hasRisk,
    this.riskLevel,
    this.riskReason,
    this.confidence,
    required this.action,
    this.promptTokens,
    this.completionTokens,
    this.totalTokens,
    required this.durationMs,
  });

  factory AuditLog.fromJson(Map<String, dynamic> json) {
    return AuditLog(
      id: json['id'] as String,
      timestamp: DateTime.parse(json['timestamp'] as String),
      requestId: json['request_id'] as String,
      assetName: json['asset_name'] as String? ?? '',
      assetID: json['asset_id'] as String? ?? '',
      model: json['model'] as String?,
      requestContent: json['request_content'] as String? ?? '',
      toolCalls:
          (json['tool_calls'] as List<dynamic>?)
              ?.map((e) => AuditToolCall.fromJson(e as Map<String, dynamic>))
              .toList() ??
          [],
      outputContent: json['output_content'] as String?,
      hasRisk: json['has_risk'] as bool? ?? false,
      riskLevel: json['risk_level'] as String?,
      riskReason: json['risk_reason'] as String?,
      confidence: json['confidence'] as int?,
      action: json['action'] as String? ?? 'ALLOW',
      promptTokens: json['prompt_tokens'] as int?,
      completionTokens: json['completion_tokens'] as int?,
      totalTokens: json['total_tokens'] as int?,
      durationMs: json['duration_ms'] as int? ?? 0,
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'id': id,
      'timestamp': timestamp.toIso8601String(),
      'request_id': requestId,
      'asset_name': assetName,
      'asset_id': assetID,
      'model': model,
      'request_content': requestContent,
      'tool_calls': toolCalls.map((e) => e.toJson()).toList(),
      'output_content': outputContent,
      'has_risk': hasRisk,
      'risk_level': riskLevel,
      'risk_reason': riskReason,
      'confidence': confidence,
      'action': action,
      'prompt_tokens': promptTokens,
      'completion_tokens': completionTokens,
      'total_tokens': totalTokens,
      'duration_ms': durationMs,
    };
  }

  /// Get risk level color
  RiskLevelColor get riskLevelColor {
    switch (riskLevel?.toUpperCase()) {
      case 'CRITICAL':
        return RiskLevelColor.critical;
      case 'DANGEROUS':
        return RiskLevelColor.dangerous;
      case 'SUSPICIOUS':
        return RiskLevelColor.suspicious;
      case 'SAFE':
      default:
        return RiskLevelColor.safe;
    }
  }

  /// Get action display text
  String get actionDisplayText {
    switch (action.toUpperCase()) {
      case 'BLOCK':
        return '已拦截';
      case 'HARD_BLOCK':
        return '强制拦截';
      case 'WARN':
        return '警告';
      case 'ALLOW':
      default:
        return '允许';
    }
  }
}

/// Audit tool call model
class AuditToolCall {
  final String name;
  final String arguments;
  final String? result;
  final bool isSensitive;

  AuditToolCall({
    required this.name,
    required this.arguments,
    this.result,
    this.isSensitive = false,
  });

  factory AuditToolCall.fromJson(Map<String, dynamic> json) {
    return AuditToolCall(
      name: json['name'] as String? ?? '',
      arguments: json['arguments'] as String? ?? '',
      result: json['result'] as String?,
      isSensitive: json['is_sensitive'] as bool? ?? false,
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'name': name,
      'arguments': arguments,
      'result': result,
      'is_sensitive': isSensitive,
    };
  }
}

/// Risk level color enum
enum RiskLevelColor { safe, suspicious, dangerous, critical }

/// Audit log query result
class AuditLogQueryResult {
  final List<AuditLog> logs;
  final int total;

  AuditLogQueryResult({required this.logs, required this.total});

  factory AuditLogQueryResult.fromJson(Map<String, dynamic> json) {
    return AuditLogQueryResult(
      logs:
          (json['logs'] as List<dynamic>?)
              ?.map((e) => AuditLog.fromJson(e as Map<String, dynamic>))
              .toList() ??
          [],
      total: json['total'] as int? ?? 0,
    );
  }
}
