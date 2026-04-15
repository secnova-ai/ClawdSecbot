import 'dart:convert';
import 'dart:ffi' as ffi;
import 'package:ffi/ffi.dart';
import '../models/protection_analysis_model.dart';
import '../models/protection_config_model.dart';
import '../utils/app_logger.dart';
import 'native_library_service.dart';

// FFI type definitions
typedef _OneArgC = ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);
typedef _OneArgDart = ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);

typedef _NoArgC = ffi.Pointer<Utf8> Function();
typedef _NoArgDart = ffi.Pointer<Utf8> Function();

/// 防护配置 FFI 持久化门面：通过 FFI 委托 Go 层进行数据持久化，Flutter 不直接操作 DB。
/// 管理 protection_state, protection_config, protection_statistics 等持久化能力。
class ProtectionDatabaseService {
  static final ProtectionDatabaseService _instance =
      ProtectionDatabaseService._internal();

  factory ProtectionDatabaseService() => _instance;

  ProtectionDatabaseService._internal();

  ffi.DynamicLibrary? get _dylib => NativeLibraryService().dylib;
  FreeStringDart? get _freeString => NativeLibraryService().freeString;

  String? _lastLoggedEnabledConfigsSummary;

  // --- Protection State methods ---

  /// Save protection state
  Future<void> saveProtectionState({
    required bool enabled,
    String? providerName,
    int? proxyPort,
    String? originalBaseUrl,
  }) async {
    final result = _callFFI(
      'SaveProtectionStateFFI',
      jsonEncode({
        'enabled': enabled,
        'provider_name': providerName ?? '',
        'proxy_port': proxyPort ?? 0,
        'original_base_url': originalBaseUrl ?? '',
      }),
    );

    if (result['success'] != true) {
      throw Exception('Failed to save protection state: ${result['error']}');
    }
    appLogger.info(
      '[ProtectionDB] Protection state saved: enabled=$enabled, provider=$providerName, port=$proxyPort',
    );
  }

  /// Get protection state
  Future<Map<String, dynamic>?> getProtectionState() async {
    final result = _callFFINoArg('GetProtectionStateFFI');
    if (result['success'] != true) return null;

    final data = result['data'];
    if (data == null) return null;

    return {
      'enabled': data['enabled'] as bool? ?? false,
      'provider_name': data['provider_name'],
      'proxy_port': data['proxy_port'],
      'original_base_url': data['original_base_url'],
      'updated_at': data['updated_at'],
    };
  }

  /// Get protection state
  Future<Map<String, dynamic>?> getProtectionStateAsync() async {
    return getProtectionState();
  }

  /// Clear protection state (set enabled to false)
  Future<void> clearProtectionState() async {
    _callFFINoArg('ClearProtectionStateFFI');
  }

  // --- Protection Config methods ---

  /// Save or update protection config for an asset
  Future<void> saveProtectionConfig(ProtectionConfig config) async {
    final result = _callFFI(
      'SaveProtectionConfigFFI',
      jsonEncode({
        'asset_name': config.assetName,
        'asset_id': config.assetID,
        'enabled': config.enabled,
        'audit_only': config.auditOnly,
        'sandbox_enabled': config.sandboxEnabled,
        'gateway_binary_path': config.gatewayBinaryPath ?? '',
        'gateway_config_path': config.gatewayConfigPath ?? '',
        'single_session_token_limit': config.singleSessionTokenLimit,
        'daily_token_limit': config.dailyTokenLimit,
        'path_permission': jsonEncode(config.pathPermission.toJson()),
        'network_permission': jsonEncode(config.networkPermission.toJson()),
        'shell_permission': jsonEncode(config.shellPermission.toJson()),
        'created_at': config.createdAt?.toIso8601String() ?? '',
      }),
    );

    if (result['success'] != true) {
      throw Exception('Failed to save protection config: ${result['error']}');
    }
    appLogger.info(
      '[ProtectionDB] Protection config saved: asset=${config.assetName}, enabled=${config.enabled}',
    );
  }

  /// Get protection config for an asset
  Future<ProtectionConfig?> getProtectionConfig(
    String assetName, [
    String assetID = '',
  ]) async {
    final result = _callFFIOneArg('GetProtectionConfigFFI', assetID);
    if (result['success'] != true) return null;

    final data = result['data'];
    if (data == null) return null;

    return _mapToProtectionConfig(data as Map<String, dynamic>);
  }

