import 'dart:convert';
import 'dart:ffi' as ffi;
import 'package:ffi/ffi.dart';
import 'package:flutter/material.dart';
import '../models/asset_model.dart';
import '../models/risk_model.dart';
import '../utils/app_logger.dart';
import 'native_library_service.dart';

// FFI type definitions for Go DB operations
typedef _SaveScanResultC = ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);
typedef _SaveScanResultDart = ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);

typedef _GetLatestScanResultC = ffi.Pointer<Utf8> Function();
typedef _GetLatestScanResultDart = ffi.Pointer<Utf8> Function();

typedef _GetScannedSkillHashesC = ffi.Pointer<Utf8> Function();
typedef _GetScannedSkillHashesDart = ffi.Pointer<Utf8> Function();

typedef _SaveSkillScanResultC = ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);
typedef _SaveSkillScanResultDart =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);

typedef _GetSkillScanByHashC = ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);
typedef _GetSkillScanByHashDart = ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);

typedef _DeleteSkillScanC = ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);
typedef _DeleteSkillScanDart = ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);

typedef _GetRiskySkillsC = ffi.Pointer<Utf8> Function();
typedef _GetRiskySkillsDart = ffi.Pointer<Utf8> Function();

typedef _GetAllSkillScansC = ffi.Pointer<Utf8> Function();
typedef _GetAllSkillScansDart = ffi.Pointer<Utf8> Function();

typedef _FreeStringDart = void Function(ffi.Pointer<Utf8>);

/// 扫描结果 FFI 持久化门面：通过 FFI 委托 Go 层进行数据持久化，Flutter 不直接操作 DB。
/// 所有操作通过 [NativeLibraryService] 获取 dylib 调用 Go 层。
class ScanDatabaseService {
  static final ScanDatabaseService _instance = ScanDatabaseService._internal();

  factory ScanDatabaseService() => _instance;

  ScanDatabaseService._internal();

  /// 从NativeLibraryService获取dylib
  ffi.DynamicLibrary? get _dylib => NativeLibraryService().dylib;

  /// 从NativeLibraryService获取FreeString函数
  _FreeStringDart? get _freeString => NativeLibraryService().freeString;

  String? _lastLoggedLatestScanSummary;

  // --- Scan Result methods ---

  Future<void> saveScanResult(ScanResult result) async {
    final dylib = _dylib;
    if (dylib == null || _freeString == null) {
      throw Exception('Native library not initialized');
    }

    try {
      final saveFn = dylib
          .lookupFunction<_SaveScanResultC, _SaveScanResultDart>(
            'SaveScanResult',
          );

      final payload = jsonEncode({
        'config_found': result.configFound,
        'config_path': result.configPath,
        'config_json': result.config != null ? jsonEncode(result.config) : null,
        'assets': result.assets.map((a) => a.toJson()).toList(),
        'risks': result.risks.map((r) => r.toJson()).toList(),
        'created_at': result.scannedAt?.toUtc().toIso8601String(),
      });

      final payloadPtr = payload.toNativeUtf8();
      final resultPtr = saveFn(payloadPtr);
      final resultStr = resultPtr.toDartString();
      _freeString!(resultPtr);
      malloc.free(payloadPtr);

      final response = jsonDecode(resultStr);
      if (response['success'] == true) {
        appLogger.info(
          '[ScanDB] Scan result saved via Go layer, id=${response['scan_id']}',
        );
        return;
      }
      throw Exception('Go save failed: ${response['error']}');
    } catch (e) {
      appLogger.error('[ScanDB] Failed to save scan result', e);
      rethrow;
    }
  }

