import 'dart:convert';
import '../core_transport/transport_registry.dart';

/// Provider scope for filtering.
enum ProviderScope {
  security('security'),
  bot('bot'),
  all('all');

  const ProviderScope(this.value);
  final String value;
}

/// Provider information from Go layer.
class ProviderInfo {
  final String name;
  final String displayName;
  final String icon;
  final String scope;
  final bool needsEndpoint;
  final bool needsAPIKey;
  final bool needsSecretKey;
  final String defaultBaseURL;
  final String defaultModel;
  final String apiKeyHint;
  final String modelHint;
  final String routingCanonical;
  final String group;
  final bool supportsModelList;
  final bool autoV1Suffix;

  const ProviderInfo({
    required this.name,
    required this.displayName,
    required this.icon,
    required this.scope,
    required this.needsEndpoint,
    required this.needsAPIKey,
    required this.needsSecretKey,
    required this.defaultBaseURL,
    required this.defaultModel,
    required this.apiKeyHint,
    required this.modelHint,
    this.routingCanonical = '',
    this.group = '',
    this.supportsModelList = false,
    this.autoV1Suffix = true,
  });

  factory ProviderInfo.fromJson(Map<String, dynamic> json) {
    return ProviderInfo(
      name: json['name'] ?? '',
      displayName: json['display_name'] ?? '',
      icon: json['icon'] ?? 'sparkles',
      scope: json['scope'] ?? 'all',
      needsEndpoint: json['needs_endpoint'] ?? false,
      needsAPIKey: json['needs_api_key'] ?? true,
      needsSecretKey: json['needs_secret_key'] ?? false,
      defaultBaseURL: json['default_base_url'] ?? '',
      defaultModel: json['default_model'] ?? '',
      apiKeyHint: json['api_key_hint'] ?? '',
      modelHint: json['model_hint'] ?? '',
      routingCanonical: json['routing_canonical'] ?? '',
      group: json['group'] ?? '',
      supportsModelList: json['supports_model_list'] == true,
      autoV1Suffix: json['auto_v1_suffix'] != false,
    );
  }
}

/// Service to get supported providers from Go layer via FFI.
class ProviderService {
  static final ProviderService _instance = ProviderService._internal();

  factory ProviderService() => _instance;

  ProviderService._internal();

  List<ProviderInfo>? _cachedSecurityProviders;
  List<ProviderInfo>? _cachedBotProviders;

  /// Get supported providers for a given scope.
  List<ProviderInfo> getProviders(ProviderScope scope) {
    // Check cache first
    if (scope == ProviderScope.security && _cachedSecurityProviders != null) {
      return _cachedSecurityProviders!;
    }
    if (scope == ProviderScope.bot && _cachedBotProviders != null) {
      return _cachedBotProviders!;
    }

    try {
      final transport = TransportRegistry.transport;
      if (!transport.isReady) {
        return [];
      }
      final jsonStr = transport.callRawOneArg(
        'GetSupportedProviders',
        scope.value,
      );

      final List<dynamic> jsonList = jsonDecode(jsonStr);
      final providers = jsonList.map((e) => ProviderInfo.fromJson(e)).toList();

      // Cache the result
      if (scope == ProviderScope.security) {
        _cachedSecurityProviders = providers;
      } else if (scope == ProviderScope.bot) {
        _cachedBotProviders = providers;
      }

      return providers;
    } catch (e) {
      // Return empty list on error
      return [];
    }
  }

  /// Clear cached providers (call when plugin is reloaded).
  void clearCache() {
    _cachedSecurityProviders = null;
    _cachedBotProviders = null;
  }
}
