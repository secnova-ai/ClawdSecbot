import 'dart:convert';

import 'package:flutter/material.dart';

import '../core_transport/transport_registry.dart';
import '../models/asset_model.dart';
import '../models/risk_model.dart';
import '../utils/app_logger.dart';
import 'protection_database_service.dart';

/// Web plugin service implemented by transport-based RPC calls.
class PluginService {
  static final PluginService _instance = PluginService._internal();

  factory PluginService() => _instance;

  PluginService._internal();

  static bool _initialized = false;

  bool get isInitialized => _initialized;

  Future<void> initializePlugin() async {
    if (_initialized) return;
    final transport = TransportRegistry.transport;
    if (!transport.isReady) {
      appLogger.warning('[PluginWeb] Transport not initialized');
      return;
    }
    _initialized = true;
    appLogger.info('[PluginWeb] Plugin service initialized');
  }

  Future<void> closePlugin() async {
    _initialized = false;
    appLogger.info('[PluginWeb] Plugin service closed');
  }

  Future<bool> requiresBotModelConfig(String assetName) async {
    final normalizedAssetName = assetName.trim().toLowerCase();
    if (normalizedAssetName.isEmpty) {
      return true;
    }

    final response = _callNoArg('GetPluginsFFI');
    if (response['success'] != true) {
      return true;
    }
    final data = response['data'];
    if (data is! List) {
      return true;
    }

    for (final item in data.whereType<Map<String, dynamic>>()) {
      final pluginAssetName = (item['asset_name'] as String? ?? '')
          .trim()
          .toLowerCase();
      if (pluginAssetName != normalizedAssetName) {
        continue;
      }
      final requires = item['requires_bot_model_config'];
      if (requires is bool) {
        return requires;
      }
      return true;
    }

    return true;
  }

  Future<Map<String, List<String>>> loadAndSyncShepherdRules(
    String assetName,
    String assetID,
  ) async {
    final sensitiveActions = await ProtectionDatabaseService()
        .getShepherdSensitiveActions(assetName, assetID);
    return {'sensitiveActions': sensitiveActions};
  }

  Future<List<Map<String, dynamic>>> getRegisteredPlugins() async {
    final response = _callNoArg('GetPluginsFFI');
    if (response['success'] != true || response['data'] is! List) {
      return const [];
    }
    return (response['data'] as List<dynamic>)
        .whereType<Map<String, dynamic>>()
        .toList(growable: false);
  }

  Future<List<String>> getScannedSkillHashesList() async {
    final response = _callNoArg('GetScannedSkillHashes');
    if (response['success'] != true || response['data'] is! List) {
      return const [];
    }
    return (response['data'] as List<dynamic>)
        .map((item) => item.toString())
        .toList(growable: false);
  }

  Future<List<Asset>> scanAssetsByPlugin(String assetName) async {
    final normalized = assetName.trim();
    if (normalized.isEmpty) {
      return const [];
    }
    final response = _callOneArg('ScanAssetsByPluginFFI', normalized);
    if (response['success'] != true || response['data'] is! List) {
      return const [];
    }
    return (response['data'] as List<dynamic>)
        .map((item) => Asset.fromJson(item as Map<String, dynamic>))
        .toList(growable: false);
  }

  Future<List<RiskInfo>> assessRisksByPlugin(
    String assetName,
    List<String> scannedHashes,
  ) async {
    final normalized = assetName.trim();
    if (normalized.isEmpty) {
      return const [];
    }
    final response = _callTwoArgs(
      'AssessRisksByPluginFFI',
      normalized,
      jsonEncode(scannedHashes),
    );
    if (response['success'] != true || response['data'] is! List) {
      return const [];
    }
    return (response['data'] as List<dynamic>)
        .map((item) => _parseRisk(item as Map<String, dynamic>))
        .toList(growable: false);
  }

  List<Map<String, dynamic>> listBundledReActSkills() {
    final result = _callNoArg('ListBundledReActSkillsFFI');
    if (result['success'] != true) {
      return const [];
    }
    final data = result['data'];
    if (data is! List) {
      return const [];
    }
    return data.whereType<Map<String, dynamic>>().toList(growable: false);
  }

