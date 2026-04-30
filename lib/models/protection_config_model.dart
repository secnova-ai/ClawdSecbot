import 'dart:convert';

import 'llm_config_model.dart';

/// 防护配置模型 - 按资产维度存储
class ProtectionConfig {
  final String assetName;
  final String assetID;

  // 防护是否启用
  final bool enabled;

  // 仅审计模式：不进行风险研判,仅记录审计日志
  final bool auditOnly;

  // 用户输入检测是否启用
  final bool userInputDetectionEnabled;

  // 沙箱防护是否启用（macOS 使用 sandbox-exec，Linux 使用 LD_PRELOAD，Windows 使用 hook sandbox）
  final bool sandboxEnabled;

  // 网关进程信息（用于沙箱监控）
  final String? gatewayBinaryPath; // 网关可执行文件路径
  final String? gatewayConfigPath; // 网关配置文件路径

  // Token限制配置
  final int singleSessionTokenLimit; // 单轮会话token上限,0表示不限制
  final int dailyTokenLimit; // 当天总token上限,0表示不限制

  // 权限设置 - 路径
  final PathPermissionConfig pathPermission;

  // 权限设置 - 网络
  final NetworkPermissionConfig networkPermission;

  // 权限设置 - Shell
  final ShellPermissionConfig shellPermission;

  // Bot模型配置（代理转发的目标LLM配置）
  final BotModelConfig? botModelConfig;

  final DateTime? createdAt;
  final DateTime? updatedAt;

  ProtectionConfig({
    required this.assetName,
    this.assetID = '',
    this.enabled = false,
    this.auditOnly = false,
    this.userInputDetectionEnabled = true,
    this.sandboxEnabled = false,
    this.gatewayBinaryPath,
    this.gatewayConfigPath,
    this.singleSessionTokenLimit = 0,
    this.dailyTokenLimit = 0,
    PathPermissionConfig? pathPermission,
    NetworkPermissionConfig? networkPermission,
    ShellPermissionConfig? shellPermission,
    this.botModelConfig,
    this.createdAt,
    this.updatedAt,
  }) : pathPermission = pathPermission ?? PathPermissionConfig(),
       networkPermission = networkPermission ?? NetworkPermissionConfig(),
       shellPermission = shellPermission ?? ShellPermissionConfig();

  /// 创建默认配置
  factory ProtectionConfig.defaultConfig(String assetName) {
    return ProtectionConfig(
      assetName: assetName,
      enabled: false,
      auditOnly: false,
      userInputDetectionEnabled: true,
      sandboxEnabled: false,
      createdAt: DateTime.now(),
      updatedAt: DateTime.now(),
    );
  }

  /// 从JSON解析
  factory ProtectionConfig.fromJson(Map<String, dynamic> json) {
    // 解析嵌入的 bot_model_config JSON
    BotModelConfig? botConfig;
    final botModelData = json['bot_model_config'];
    if (botModelData != null) {
      Map<String, dynamic> botJson;
      if (botModelData is String && botModelData.isNotEmpty) {
        botJson = jsonDecode(botModelData);
      } else if (botModelData is Map<String, dynamic>) {
        botJson = botModelData;
      } else {
        botJson = {};
      }
      if (botJson.isNotEmpty) {
        // 添加 asset_name 以保持 BotModelConfig 的完整性
        botJson['asset_name'] = json['asset_name'] ?? '';
        botConfig = BotModelConfig.fromJson(botJson);
      }
    }

    return ProtectionConfig(
      assetName: json['asset_name'] ?? '',
      assetID: json['asset_id'] as String? ?? '',
      enabled: json['enabled'] == true || json['enabled'] == 1,
      auditOnly: json['audit_only'] == true || json['audit_only'] == 1,
      userInputDetectionEnabled:
          json['user_input_detection_enabled'] == null ||
          json['user_input_detection_enabled'] == true ||
          json['user_input_detection_enabled'] == 1,
      sandboxEnabled:
          json['sandbox_enabled'] == true || json['sandbox_enabled'] == 1,
      gatewayBinaryPath: json['gateway_binary_path'],
      gatewayConfigPath: json['gateway_config_path'],
      singleSessionTokenLimit: json['single_session_token_limit'] ?? 0,
      dailyTokenLimit: json['daily_token_limit'] ?? 0,
      pathPermission: json['path_permission'] != null
          ? PathPermissionConfig.fromJson(
              json['path_permission'] is String
                  ? jsonDecode(json['path_permission'])
                  : json['path_permission'],
            )
          : PathPermissionConfig(),
      networkPermission: json['network_permission'] != null
          ? NetworkPermissionConfig.fromJson(
              json['network_permission'] is String
                  ? jsonDecode(json['network_permission'])
                  : json['network_permission'],
            )
          : NetworkPermissionConfig(),
      shellPermission: json['shell_permission'] != null
          ? ShellPermissionConfig.fromJson(
              json['shell_permission'] is String
                  ? jsonDecode(json['shell_permission'])
                  : json['shell_permission'],
            )
          : ShellPermissionConfig(),
      botModelConfig: botConfig,
      createdAt: json['created_at'] != null
          ? DateTime.parse(json['created_at'])
          : null,
      updatedAt: json['updated_at'] != null
          ? DateTime.parse(json['updated_at'])
          : null,
    );
  }

