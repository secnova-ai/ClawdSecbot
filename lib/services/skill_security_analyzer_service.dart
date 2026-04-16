import 'dart:async';
import 'dart:convert';

import 'package:path/path.dart' as path;

import '../core_transport/transport_registry.dart';
import '../models/llm_config_model.dart';
import '../utils/app_logger.dart';
import 'model_config_database_service.dart';
import 'scan_database_service.dart';

/// Batch scan progress info.
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

  Stream<String> get logStream => _logController.stream;
  Stream<BatchScanProgress> get progressStream => _progressController.stream;
  bool get isScanning => _currentBatchID != null;

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

    return SecurityModelConfig(
      provider: 'ollama',
      endpoint: 'http://localhost:11434',
      apiKey: '',
      model: 'llama3',
    );
  }

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

  Future<bool> saveConfig(SecurityModelConfig config) async {
    try {
      final dbService = ModelConfigDatabaseService();
      return await dbService.saveSecurityModelConfig(config);
    } catch (e) {
      appLogger.error('[SkillSecurityAnalyzer] Failed to save config to db', e);
      return false;
    }
  }

  Future<Map<String, dynamic>> testConnection(SecurityModelConfig config) async {
    final request = {
      'provider': config.provider,
      'endpoint': config.endpoint,
      'api_key': config.apiKey,
      'model': config.model,
      if (config.secretKey.isNotEmpty) 'secret_key': config.secretKey,
    };
    return _callOneArg('TestModelConnectionFFI', jsonEncode(request));
  }

  Future<Map<String, dynamic>> startBatchScan() async {
    if (_currentBatchID != null) {
      throw Exception('A batch scan is already in progress');
    }

    final result = _callNoArg('StartBatchSkillScan');
    if (result['success'] != true) {
      return result;
    }

    if ((result['total'] as int? ?? 0) == 0) {
      return result;
    }

    _currentBatchID = result['batch_id'] as String?;
    _startBatchPolling();
    return result;
  }

  void _startBatchPolling() {
    _pollTimer?.cancel();
    _pollTimer = Timer.periodic(const Duration(milliseconds: 200), (_) {
      if (_currentBatchID != null) {
        _pollBatchLogs(_currentBatchID!);
      }
    });
  }

  void _pollBatchLogs(String batchID) {
    try {
      final result = _callOneArg('GetBatchSkillScanLog', batchID);

      final logs = result['logs'] as List?;
      if (logs != null) {
        for (final log in logs) {
          _logController.add(log.toString());
        }
      }

      final progress = BatchScanProgress(
        logs: (logs ?? const []).map((e) => e.toString()).toList(),
        currentIndex: result['current_index'] as int? ?? 0,
        total: result['total'] as int? ?? 0,
        currentSkill: result['current_skill'] as String? ?? '',
        completed: result['completed'] as bool? ?? false,
        error: result['error'] as String?,
      );
      _progressController.add(progress);

      if (result['completed'] == true) {
        _pollTimer?.cancel();
        _currentBatchID = null;
        if ((result['error'] as String?)?.isNotEmpty == true) {
          _logController.add('Error: ${result['error']}');
        }
      }
    } catch (e) {
      appLogger.error('[SkillSecurityAnalyzer] Poll batch logs error', e);
    }
  }

  Future<Map<String, dynamic>> getBatchScanResults(String batchID) async {
    return _callOneArg('GetBatchSkillScanResults', batchID);
  }

  Future<void> cancelBatchScan() async {
    if (_currentBatchID == null) return;
    try {
      _callOneArg('CancelBatchSkillScan', _currentBatchID!);
    } catch (e) {
      appLogger.error('[SkillSecurityAnalyzer] Cancel batch scan error', e);
    } finally {
      _pollTimer?.cancel();
      _currentBatchID = null;
    }
  }

  Future<bool> deleteSkill(String skillPath) async {
    try {
      final result = _callOneArg('DeleteSkill', skillPath);
      if (result['success'] == true) {
        final skillName = path.basename(skillPath);
        await ScanDatabaseService().deleteSkillScan(skillName);
        return true;
      }
      return false;
    } catch (e) {
      appLogger.error('[SkillSecurityAnalyzer] Delete skill error', e);
      return false;
    }
  }

  Future<bool> trustSkill(String skillName) async {
    try {
      final result = _callOneArg('TrustSkillScan', skillName);
      return result['success'] == true;
    } catch (e) {
      appLogger.error('[SkillSecurityAnalyzer] Trust skill error', e);
      return false;
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
      appLogger.error('[SkillSecurityAnalyzer] $method failed', e);
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
      appLogger.error('[SkillSecurityAnalyzer] $method failed', e);
      return {'success': false, 'error': '$method failed: $e'};
    }
  }

  void dispose() {
    _pollTimer?.cancel();
    _logController.close();
    _progressController.close();
  }
}
