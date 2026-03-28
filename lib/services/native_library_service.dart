import 'dart:ffi' as ffi;
import 'dart:io';
import 'dart:convert';
import 'package:ffi/ffi.dart';
import 'package:package_info_plus/package_info_plus.dart';
import 'package:path/path.dart' as path;
import '../utils/app_logger.dart';
import 'database_service.dart';

// FFI type definitions for core Go functions
typedef _InitPathsFFIC =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>, ffi.Pointer<Utf8>);
typedef _InitPathsFFIDart =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>, ffi.Pointer<Utf8>);

typedef _InitLoggingFFIC = ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);
typedef _InitLoggingFFIDart = ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);

typedef _InitDatabaseC = ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);
typedef _InitDatabaseDart = ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);

typedef _CloseDatabaseC = ffi.Pointer<Utf8> Function();
typedef _CloseDatabaseDart = ffi.Pointer<Utf8> Function();

typedef _FreeStringC = ffi.Void Function(ffi.Pointer<Utf8>);
typedef FreeStringDart = void Function(ffi.Pointer<Utf8>);

/// 原生库服务：管理Go动态库（dylib）加载和全局Go运行时（日志、数据库）生命周期
///
/// 该服务独立于任何插件业务逻辑，负责：
/// 1. 查找并加载Go动态库（dylib/so/dll）
/// 2. 初始化Go日志系统
/// 3. 初始化和关闭Go全局数据库
///
/// 所有需要使用Go FFI的服务都应从此服务获取 [dylib] 和 [freeString]。
class NativeLibraryService {
  static final NativeLibraryService _instance =
      NativeLibraryService._internal();

  factory NativeLibraryService() => _instance;

  NativeLibraryService._internal();

  /// 缓存的dylib实例
  static ffi.DynamicLibrary? _cachedDylib;

  /// 缓存的dylib文件路径（用于在后台 Isolate 中重新打开）
  static String? _cachedLibPath;

  /// 缓存的FreeString函数
  static FreeStringDart? _cachedFreeString;

  /// 是否已完成初始化
  static bool _initialized = false;

  /// 获取dylib实例
  ffi.DynamicLibrary? get dylib => _cachedDylib;

  /// 获取dylib文件路径（用于后台 Isolate 中 DynamicLibrary.open）
  String? get libraryPath => _cachedLibPath;

  /// 获取FreeString函数
  FreeStringDart? get freeString => _cachedFreeString;

  /// 是否已初始化
  bool get isInitialized => _initialized;

  /// 初始化原生库：加载dylib + 初始化Go日志 + 初始化Go数据库
  ///
  /// 在应用启动时调用，在 DatabaseService.init() 之后
  Future<void> initialize() async {
    if (_initialized) return;

    final pluginDir = _getPluginDirectory();
    if (!await pluginDir.exists()) {
      appLogger.warning(
        '[NativeLib] Plugin directory not found: ${pluginDir.path}',
      );
      return;
    }

    // 查找并加载dylib
    await for (final entity in pluginDir.list()) {
      if (entity is File && _isSharedLibrary(entity.path)) {
        try {
          final dylib = ffi.DynamicLibrary.open(entity.path);
          final freeStr = dylib.lookupFunction<_FreeStringC, FreeStringDart>(
            'FreeString',
          );

          _cachedDylib = dylib;
          _cachedLibPath = entity.path;
          _cachedFreeString = freeStr;
          appLogger.info('[NativeLib] Library loaded: ${entity.path}');

          // 初始化Go路径管理器
          _initGoPaths(dylib, freeStr);

          // 初始化Go日志
          _initGoLogging(dylib, freeStr);

          // 初始化Go数据库
          await _initGoDatabase(dylib, freeStr);

          _initialized = true;
          appLogger.info('[NativeLib] Native library initialized successfully');
          return;
        } catch (e) {
          appLogger.error(
            '[NativeLib] Failed to load library: ${entity.path}',
            e,
          );
        }
      }
    }

    appLogger.warning(
      '[NativeLib] No shared library found in: ${pluginDir.path}',
    );
  }

  /// 关闭原生库：关闭Go数据库连接
  ///
  /// 在应用退出时调用
  Future<void> close() async {
    if (!_initialized || _cachedDylib == null) return;

    try {
      final closeDatabase = _cachedDylib!
          .lookupFunction<_CloseDatabaseC, _CloseDatabaseDart>('CloseDatabase');
      final resultPtr = closeDatabase();
      final result = resultPtr.toDartString();
      _cachedFreeString!(resultPtr);
      appLogger.info('[NativeLib] Go DB closed: $result');
    } catch (e) {
      appLogger.error('[NativeLib] Failed to close Go DB: $e');
    }

    _initialized = false;
    appLogger.info('[NativeLib] Native library closed');
  }

  /// 通用FFI调用辅助：发送JSON请求并返回JSON结果
  Map<String, dynamic> callFFI(
    String funcName,
    Map<String, dynamic> Function(ffi.DynamicLibrary dylib) executor,
  ) {
    if (_cachedDylib == null) {
      return {'success': false, 'error': 'Native library not initialized'};
    }

    try {
      return executor(_cachedDylib!);
    } catch (e) {
      appLogger.debug('[NativeLib] $funcName failed: $e');
      return {'success': false, 'error': '$funcName failed: $e'};
    }
  }

