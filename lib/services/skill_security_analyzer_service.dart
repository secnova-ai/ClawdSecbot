import 'dart:async';
import 'dart:convert';
import 'dart:ffi' as ffi;
import 'dart:io';
import 'package:ffi/ffi.dart';
import '../models/llm_config_model.dart';
import '../utils/app_logger.dart';
import 'model_config_database_service.dart';
import 'native_library_service.dart' hide FreeStringDart;
import 'scan_database_service.dart';

// C function signatures - 单技能扫描（保留兼容）
typedef StartSkillSecurityScanC =
    ffi.Pointer<Utf8> Function(
      ffi.Pointer<Utf8> skillPath,
      ffi.Pointer<Utf8> modelConfigJSON,
    );
typedef StartSkillSecurityScanDart =
    ffi.Pointer<Utf8> Function(
      ffi.Pointer<Utf8> skillPath,
      ffi.Pointer<Utf8> modelConfigJSON,
    );

typedef GetSkillSecurityScanLogC =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> scanID);
typedef GetSkillSecurityScanLogDart =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> scanID);

typedef GetSkillSecurityScanResultC =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> scanID);
typedef GetSkillSecurityScanResultDart =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> scanID);

typedef CancelSkillSecurityScanC =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> scanID);
typedef CancelSkillSecurityScanDart =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> scanID);

// C function signatures - 批量扫描
typedef StartBatchSkillScanC = ffi.Pointer<Utf8> Function();
typedef StartBatchSkillScanDart = ffi.Pointer<Utf8> Function();

typedef GetBatchSkillScanLogC =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> batchID);
typedef GetBatchSkillScanLogDart =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> batchID);

typedef GetBatchSkillScanResultsC =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> batchID);
typedef GetBatchSkillScanResultsDart =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> batchID);

typedef CancelBatchSkillScanC =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> batchID);
typedef CancelBatchSkillScanDart =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> batchID);

typedef TestModelConnectionFFIC =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> configJSON);
typedef TestModelConnectionFFIDart =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> configJSON);

typedef DeleteSkillC = ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> skillPath);
typedef DeleteSkillDart =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> skillPath);

typedef TrustSkillScanC =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> skillHash);
typedef TrustSkillScanDart =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> skillHash);

typedef FreeStringC = ffi.Void Function(ffi.Pointer<Utf8>);
typedef FreeStringDart = void Function(ffi.Pointer<Utf8>);

/// 批量扫描进度信息
class BatchScanProgress {
  final List<String> logs;
  final int currentIndex;
  final int total;
  final String currentSkill;
  final bool completed;
  final String? error;

  BatchScanProgress({
    required this.logs,
    required this.currentIndex,
    required this.total,
    required this.currentSkill,
    required this.completed,
    this.error,
  });
}

/// Skill 删除结果。
class SkillDeleteResult {
  final bool success;
  final bool alreadyMissing;

  const SkillDeleteResult({
    required this.success,
    this.alreadyMissing = false,
  });
}

class SkillSecurityAnalyzerService {
  final StreamController<String> _logController =
      StreamController<String>.broadcast();
  final StreamController<BatchScanProgress> _progressController =
      StreamController<BatchScanProgress>.broadcast();
  Timer? _pollTimer;
  String? _currentBatchID;

  Stream<String> get logStream => _logController.stream;
  Stream<BatchScanProgress> get progressStream => _progressController.stream;
  bool get isScanning => _currentBatchID != null;

  ffi.DynamicLibrary _getDylib() {
    final dylib = NativeLibraryService().dylib;
    if (dylib == null) {
      throw Exception('Plugin library not loaded');
    }
    return dylib;
  }

  /// 加载安全模型配置（用于 skill 扫描）
  Future<SecurityModelConfig> loadConfig() async {
    try {
      final dbService = ModelConfigDatabaseService();
      final config = await dbService.getSecurityModelConfig();
      if (config != null) {
        return config;
      }
    } catch (e) {
      appLogger.error(
        '[SkillSecurityAnalyzer] Failed to load config from db',
        e,
      );
    }

    // 返回默认配置
    return SecurityModelConfig(
      provider: 'ollama',
      endpoint: 'http://localhost:11434',
      apiKey: '',
      model: 'llama3',
    );
  }

  /// Check if model is configured (has valid configuration saved)
  Future<bool> isModelConfigured() async {
    try {
      final dbService = ModelConfigDatabaseService();
      final configured = await dbService.hasValidSecurityModelConfig();
      appLogger.info('[SkillSecurityAnalyzer] Model configured: $configured');
      return configured;
    } catch (e) {
      appLogger.error(
        '[SkillSecurityAnalyzer] Failed to check model config in db',
        e,
      );
      return false;
    }
  }