  /// 转换为JSON
  Map<String, dynamic> toJson() {
    return {
      'asset_name': assetName,
      'asset_id': assetID,
      'enabled': enabled,
      'audit_only': auditOnly,
      'user_input_detection_enabled': userInputDetectionEnabled,
      'sandbox_enabled': sandboxEnabled,
      'gateway_binary_path': gatewayBinaryPath,
      'gateway_config_path': gatewayConfigPath,
      'single_session_token_limit': singleSessionTokenLimit,
      'daily_token_limit': dailyTokenLimit,
      'path_permission': pathPermission.toJson(),
      'network_permission': networkPermission.toJson(),
      'shell_permission': shellPermission.toJson(),
      'bot_model_config': botModelConfig != null
          ? {
              'provider': botModelConfig!.provider,
              'base_url': botModelConfig!.baseUrl,
              'api_key': botModelConfig!.apiKey,
              'model': botModelConfig!.model,
              if (botModelConfig!.secretKey.isNotEmpty)
                'secret_key': botModelConfig!.secretKey,
            }
          : null,
      'created_at': createdAt?.toIso8601String(),
      'updated_at': updatedAt?.toIso8601String(),
    };
  }

  /// 复制并修改
  ProtectionConfig copyWith({
    String? assetName,
    String? assetID,
    bool? enabled,
    bool? auditOnly,
    bool? userInputDetectionEnabled,
    bool? sandboxEnabled,
    String? gatewayBinaryPath,
    String? gatewayConfigPath,
    int? singleSessionTokenLimit,
    int? dailyTokenLimit,
    PathPermissionConfig? pathPermission,
    NetworkPermissionConfig? networkPermission,
    ShellPermissionConfig? shellPermission,
    BotModelConfig? botModelConfig,
    bool clearBotModelConfig = false,
    DateTime? createdAt,
    DateTime? updatedAt,
  }) {
    return ProtectionConfig(
      assetName: assetName ?? this.assetName,
      assetID: assetID ?? this.assetID,
      enabled: enabled ?? this.enabled,
      auditOnly: auditOnly ?? this.auditOnly,
      userInputDetectionEnabled:
          userInputDetectionEnabled ?? this.userInputDetectionEnabled,
      sandboxEnabled: sandboxEnabled ?? this.sandboxEnabled,
      gatewayBinaryPath: gatewayBinaryPath ?? this.gatewayBinaryPath,
      gatewayConfigPath: gatewayConfigPath ?? this.gatewayConfigPath,
      singleSessionTokenLimit:
          singleSessionTokenLimit ?? this.singleSessionTokenLimit,
      dailyTokenLimit: dailyTokenLimit ?? this.dailyTokenLimit,
      pathPermission: pathPermission ?? this.pathPermission,
      networkPermission: networkPermission ?? this.networkPermission,
      shellPermission: shellPermission ?? this.shellPermission,
      botModelConfig: clearBotModelConfig
          ? null
          : (botModelConfig ?? this.botModelConfig),
      createdAt: createdAt ?? this.createdAt,
      updatedAt: updatedAt ?? DateTime.now(),
    );
  }

  /// 检查是否超过单轮会话Token限制
  bool exceedsSingleSessionLimit(int tokens) {
    if (singleSessionTokenLimit <= 0) return false;
    return tokens > singleSessionTokenLimit;
  }

