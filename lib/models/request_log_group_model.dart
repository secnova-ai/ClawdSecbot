/// 请求日志分组数据模型
/// 用于在防护监控窗口中按 request_id 对日志进行分组展示
class RequestLogGroup {
  final String requestId;
  final DateTime startTime;
  String model = '';
  int messageCount = 0;
  String stream = '';
  final List<RequestMessageSummary> messages = [];
  String finishReason = '';
  String responseContent = '';
  String streamContent = '';
  String originalStreamContent = '';
  String securityMessage = '';
  int promptTokens = 0;
  int completionTokens = 0;
  int totalTokens = 0;
  int toolCallCount = 0;
  final List<String> toolNames = [];

  /// 工具执行结果的单行摘要列表,由 proxy_tool_result_content 填充,
  /// 与 messages 列表解耦,避免原始 JSON 污染卡片展示。
  final List<String> toolResultSummaries = [];

  String decisionStatus = '';
  String decisionReason = '';
  bool decisionBlocked = false;
  RequestLogGroup(this.requestId, this.startTime);
}

/// 请求消息摘要
class RequestMessageSummary {
  final int index;
  final String role;
  final String content;
  RequestMessageSummary(this.index, this.role, this.content);
}
