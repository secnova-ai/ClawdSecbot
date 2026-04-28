import 'dart:convert';

import 'package:flutter/material.dart';

import '../core_transport/transport_registry.dart';
import '../models/asset_model.dart';
import '../models/risk_model.dart';
import '../utils/app_logger.dart';

/// Scan DB facade delegated to Go layer through transport.
class ScanDatabaseService {
  static final ScanDatabaseService _instance = ScanDatabaseService._internal();
  String? _lastLoadedScanSignature;

  factory ScanDatabaseService() => _instance;

  ScanDatabaseService._internal();

  Future<void> saveScanResult(ScanResult result) async {
    final payload = jsonEncode({
      'config_found': result.configFound,
      'config_path': result.configPath,
      'config_json': result.config != null ? jsonEncode(result.config) : null,
      'assets': result.assets.map((a) => a.toJson()).toList(),
      'risks': result.risks.map((r) => r.toJson()).toList(),
      'created_at': result.scannedAt?.toUtc().toIso8601String(),
    });

    final response = _callOneArg('SaveScanResult', payload);
    if (response['success'] == true) {
      appLogger.info(
        '[ScanDB] Scan result saved via Go layer, id=${response['scan_id']}',
      );
      return;
    }
    throw Exception('Go save failed: ${response['error']}');
  }

  Future<ScanResult?> getLatestScanResult() async {
    final response = _callNoArg('GetLatestScanResult');
    if (response['success'] != true) {
      return null;
    }
    if (response['data'] == null) {
      return null;
    }

    try {
      final data = response['data'] as Map<String, dynamic>;
      final configFound = data['config_found'] as bool? ?? false;
      final configPath = data['config_path'] as String?;
      final configJSON = data['config_json'] as String?;
      final scannedAtRaw = data['created_at'] as String?;

      final assets =
          (data['assets'] as List?)
              ?.map((a) => Asset.fromJson(a as Map<String, dynamic>))
              .toList() ??
          [];

      final risks = <RiskInfo>[];
      for (final r in (data['risks'] as List?) ?? const []) {
        final riskMap = r as Map<String, dynamic>;
        final args = riskMap['args'] != null
            ? Map<String, Object>.from(riskMap['args'])
            : null;
        final sourcePlugin = _resolveSourcePlugin(
          riskMap['source_plugin'] as String?,
          args,
        );
        final assetID = _resolveAssetID(riskMap['asset_id'] as String?, args);
        risks.add(
          RiskInfo(
            id: riskMap['id'] ?? 'unknown',
            title: riskMap['title'] ?? 'Unknown Risk',
            titleEn: riskMap['title_en'] as String?,
            description: riskMap['description'] ?? '',
            descriptionEn: riskMap['description_en'] as String?,
            level: _parseRiskLevel(riskMap['level']),
            icon: _getIconForRisk(riskMap['level']),
            args: args,
            assetID: assetID,
            mitigation: riskMap['mitigation'] != null
                ? Mitigation.fromJson(riskMap['mitigation'])
                : null,
            sourcePlugin: sourcePlugin,
          ),
        );
      }

      _logLoadedLatestScanIfChanged(
        assetCount: assets.length,
        riskCount: risks.length,
      );
      return ScanResult(
        config: configJSON != null
            ? jsonDecode(configJSON) as Map<String, dynamic>
            : null,
        risks: risks,
        configFound: configFound,
        configPath: configPath,
        assets: assets,
        scannedAt: scannedAtRaw != null
            ? DateTime.tryParse(scannedAtRaw)
            : null,
      );
    } catch (e) {
      appLogger.error('[ScanDB] Failed to parse latest scan result', e);
      return null;
    }
  }

  String? _resolveSourcePlugin(
    String? sourcePlugin,
    Map<String, Object>? args,
  ) {
    final normalized = sourcePlugin?.trim();
    if (normalized != null && normalized.isNotEmpty) {
      return normalized;
    }
    final fromArgs = args?['source_plugin'];
    final fallback = fromArgs?.toString().trim();
    if (fallback == null || fallback.isEmpty) {
      return null;
    }
    return fallback;
  }

  String? _resolveAssetID(String? assetID, Map<String, Object>? args) {
    final normalized = assetID?.trim();
    if (normalized != null && normalized.isNotEmpty) {
      return normalized;
    }
    final fromArgs = args?['asset_id'];
    final fallback = fromArgs?.toString().trim();
    if (fallback == null || fallback.isEmpty) {
      return null;
    }
    return fallback;
  }

  /// 仅在扫描结果发生变化时记录加载日志，避免重复日志占用磁盘空间。
  void _logLoadedLatestScanIfChanged({
    required int assetCount,
    required int riskCount,
  }) {
    final signature = _buildLatestScanSignature(
      assetCount: assetCount,
      riskCount: riskCount,
    );
    if (_lastLoadedScanSignature == signature) {
      return;
    }
    _lastLoadedScanSignature = signature;
    appLogger.info(
      '[ScanDB] Loaded latest scan via Go layer: $assetCount assets, $riskCount risks',
    );
  }

  /// 构造最新扫描结果签名，用于判断数据是否变化。
  String _buildLatestScanSignature({
    required int assetCount,
    required int riskCount,
  }) {
    return '$assetCount|$riskCount';
  }

  Future<Set<String>> getScannedSkillHashes() async {
    final response = _callNoArg('GetScannedSkillHashes');
    if (response['success'] == true && response['data'] is List) {
      return (response['data'] as List).map((e) => e.toString()).toSet();
    }
    return {};
  }

