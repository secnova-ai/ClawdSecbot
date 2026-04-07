import 'dart:async';
import 'dart:convert';
import 'dart:ffi' as ffi;
import 'package:ffi/ffi.dart';
import 'package:path/path.dart' as path;
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
typedef StartBatchSkillScanByAssetC =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);
typedef StartBatchSkillScanByAssetDart =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);

typedef GetBatchSkillScanLogC =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> batchID);
typedef GetBatchSkillScanLogDart =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> batchID);
typedef GetBatchSkillScanLogByAssetC =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>, ffi.Pointer<Utf8>);
typedef GetBatchSkillScanLogByAssetDart =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>, ffi.Pointer<Utf8>);

typedef GetBatchSkillScanResultsC =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> batchID);
typedef GetBatchSkillScanResultsDart =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> batchID);
typedef GetBatchSkillScanResultsByAssetC =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>, ffi.Pointer<Utf8>);
typedef GetBatchSkillScanResultsByAssetDart =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>, ffi.Pointer<Utf8>);

typedef CancelBatchSkillScanC =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> batchID);
typedef CancelBatchSkillScanDart =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> batchID);
typedef CancelBatchSkillScanByAssetC =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>, ffi.Pointer<Utf8>);
typedef CancelBatchSkillScanByAssetDart =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>, ffi.Pointer<Utf8>);

typedef TestModelConnectionFFIC =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> configJSON);
typedef TestModelConnectionFFIDart =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> configJSON);

typedef DeleteSkillC = ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> skillPath);
typedef DeleteSkillDart =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> skillPath);

typedef TrustSkillScanC =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> skillName);
typedef TrustSkillScanDart =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> skillName);

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

