/// TruthRecord 安全决策子模型
class SecurityDecisionModel {
  final String action;
  final String riskLevel;
  final String reason;
  final int confidence;

  SecurityDecisionModel({
    this.action = 'ALLOW',
    this.riskLevel = '',
    this.reason = '',
    this.confidence = 0,
  });

  factory SecurityDecisionModel.fromJson(Map<String, dynamic> json) {
    return SecurityDecisionModel(
      action: json['action'] as String? ?? 'ALLOW',
      riskLevel: json['risk_level'] as String? ?? '',
      reason: json['reason'] as String? ?? '',
      confidence: json['confidence'] as int? ?? 0,
    );
  }

  Map<String, dynamic> toJson() => {
    'action': action,
    'risk_level': riskLevel,
    'reason': reason,
    'confidence': confidence,
  };
}

/// TruthRecord 工具调用子模型
class RecordToolCallModel {
  final String id;
  final String name;
  final String arguments;
  final String result;
  final bool isSensitive;
  final String source;
  final bool latestRound;

  RecordToolCallModel({
    this.id = '',
    required this.name,
    this.arguments = '',
    this.result = '',
    this.isSensitive = false,
    this.source = 'response',
    this.latestRound = false,
  });

  factory RecordToolCallModel.fromJson(Map<String, dynamic> json) {
    return RecordToolCallModel(
      id: json['id'] as String? ?? '',
      name: json['name'] as String? ?? '',
      arguments: json['arguments'] as String? ?? '',
      result: json['result'] as String? ?? '',
      isSensitive: json['is_sensitive'] as bool? ?? false,
      source: json['source'] as String? ?? 'response',
      latestRound: json['latest_round'] as bool? ?? false,
    );
  }

  Map<String, dynamic> toJson() => {
    'id': id,
    'name': name,
    'arguments': arguments,
    'result': result,
    'is_sensitive': isSensitive,
    'source': source,
    'latest_round': latestRound,
  };
}

/// TruthRecord 消息条目子模型
class RecordMessageModel {
  final int index;
  final String role;
  final String content;

  RecordMessageModel({
    required this.index,
    required this.role,
    required this.content,
  });

  factory RecordMessageModel.fromJson(Map<String, dynamic> json) {
    return RecordMessageModel(
      index: json['index'] as int? ?? 0,
      role: json['role'] as String? ?? '',
      content: json['content'] as String? ?? '',
    );
  }
}

/// TruthRecordModel 是代理防护层核心数据实体的 Flutter 侧模型。
/// 从 Go 端 TruthRecord 快照直接映射，每次收到完整快照直接替换（不做 merge）。
/// 所有可推导属性通过 getter 计算，不存储冗余字段。
class TruthRecordModel {
  // 身份与归属
  final String requestId;
  final String assetName;
  final String assetID;

  // 时间线
  final DateTime startedAt;
  final DateTime updatedAt;
  final DateTime? completedAt;

  // 请求上下文
  final String model;
  final int messageCount;
  final List<RecordMessageModel> messages;

  // 响应
  final String phase;
  final String finishReason;
  final String primaryContent;
  final String primaryContentType;
  final String outputContent;

  // 工具链路
  final List<RecordToolCallModel> toolCalls;

  // 安全决策
  final SecurityDecisionModel? decision;

  // Token 指标
  final int promptTokens;
  final int completionTokens;

  TruthRecordModel({
    required this.requestId,
    required this.startedAt,
    required this.updatedAt,
    this.completedAt,
    this.assetName = '',
    this.assetID = '',
    this.model = '',
    this.messageCount = 0,
    this.messages = const [],
    this.phase = 'starting',
    this.finishReason = '',
    this.primaryContent = '',
    this.primaryContentType = 'unavailable',
    this.outputContent = '',
    this.toolCalls = const [],
    this.decision,
    this.promptTokens = 0,
    this.completionTokens = 0,
  });

  // ==================== 计算 getter（替代原冗余字段） ====================

  /// 请求是否已完成
  bool get isComplete => phase == 'completed' || phase == 'stopped';

  /// 是否包含工具调用
  bool get hasToolCall => toolCalls.isNotEmpty;

