import 'dart:convert';
import 'dart:io';

import 'package:path/path.dart' as path;

import 'database_service.dart';

/// 应用配置服务，负责读写应用数据目录中的配置文件。
class AppConfigService {
  static final AppConfigService _instance = AppConfigService._internal();

  factory AppConfigService() => _instance;

  AppConfigService._internal();

  static const String _configFileName = 'app_config.json';
  static const String _logDirKey = 'log_dir';
  static const String _sandboxDirKey = 'sandbox_dir';
  static const String _installDirKey = 'install_dir';

  /// 获取当前生效的日志目录。
  ///
  /// 返回 `null` 表示配置中尚未设置日志目录。
  Future<String?> getLogDir() async {
    final config = await _loadConfig();
    final logDir = config[_logDirKey];
    if (logDir is! String) {
      return null;
    }
    final trimmed = logDir.trim();
    if (trimmed.isEmpty) {
      return null;
    }
    return _normalizeLogDir(trimmed);
  }

  /// 获取沙箱目录根路径（例如 `~/.botsec`）。
  Future<String> getSandboxDir() async {
    final config = await _loadConfig();
    final configured = config[_sandboxDirKey];
    if (configured is String && configured.trim().isNotEmpty) {
      return _normalizeDir(configured);
    }
    final fallback = _resolveDefaultSandboxDir();
    config[_sandboxDirKey] = fallback;
    await _writeConfig(config);
    return fallback;
  }

  /// 获取安装目录根路径（与数据库同级根目录）。
  Future<String> getInstallDir() async {
    final config = await _loadConfig();
    final configured = config[_installDirKey];
    if (configured is String && configured.trim().isNotEmpty) {
      return _normalizeDir(configured);
    }
    final fallback = await _resolveDefaultInstallDir();
    config[_installDirKey] = fallback;
    await _writeConfig(config);
    return fallback;
  }

  /// 设置日志目录并写入配置文件。
  Future<void> setLogDir(String logDir) async {
    final normalizedDir = _normalizeLogDir(logDir);
    final config = await _loadConfig();
    config[_logDirKey] = normalizedDir;
    await _writeConfig(config);
  }

  /// 加载配置文件内容，不存在时自动创建默认配置。
  Future<Map<String, dynamic>> _loadConfig() async {
    final configFile = await _ensureConfigFile();
    try {
      final content = await configFile.readAsString();
      final decoded = jsonDecode(content);
      if (decoded is Map<String, dynamic>) {
        final original = Map<String, dynamic>.from(decoded);
        final merged = _applyDefaults(decoded);
        var changed = merged.length != original.length;
        if (merged[_installDirKey] is! String ||
            (merged[_installDirKey] as String).trim().isEmpty) {
          merged[_installDirKey] = await _resolveDefaultInstallDir();
          changed = true;
        }
        if (!changed) {
          for (final entry in merged.entries) {
            if (original[entry.key] != entry.value) {
              changed = true;
              break;
            }
          }
        }
        if (changed) {
          await _writeConfig(merged);
        }
        return merged;
      }
    } catch (_) {
      // 解析失败时回退为默认配置。
    }

    final fallback = await _defaultConfig();
    await _writeConfig(fallback);
    return fallback;
  }

  /// 确保配置文件存在，缺失时创建默认文件。
  Future<File> _ensureConfigFile() async {
    final configPath = await _resolveConfigPath();
    final configFile = File(configPath);
    if (!await configFile.exists()) {
      await _writeConfig(await _defaultConfig());
    }
    return configFile;
  }

  /// 通过临时文件 + 重命名方式原子写入配置。
  Future<void> _writeConfig(Map<String, dynamic> config) async {
    final configPath = await _resolveConfigPath();
    final configFile = File(configPath);
    final tmpFile = File('$configPath.tmp');
    final content = const JsonEncoder.withIndent('  ').convert(config);
    await tmpFile.writeAsString('$content\n', flush: true);
    if (await configFile.exists()) {
      await configFile.delete();
    }
    await tmpFile.rename(configPath);
  }

  /// 构造默认配置。
  Future<Map<String, dynamic>> _defaultConfig() async {
    return <String, dynamic>{
      _logDirKey: '',
      _sandboxDirKey: _resolveDefaultSandboxDir(),
      _installDirKey: await _resolveDefaultInstallDir(),
    };
  }

  /// 为旧配置补齐默认字段。
  Map<String, dynamic> _applyDefaults(Map<String, dynamic> config) {
    final merged = Map<String, dynamic>.from(config);
    merged.putIfAbsent(_logDirKey, () => '');
    merged.putIfAbsent(_sandboxDirKey, _resolveDefaultSandboxDir);
    merged.putIfAbsent(_installDirKey, () => '');
    return merged;
  }

  /// 解析配置文件路径（应用数据目录，与数据库同级）。
  Future<String> _resolveConfigPath() async {
    var appDataDir = DatabaseService().appDataDir;
    if (appDataDir == null || appDataDir.trim().isEmpty) {
      await DatabaseService().init();
      appDataDir = DatabaseService().appDataDir;
    }
    if (appDataDir == null || appDataDir.trim().isEmpty) {
      throw StateError('app data directory is unavailable');
    }
    return path.join(appDataDir, _configFileName);
  }

  /// 解析默认沙箱根目录。
  String _resolveDefaultSandboxDir() {
    final homeDir =
        Platform.environment['HOME'] ??
        Platform.environment['USERPROFILE'] ??
        '';
    if (homeDir.trim().isEmpty) {
      return _normalizeDir(path.join(Directory.current.path, '.botsec'));
    }
    return _normalizeDir(path.join(homeDir, '.botsec'));
  }

  /// 解析默认安装根目录。
  Future<String> _resolveDefaultInstallDir() async {
    var appDataDir = DatabaseService().appDataDir;
    if (appDataDir == null || appDataDir.trim().isEmpty) {
      await DatabaseService().init();
      appDataDir = DatabaseService().appDataDir;
    }
    if (appDataDir == null || appDataDir.trim().isEmpty) {
      return _normalizeDir(Directory.current.path);
    }
    return _normalizeDir(appDataDir);
  }

  /// 标准化日志目录路径。
  String _normalizeLogDir(String logDir) {
    return _normalizeDir(logDir);
  }

  /// 标准化目录路径。
  String _normalizeDir(String dirPath) {
    final trimmed = dirPath.trim();
    if (trimmed.isEmpty) {
      throw ArgumentError('dirPath must not be empty');
    }
    final absolutePath = path.isAbsolute(trimmed)
        ? trimmed
        : path.absolute(trimmed);
    return path.normalize(absolutePath);
  }
}