  Future<void> updateShepherdRules(
    String assetName,
    String assetID,
    List<String> sensitiveActions,
  ) async {
    await ProtectionDatabaseService().saveShepherdSensitiveActions(
      assetName,
      assetID,
      sensitiveActions,
    );
  }

  Future<Map<String, dynamic>> notifyPluginAppExit(
    String assetName, [
    String assetID = '',
  ]) async {
    return _callTwoArgs('NotifyPluginAppExitFFI', assetName, assetID);
  }

  Future<Map<String, dynamic>> restoreBotDefaultState(
    String assetName, [
    String assetID = '',
  ]) async {
    return _callTwoArgs('RestoreBotDefaultStateFFI', assetName, assetID);
  }

  Future<Map<String, dynamic>> saveSecurityModelConfig(
    Map<String, dynamic> config,
  ) async {
    return _callOneArg('SaveSecurityModelConfigFFI', jsonEncode(config));
  }

  Future<Map<String, dynamic>> getSecurityModelConfig() async {
    return _callNoArg('GetSecurityModelConfigFFI');
  }

  Future<Map<String, dynamic>> saveBotModelConfig(
    Map<String, dynamic> config,
  ) async {
    return _callOneArg('SaveBotModelConfigFFI', jsonEncode(config));
  }

  Future<Map<String, dynamic>> getBotModelConfig(
    String assetName, [
    String assetID = '',
  ]) async {
    return _callOneArg('GetBotModelConfigFFI', assetID);
  }

  Future<Map<String, dynamic>> deleteBotModelConfig(
    String assetName, [
    String assetID = '',
  ]) async {
    return _callOneArg('DeleteBotModelConfigFFI', assetID);
  }

  Future<ScanResult> scan() async {
    final transport = TransportRegistry.transport;
    if (!transport.isReady) {
      appLogger.warning('[PluginWeb] Transport not initialized, cannot scan');
      return ScanResult(risks: [], configFound: false);
    }

    final allAssets = <Asset>[];
    final allRisks = <RiskInfo>[];

    try {
      final scanResp = _callNoArg('ScanAssetsFFI');
      if (scanResp['success'] == true && scanResp['data'] is List) {
        final list = scanResp['data'] as List<dynamic>;
        allAssets.addAll(
          list.map((e) => Asset.fromJson(e as Map<String, dynamic>)),
        );
      }

      var scannedHashesJson = '[]';
      final hashesResp = _callNoArg('GetScannedSkillHashes');
      if (hashesResp['success'] == true && hashesResp['data'] != null) {
        scannedHashesJson = jsonEncode(hashesResp['data']);
      }

      final risksResp = _callOneArg('AssessRisksFFI', scannedHashesJson);
      if (risksResp['success'] == true && risksResp['data'] is List) {
        final list = risksResp['data'] as List<dynamic>;
        for (final item in list) {
          allRisks.add(_parseRisk(item as Map<String, dynamic>));
        }
      }
    } catch (e) {
      appLogger.error('[PluginWeb] Scan error', e);
    }

    final configPath = allAssets
        .map((a) => a.metadata['config_path'] ?? '')
        .firstWhere((p) => p.isNotEmpty, orElse: () => '');

    return ScanResult(
      assets: allAssets,
      risks: allRisks,
      configFound: configPath.isNotEmpty,
      configPath: configPath.isNotEmpty ? configPath : null,
      config: null,
    );
  }

  Future<Map<String, dynamic>> mitigateRisk(
    RiskInfo risk,
    Map<String, dynamic> formData,
  ) async {
    final assetID = _resolveMitigationAssetID(risk);
    if (assetID == null) {
      return {
        'success': false,
        'error': 'asset_id is required for mitigation routing',
      };
    }
    final sourcePlugin = _resolveMitigationSourcePlugin(risk);
    final req = {
      'id': risk.id,
      'args': risk.args,
      'form_data': formData,
      'asset_id': assetID,
      'source_plugin': sourcePlugin,
    };
    return _callOneArg('MitigateRiskFFI', jsonEncode(req));
  }

