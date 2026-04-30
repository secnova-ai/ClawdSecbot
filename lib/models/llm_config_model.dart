/// 安全模型配置
/// 用于 ShepherdGate 风险检测的 LLM 配置
/// 注意: language 已移到全局 app_settings 管理，customSecurityPrompt 已废弃
class SecurityModelConfig {
  /// LLM 提供商类型
  /// 如: 'openai', 'anthropic', 'google', 'deepseek', 'ollama' 等
  final String provider;

  /// API 端点或基础 URL
  final String endpoint;

  /// API 密钥
  final String apiKey;

  /// 模型名称
  /// 如: 'gpt-4', 'claude-3.5-sonnet', 'gemini-pro' 等
  final String model;

  /// 密钥 (特定提供商需要,如千帆的 secret_key)
  final String secretKey;

  /// 创建安全模型配置
  const SecurityModelConfig({
    required this.provider,
    required this.endpoint,
    required this.apiKey,
    required this.model,
    this.secretKey = '',
  });

  /// 从 JSON 创建配置
  factory SecurityModelConfig.fromJson(Map<String, dynamic> json) {
    // 兼容新旧字段名: provider (新) / type (旧)
    final providerValue = json['provider'] ?? json['type'] ?? '';
    return SecurityModelConfig(
      provider: providerValue,
      endpoint: json['endpoint'] ?? '',
      apiKey: json['api_key'] ?? '',
      model: json['model'] ?? '',
      secretKey: json['secret_key'] ?? '',
    );
  }

  /// 转换为 JSON
  Map<String, dynamic> toJson() {
    return {
      'provider': provider,
      'endpoint': endpoint,
      'api_key': apiKey,
      'model': model,
      if (secretKey.isNotEmpty) 'secret_key': secretKey,
    };
  }

  /// 验证配置是否有效
  bool get isValid {
    return provider.isNotEmpty && model.isNotEmpty;
  }

  /// 复制并修改配置
  SecurityModelConfig copyWith({
    String? provider,
    String? endpoint,
    String? apiKey,
    String? model,
    String? secretKey,
  }) {
    return SecurityModelConfig(
      provider: provider ?? this.provider,
      endpoint: endpoint ?? this.endpoint,
      apiKey: apiKey ?? this.apiKey,
      model: model ?? this.model,
      secretKey: secretKey ?? this.secretKey,
    );
  }

  @override
  String toString() {
    return 'SecurityModelConfig(provider: $provider, model: $model, endpoint: $endpoint)';
  }

  @override
  bool operator ==(Object other) {
    if (identical(this, other)) return true;
    return other is SecurityModelConfig &&
        other.provider == provider &&
        other.endpoint == endpoint &&
        other.apiKey == apiKey &&
        other.model == model &&
        other.secretKey == secretKey;
  }

  @override
  int get hashCode {
    return Object.hash(provider, endpoint, apiKey, model, secretKey);
  }
}

/// Bot 模型配置
/// 用于代理转发的目标 LLM 配置
class BotModelConfig {
  /// 关联的资产名称
  final String assetName;

  /// 关联的资产实例ID（多实例场景）
  final String assetID;

  /// LLM 提供商类型
  final String provider;

  /// API 基础 URL
  final String baseUrl;

  /// API 密钥
  final String apiKey;

  /// 模型名称
  final String model;

  /// 密钥 (特定提供商需要)
  final String secretKey;

  /// 创建 Bot 模型配置
  const BotModelConfig({
    required this.assetName,
    this.assetID = '',
    required this.provider,
    required this.baseUrl,
    required this.apiKey,
    required this.model,
    this.secretKey = '',
  });

  /// 从 JSON 创建配置
  factory BotModelConfig.fromJson(Map<String, dynamic> json) {
    // 兼容新旧字段名: base_url (新) / endpoint (旧)
    final baseUrlValue = json['base_url'] ?? json['endpoint'] ?? '';
    // 兼容新旧字段名: provider (新) / type (旧)
    final providerValue = json['provider'] ?? json['type'] ?? '';
    return BotModelConfig(
      assetName: json['asset_name'] ?? '',
      assetID: json['asset_id'] ?? '',
      provider: providerValue,
      baseUrl: baseUrlValue,
      apiKey: json['api_key'] ?? '',
      model: json['model'] ?? '',
      secretKey: json['secret_key'] ?? '',
    );
  }