  /// 保存安全模型配置到数据库
  Future<bool> saveConfig(SecurityModelConfig config) async {
    try {
      final dbService = ModelConfigDatabaseService();
      return await dbService.saveSecurityModelConfig(config);
    } catch (e) {
      appLogger.error('[SkillSecurityAnalyzer] Failed to save config to db', e);
      return false;
    }
  }

  /// 测试模型连接
  Future<Map<String, dynamic>> testConnection(
    SecurityModelConfig config,
  ) async {
    try {
      final dylib = _getDylib();
      final testModelConnection = dylib
          .lookupFunction<TestModelConnectionFFIC, TestModelConnectionFFIDart>(
            'TestModelConnectionFFI',
          );
      final freeString = dylib.lookupFunction<FreeStringC, FreeStringDart>(
        'FreeString',
      );

      final configJSON = jsonEncode(config.toJson());
      final configPtr = configJSON.toNativeUtf8();
      final resultPtr = testModelConnection(configPtr);
      malloc.free(configPtr);

      final resultStr = resultPtr.toDartString();
      freeString(resultPtr);

      return jsonDecode(resultStr) as Map<String, dynamic>;
    } catch (e) {
      appLogger.error('[SkillSecurityAnalyzer] Failed to test connection', e);
      return {'success': false, 'error': e.toString()};
    }
  }

  // ==================== 批量扫描 ====================

  /// 启动批量技能扫描（零参数，Go 层自动发现技能和配置）
  Future<Map<String, dynamic>> startBatchScan() async {
    if (_currentBatchID != null) {
      throw Exception('A batch scan is already in progress');
    }

    final dylib = _getDylib();
    final startBatch = dylib
        .lookupFunction<StartBatchSkillScanC, StartBatchSkillScanDart>(
          'StartBatchSkillScan',
        );
    final freeString = dylib.lookupFunction<FreeStringC, FreeStringDart>(
      'FreeString',
    );

    final resultPtr = startBatch();
    final resultStr = resultPtr.toDartString();
    freeString(resultPtr);

    final result = jsonDecode(resultStr) as Map<String, dynamic>;
    if (result['success'] != true) {
      return result;
    }

    // Handle "no skills to scan" case - return early without attempting to get batch_id
    if (result['success'] == true && (result['total'] as int? ?? 0) == 0) {
      return result;
    }

    _currentBatchID = result['batch_id'] as String;
    _startBatchPolling();

    return result;
  }

  void _startBatchPolling() {
    _pollTimer?.cancel();
    _pollTimer = Timer.periodic(const Duration(milliseconds: 200), (timer) {
      if (_currentBatchID != null) {
        _pollBatchLogs(_currentBatchID!);
      }
    });
  }

  void _pollBatchLogs(String batchID) {
    try {
      final dylib = _getDylib();
      final getLog = dylib
          .lookupFunction<GetBatchSkillScanLogC, GetBatchSkillScanLogDart>(
            'GetBatchSkillScanLog',
          );
      final freeString = dylib.lookupFunction<FreeStringC, FreeStringDart>(
        'FreeString',
      );

      final batchIDPtr = batchID.toNativeUtf8();
      final resultPtr = getLog(batchIDPtr);
      malloc.free(batchIDPtr);

      final resultStr = resultPtr.toDartString();
      freeString(resultPtr);

      final result = jsonDecode(resultStr) as Map<String, dynamic>;

      // 发送日志到 stream
      final logs = result['logs'] as List?;
      if (logs != null) {
        for (var log in logs) {
          _logController.add(log.toString());
        }
      }

      // 发送进度到 stream
      final progress = BatchScanProgress(
        logs: (logs ?? []).map((e) => e.toString()).toList(),
        currentIndex: result['current_index'] as int? ?? 0,
        total: result['total'] as int? ?? 0,
        currentSkill: result['current_skill'] as String? ?? '',
        completed: result['completed'] as bool? ?? false,
        error: result['error'] as String?,
      );
      _progressController.add(progress);

      // 检查是否完成
      if (result['completed'] == true) {
        _pollTimer?.cancel();
        _currentBatchID = null;

        if (result['error'] != null && result['error'].toString().isNotEmpty) {
          _logController.add('Error: ${result['error']}');
        }
      }
    } catch (e) {
      appLogger.error('[SkillSecurityAnalyzer] Poll batch logs error', e);
    }
  }