  Future<ScanResult?> getLatestScanResult() async {
    final dylib = _dylib;
    if (dylib == null || _freeString == null) {
      appLogger.warning('[ScanDB] Native library not initialized');
      return null;
    }

    try {
      final getFn = dylib
          .lookupFunction<_GetLatestScanResultC, _GetLatestScanResultDart>(
            'GetLatestScanResult',
          );

      final resultPtr = getFn();
      final resultStr = resultPtr.toDartString();
      _freeString!(resultPtr);

      final response = jsonDecode(resultStr);
      if (response['success'] == true) {
        if (response['data'] == null) return null;

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

        final baseRisks = <RiskInfo>[];
        final legacySkillRisks = <RiskInfo>[];
        for (final r in (data['risks'] as List?) ?? []) {
          final riskMap = r as Map<String, dynamic>;
          final risk = RiskInfo(
            id: riskMap['id'] ?? 'unknown',
            title: riskMap['title'] ?? 'Unknown Risk',
            description: riskMap['description'] ?? '',
            level: _parseRiskLevel(riskMap['level']),
            icon: _getIconForRisk(riskMap['level']),
            args: riskMap['args'] != null
                ? Map<String, Object>.from(riskMap['args'])
                : null,
            mitigation: riskMap['mitigation'] != null
                ? Mitigation.fromJson(riskMap['mitigation'])
                : null,
            sourcePlugin: riskMap['source_plugin'] as String?,
          );
          if (_isSkillRisk(risk)) {
            legacySkillRisks.add(risk);
          } else {
            baseRisks.add(risk);
          }
        }

        final skillRisks = await _loadSkillRisksFromDatabase();
        if (skillRisks.isEmpty && legacySkillRisks.isNotEmpty) {
          final enrichedLegacySkillRisks = await _enrichLegacySkillRisksFromDatabase(
            legacySkillRisks,
          );
          skillRisks.addAll(enrichedLegacySkillRisks);
        }

        final latestScanSummary =
            '${assets.length}|${baseRisks.length}|${skillRisks.length}|'
            '${configFound ? 1 : 0}|${configPath ?? ''}|${scannedAtRaw ?? ''}';
        if (_lastLoggedLatestScanSummary != latestScanSummary) {
          _lastLoggedLatestScanSummary = latestScanSummary;
          appLogger.info(
            '[ScanDB] Loaded latest scan via Go layer: ${assets.length} assets, ${baseRisks.length} base risks, ${skillRisks.length} skill risks',
          );
        }
        return ScanResult(
          config: configJSON != null
              ? jsonDecode(configJSON) as Map<String, dynamic>
              : null,
          riskInfo: baseRisks,
          skillResult: skillRisks,
          configFound: configFound,
          configPath: configPath,
          assets: assets,
          scannedAt: scannedAtRaw != null
              ? DateTime.tryParse(scannedAtRaw)
              : null,
        );
      }
      return null;
    } catch (e) {
      appLogger.error('[ScanDB] Failed to load latest scan result', e);
      return null;
    }
  }

  // --- Skill Scan methods ---

  /// Get all scanned skill hashes
  Future<Set<String>> getScannedSkillHashes() async {
    final dylib = _dylib;
    if (dylib == null || _freeString == null) return {};

    try {
      final getFn = dylib
          .lookupFunction<_GetScannedSkillHashesC, _GetScannedSkillHashesDart>(
            'GetScannedSkillHashes',
          );

      final resultPtr = getFn();
      final resultStr = resultPtr.toDartString();
      _freeString!(resultPtr);

      final response = jsonDecode(resultStr);
      if (response['success'] == true && response['data'] != null) {
        return (response['data'] as List).cast<String>().toSet();
      }
      return {};
    } catch (e) {
      appLogger.error('[ScanDB] Failed to get scanned skill hashes', e);
      return {};
    }
  }

  /// Save a skill scan result
  Future<void> saveSkillScanResult({
    required String skillName,
    required String skillHash,
    required bool safe,
    List<String>? issues,
  }) async {
    final dylib = _dylib;
    if (dylib == null || _freeString == null) {
      throw Exception('Native library not initialized');
    }

    try {
      final saveFn = dylib
          .lookupFunction<_SaveSkillScanResultC, _SaveSkillScanResultDart>(
            'SaveSkillScanResult',
          );

      final payload = jsonEncode({
        'skill_name': skillName,
        'skill_hash': skillHash,
        'safe': safe,
        'issues': issues,
      });

      final payloadPtr = payload.toNativeUtf8();
      final resultPtr = saveFn(payloadPtr);
      final resultStr = resultPtr.toDartString();
      _freeString!(resultPtr);
      malloc.free(payloadPtr);

      final response = jsonDecode(resultStr);
      if (response['success'] == true) {
        appLogger.info('[ScanDB] Skill scan saved via Go layer: $skillName');
        return;
      }
      throw Exception('Failed to save skill scan: ${response['error']}');
    } catch (e) {
      appLogger.error('[ScanDB] Failed to save skill scan result', e);
      rethrow;
    }
  }

