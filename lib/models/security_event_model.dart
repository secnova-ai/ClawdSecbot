/// 安全事件记录模型，对应 Go 层 SecurityEvent 结构。
class SecurityEvent {
  final String id;
  final DateTime timestamp;
  final String eventType; // tool_execution | blocked | other
  final String actionDesc; // 工具执行动作描述（LLM 生成的中文）
  final String riskType; // 风险类型（中文）
  final String detail; // 补充细节
  final String source; // react_agent | heuristic
  final String assetName; // openclaw | nullclaw
  final String assetID; // 资产实例ID
  final String requestID; // 关联的请求ID（审计日志 request_id）

  SecurityEvent({
    required this.id,
    required this.timestamp,
    required this.eventType,
    required this.actionDesc,
    required this.riskType,
    required this.detail,
    required this.source,
    this.assetName = '',
    this.assetID = '',
    this.requestID = '',
  });

  factory SecurityEvent.fromJson(Map<String, dynamic> json) {
    return SecurityEvent(
      id: json['id'] as String? ?? '',
      timestamp: _parseTimestamp(json['timestamp']),
      eventType: json['event_type'] as String? ?? 'other',
      actionDesc: json['action_desc'] as String? ?? '',
      riskType: json['risk_type'] as String? ?? '',
      detail: json['detail'] as String? ?? '',
      source: json['source'] as String? ?? '',
      assetName: json['asset_name'] as String? ?? '',
      assetID: json['asset_id'] as String? ?? '',
      requestID: json['request_id'] as String? ?? '',
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'id': id,
      'timestamp': timestamp.toIso8601String(),
      'event_type': eventType,
      'action_desc': actionDesc,
      'risk_type': riskType,
      'detail': detail,
      'source': source,
      'asset_name': assetName,
      'asset_id': assetID,
      'request_id': requestID,
    };
  }

  static DateTime _parseTimestamp(dynamic value) {
    if (value == null) return DateTime.now();
    if (value is String && value.isNotEmpty) {
      return DateTime.tryParse(value) ?? DateTime.now();
    }
    return DateTime.now();
  }

  /// 是否为拦截事件
  bool get isBlocked => eventType == 'blocked';

  /// 是否为工具执行事件
  bool get isToolExecution => eventType == 'tool_execution';

  /// 是否来自 ReAct Agent
  bool get isFromReactAgent => source == 'react_agent';

  /// 是否来自启发式检测
  bool get isFromHeuristic => source == 'heuristic';
}