  /// Get all enabled protection configs
  Future<List<ProtectionConfig>> getEnabledProtectionConfigs() async {
    final result = _callFFINoArg('GetEnabledProtectionConfigsFFI');
    if (result['success'] != true) return [];

    final data = result['data'];
    if (data == null || data is! List) return [];

    final configs = <ProtectionConfig>[];
    for (final item in data) {
      try {
        configs.add(
          _mapToProtectionConfig(Map<String, dynamic>.from(item as Map)),
        );
      } catch (e) {
        appLogger.warning('[ProtectionDB] Failed to parse config: $e');
      }
    }
    final enabledConfigsSummary = configs
        .map(
          (config) =>
              '${config.assetName}|${config.assetID}|${config.enabled ? 1 : 0}',
        )
        .join(';');
    if (_lastLoggedEnabledConfigsSummary != enabledConfigsSummary) {
      _lastLoggedEnabledConfigsSummary = enabledConfigsSummary;
      appLogger.info(
        '[ProtectionDB] Enabled protection configs count: ${configs.length}',
      );
    }
    return configs;
  }

  /// Get active protection count (获取正在防护中的资产数量)
  Future<int> getActiveProtectionCount() async {
    final result = _callFFINoArg('GetActiveProtectionCountFFI');
    if (result['success'] != true) return 0;

    final data = result['data'];
    if (data == null) return 0;

    return (data['count'] as int?) ?? 0;
  }

  /// Set protection enabled status for an asset
  Future<void> setProtectionEnabled(
    String assetName,
    bool enabled, [
    String assetID = '',
  ]) async {
    _callFFI(
      'SetProtectionEnabledFFI',
      jsonEncode({
        'asset_name': assetName,
        'asset_id': assetID,
        'enabled': enabled,
      }),
    );
  }

  /// Delete protection config for an asset
  Future<void> deleteProtectionConfig(
    String assetName, [
    String assetID = '',
  ]) async {
    _callFFIOneArg('DeleteProtectionConfigFFI', assetID);
  }

  // --- Protection Statistics methods ---

  /// Save or update protection statistics for an asset
  Future<void> saveProtectionStatistics({
    required String assetName,
    String assetID = '',
    required int analysisCount,
    required int messageCount,
    required int warningCount,
    required int blockedCount,
    required int totalTokens,
    required int totalPromptTokens,
    required int totalCompletionTokens,
    required int totalToolCalls,
    required int requestCount,
    int auditTokens = 0,
    int auditPromptTokens = 0,
    int auditCompletionTokens = 0,
  }) async {
    final result = _callFFI(
      'SaveProtectionStatisticsFFI',
      jsonEncode({
        'asset_name': assetName,
        'asset_id': assetID,
        'analysis_count': analysisCount,
        'message_count': messageCount,
        'warning_count': warningCount,
        'blocked_count': blockedCount,
        'total_tokens': totalTokens,
        'total_prompt_tokens': totalPromptTokens,
        'total_completion_tokens': totalCompletionTokens,
        'total_tool_calls': totalToolCalls,
        'request_count': requestCount,
        'audit_tokens': auditTokens,
        'audit_prompt_tokens': auditPromptTokens,
        'audit_completion_tokens': auditCompletionTokens,
      }),
    );

    if (result['success'] != true) {
      throw Exception(
        'Failed to save protection statistics: ${result['error']}',
      );
    }
  }

  /// Get protection statistics for an asset
  Future<ProtectionStatistics?> getProtectionStatistics(
    String assetName, [
    String assetID = '',
  ]) async {
    final result = _callFFIOneArg('GetProtectionStatisticsFFI', assetID);
    if (result['success'] != true) return null;

    final data = result['data'];
    if (data == null) return null;

    final map = data as Map<String, dynamic>;
    return ProtectionStatistics(
      assetName: map['asset_name'] as String? ?? assetName,
      analysisCount: map['analysis_count'] as int? ?? 0,
      messageCount: map['message_count'] as int? ?? 0,
      warningCount: map['warning_count'] as int? ?? 0,
      blockedCount: map['blocked_count'] as int? ?? 0,
      totalTokens: map['total_tokens'] as int? ?? 0,
      totalPromptTokens: map['total_prompt_tokens'] as int? ?? 0,
      totalCompletionTokens: map['total_completion_tokens'] as int? ?? 0,
      totalToolCalls: map['total_tool_calls'] as int? ?? 0,
      requestCount: map['request_count'] as int? ?? 0,
      auditTokens: map['audit_tokens'] as int? ?? 0,
      auditPromptTokens: map['audit_prompt_tokens'] as int? ?? 0,
      auditCompletionTokens: map['audit_completion_tokens'] as int? ?? 0,
      updatedAt: map['updated_at'] != null
          ? DateTime.parse(map['updated_at'] as String)
          : DateTime.now(),
    );
  }

  /// Clear protection statistics for an asset
  Future<void> clearProtectionStatistics(
    String assetName, [
    String assetID = '',
  ]) async {
    _callFFIOneArg('ClearProtectionStatisticsFFI', assetID);
  }

  // --- Shepherd Rules methods ---