  /// 获取批量扫描最终结果
  Future<Map<String, dynamic>> getBatchScanResults(String batchID) async {
    final dylib = _getDylib();
    final getResults = dylib
        .lookupFunction<
          GetBatchSkillScanResultsC,
          GetBatchSkillScanResultsDart
        >('GetBatchSkillScanResults');
    final freeString = dylib.lookupFunction<FreeStringC, FreeStringDart>(
      'FreeString',
    );

    final batchIDPtr = batchID.toNativeUtf8();
    final resultPtr = getResults(batchIDPtr);
    malloc.free(batchIDPtr);

    final resultStr = resultPtr.toDartString();
    freeString(resultPtr);

    return jsonDecode(resultStr) as Map<String, dynamic>;
  }

  /// 取消批量扫描
  Future<void> cancelBatchScan() async {
    if (_currentBatchID == null) return;

    try {
      final dylib = _getDylib();
      final cancelBatch = dylib
          .lookupFunction<CancelBatchSkillScanC, CancelBatchSkillScanDart>(
            'CancelBatchSkillScan',
          );
      final freeString = dylib.lookupFunction<FreeStringC, FreeStringDart>(
        'FreeString',
      );

      final batchIDPtr = _currentBatchID!.toNativeUtf8();
      final resultPtr = cancelBatch(batchIDPtr);
      malloc.free(batchIDPtr);
      freeString(resultPtr);
    } catch (e) {
      appLogger.error('[SkillSecurityAnalyzer] Cancel batch scan error', e);
    } finally {
      _pollTimer?.cancel();
      _currentBatchID = null;
    }
  }

  /// Delete a skill directory.
  /// If the directory is already missing, treat it as successful deletion.
  Future<SkillDeleteResult> deleteSkill({
    required String skillPath,
    required String skillHash,
  }) async {
    try {
      final normalizedSkillPath = skillPath.trim();
      final normalizedSkillHash = skillHash.trim();
      final targetDirectory = Directory(normalizedSkillPath);
      if (!await targetDirectory.exists()) {
        await ScanDatabaseService().deleteSkillScan(normalizedSkillHash);
        appLogger.info(
          '[SkillSecurityAnalyzer] Skill path already missing, treat as deleted: path=$normalizedSkillPath, hash=$normalizedSkillHash',
        );
        return const SkillDeleteResult(success: true, alreadyMissing: true);
      }

      final dylib = _getDylib();
      final deleteSkill = dylib.lookupFunction<DeleteSkillC, DeleteSkillDart>(
        'DeleteSkill',
      );
      final freeString = dylib.lookupFunction<FreeStringC, FreeStringDart>(
        'FreeString',
      );

      final skillPathPtr = normalizedSkillPath.toNativeUtf8();
      final resultPtr = deleteSkill(skillPathPtr);
      malloc.free(skillPathPtr);

      final resultStr = resultPtr.toDartString();
      freeString(resultPtr);

      final result = jsonDecode(resultStr);
      if (result['success'] == true) {
        await ScanDatabaseService().deleteSkillScan(normalizedSkillHash);
        return SkillDeleteResult(
          success: true,
          alreadyMissing: result['already_missing'] == true,
        );
      } else {
        appLogger.warning(
          '[SkillSecurityAnalyzer] Delete skill failed: path=$normalizedSkillPath, hash=$normalizedSkillHash, error=${result['error'] ?? 'unknown'}',
        );
      }
      return const SkillDeleteResult(success: false);
    } catch (e) {
      appLogger.error('[SkillSecurityAnalyzer] Delete skill error', e);
      return const SkillDeleteResult(success: false);
    }
  }

  /// Trust a skill (mark as trusted so it doesn't appear in risky skills list)
  Future<bool> trustSkill(String skillHash) async {
    try {
      final dylib = _getDylib();
      final trustSkillScan = dylib
          .lookupFunction<TrustSkillScanC, TrustSkillScanDart>(
            'TrustSkillScan',
          );
      final freeString = dylib.lookupFunction<FreeStringC, FreeStringDart>(
        'FreeString',
      );

      final skillHashPtr = skillHash.toNativeUtf8();
      final resultPtr = trustSkillScan(skillHashPtr);
      malloc.free(skillHashPtr);

      final resultStr = resultPtr.toDartString();
      freeString(resultPtr);

      final result = jsonDecode(resultStr);
      return result['success'] == true;
    } catch (e) {
      appLogger.error('[SkillSecurityAnalyzer] Trust skill error', e);
      return false;
    }
  }

  void dispose() {
    _pollTimer?.cancel();
    _logController.close();
    _progressController.close();
  }
}