class SkillSecurityAnalyzerService {
  final StreamController<String> _logController =
      StreamController<String>.broadcast();
  final StreamController<BatchScanProgress> _progressController =
      StreamController<BatchScanProgress>.broadcast();
  Timer? _pollTimer;
  String? _currentBatchID;
  String? _currentAssetName;

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
  Future<Map<String, dynamic>> startBatchScan([String? assetName]) async {
    if (_currentBatchID != null) {
      throw Exception('A batch scan is already in progress');
    }

    final dylib = _getDylib();
    final freeString = dylib.lookupFunction<FreeStringC, FreeStringDart>(
      'FreeString',
    );
    ffi.Pointer<Utf8> resultPtr;
    if (assetName != null && assetName.trim().isNotEmpty) {
      final startBatch = dylib
          .lookupFunction<
            StartBatchSkillScanByAssetC,
            StartBatchSkillScanByAssetDart
          >('StartBatchSkillScanByAssetFFI');
      final assetNamePtr = assetName.toNativeUtf8();
      resultPtr = startBatch(assetNamePtr);
      malloc.free(assetNamePtr);
      _currentAssetName = assetName;
    } else {
      final startBatch = dylib
          .lookupFunction<StartBatchSkillScanC, StartBatchSkillScanDart>(
            'StartBatchSkillScan',
          );
      resultPtr = startBatch();
      _currentAssetName = null;
    }
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
        _pollBatchLogs(_currentBatchID!, _currentAssetName);
      }
    });
  }

  void _pollBatchLogs(String batchID, String? assetName) {
    try {
      final dylib = _getDylib();
      final freeString = dylib.lookupFunction<FreeStringC, FreeStringDart>(
        'FreeString',
      );
      ffi.Pointer<Utf8> resultPtr;
      if (assetName != null && assetName.trim().isNotEmpty) {
        final getLog = dylib
            .lookupFunction<
              GetBatchSkillScanLogByAssetC,
              GetBatchSkillScanLogByAssetDart
            >('GetBatchSkillScanLogByAssetFFI');
        final assetNamePtr = assetName.toNativeUtf8();
        final batchIDPtr = batchID.toNativeUtf8();
        resultPtr = getLog(assetNamePtr, batchIDPtr);
        malloc.free(assetNamePtr);
        malloc.free(batchIDPtr);
      } else {
        final getLog = dylib
            .lookupFunction<GetBatchSkillScanLogC, GetBatchSkillScanLogDart>(
              'GetBatchSkillScanLog',
            );
        final batchIDPtr = batchID.toNativeUtf8();
        resultPtr = getLog(batchIDPtr);
        malloc.free(batchIDPtr);
      }

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
        _currentAssetName = null;

        if (result['error'] != null && result['error'].toString().isNotEmpty) {
          _logController.add('Error: ${result['error']}');
        }
      }
    } catch (e) {
      appLogger.error('[SkillSecurityAnalyzer] Poll batch logs error', e);
    }
  }

  /// 获取批量扫描最终结果
  Future<Map<String, dynamic>> getBatchScanResults(
    String batchID, [
    String? assetName,
  ]) async {
    final dylib = _getDylib();
    final freeString = dylib.lookupFunction<FreeStringC, FreeStringDart>(
      'FreeString',
    );
    ffi.Pointer<Utf8> resultPtr;
    if (assetName != null && assetName.trim().isNotEmpty) {
      final getResults = dylib
          .lookupFunction<
            GetBatchSkillScanResultsByAssetC,
            GetBatchSkillScanResultsByAssetDart
          >('GetBatchSkillScanResultsByAssetFFI');
      final assetNamePtr = assetName.toNativeUtf8();
      final batchIDPtr = batchID.toNativeUtf8();
      resultPtr = getResults(assetNamePtr, batchIDPtr);
      malloc.free(assetNamePtr);
      malloc.free(batchIDPtr);
    } else {
      final getResults = dylib
          .lookupFunction<
            GetBatchSkillScanResultsC,
            GetBatchSkillScanResultsDart
          >('GetBatchSkillScanResults');
      final batchIDPtr = batchID.toNativeUtf8();
      resultPtr = getResults(batchIDPtr);
      malloc.free(batchIDPtr);
    }

    final resultStr = resultPtr.toDartString();
    freeString(resultPtr);

    return jsonDecode(resultStr) as Map<String, dynamic>;
  }

  /// 取消批量扫描
  Future<void> cancelBatchScan() async {
    if (_currentBatchID == null) return;

    try {
      final dylib = _getDylib();
      final freeString = dylib.lookupFunction<FreeStringC, FreeStringDart>(
        'FreeString',
      );
      ffi.Pointer<Utf8> resultPtr;
      if (_currentAssetName != null && _currentAssetName!.trim().isNotEmpty) {
        final cancelBatch = dylib
            .lookupFunction<
              CancelBatchSkillScanByAssetC,
              CancelBatchSkillScanByAssetDart
            >('CancelBatchSkillScanByAssetFFI');
        final assetNamePtr = _currentAssetName!.toNativeUtf8();
        final batchIDPtr = _currentBatchID!.toNativeUtf8();
        resultPtr = cancelBatch(assetNamePtr, batchIDPtr);
        malloc.free(assetNamePtr);
        malloc.free(batchIDPtr);
      } else {
        final cancelBatch = dylib
            .lookupFunction<CancelBatchSkillScanC, CancelBatchSkillScanDart>(
              'CancelBatchSkillScan',
            );
        final batchIDPtr = _currentBatchID!.toNativeUtf8();
        resultPtr = cancelBatch(batchIDPtr);
        malloc.free(batchIDPtr);
      }
      freeString(resultPtr);
    } catch (e) {
      appLogger.error('[SkillSecurityAnalyzer] Cancel batch scan error', e);
    } finally {
      _pollTimer?.cancel();
      _currentBatchID = null;
      _currentAssetName = null;
    }
  }

  /// Delete a skill directory
  Future<bool> deleteSkill(String skillPath) async {
    try {
      final dylib = _getDylib();
      final deleteSkill = dylib.lookupFunction<DeleteSkillC, DeleteSkillDart>(
        'DeleteSkill',
      );
      final freeString = dylib.lookupFunction<FreeStringC, FreeStringDart>(
        'FreeString',
      );

      final skillPathPtr = skillPath.toNativeUtf8();
      final resultPtr = deleteSkill(skillPathPtr);
      malloc.free(skillPathPtr);

      final resultStr = resultPtr.toDartString();
      freeString(resultPtr);

      final result = jsonDecode(resultStr);
      if (result['success'] == true) {
        final skillName = path.basename(skillPath);
        await ScanDatabaseService().deleteSkillScan(skillName);
      }
      return result['success'] == true;
    } catch (e) {
      appLogger.error('[SkillSecurityAnalyzer] Delete skill error', e);
      return false;
    }
  }

  /// Trust a skill (mark as trusted so it doesn't appear in risky skills list)
  Future<bool> trustSkill(String skillName) async {
    try {
      final dylib = _getDylib();
      final trustSkillScan = dylib
          .lookupFunction<TrustSkillScanC, TrustSkillScanDart>(
            'TrustSkillScan',
          );
      final freeString = dylib.lookupFunction<FreeStringC, FreeStringDart>(
        'FreeString',
      );

      final skillNamePtr = skillName.toNativeUtf8();
      final resultPtr = trustSkillScan(skillNamePtr);
      malloc.free(skillNamePtr);

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