  Future<List<String>> getShepherdSensitiveActions(
    String assetName,
    String assetID,
  ) async {
    final result = _callFFIOneArg('GetShepherdSensitiveActionsFFI', assetID);
    if (result['success'] != true) return [];

    final data = result['data'];
    if (data == null || data is! List) return [];

    return data.cast<String>();
  }

  Future<List<String>> getShepherdSensitiveActionsByAsset(
    String assetName, [
    String assetID = '',
  ]) async {
    return getShepherdSensitiveActions(assetName, assetID);
  }

  Future<void> saveShepherdSensitiveActions(
    String assetName,
    String assetID,
    List<String> actions,
  ) async {
    final result = _callFFI(
      'SaveShepherdSensitiveActionsFFI',
      jsonEncode({
        'asset_name': assetName,
        'asset_id': assetID,
        'actions': actions,
      }),
    );

    if (result['success'] != true) {
      throw Exception('Failed to save shepherd rules: ${result['error']}');
    }
  }

  // --- Helper methods ---

  /// 调用无参数FFI函数
  Map<String, dynamic> _callFFINoArg(String funcName) {
    final dylib = _dylib;
    if (dylib == null || _freeString == null) {
      return {'success': false, 'error': 'Native library not initialized'};
    }

    try {
      final func = dylib.lookupFunction<_NoArgC, _NoArgDart>(funcName);
      final resultPtr = func();
      final result = resultPtr.toDartString();
      _freeString!(resultPtr);
      return jsonDecode(result) as Map<String, dynamic>;
    } catch (e) {
      appLogger.error('[ProtectionDB] $funcName failed: $e');
      return {'success': false, 'error': '$funcName failed: $e'};
    }
  }

  /// 调用接收单个字符串参数的FFI函数
  Map<String, dynamic> _callFFIOneArg(String funcName, String arg) {
    final dylib = _dylib;
    if (dylib == null || _freeString == null) {
      return {'success': false, 'error': 'Native library not initialized'};
    }

    try {
      final func = dylib.lookupFunction<_OneArgC, _OneArgDart>(funcName);
      final argPtr = arg.toNativeUtf8();
      final resultPtr = func(argPtr);
      final result = resultPtr.toDartString();
      _freeString!(resultPtr);
      malloc.free(argPtr);
      return jsonDecode(result) as Map<String, dynamic>;
    } catch (e) {
      appLogger.error('[ProtectionDB] $funcName failed: $e');
      return {'success': false, 'error': '$funcName failed: $e'};
    }
  }

  /// 调用接收JSON字符串参数的FFI函数
  Map<String, dynamic> _callFFI(String funcName, String jsonStr) {
    return _callFFIOneArg(funcName, jsonStr);
  }

  /// 将Go层返回的map转换为ProtectionConfig
  ProtectionConfig _mapToProtectionConfig(Map<String, dynamic> map) {
    return ProtectionConfig(
      assetName: map['asset_name'] as String? ?? '',
      assetID: map['asset_id'] as String? ?? '',
      enabled: map['enabled'] as bool? ?? false,
      auditOnly: map['audit_only'] as bool? ?? false,
      sandboxEnabled: map['sandbox_enabled'] as bool? ?? false,
      gatewayBinaryPath: map['gateway_binary_path'] as String?,
      gatewayConfigPath: map['gateway_config_path'] as String?,
      singleSessionTokenLimit: map['single_session_token_limit'] as int? ?? 0,
      dailyTokenLimit: map['daily_token_limit'] as int? ?? 0,
      pathPermission:
          map['path_permission'] != null &&
              map['path_permission'] is String &&
              (map['path_permission'] as String).isNotEmpty
          ? PathPermissionConfig.fromJson(
              jsonDecode(map['path_permission'] as String),
            )
          : PathPermissionConfig(),
      networkPermission:
          map['network_permission'] != null &&
              map['network_permission'] is String &&
              (map['network_permission'] as String).isNotEmpty
          ? NetworkPermissionConfig.fromJson(
              jsonDecode(map['network_permission'] as String),
            )
          : NetworkPermissionConfig(),
      shellPermission:
          map['shell_permission'] != null &&
              map['shell_permission'] is String &&
              (map['shell_permission'] as String).isNotEmpty
          ? ShellPermissionConfig.fromJson(
              jsonDecode(map['shell_permission'] as String),
            )
          : ShellPermissionConfig(),
      createdAt:
          map['created_at'] != null && (map['created_at'] as String).isNotEmpty
          ? DateTime.parse(map['created_at'] as String)
          : null,
      updatedAt:
          map['updated_at'] != null && (map['updated_at'] as String).isNotEmpty
          ? DateTime.parse(map['updated_at'] as String)
          : null,
    );
  }
}