  /// Get skill scan result by hash
  Future<Map<String, dynamic>?> getSkillScanByHash(String hash) async {
    final dylib = _dylib;
    if (dylib == null || _freeString == null) return null;

    try {
      final getFn = dylib
          .lookupFunction<_GetSkillScanByHashC, _GetSkillScanByHashDart>(
            'GetSkillScanByHash',
          );

      final hashPtr = hash.toNativeUtf8();
      final resultPtr = getFn(hashPtr);
      final resultStr = resultPtr.toDartString();
      _freeString!(resultPtr);
      malloc.free(hashPtr);

      final response = jsonDecode(resultStr);
      if (response['success'] == true) {
        if (response['data'] == null) return null;
        final data = response['data'] as Map<String, dynamic>;
        return {
          'skill_name': data['skill_name'],
          'skill_hash': data['skill_hash'],
          'scanned_at': data['scanned_at'],
          'safe': data['safe'] as bool? ?? false,
          'issues': (data['issues'] as List?)?.cast<String>() ?? <String>[],
        };
      }
      return null;
    } catch (e) {
      appLogger.error('[ScanDB] Failed to get skill scan by hash', e);
      return null;
    }
  }

  /// Delete skill scan record by skill hash
  Future<void> deleteSkillScan(String skillHash) async {
    final dylib = _dylib;
    if (dylib == null || _freeString == null) return;

    try {
      final deleteFn = dylib
          .lookupFunction<_DeleteSkillScanC, _DeleteSkillScanDart>(
            'DeleteSkillScanFFI',
          );

      final hashPtr = skillHash.toNativeUtf8();
      final resultPtr = deleteFn(hashPtr);
      final resultStr = resultPtr.toDartString();
      _freeString!(resultPtr);
      malloc.free(hashPtr);

      final response = jsonDecode(resultStr);
      if (response['success'] == true) {
        appLogger.info('[ScanDB] Skill scan deleted via Go layer: $skillHash');
      }
    } catch (e) {
      appLogger.error('[ScanDB] Failed to delete skill scan', e);
    }
  }

  /// Get all risky (unsafe) skill scan records
  Future<List<Map<String, dynamic>>> getRiskySkills() async {
    final dylib = _dylib;
    if (dylib == null || _freeString == null) return [];

    try {
      final getFn = dylib.lookupFunction<_GetRiskySkillsC, _GetRiskySkillsDart>(
        'GetRiskySkills',
      );

      final resultPtr = getFn();
      final resultStr = resultPtr.toDartString();
      _freeString!(resultPtr);

      final response = jsonDecode(resultStr);
      if (response['success'] == true && response['data'] != null) {
        return (response['data'] as List).map((item) {
          final data = item as Map<String, dynamic>;
          final rawIssues =
              (data['issues'] as List?)?.cast<String>() ?? <String>[];
          final rawIssueCount = rawIssues.length;
          return {
            'skill_name': data['skill_name'],
            'skill_hash': data['skill_hash'],
            'skill_path': data['skill_path'] as String? ?? '',
            'source_plugin': data['source_plugin'] as String? ?? '',
            'asset_id': data['asset_id'] as String? ?? '',
            'scanned_at': data['scanned_at'],
            'safe': data['safe'] as bool? ?? false,
            'risk_level': data['risk_level'] as String? ?? '',
            'issues': rawIssues.map(_formatSkillIssueText).toList(),
            'issue_details': rawIssues.map(_parseSkillIssueDetail).toList(),
            'issue_count': rawIssueCount,
          };
        }).toList();
      }
      return [];
    } catch (e) {
      appLogger.error('[ScanDB] Failed to get risky skills', e);
      return [];
    }
  }