  RiskInfo _parseRisk(Map<String, dynamic> json) {
    final args = json['args'] != null
        ? Map<String, Object>.from(json['args'])
        : null;
    final assetID = _resolveAssetID(json['asset_id'] as String?, args);
    final sourcePlugin =
        _normalizeSourcePlugin(json['source_plugin'] as String?) ??
        _resolveSourcePluginFromArgs(args);
    return RiskInfo(
      id: json['id'] ?? 'unknown',
      title: json['title'] ?? 'Unknown Risk',
      titleEn: json['title_en'] as String?,
      description: json['description'] ?? '',
      descriptionEn: json['description_en'] as String?,
      level: _parseRiskLevel(json['level']),
      icon: _getIconForRisk(json['level']),
      args: args,
      assetID: assetID,
      mitigation: json['mitigation'] != null
          ? Mitigation.fromJson(json['mitigation'])
          : null,
      sourcePlugin: sourcePlugin,
    );
  }

  String? _resolveMitigationAssetID(RiskInfo risk) {
    return _normalizeAssetID(risk.assetID) ??
        _resolveAssetIDFromArgs(risk.args);
  }

  String? _resolveMitigationSourcePlugin(RiskInfo risk) {
    return _normalizeSourcePlugin(risk.sourcePlugin) ??
        _resolveSourcePluginFromArgs(risk.args);
  }

  String? _resolveAssetID(String? assetID, Map<String, Object>? args) {
    return _normalizeAssetID(assetID) ?? _resolveAssetIDFromArgs(args);
  }

  String? _resolveAssetIDFromArgs(Map<String, Object>? args) {
    return _normalizeAssetID(args?['asset_id']?.toString());
  }

  String? _resolveSourcePluginFromArgs(Map<String, Object>? args) {
    final fromArgs = args?['source_plugin'];
    final normalized = fromArgs?.toString().trim();
    if (normalized == null || normalized.isEmpty) {
      return null;
    }
    return normalized;
  }

  String? _normalizeSourcePlugin(String? sourcePlugin) {
    final normalized = sourcePlugin?.trim();
    if (normalized == null || normalized.isEmpty) return null;
    return normalized;
  }

  String? _normalizeAssetID(String? assetID) {
    final normalized = assetID?.trim();
    if (normalized == null || normalized.isEmpty) return null;
    return normalized;
  }

  RiskLevel _parseRiskLevel(String? level) {
    switch (level?.toLowerCase()) {
      case 'critical':
        return RiskLevel.critical;
      case 'high':
        return RiskLevel.high;
      case 'medium':
        return RiskLevel.medium;
      case 'low':
        return RiskLevel.low;
      default:
        return RiskLevel.low;
    }
  }

  IconData _getIconForRisk(String? level) {
    switch (level?.toLowerCase()) {
      case 'critical':
        return Icons.gpp_bad;
      case 'high':
        return Icons.warning;
      case 'medium':
        return Icons.info;
      default:
        return Icons.check_circle;
    }
  }

  Map<String, dynamic> _callNoArg(String method) {
    final transport = TransportRegistry.transport;
    if (!transport.isReady) {
      return {'success': false, 'error': 'Transport not initialized'};
    }
    try {
      return transport.callNoArg(method);
    } catch (e) {
      appLogger.error('[PluginWeb] $method failed', e);
      return {'success': false, 'error': '$method failed: $e'};
    }
  }

  Map<String, dynamic> _callOneArg(String method, String arg) {
    final transport = TransportRegistry.transport;
    if (!transport.isReady) {
      return {'success': false, 'error': 'Transport not initialized'};
    }
    try {
      return transport.callOneArg(method, arg);
    } catch (e) {
      appLogger.error('[PluginWeb] $method failed', e);
      return {'success': false, 'error': '$method failed: $e'};
    }
  }

  Map<String, dynamic> _callTwoArgs(String method, String a, String b) {
    final transport = TransportRegistry.transport;
    if (!transport.isReady) {
      return {'success': false, 'error': 'Transport not initialized'};
    }
    try {
      return transport.callTwoArgs(method, a, b);
    } catch (e) {
      appLogger.error('[PluginWeb] $method failed', e);
      return {'success': false, 'error': '$method failed: $e'};
    }
  }
}