  /// 检查是否超过当日Token限制
  bool exceedsDailyLimit(int dailyUsedTokens) {
    if (dailyTokenLimit <= 0) return false;
    return dailyUsedTokens > dailyTokenLimit;
  }
}

/// 权限模式枚举
enum PermissionMode {
  whitelist, // 白名单模式：只允许列表中的内容
  blacklist, // 黑名单模式：禁止列表中的内容
}

extension PermissionModeExtension on PermissionMode {
  String get name {
    switch (this) {
      case PermissionMode.whitelist:
        return 'whitelist';
      case PermissionMode.blacklist:
        return 'blacklist';
    }
  }

  static PermissionMode fromString(String? value) {
    switch (value) {
      case 'whitelist':
        return PermissionMode.whitelist;
      case 'blacklist':
      default:
        return PermissionMode.blacklist;
    }
  }
}

/// 路径权限设置
/// TODO: 后续实现路径检查逻辑
class PathPermissionConfig {
  final PermissionMode mode;
  final List<String> paths;

  PathPermissionConfig({
    this.mode = PermissionMode.blacklist,
    this.paths = const [],
  });

  factory PathPermissionConfig.fromJson(Map<String, dynamic> json) {
    return PathPermissionConfig(
      mode: PermissionModeExtension.fromString(json['mode']),
      paths: (json['paths'] as List?)?.cast<String>() ?? [],
    );
  }

  Map<String, dynamic> toJson() {
    return {'mode': mode.name, 'paths': paths};
  }

  PathPermissionConfig copyWith({PermissionMode? mode, List<String>? paths}) {
    return PathPermissionConfig(
      mode: mode ?? this.mode,
      paths: paths ?? this.paths,
    );
  }

  /// 检查路径是否被允许访问
  /// TODO: 实现实际的路径检查逻辑
  bool isPathAllowed(String path) {
    // 如果未配置任何路径,默认允许所有
    if (paths.isEmpty) return true;

    final isInList = paths.any((p) => path.startsWith(p) || p == path);

    if (mode == PermissionMode.whitelist) {
      // 白名单模式：只有在列表中的才允许
      return isInList;
    } else {
      // 黑名单模式：不在列表中的才允许
      return !isInList;
    }
  }
}

/// 单方向网络配置（入栈或出栈）
class DirectionalNetworkConfig {
  final PermissionMode mode;
  final List<String> addresses; // sandbox-exec 仅支持 * 或 localhost 作为 host

  DirectionalNetworkConfig({
    this.mode = PermissionMode.blacklist,
    this.addresses = const [],
  });

  factory DirectionalNetworkConfig.fromJson(Map<String, dynamic> json) {
    return DirectionalNetworkConfig(
      mode: PermissionModeExtension.fromString(json['mode']),
      addresses: (json['addresses'] as List?)?.cast<String>() ?? [],
    );
  }

  Map<String, dynamic> toJson() {
    return {'mode': mode.name, 'addresses': addresses};
  }

  DirectionalNetworkConfig copyWith({
    PermissionMode? mode,
    List<String>? addresses,
  }) {
    return DirectionalNetworkConfig(
      mode: mode ?? this.mode,
      addresses: addresses ?? this.addresses,
    );
  }
}

/// 网络权限设置
/// 支持入栈(inbound)和出栈(outbound)分别配置
/// - outbound: 控制进程主动发起的连接 -> sandbox-exec network-outbound
/// - inbound: 控制外部对进程发起的连接 -> sandbox-exec network-inbound
class NetworkPermissionConfig {
  final DirectionalNetworkConfig inbound;
  final DirectionalNetworkConfig outbound;

  NetworkPermissionConfig({
    DirectionalNetworkConfig? inbound,
    DirectionalNetworkConfig? outbound,
  }) : inbound = inbound ?? DirectionalNetworkConfig(),
       outbound = outbound ?? DirectionalNetworkConfig();

