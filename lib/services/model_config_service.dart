import 'dart:convert';

import '../core_transport/transport_registry.dart';
import '../models/llm_config_model.dart';
import '../utils/app_logger.dart';
import 'model_config_database_service.dart';

/// Security model config service.
class SecurityModelConfigService {
  static final SecurityModelConfigService _instance =
      SecurityModelConfigService._internal();

  factory SecurityModelConfigService() => _instance;

  SecurityModelConfigService._internal();

  Future<SecurityModelConfig> loadConfig() async {
    try {
      final dbService = ModelConfigDatabaseService();
      final config = await dbService.getSecurityModelConfig();
      if (config != null) {
        return config;
      }
    } catch (e) {
      appLogger.error('[SecurityModelConfig] Failed to load config', e);
    }
    return SecurityModelConfig(
      provider: 'ollama',
      endpoint: 'http://localhost:11434',
      apiKey: '',
      model: 'llama3',
    );
  }

  Future<bool> saveConfig(SecurityModelConfig config) async {
    try {
      final dbService = ModelConfigDatabaseService();
      return await dbService.saveSecurityModelConfig(config);
    } catch (e) {
      appLogger.error('[SecurityModelConfig] Failed to save config', e);
      return false;
    }
  }

  Future<Map<String, dynamic>> testConnection(
    SecurityModelConfig config,
  ) async {
    final transport = TransportRegistry.transport;
    if (!transport.isReady) {
      return {'success': false, 'error': 'Transport not initialized'};
    }

    final request = {
      'provider': config.provider,
      'endpoint': config.endpoint,
      'api_key': config.apiKey,
      'model': config.model,
      if (config.secretKey.isNotEmpty) 'secret_key': config.secretKey,
    };

    try {
      return transport.callOneArg('TestModelConnectionFFI', jsonEncode(request));
    } catch (e) {
      appLogger.error('[SecurityModelConfig] Failed to test connection', e);
      return {'success': false, 'error': e.toString()};
    }
  }

  Future<bool> hasValidConfig() async {
    final dbService = ModelConfigDatabaseService();
    return await dbService.hasValidSecurityModelConfig();
  }
}

/// Bot model config service.
class BotModelConfigService {
  BotModelConfigService({required this.assetName, this.assetID = ''});

  final String assetName;
  final String assetID;

  Future<BotModelConfig?> loadConfig() async {
    try {
      final dbService = ModelConfigDatabaseService();
      return await dbService.getBotModelConfig(assetName, assetID);
    } catch (e) {
      appLogger.error('[BotModelConfig] Failed to load config', e);
      return null;
    }
  }

  Future<bool> saveConfig(BotModelConfig config) async {
    try {
      final dbService = ModelConfigDatabaseService();
      return await dbService.saveBotModelConfig(config);
    } catch (e) {
      appLogger.error('[BotModelConfig] Failed to save config', e);
      return false;
    }
  }

  Future<bool> deleteConfig() async {
    try {
      final dbService = ModelConfigDatabaseService();
      return await dbService.deleteBotModelConfig(assetName, assetID);
    } catch (e) {
      appLogger.error('[BotModelConfig] Failed to delete config', e);
      return false;
    }
  }

  Future<Map<String, dynamic>> testConnection(BotModelConfig config) async {
    final transport = TransportRegistry.transport;
    if (!transport.isReady) {
      return {'success': false, 'error': 'Transport not initialized'};
    }

    final request = {
      'provider': config.provider,
      'endpoint': config.baseUrl,
      'api_key': config.apiKey,
      'model': config.model,
      if (config.secretKey.isNotEmpty) 'secret_key': config.secretKey,
    };

    try {
      return transport.callOneArg('TestModelConnectionFFI', jsonEncode(request));
    } catch (e) {
      appLogger.error('[BotModelConfig] Failed to test connection', e);
      return {'success': false, 'error': e.toString()};
    }
  }

  Future<bool> hasValidConfig() async {
    final dbService = ModelConfigDatabaseService();
    return await dbService.hasValidBotModelConfig(assetName, assetID);
  }
}