  /// 转换为 JSON
  Map<String, dynamic> toJson() {
    return {
      'asset_name': assetName,
      'asset_id': assetID,
      'provider': provider,
      'base_url': baseUrl,
      'api_key': apiKey,
      'model': model,
      if (secretKey.isNotEmpty) 'secret_key': secretKey,
    };
  }

  /// 验证配置是否有效
  bool get isValid {
    return assetName.isNotEmpty && provider.isNotEmpty && baseUrl.isNotEmpty;
  }

  /// 复制并修改配置
  BotModelConfig copyWith({
    String? assetName,
    String? assetID,
    String? provider,
    String? baseUrl,
    String? apiKey,
    String? model,
    String? secretKey,
  }) {
    return BotModelConfig(
      assetName: assetName ?? this.assetName,
      assetID: assetID ?? this.assetID,
      provider: provider ?? this.provider,
      baseUrl: baseUrl ?? this.baseUrl,
      apiKey: apiKey ?? this.apiKey,
      model: model ?? this.model,
      secretKey: secretKey ?? this.secretKey,
    );
  }

  @override
  String toString() {
    return 'BotModelConfig(asset: $assetName/$assetID, provider: $provider, model: $model)';
  }

  @override
  bool operator ==(Object other) {
    if (identical(this, other)) return true;
    return other is BotModelConfig &&
        other.assetName == assetName &&
        other.assetID == assetID &&
        other.provider == provider &&
        other.baseUrl == baseUrl &&
        other.apiKey == apiKey &&
        other.model == model &&
        other.secretKey == secretKey;
  }

  @override
  int get hashCode {
    return Object.hash(
      assetName,
      assetID,
      provider,
      baseUrl,
      apiKey,
      model,
      secretKey,
    );
  }
}

/// 防护运行时配置
/// 包含代理运行时的各种参数
class ProtectionRuntimeConfig {
  /// 仅审计模式 (true: 跳过风险分析,仅记录日志)
  final bool auditOnly;

  /// 单次会话 token 上限 (0 表示不限制)
  final int singleSessionTokenLimit;

  /// 每日 token 上限 (0 表示不限制)
  final int dailyTokenLimit;

  /// 初始每日 token 使用量 (从数据库恢复)
  final int initialDailyTokenUsage;

  /// 用户输入检测是否启用
  final bool? userInputDetectionEnabled;

  /// 创建运行时配置
  const ProtectionRuntimeConfig({
    this.auditOnly = false,
    this.singleSessionTokenLimit = 0,
    this.dailyTokenLimit = 0,
    this.initialDailyTokenUsage = 0,
    this.userInputDetectionEnabled,
  });

  /// 从 JSON 创建配置
  factory ProtectionRuntimeConfig.fromJson(Map<String, dynamic> json) {
    return ProtectionRuntimeConfig(
      auditOnly: json['audit_only'] == true,
      singleSessionTokenLimit: json['single_session_token_limit'] ?? 0,
      dailyTokenLimit: json['daily_token_limit'] ?? 0,
      initialDailyTokenUsage: json['initial_daily_token_usage'] ?? 0,
      userInputDetectionEnabled: json['user_input_detection_enabled'] == null
          ? null
          : (json['user_input_detection_enabled'] == true ||
                json['user_input_detection_enabled'] == 1),
    );
  }

  /// 转换为 JSON
  Map<String, dynamic> toJson() {
    return {
      'audit_only': auditOnly,
      'single_session_token_limit': singleSessionTokenLimit,
      'daily_token_limit': dailyTokenLimit,
      'initial_daily_token_usage': initialDailyTokenUsage,
      if (userInputDetectionEnabled != null)
        'user_input_detection_enabled': userInputDetectionEnabled,
    };
  }

