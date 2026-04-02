class RequestViewMessage {
  final int index;
  final String role;
  final String content;

  RequestViewMessage({
    required this.index,
    required this.role,
    required this.content,
  });

  factory RequestViewMessage.fromJson(Map<String, dynamic> json) {
    return RequestViewMessage(
      index: json['index'] as int? ?? 0,
      role: json['role'] as String? ?? '',
      content: json['content'] as String? ?? '',
    );
  }
}

class RequestViewModel {
  final String requestId;
  final DateTime startedAt;
  final DateTime updatedAt;
  final DateTime? completedAt;
  final String assetName;
  final String assetID;
  final String model;
  final int messageCount;
  final List<RequestViewMessage> messages;
  final String primaryContent;
  final String primaryContentType;
  final String contentState;
  final int fullContentLength;
  final bool contentTruncated;
  final int toolCallCount;
  final List<String> toolNames;
  final List<String> toolArgsPreview;
  final String finishReason;
  final String decisionStatus;
  final String decisionReason;
  final bool decisionBlocked;
  final String phase;
  final bool hasToolCall;
  final bool hasSecurityCheck;
  final bool isComplete;
  final int promptTokens;
  final int completionTokens;
  final int totalTokens;

  RequestViewModel({
    required this.requestId,
    required this.startedAt,
    required this.updatedAt,
    this.completedAt,
    this.assetName = '',
    this.assetID = '',
    this.model = '',
    this.messageCount = 0,
    this.messages = const [],
    this.primaryContent = '',
    this.primaryContentType = 'unavailable',
    this.contentState = 'unavailable',
    this.fullContentLength = 0,
    this.contentTruncated = false,
    this.toolCallCount = 0,
    this.toolNames = const [],
    this.toolArgsPreview = const [],
    this.finishReason = '',
    this.decisionStatus = '',
    this.decisionReason = '',
    this.decisionBlocked = false,
    this.phase = 'starting',
    this.hasToolCall = false,
    this.hasSecurityCheck = false,
    this.isComplete = false,
    this.promptTokens = 0,
    this.completionTokens = 0,
    this.totalTokens = 0,
  });

  factory RequestViewModel.fromJson(Map<String, dynamic> json) {
    return RequestViewModel(
      requestId: json['request_id'] as String? ?? '',
      startedAt: _parseTimestamp(json['started_at']),
      updatedAt: _parseTimestamp(json['updated_at']),
      completedAt: json['completed_at'] != null
          ? _parseTimestamp(json['completed_at'])
          : null,
      assetName: json['asset_name'] as String? ?? '',
      assetID: json['asset_id'] as String? ?? '',
      model: json['model'] as String? ?? '',
      messageCount: json['message_count'] as int? ?? 0,
      messages: ((json['messages'] as List?) ?? const [])
          .whereType<Map>()
          .map((e) => RequestViewMessage.fromJson(Map<String, dynamic>.from(e)))
          .toList(),
      primaryContent: json['primary_content'] as String? ?? '',
      primaryContentType:
          json['primary_content_type'] as String? ?? 'unavailable',
      contentState: json['content_state'] as String? ?? 'unavailable',
      fullContentLength: json['full_content_length'] as int? ?? 0,
      contentTruncated: json['content_truncated'] as bool? ?? false,
      toolCallCount: json['tool_call_count'] as int? ?? 0,
      toolNames: ((json['tool_names'] as List?) ?? const [])
          .map((e) => e.toString())
          .toList(),
      toolArgsPreview: ((json['tool_args_preview'] as List?) ?? const [])
          .map((e) => e.toString())
          .toList(),
      finishReason: json['finish_reason'] as String? ?? '',
      decisionStatus: json['decision_status'] as String? ?? '',
      decisionReason: json['decision_reason'] as String? ?? '',
      decisionBlocked: json['decision_blocked'] as bool? ?? false,
      phase: json['phase'] as String? ?? 'starting',
      hasToolCall: json['has_tool_call'] as bool? ?? false,
      hasSecurityCheck: json['has_security_check'] as bool? ?? false,
      isComplete: json['is_complete'] as bool? ?? false,
      promptTokens: json['prompt_tokens'] as int? ?? 0,
      completionTokens: json['completion_tokens'] as int? ?? 0,
      totalTokens: json['total_tokens'] as int? ?? 0,
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
          .map((m) => {'index': m.index, 'role': m.role, 'content': m.content})
          .toList(),
      'primary_content': primaryContent,
      'primary_content_type': primaryContentType,
      'content_state': contentState,
      'full_content_length': fullContentLength,
      'content_truncated': contentTruncated,
      'tool_call_count': toolCallCount,
      'tool_names': toolNames,
      'tool_args_preview': toolArgsPreview,
      'finish_reason': finishReason,
      'decision_status': decisionStatus,
      'decision_reason': decisionReason,
      'decision_blocked': decisionBlocked,
      'phase': phase,
      'has_tool_call': hasToolCall,
      'has_security_check': hasSecurityCheck,
      'is_complete': isComplete,
      'prompt_tokens': promptTokens,
      'completion_tokens': completionTokens,
      'total_tokens': totalTokens,
    };
  }