  /// 初始化Go路径管理器
  void _initGoPaths(ffi.DynamicLibrary dylib, FreeStringDart freeStr) {
    final appDataDir = DatabaseService().appDataDir;
    if (appDataDir == null) {
      appLogger.warning(
        '[NativeLib] Cannot init Go paths: app data dir unavailable',
      );
      return;
    }
    // homeDir 使用用户主目录
    final homeDir =
        Platform.environment['HOME'] ??
        Platform.environment['USERPROFILE'] ??
        '';

    try {
      final initPathsFFI = dylib
          .lookupFunction<_InitPathsFFIC, _InitPathsFFIDart>('InitPathsFFI');
      final workspaceDirPtr = appDataDir.toNativeUtf8();
      final homeDirPtr = homeDir.toNativeUtf8();
      final resultPtr = initPathsFFI(workspaceDirPtr, homeDirPtr);
      final result = resultPtr.toDartString();
      freeStr(resultPtr);
      malloc.free(workspaceDirPtr);
      malloc.free(homeDirPtr);
      appLogger.info('[NativeLib] Go paths initialized: $result');
    } catch (e) {
      appLogger.debug('[NativeLib] InitPathsFFI not available: $e');
    }
  }

  /// 初始化Go日志
  void _initGoLogging(ffi.DynamicLibrary dylib, FreeStringDart freeStr) {
    try {
      final initLoggingFFI = dylib
          .lookupFunction<_InitLoggingFFIC, _InitLoggingFFIDart>(
            'InitLoggingFFI',
          );
      final logDirPtr = ''.toNativeUtf8();
      final resultPtr = initLoggingFFI(logDirPtr);
      final result = resultPtr.toDartString();
      freeStr(resultPtr);
      malloc.free(logDirPtr);
      appLogger.info('[NativeLib] Go logging initialized: $result');
    } catch (e) {
      appLogger.debug('[NativeLib] InitLoggingFFI not available: $e');
    }
  }

  /// 初始化Go数据库
  Future<void> _initGoDatabase(
    ffi.DynamicLibrary dylib,
    FreeStringDart freeStr,
  ) async {
    if (DatabaseService().appDataDir == null) {
      appLogger.warning(
        '[NativeLib] Cannot init Go DB: app data dir unavailable',
      );
      return;
    }

    try {
      final packageInfo = await PackageInfo.fromPlatform();
      final initDatabase = dylib
          .lookupFunction<_InitDatabaseC, _InitDatabaseDart>('InitDatabase');
      final requestJson = jsonEncode({'current_version': packageInfo.version});
      final requestPtr = requestJson.toNativeUtf8();
      final resultPtr = initDatabase(requestPtr);
      final result = resultPtr.toDartString();
      freeStr(resultPtr);
      malloc.free(requestPtr);
      appLogger.info('[NativeLib] Go DB initialized: $result');
    } catch (e) {
      appLogger.debug('[NativeLib] InitDatabase not available: $e');
    }
  }

  /// 获取插件目录
  Directory _getPluginDirectory() {
    // For macOS App Bundle, try Resources/plugins first
    if (Platform.isMacOS) {
      try {
        final executablePath = Platform.resolvedExecutable;
        final appDir = Directory(executablePath).parent.parent;
        final resourcesPlugins = path.join(appDir.path, 'Resources', 'plugins');

        if (Directory(resourcesPlugins).existsSync()) {
          appLogger.info(
            '[NativeLib] Using App Bundle plugins: $resourcesPlugins',
          );
          return Directory(resourcesPlugins);
        }
      } catch (e) {
        appLogger.debug('[NativeLib] Failed to locate App Bundle plugins: $e');
      }
    }

    // Fallback: try multiple possible locations
    // 1. Current directory + plugins (for flutter run from project root)
    var dirPath = path.join(Directory.current.path, 'plugins');
    if (Directory(dirPath).existsSync()) {
      appLogger.info('[NativeLib] Using plugins from current dir: $dirPath');
      return Directory(dirPath);
    }

    // 2. Executable's parent + plugins (for development builds)
    try {
      final execDir = Directory(Platform.resolvedExecutable).parent.path;
      final execPluginsPath = path.join(execDir, 'plugins');
      if (Directory(execPluginsPath).existsSync()) {
        appLogger.info(
          '[NativeLib] Using plugins from executable dir: $execPluginsPath',
        );
        return Directory(execPluginsPath);
      }
    } catch (e) {
      appLogger.debug('[NativeLib] Failed to locate plugins from exec: $e');
    }

    // 3. Try parent directories (up to 3 levels) for flutter run in nested dirs
    var currentDir = Directory.current;
    for (var i = 0; i < 3; i++) {
      final pluginsPath = path.join(currentDir.path, 'plugins');
      if (Directory(pluginsPath).existsSync()) {
        appLogger.info(
          '[NativeLib] Using plugins from parent $i: $pluginsPath',
        );
        return Directory(pluginsPath);
      }
      final parent = currentDir.parent;
      if (parent.path == currentDir.path) break; // reached root
      currentDir = parent;
    }

    // 4. Last resort: return current directory + plugins (will fail gracefully)
    appLogger.warning(
      '[NativeLib] No plugins directory found, using: $dirPath',
    );
    return Directory(dirPath);
  }

  /// 判断文件是否为动态库
  bool _isSharedLibrary(String filePath) {
    final ext = path.extension(filePath);
    if (Platform.isMacOS) return ext == '.dylib';
    if (Platform.isWindows) return ext == '.dll';
    if (Platform.isLinux) return ext == '.so';
    return false;
  }
}