  /// 工具调用数量
  int get toolCallCount => toolCalls.length;

  /// 工具名称列表
  List<String> get toolNames => toolCalls.map((t) => t.name).toList();

  /// 工具参数预览列表
  List<String> get toolArgsPreview =>
      toolCalls.map((t) => t.arguments).where((a) => a.isNotEmpty).toList();

  /// 是否经过安全检测
  bool get hasSecurityCheck => decision != null;

  /// 是否被安全阻断
  bool get decisionBlocked =>
      decision?.action == 'BLOCK' || decision?.action == 'HARD_BLOCK';

  /// 是否存在风险
  bool get hasRisk =>
      decision != null &&
      decision!.riskLevel.isNotEmpty &&
      decision!.riskLevel != 'SAFE';

  /// 安全决策状态
  String get decisionStatus => decision?.action ?? '';

  /// 安全决策原因
  String get decisionReason => decision?.reason ?? '';

  /// 总 Token 数
  int get totalTokens => promptTokens + completionTokens;

  /// 请求耗时（毫秒）
  int get durationMs => completedAt != null
      ? completedAt!.difference(startedAt).inMilliseconds
      : 0;

  // ==================== 序列化 ====================

  factory TruthRecordModel.fromJson(Map<String, dynamic> json) {
    return TruthRecordModel(
      requestId: (json['request_id'] as String? ?? '').trim(),
      startedAt: _parseTimestamp(json['started_at']),
      updatedAt: _parseTimestamp(json['updated_at']),
      completedAt: json['completed_at'] != null && json['completed_at'] != ''
          ? _parseTimestamp(json['completed_at'])
          : null,
      assetName: json['asset_name'] as String? ?? '',
      assetID: json['asset_id'] as String? ?? '',
      model: json['model'] as String? ?? '',
      messageCount: json['message_count'] as int? ?? 0,
      messages: ((json['messages'] as List?) ?? const [])
          .whereType<Map>()
          .map((e) => RecordMessageModel.fromJson(Map<String, dynamic>.from(e)))
          .toList(),
      phase: json['phase'] as String? ?? 'starting',
      finishReason: json['finish_reason'] as String? ?? '',
      primaryContent: json['primary_content'] as String? ?? '',
      primaryContentType:
          json['primary_content_type'] as String? ?? 'unavailable',
      outputContent: json['output_content'] as String? ?? '',
      toolCalls: ((json['tool_calls'] as List?) ?? const [])
          .whereType<Map>()
          .map(
            (e) => RecordToolCallModel.fromJson(Map<String, dynamic>.from(e)),
          )
          .toList(),
      decision: json['decision'] != null && json['decision'] is Map
          ? SecurityDecisionModel.fromJson(
              Map<String, dynamic>.from(json['decision'] as Map),
            )
          : null,
      promptTokens: json['prompt_tokens'] as int? ?? 0,
      completionTokens: json['completion_tokens'] as int? ?? 0,
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'request_id': requestId,
      'started_at': startedAt.toIso8601String(),
      'updated_at': updatedAt.toIso8601String(),
      if (completedAt != null) 'completed_at': completedAt!.toIso8601String(),
      'asset_name': assetName,
      'asset_id': assetID,
      'model': model,
      'message_count': messageCount,
      'messages': messages
          .map(
            (m) => {'index': m.index, 'role': m.role, 'content': m.content},
          )
          .toList(),
      'phase': phase,
      'finish_reason': finishReason,
      'primary_content': primaryContent,
      'primary_content_type': primaryContentType,
      'output_content': outputContent,
      'tool_calls': toolCalls.map((t) => t.toJson()).toList(),
      if (decision != null) 'decision': decision!.toJson(),
      'prompt_tokens': promptTokens,
      'completion_tokens': completionTokens,
    };
  }

  static DateTime _parseTimestamp(dynamic value) {
    if (value == null) return DateTime.now().toLocal();
    final parsed = DateTime.tryParse(value.toString());
    if (parsed == null) {
      return DateTime.now().toLocal();
    }
    return parsed.toLocal();
  }
}