  /// Get all skill scan records (safe, risky, trusted)
  Future<List<Map<String, dynamic>>> getAllSkillScans() async {
    final dylib = _dylib;
    if (dylib == null || _freeString == null) return [];

    try {
      final getFn = dylib
          .lookupFunction<_GetAllSkillScansC, _GetAllSkillScansDart>(
            'GetAllSkillScansFFI',
          );

      final resultPtr = getFn();
      final resultStr = resultPtr.toDartString();
      _freeString!(resultPtr);

      final response = jsonDecode(resultStr);
      if (response['success'] == true && response['data'] != null) {
        return (response['data'] as List).map((item) {
          final data = item as Map<String, dynamic>;
          final rawIssues =
              (data['issues'] as List?)?.cast<String>() ?? <String>[];
          final rawIssueCount = rawIssues.length;
          final deletedAt = data['deleted_at'] as String? ?? '';
          return {
            'skill_name': data['skill_name'],
            'skill_hash': data['skill_hash'],
            'scanned_at': data['scanned_at'],
            'safe': data['safe'] as bool? ?? false,
            'risk_level': data['risk_level'] as String? ?? '',
            'trusted': data['trusted'] as bool? ?? false,
            'issues': rawIssues.map(_formatSkillIssueText).toList(),
            'issue_details': rawIssues.map(_parseSkillIssueDetail).toList(),
            'issue_count': rawIssueCount,
            'deleted_at': deletedAt,
            'deleted': deletedAt.trim().isNotEmpty,
          };
        }).toList();
      }
      return [];
    } catch (e) {
      appLogger.error('[ScanDB] Failed to get all skill scans', e);
      return [];
    }
  }

  // --- Helper methods ---

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

  bool _isSkillRisk(RiskInfo risk) => risk.id == 'riskSkillSecurityIssue';

  Future<List<RiskInfo>> _loadSkillRisksFromDatabase() async {
    final riskySkills = await getRiskySkills();
    final mergedRiskySkills = _mergeRiskySkillsByPath(riskySkills);
    return mergedRiskySkills.map((skill) {
      final skillName = skill['skill_name'] as String? ?? 'Unknown Skill';
      final issues = skill['issues'] as List<String>? ?? const <String>[];
      final persistedIssueCount = (skill['issue_count'] as num?)?.toInt() ?? 0;
      final issueCount = persistedIssueCount > issues.length
          ? persistedIssueCount
          : issues.length;
      final normalizedIssueCount = issueCount > 0 ? issueCount : 1;

      return RiskInfo(
        id: 'riskSkillSecurityIssue',
        args: {
          'skillName': skillName,
          if ((skill['skill_hash'] as String? ?? '').isNotEmpty)
            'skillHash': skill['skill_hash'] as String,
          'asset_name': skill['source_plugin'] as String? ?? '',
          'asset_id': skill['asset_id'] as String? ?? '',
          'issueCount': normalizedIssueCount,
          'issues': issues.join('; '),
          if ((skill['skill_path'] as String? ?? '').isNotEmpty)
            'skillPath': skill['skill_path'] as String,
        },
        title: 'Risky Skill: $skillName',
        description: normalizedIssueCount > 0
            ? 'Skill "$skillName" has $normalizedIssueCount security issue(s): ${issues.join("; ")}'
            : 'Skill "$skillName" was flagged as potentially risky.',
        level: _parseRiskLevel(skill['risk_level']),
        icon: Icons.warning,
        sourcePlugin: skill['source_plugin'] as String? ?? '',
      );
    }).toList();
  }

  /// 按技能路径去重并合并问题列表，避免风险技能展示重复。
  List<Map<String, dynamic>> _mergeRiskySkillsByPath(
    List<Map<String, dynamic>> riskySkills,
  ) {
    final merged = <String, Map<String, dynamic>>{};
    for (final skill in riskySkills) {
      final skillName = (skill['skill_name'] as String? ?? '').trim();
      final skillHash = (skill['skill_hash'] as String? ?? '').trim();
      final skillPath = (skill['skill_path'] as String? ?? '').trim();
      final sourcePlugin = (skill['source_plugin'] as String? ?? '').trim();
      final assetID = (skill['asset_id'] as String? ?? '').trim();
      final key = skillPath.isNotEmpty
          ? 'path:${skillPath.toLowerCase()}|${sourcePlugin.toLowerCase()}|${assetID.toLowerCase()}'
          : 'fallback:${skillName.toLowerCase()}|${skillHash.toLowerCase()}|${sourcePlugin.toLowerCase()}|${assetID.toLowerCase()}';

      final issueList = (skill['issues'] as List<String>? ?? const <String>[])
          .where((issue) => issue.trim().isNotEmpty)
          .toList();
      final rawIssueCount = (skill['issue_count'] as num?)?.toInt() ?? 0;
      final normalizedIssueCount = rawIssueCount > 0
          ? rawIssueCount
          : issueList.length;

      final existed = merged[key];
      if (existed == null) {
        final newSkill = Map<String, dynamic>.from(skill);
        newSkill['issues'] = issueList.toSet().toList();
        newSkill['issue_count'] = normalizedIssueCount;
        merged[key] = newSkill;
        continue;
      }

      final existedIssues =
          (existed['issues'] as List<String>? ?? const <String>[]).toSet();
      existedIssues.addAll(issueList);
      existed['issues'] = existedIssues.toList();
      final mergedIssueCount = (existed['issue_count'] as num?)?.toInt() ?? 0;
      final dedupIssueCount = existedIssues.length;
      final nextIssueCount = [
        mergedIssueCount,
        normalizedIssueCount,
        dedupIssueCount,
      ].reduce((a, b) => a > b ? a : b);
      existed['issue_count'] = nextIssueCount;
    }
    return merged.values.toList();
  }