  /// 复制并修改配置
  ProtectionRuntimeConfig copyWith({
    bool? auditOnly,
    int? singleSessionTokenLimit,
    int? dailyTokenLimit,
    int? initialDailyTokenUsage,
    bool? userInputDetectionEnabled,
  }) {
    return ProtectionRuntimeConfig(
      auditOnly: auditOnly ?? this.auditOnly,
      singleSessionTokenLimit:
          singleSessionTokenLimit ?? this.singleSessionTokenLimit,
      dailyTokenLimit: dailyTokenLimit ?? this.dailyTokenLimit,
      initialDailyTokenUsage:
          initialDailyTokenUsage ?? this.initialDailyTokenUsage,
      userInputDetectionEnabled:
          userInputDetectionEnabled ?? this.userInputDetectionEnabled,
    );
  }

  @override
  String toString() {
    return 'ProtectionRuntimeConfig(auditOnly: $auditOnly, userInputDetection: $userInputDetectionEnabled, sessionLimit: $singleSessionTokenLimit, dailyLimit: $dailyTokenLimit)';
  }
}

/// 防护基线统计数据
/// 从数据库恢复的历史统计信息
class ProtectionBaselineStatistics {
  /// 历史分析次数
  final int analysisCount;

  /// 历史拦截次数
  final int blockedCount;

  /// 历史警告次数
  final int warningCount;

  /// 总 token 使用量
  final int totalTokens;

  /// 总 prompt token 使用量
  final int totalPromptTokens;

  /// 总 completion token 使用量
  final int totalCompletionTokens;

  /// 总工具调用次数
  final int totalToolCalls;

  /// 总请求次数
  final int requestCount;

  /// 审计 token 使用量
  final int auditTokens;

  /// 审计 prompt token 使用量
  final int auditPromptTokens;

  /// 审计 completion token 使用量
  final int auditCompletionTokens;

  /// 创建基线统计数据
  const ProtectionBaselineStatistics({
    this.analysisCount = 0,
    this.blockedCount = 0,
    this.warningCount = 0,
    this.totalTokens = 0,
    this.totalPromptTokens = 0,
    this.totalCompletionTokens = 0,
    this.totalToolCalls = 0,
    this.requestCount = 0,
    this.auditTokens = 0,
    this.auditPromptTokens = 0,
    this.auditCompletionTokens = 0,
  });

  /// 从 JSON 创建统计数据
  factory ProtectionBaselineStatistics.fromJson(Map<String, dynamic> json) {
    return ProtectionBaselineStatistics(
      analysisCount: json['baseline_analysis_count'] ?? 0,
      blockedCount: json['baseline_blocked_count'] ?? 0,
      warningCount: json['baseline_warning_count'] ?? 0,
      totalTokens: json['baseline_total_tokens'] ?? 0,
      totalPromptTokens: json['baseline_total_prompt_tokens'] ?? 0,
      totalCompletionTokens: json['baseline_total_completion_tokens'] ?? 0,
      totalToolCalls: json['baseline_total_tool_calls'] ?? 0,
      requestCount: json['baseline_request_count'] ?? 0,
      auditTokens: json['baseline_audit_tokens'] ?? 0,
      auditPromptTokens: json['baseline_audit_prompt_tokens'] ?? 0,
      auditCompletionTokens: json['baseline_audit_completion_tokens'] ?? 0,
    );
  }

  /// 转换为 JSON
  Map<String, dynamic> toJson() {
    return {
      'baseline_analysis_count': analysisCount,
      'baseline_blocked_count': blockedCount,
      'baseline_warning_count': warningCount,
      'baseline_total_tokens': totalTokens,
      'baseline_total_prompt_tokens': totalPromptTokens,
      'baseline_total_completion_tokens': totalCompletionTokens,
      'baseline_total_tool_calls': totalToolCalls,
      'baseline_request_count': requestCount,
      'baseline_audit_tokens': auditTokens,
      'baseline_audit_prompt_tokens': auditPromptTokens,
      'baseline_audit_completion_tokens': auditCompletionTokens,
    };
  }

  @override
  String toString() {
    return 'ProtectionBaselineStatistics(analysis: $analysisCount, blocked: $blockedCount, tokens: $totalTokens)';
  }
}