  RequestViewModel merge(RequestViewModel incoming) {
    if (requestId != incoming.requestId) {
      return incoming;
    }

    final newer = incoming.updatedAt.isAfter(updatedAt) ? incoming : this;
    final older = identical(newer, this) ? incoming : this;

    final mergedMessages = <RequestViewMessage>[];
    final seenMessages = <String>{};
    void appendMessages(List<RequestViewMessage> source) {
      for (final message in source) {
        final key = '${message.index}|${message.role}|${message.content}';
        if (seenMessages.add(key)) {
          mergedMessages.add(message);
        }
      }
    }

    appendMessages(older.messages);
    appendMessages(newer.messages);
    mergedMessages.sort((a, b) => a.index.compareTo(b.index));

    final mergedToolNames = <String>{
      ...toolNames,
      ...incoming.toolNames,
    }.toList();
    final mergedToolArgs = <String>[
      ...toolArgsPreview,
      ...incoming.toolArgsPreview,
    ].where((item) => item.trim().isNotEmpty).toSet().toList();

    final preferIncomingContent =
        incoming.primaryContent.trim().isNotEmpty ||
        incoming.contentState != 'unavailable' ||
        _primaryContentRank(incoming.primaryContentType) >
            _primaryContentRank(primaryContentType);

    final mergedPrimaryContent = preferIncomingContent
        ? incoming.primaryContent
        : primaryContent;
    final mergedPrimaryContentType = preferIncomingContent
        ? incoming.primaryContentType
        : primaryContentType;
    final mergedContentState = preferIncomingContent
        ? incoming.contentState
        : contentState;
    final mergedFullContentLength = preferIncomingContent
        ? incoming.fullContentLength
        : fullContentLength;
    final mergedContentTruncated = preferIncomingContent
        ? incoming.contentTruncated
        : contentTruncated;
    var effectivePrimaryContent = mergedPrimaryContent;
    var effectivePrimaryContentType = mergedPrimaryContentType;
    var effectiveContentState = mergedContentState;
    var effectiveFullContentLength = mergedFullContentLength;
    var effectiveContentTruncated = mergedContentTruncated;

    // 回填兜底：若主内容缺失但消息里已有 assistant 文本，则优先展示该文本。
    if (effectivePrimaryContent.trim().isEmpty ||
        effectiveContentState == 'unavailable') {
      for (var i = mergedMessages.length - 1; i >= 0; i--) {
        final message = mergedMessages[i];
        if (message.role.toLowerCase() != 'assistant') {
          continue;
        }
        final candidate = message.content.trim();
        if (candidate.isEmpty) {
          continue;
        }
        effectivePrimaryContent = candidate;
        effectivePrimaryContentType = 'assistant_response';
        effectiveContentState = 'present';
        effectiveFullContentLength = candidate.length;
        effectiveContentTruncated = false;
        break;
      }
    }

    final mergedCompletedAt = _latestCompletedAt(completedAt, incoming.completedAt);
    final inferredComplete =
        mergedCompletedAt != null ||
        isComplete ||
        incoming.isComplete ||
        newer.finishReason.trim().isNotEmpty;
    final mergedPhase = inferredComplete
        ? 'completed'
        : (_phaseRank(incoming.phase) > _phaseRank(phase)
              ? incoming.phase
              : phase);

    return RequestViewModel(
      requestId: requestId,
      startedAt: startedAt.isBefore(incoming.startedAt)
          ? startedAt
          : incoming.startedAt,
      updatedAt: updatedAt.isAfter(incoming.updatedAt)
          ? updatedAt
          : incoming.updatedAt,
      completedAt: mergedCompletedAt,
      assetName: newer.assetName.isNotEmpty ? newer.assetName : older.assetName,
      assetID: newer.assetID.isNotEmpty ? newer.assetID : older.assetID,
      model: newer.model.isNotEmpty ? newer.model : older.model,
      messageCount: newer.messageCount > older.messageCount
          ? newer.messageCount
          : older.messageCount,
      messages: mergedMessages,
      primaryContent: effectivePrimaryContent,
      primaryContentType: effectivePrimaryContentType,
      contentState: effectiveContentState,
      fullContentLength: effectiveFullContentLength,
      contentTruncated: effectiveContentTruncated,
      toolCallCount: newer.toolCallCount > older.toolCallCount
          ? newer.toolCallCount
          : older.toolCallCount,
      toolNames: mergedToolNames,
      toolArgsPreview: mergedToolArgs,
      finishReason: newer.finishReason.isNotEmpty
          ? newer.finishReason
          : older.finishReason,
      decisionStatus: newer.decisionStatus.isNotEmpty
          ? newer.decisionStatus
          : older.decisionStatus,
      decisionReason: newer.decisionReason.isNotEmpty
          ? newer.decisionReason
          : older.decisionReason,
      decisionBlocked: decisionBlocked || incoming.decisionBlocked,
      phase: mergedPhase,
      hasToolCall: hasToolCall || incoming.hasToolCall,
      hasSecurityCheck: hasSecurityCheck || incoming.hasSecurityCheck,
      isComplete: inferredComplete,
      promptTokens: promptTokens > incoming.promptTokens
          ? promptTokens
          : incoming.promptTokens,
      completionTokens: completionTokens > incoming.completionTokens
          ? completionTokens
          : incoming.completionTokens,
      totalTokens: totalTokens > incoming.totalTokens
          ? totalTokens
          : incoming.totalTokens,
    );
  }

  static DateTime _parseTimestamp(dynamic value) {
    if (value == null) return DateTime.now().toLocal();
    final parsed = DateTime.tryParse(value.toString());
    if (parsed == null) {
      return DateTime.now().toLocal();
    }
    return parsed.toLocal();
  }

  static DateTime? _latestCompletedAt(DateTime? left, DateTime? right) {
    if (left == null) return right;
    if (right == null) return left;
    return left.isAfter(right) ? left : right;
  }

  static int _phaseRank(String phase) {
    switch (phase.trim().toLowerCase()) {
      case 'completed':
        return 1;
      case 'stopped':
        return 2;
      case 'starting':
      default:
        return 0;
    }
  }

  static int _primaryContentRank(String type) {
    switch (type.trim().toLowerCase()) {
      case 'security_warning':
        return 4;
      case 'assistant_response':
        return 3;
      case 'tool_result_summary':
        return 2;
      case 'no_text_response':
        return 1;
      default:
        return 0;
    }
  }
}