  /// 从JSON解析，兼容旧格式
  /// 旧格式: {"mode": "blacklist", "addresses": [...]}
  /// 新格式: {"inbound": {...}, "outbound": {...}}
  factory NetworkPermissionConfig.fromJson(Map<String, dynamic> json) {
    // 检测旧格式：顶层有 mode 字段
    if (json.containsKey('mode')) {
      // 旧格式迁移：将旧的 addresses 归入 outbound
      final legacyConfig = DirectionalNetworkConfig(
        mode: PermissionModeExtension.fromString(json['mode']),
        addresses: (json['addresses'] as List?)?.cast<String>() ?? [],
      );
      return NetworkPermissionConfig(
        inbound: DirectionalNetworkConfig(),
        outbound: legacyConfig,
      );
    }
    // 新格式
    return NetworkPermissionConfig(
      inbound: json['inbound'] != null
          ? DirectionalNetworkConfig.fromJson(
              json['inbound'] as Map<String, dynamic>,
            )
          : DirectionalNetworkConfig(),
      outbound: json['outbound'] != null
          ? DirectionalNetworkConfig.fromJson(
              json['outbound'] as Map<String, dynamic>,
            )
          : DirectionalNetworkConfig(),
    );
  }

  Map<String, dynamic> toJson() {
    return {'inbound': inbound.toJson(), 'outbound': outbound.toJson()};
  }

  NetworkPermissionConfig copyWith({
    DirectionalNetworkConfig? inbound,
    DirectionalNetworkConfig? outbound,
  }) {
    return NetworkPermissionConfig(
      inbound: inbound ?? this.inbound,
      outbound: outbound ?? this.outbound,
    );
  }

  /// 检查网络地址是否被允许（出栈方向）
  bool isOutboundAllowed(String address) {
    return _isAllowed(outbound, address);
  }

  /// 检查网络地址是否被允许（入栈方向）
  bool isInboundAllowed(String address) {
    return _isAllowed(inbound, address);
  }

  bool _isAllowed(DirectionalNetworkConfig config, String address) {
    if (config.addresses.isEmpty) return true;

    final isInList = config.addresses.any((a) => _matchAddress(address, a));

    if (config.mode == PermissionMode.whitelist) {
      return isInList;
    } else {
      return !isInList;
    }
  }

  /// 地址匹配逻辑（简化版本）
  bool _matchAddress(String address, String pattern) {
    return address == pattern ||
        address.startsWith(pattern) ||
        pattern.endsWith('*') &&
            address.startsWith(pattern.substring(0, pattern.length - 1));
  }

  /// 校验地址是否对 sandbox-exec 有效
  /// sandbox-exec 的 (remote/local ip "host:port") 只支持 * 或 localhost 作为 host
  static bool isValidSandboxAddress(String addr) {
    String host = addr;
    final lastColon = addr.lastIndexOf(':');
    if (lastColon >= 0) {
      host = addr.substring(0, lastColon);
    }
    final lowerHost = host.toLowerCase();
    return lowerHost == '*' ||
        lowerHost == 'localhost' ||
        lowerHost == '127.0.0.1';
  }
}

/// Shell命令权限设置
/// TODO: 后续实现Shell命令检查逻辑
class ShellPermissionConfig {
  final PermissionMode mode;
  final List<String> commands; // 命令或命令前缀

  ShellPermissionConfig({
    this.mode = PermissionMode.blacklist,
    this.commands = const [],
  });

  factory ShellPermissionConfig.fromJson(Map<String, dynamic> json) {
    return ShellPermissionConfig(
      mode: PermissionModeExtension.fromString(json['mode']),
      commands: (json['commands'] as List?)?.cast<String>() ?? [],
    );
  }

  Map<String, dynamic> toJson() {
    return {'mode': mode.name, 'commands': commands};
  }

  ShellPermissionConfig copyWith({
    PermissionMode? mode,
    List<String>? commands,
  }) {
    return ShellPermissionConfig(
      mode: mode ?? this.mode,
      commands: commands ?? this.commands,
    );
  }

  /// 检查Shell命令是否被允许执行
  /// TODO: 实现实际的命令检查逻辑
  bool isCommandAllowed(String command) {
    // 如果未配置任何命令,默认允许所有
    if (commands.isEmpty) return true;

    final normalizedCmd = command.trim().toLowerCase();
    final isInList = commands.any((c) {
      final normalizedPattern = c.trim().toLowerCase();
      return normalizedCmd.startsWith(normalizedPattern) ||
          normalizedCmd == normalizedPattern;
    });

    if (mode == PermissionMode.whitelist) {
      return isInList;
    } else {
      return !isInList;
    }
  }
}