  /// 为历史扫描结果中的技能风险补齐删除所需参数（skillHash/skillPath）。
  Future<List<RiskInfo>> _enrichLegacySkillRisksFromDatabase(
    List<RiskInfo> legacySkillRisks,
  ) async {
    final riskySkills = await getRiskySkills();
    final skillMetaByName = <String, Map<String, String>>{};
    for (final skill in riskySkills) {
      final skillName = (skill['skill_name'] as String? ?? '').trim();
      if (skillName.isEmpty) {
        continue;
      }
      skillMetaByName[skillName] = {
        'skillHash': skill['skill_hash'] as String? ?? '',
        'skillPath': skill['skill_path'] as String? ?? '',
        'sourcePlugin': skill['source_plugin'] as String? ?? '',
        'assetID': skill['asset_id'] as String? ?? '',
      };
    }

    return legacySkillRisks.map((risk) {
      final args = risk.args != null
          ? Map<String, Object>.from(risk.args!)
          : <String, Object>{};
      final skillName = (args['skillName']?.toString() ?? '').trim();
      if (skillName.isEmpty) {
        return risk;
      }

      final metadata = skillMetaByName[skillName];
      if (metadata == null) {
        return risk;
      }

      final riskSkillHash = (args['skillHash']?.toString() ?? '').trim();
      final riskSkillPath = (args['skillPath']?.toString() ?? '').trim();
      if (riskSkillHash.isEmpty && (metadata['skillHash'] ?? '').isNotEmpty) {
        args['skillHash'] = metadata['skillHash']!;
      }
      if (riskSkillPath.isEmpty && (metadata['skillPath'] ?? '').isNotEmpty) {
        args['skillPath'] = metadata['skillPath']!;
      }
      if ((args['asset_name']?.toString() ?? '').trim().isEmpty &&
          (metadata['sourcePlugin'] ?? '').isNotEmpty) {
        args['asset_name'] = metadata['sourcePlugin']!;
      }
      if ((args['asset_id']?.toString() ?? '').trim().isEmpty &&
          (metadata['assetID'] ?? '').isNotEmpty) {
        args['asset_id'] = metadata['assetID']!;
      }

      return RiskInfo(
        id: risk.id,
        title: risk.title,
        description: risk.description,
        level: risk.level,
        icon: risk.icon,
        args: args,
        mitigation: risk.mitigation,
        sourcePlugin:
            risk.sourcePlugin ??
            (metadata['sourcePlugin']?.trim().isNotEmpty == true
                ? metadata['sourcePlugin']
                : null),
      );
    }).toList();
  }

  Map<String, String> _parseSkillIssueDetail(String raw) {
    try {
      final decoded = jsonDecode(raw);
      if (decoded is Map<String, dynamic>) {
        return {
          'type': decoded['type']?.toString() ?? 'security_risk',
          'severity': decoded['severity']?.toString() ?? 'medium',
          'file': decoded['file']?.toString() ?? '',
          'description': decoded['description']?.toString() ?? raw,
          'evidence': decoded['evidence']?.toString() ?? '',
        };
      }
    } catch (_) {}

    return {
      'type': 'security_risk',
      'severity': 'medium',
      'file': '',
      'description': raw,
      'evidence': '',
    };
  }

  String _formatSkillIssueText(String raw) {
    final detail = _parseSkillIssueDetail(raw);
    final file = detail['file'] ?? '';
    final description = detail['description'] ?? raw;
    if (file.isEmpty) {
      return description;
    }
    return '[$file] $description';
  }
}
