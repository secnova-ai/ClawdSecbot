/// 请求卡片状态
enum RequestCardStatus {
  created,
  forwarding,
  streaming,
  toolCalling,
  completed,
  blocked,
  errored,
}

/// 请求元数据
class RequestMetaSection {
  String requestId;
  String assetId;
  DateTime startTime;
  String providerName;
  String providerProtocol;
  String clientModel;
  bool stream;

  RequestMetaSection({
    required this.requestId,
    required this.startTime,
    this.assetId = '',
    this.providerName = '',
    this.providerProtocol = '',
    this.clientModel = '',
    this.stream = false,
  });
}

/// 客户端输入区
class ClientInputSection {
  String messagesRaw;
  String latestUserMessage;
  final List<String> toolMessages;
  int messageCount;

  ClientInputSection({
    this.messagesRaw = '',
    this.latestUserMessage = '',
    List<String>? toolMessages,
    this.messageCount = 0,
  }) : toolMessages = toolMessages ?? [];
}

/// 上游请求区
class UpstreamRequestSection {
  String forwardedRaw;
  String forwardedNormalized;
  DateTime? forwardStartTime;
  bool sent;

  UpstreamRequestSection({
    this.forwardedRaw = '',
    this.forwardedNormalized = '',
    this.forwardStartTime,
    this.sent = false,
  });
}

/// 上游响应区
class UpstreamResponseSection {
  String responseMode;
  String streamPreviewText;
  String bufferedStreamText;
  String finalAssistantText;
  final List<String> streamDeltas;
  final List<String> toolCalls;
  final List<String> toolResults;
  String finishReason;
  int promptTokens;
  int completionTokens;
  int totalTokens;
  bool started;

  UpstreamResponseSection({
    this.responseMode = '',
    this.streamPreviewText = '',
    this.bufferedStreamText = '',
    this.finalAssistantText = '',
    List<String>? streamDeltas,
    List<String>? toolCalls,
    List<String>? toolResults,
    this.finishReason = '',
    this.promptTokens = 0,
    this.completionTokens = 0,
    this.totalTokens = 0,
    this.started = false,
  }) : streamDeltas = streamDeltas ?? [],
       toolCalls = toolCalls ?? [],
       toolResults = toolResults ?? [];
}

/// 代理返回区
class ProxyResultSection {
  String returnedToUserRaw;
  String returnedToUserText;
  String securityDecision;
  String decisionReason;
  bool blocked;
  String status;
  String securityMessage;

  ProxyResultSection({
    this.returnedToUserRaw = '',
    this.returnedToUserText = '',
    this.securityDecision = '',
    this.decisionReason = '',
    this.blocked = false,
    this.status = '',
    this.securityMessage = '',
  });
}

/// 请求日志卡片模型
class RequestLogGroup {
  final RequestMetaSection requestMeta;
  final ClientInputSection clientInput;
  final UpstreamRequestSection upstreamRequest;
  final UpstreamResponseSection upstreamResponse;
  final ProxyResultSection proxyResult;
  final List<RequestMessageSummary> messages;
  RequestCardStatus status;
  String latestRawResponsePreview;

  RequestLogGroup(String requestId, DateTime startTime, {String assetId = ''})
    : requestMeta = RequestMetaSection(
        requestId: requestId,
        startTime: startTime,
        assetId: assetId,
      ),
      clientInput = ClientInputSection(),
      upstreamRequest = UpstreamRequestSection(),
      upstreamResponse = UpstreamResponseSection(),
      proxyResult = ProxyResultSection(),
      messages = [],
      status = RequestCardStatus.created,
      latestRawResponsePreview = '';

  String get requestId => requestMeta.requestId;
  DateTime get startTime => requestMeta.startTime;

  String get model => requestMeta.clientModel;
  set model(String value) => requestMeta.clientModel = value;

  int get messageCount => clientInput.messageCount;
  set messageCount(int value) => clientInput.messageCount = value;

  String get stream => requestMeta.stream ? 'true' : 'false';
  set stream(String value) {
    final normalized = value.trim().toLowerCase();
    requestMeta.stream = normalized == 'true' || normalized == '1';
  }

  String get finishReason => upstreamResponse.finishReason;
  set finishReason(String value) => upstreamResponse.finishReason = value;

  String get responseContent => upstreamResponse.finalAssistantText;
  set responseContent(String value) =>
      upstreamResponse.finalAssistantText = value;

  String get latestUserMessage => clientInput.latestUserMessage;
  set latestUserMessage(String value) => clientInput.latestUserMessage = value;

  String get streamContent => upstreamResponse.streamPreviewText;
  set streamContent(String value) => upstreamResponse.streamPreviewText = value;

  String get originalStreamContent => upstreamResponse.streamPreviewText;
  set originalStreamContent(String value) =>
      upstreamResponse.streamPreviewText = value;

  String get pendingStreamContent => upstreamResponse.bufferedStreamText;
  set pendingStreamContent(String value) =>
      upstreamResponse.bufferedStreamText = value;

  String get securityMessage => proxyResult.securityMessage;
  set securityMessage(String value) => proxyResult.securityMessage = value;

  int get promptTokens => upstreamResponse.promptTokens;
  set promptTokens(int value) => upstreamResponse.promptTokens = value;

  int get completionTokens => upstreamResponse.completionTokens;
  set completionTokens(int value) => upstreamResponse.completionTokens = value;

  int get totalTokens => upstreamResponse.totalTokens;
  set totalTokens(int value) => upstreamResponse.totalTokens = value;

  int get toolCallCount => upstreamResponse.toolCalls.length;
  set toolCallCount(int value) {}

  List<String> get toolNames => upstreamResponse.toolCalls;
  List<String> get toolResultSummaries => upstreamResponse.toolResults;

  String get decisionStatus => proxyResult.securityDecision;
  set decisionStatus(String value) => proxyResult.securityDecision = value;

  String get decisionReason => proxyResult.decisionReason;
  set decisionReason(String value) => proxyResult.decisionReason = value;

  bool get decisionBlocked => proxyResult.blocked;
  set decisionBlocked(bool value) => proxyResult.blocked = value;
}

/// 请求消息摘要
class RequestMessageSummary {
  final int index;
  final String role;
  final String content;
  RequestMessageSummary(this.index, this.role, this.content);
}
