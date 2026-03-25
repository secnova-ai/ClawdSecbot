import 'dart:convert';
import 'dart:io';

import 'package:path/path.dart' as path;
import 'package:path_provider/path_provider.dart';

import '../utils/app_logger.dart';
import 'database_service.dart';
import 'native_library_service.dart';

/// API Server 状态信息。
class ApiServerStatus {
  final bool isRunning;
  final int? pid;
  final int? port;
  final String? token;
  final String? url;
  final DateTime? startedAt;

  ApiServerStatus({
    required this.isRunning,
    this.pid,
    this.port,
    this.token,
    this.url,
    this.startedAt,
  });

  factory ApiServerStatus.notRunning() {
    return ApiServerStatus(isRunning: false);
  }

  factory ApiServerStatus.fromDiscoveryFile(Map<String, dynamic> data) {
    return ApiServerStatus(
      isRunning: true,
      pid: data['pid'] as int?,
      port: data['port'] as int?,
      token: data['token'] as String?,
      url: data['url'] as String?,
      startedAt: data['startedAt'] != null
          ? DateTime.parse(data['startedAt'] as String)
          : null,
    );
  }
}

/// API 服务：管理 API Server 的启动、停止和状态检测。
class ApiService {
  static final ApiService _instance = ApiService._internal();

  factory ApiService() => _instance;

  ApiService._internal();

  /// 获取 api.json 路径，始终与 bot_sec_manager.db 同级。
  Future<String> getDiscoveryFilePath() async {
    await DatabaseService().init();
    final dbPath = DatabaseService().dbPath;
    if (dbPath != null && dbPath.isNotEmpty) {
      return path.join(path.dirname(dbPath), 'api.json');
    }

    // 回退：保持与 Application Support 目录一致。
    final supportDir = await getApplicationSupportDirectory();
    return path.join(supportDir.path, 'api.json');
  }

  Future<void> _cleanupLegacyDiscoveryFile() async {
    try {
      final appDir = await getApplicationDocumentsDirectory();
      final legacyFile = File(path.join(appDir.path, '.botsec', 'api.json'));
      if (await legacyFile.exists()) {
        await legacyFile.delete();
        appLogger.info(
          '[ApiService] Removed legacy discovery file: ${legacyFile.path}',
        );
      }
    } catch (e) {
      appLogger.warning(
        '[ApiService] Failed to cleanup legacy discovery file: $e',
      );
    }
  }

  Future<bool> _isProcessAlive(int pid) async {
    try {
      if (Platform.isWindows) {
        final result = await Process.run('tasklist', [
          '/FI',
          'PID eq $pid',
        ], runInShell: true);
        if (result.exitCode != 0) {
          return false;
        }
        final output = '${result.stdout}'.toLowerCase();
        return output.contains(' $pid ') || output.contains('\n$pid ');
      }

      final result = await Process.run('kill', [
        '-0',
        pid.toString(),
      ], runInShell: true);
      return result.exitCode == 0;
    } catch (e) {
      appLogger.debug('[ApiService] Process check failed: $e');
      return false;
    }
  }

  /// 检查 API Server 是否正在运行。
  Future<ApiServerStatus> checkStatus() async {
    try {
      await _cleanupLegacyDiscoveryFile();
      final discoveryPath = await getDiscoveryFilePath();
      final file = File(discoveryPath);

      if (!await file.exists()) {
        appLogger.debug('[ApiService] Discovery file not found');
        return ApiServerStatus.notRunning();
      }

      final content = await file.readAsString();
      final data = jsonDecode(content) as Map<String, dynamic>;
      final pid = data['pid'] as int?;
      if (pid != null && await _isProcessAlive(pid)) {
        appLogger.debug('[ApiService] API Server is running (PID: $pid)');
        return ApiServerStatus.fromDiscoveryFile(data);
      }

      try {
        await file.delete();
        appLogger.info('[ApiService] Cleaned up stale discovery file');
      } catch (e) {
        appLogger.warning('[ApiService] Failed to cleanup discovery file: $e');
      }

      return ApiServerStatus.notRunning();
    } catch (e) {
      appLogger.error('[ApiService] Failed to check status', e);
      return ApiServerStatus.notRunning();
    }
  }

  /// 启动 API Server。
  ///
  /// [port] 可选，默认 0（自动分配可用端口）。
  Future<Map<String, dynamic>> startServer({int port = 0}) async {
    try {
      appLogger.info(
        '[ApiService] Starting API Server (port: ${port == 0 ? 'auto' : port})',
      );
      await _cleanupLegacyDiscoveryFile();

      final result = await NativeLibraryService().startApiServer(port: port);
      if (result['success'] == true) {
        appLogger.info('[ApiService] API Server started successfully');
        await Future.delayed(const Duration(milliseconds: 100));
      } else {
        appLogger.error(
          '[ApiService] Failed to start API Server: ${result['error']}',
        );
      }

      return result;
    } catch (e) {
      appLogger.error('[ApiService] startServer failed', e);
      return {'success': false, 'error': 'startServer failed: $e'};
    }
  }

  /// 停止 API Server。
  Future<Map<String, dynamic>> stopServer() async {
    try {
      appLogger.info('[ApiService] Stopping API Server');
      final result = await NativeLibraryService().stopApiServer();
      if (result['success'] == true) {
        appLogger.info('[ApiService] API Server stopped successfully');
        await Future.delayed(const Duration(milliseconds: 100));
      } else {
        appLogger.error(
          '[ApiService] Failed to stop API Server: ${result['error']}',
        );
      }

      return result;
    } catch (e) {
      appLogger.error('[ApiService] stopServer failed', e);
      return {'success': false, 'error': 'stopServer failed: $e'};
    }
  }

  /// 切换 API Server 状态。
  Future<Map<String, dynamic>> toggleServer(bool enable) async {
    if (enable) {
      return startServer();
    }
    return stopServer();
  }
}