  Future<void> saveSkillScanResult({
    required String skillName,
    required String skillHash,
    required bool safe,
    List<String>? issues,
  }) async {
    final response = _callOneArg(
      'SaveSkillScanResult',
      jsonEncode({
        'skill_name': skillName,
        'skill_hash': skillHash,
        'safe': safe,
        'issues': issues,
      }),
    );

    if (response['success'] == true) {
      appLogger.info('[ScanDB] Skill scan saved via Go layer: $skillName');
      return;
    }
    throw Exception('Failed to save skill scan: ${response['error']}');
  }

  Future<Map<String, dynamic>?> getSkillScanByHash(String hash) async {
    final response = _callOneArg('GetSkillScanByHash', hash);
    if (response['success'] != true || response['data'] == null) {
      return null;
    }
    final data = response['data'] as Map<String, dynamic>;
    return {
      'skill_name': data['skill_name'],
      'skill_hash': data['skill_hash'],
      'scanned_at': data['scanned_at'],
      'safe': data['safe'] as bool? ?? false,
      'issues':
          (data['issues'] as List?)?.map((e) => e.toString()).toList() ??
          <String>[],
    };
  }

  Future<void> deleteSkillScan(String skillName) async {
    final response = _callOneArg('DeleteSkillScanFFI', skillName);
    if (response['success'] == true) {
      appLogger.info('[ScanDB] Skill scan deleted via Go layer: $skillName');
    }
  }

  Future<List<Map<String, dynamic>>> getRiskySkills() async {
    final response = _callNoArg('GetRiskySkills');
    if (response['success'] != true || response['data'] is! List) {
      return [];
    }

    return (response['data'] as List).map((item) {
      final data = item as Map<String, dynamic>;
      final rawIssues =
          (data['issues'] as List?)?.map((e) => e.toString()).toList() ??
          <String>[];
      return {
        'skill_name': data['skill_name'],
        'skill_hash': data['skill_hash'],
        'skill_path': data['skill_path'] ?? '',
        'source_plugin': data['source_plugin'] ?? '',
        'asset_id': data['asset_id'] ?? '',
        'scanned_at': data['scanned_at'],
        'safe': data['safe'] as bool? ?? false,
        'risk_level': data['risk_level'] as String? ?? '',
        'issues': rawIssues.map(_formatSkillIssue).toList(),
        'issue_details': _parseSkillIssueDetails(rawIssues),
      };
    }).toList();
  }

  Future<List<Map<String, dynamic>>> getAllSkillScans() async {
    final response = _callNoArg('GetAllSkillScansFFI');
    if (response['success'] != true || response['data'] is! List) {
      return [];
    }

    return (response['data'] as List).map((item) {
      final data = item as Map<String, dynamic>;
      final rawIssues =
          (data['issues'] as List?)?.map((e) => e.toString()).toList() ??
          <String>[];
      return {
        'skill_name': data['skill_name'],
        'skill_hash': data['skill_hash'],
        'skill_path': data['skill_path'] ?? '',
        'source_plugin': data['source_plugin'] ?? '',
        'asset_id': data['asset_id'] ?? '',
        'scanned_at': data['scanned_at'],
        'safe': data['safe'] as bool? ?? false,
        'risk_level': data['risk_level'] as String? ?? '',
        'trusted': data['trusted'] as bool? ?? false,
        'issues': rawIssues.map(_formatSkillIssue).toList(),
        'issue_details': _parseSkillIssueDetails(rawIssues),
      };
    }).toList();
  }

  String _formatSkillIssue(String issue) {
    try {
      final parsed = jsonDecode(issue);
      if (parsed is Map<String, dynamic>) {
        final description = parsed['description']?.toString().trim();
        if (description != null && description.isNotEmpty) {
          return description;
        }
      }
    } catch (_) {
      // Keep legacy plain issue strings as-is.
    }
    return issue;
  }

  List<Map<String, String>> _parseSkillIssueDetails(List<String> issues) {
    return issues.map((issue) {
      try {
        final parsed = jsonDecode(issue);
        if (parsed is Map<String, dynamic>) {
          return parsed.map(
            (key, value) => MapEntry(key, value?.toString() ?? ''),
          );
        }
      } catch (_) {
        // Legacy plain issue strings do not have structured details.
      }
      return <String, String>{};
    }).toList();
  }

  Map<String, dynamic> _callNoArg(String funcName) {
    final transport = TransportRegistry.transport;
    if (!transport.isReady) {
      return {'success': false, 'error': 'Transport not initialized'};
    }
    try {
      return transport.callNoArg(funcName);
    } catch (e) {
      appLogger.error('[ScanDB] $funcName failed', e);
      return {'success': false, 'error': '$funcName failed: $e'};
    }
  }

  Map<String, dynamic> _callOneArg(String funcName, String arg) {
    final transport = TransportRegistry.transport;
    if (!transport.isReady) {
      return {'success': false, 'error': 'Transport not initialized'};
    }
    try {
      return transport.callOneArg(funcName, arg);
    } catch (e) {
      appLogger.error('[ScanDB] $funcName failed', e);
      return {'success': false, 'error': '$funcName failed: $e'};
    }
  }

  RiskLevel _parseRiskLevel(dynamic level) {
    if (level is String) {
      switch (level.toLowerCase()) {
        case 'critical':
          return RiskLevel.critical;
        case 'high':
          return RiskLevel.high;
        case 'medium':
          return RiskLevel.medium;
        case 'low':
          return RiskLevel.low;
      }
    }
    return RiskLevel.low;
  }

  IconData _getIconForRisk(dynamic level) {
    if (level is String) {
      switch (level.toLowerCase()) {
        case 'critical':
          return Icons.gpp_bad;
        case 'high':
          return Icons.warning;
        case 'medium':
          return Icons.info;
      }
    }
    return Icons.check_circle;
  }
}
