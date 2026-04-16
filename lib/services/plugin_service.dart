import 'dart:convert';
import 'dart:ffi' as ffi;
import 'dart:io';
import 'dart:isolate';
import 'package:ffi/ffi.dart';
import 'package:flutter/material.dart';
import '../models/asset_model.dart';
import '../models/risk_model.dart';
import '../config/build_config.dart';
import '../utils/app_logger.dart';
import 'native_library_service.dart';
import 'bookmark_service.dart';
import 'protection_database_service.dart';

// C function signatures

// 资产扫描FFI：无参数，返回 {"success":true,"data":[...],"count":N}
typedef ScanAssetsFFIC = ffi.Pointer<Utf8> Function();
typedef ScanAssetsFFIDart = ffi.Pointer<Utf8> Function();

// 风险评估FFI：接收scannedHashes JSON，返回 {"success":true,"data":[...],"count":N}
typedef AssessRisksFFIC = ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);
typedef AssessRisksFFIDart = ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);

typedef MitigateRiskC = ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);
typedef MitigateRiskDart = ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);

typedef SetConfigPathC = ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);
typedef SetConfigPathDart = ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);

typedef SetAppStoreBuildC = ffi.Pointer<Utf8> Function(ffi.Int32);
typedef SetAppStoreBuildDart = ffi.Pointer<Utf8> Function(int);
typedef GetPluginsC = ffi.Pointer<Utf8> Function();
typedef GetPluginsDart = ffi.Pointer<Utf8> Function();

// Database FFI signatures (used by model config methods)
typedef SaveScanResultC = ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);
typedef SaveScanResultDart = ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);

typedef GetLatestScanResultC = ffi.Pointer<Utf8> Function();
typedef GetLatestScanResultDart = ffi.Pointer<Utf8> Function();

typedef GetScannedSkillHashesC = ffi.Pointer<Utf8> Function();
typedef GetScannedSkillHashesDart = ffi.Pointer<Utf8> Function();

typedef SaveSkillScanResultC = ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);
typedef SaveSkillScanResultDart = ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);

typedef GetSkillScanByHashC = ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);
typedef GetSkillScanByHashDart = ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);

typedef DeleteSkillScanC = ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);
typedef DeleteSkillScanDart = ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);

typedef GetRiskySkillsC = ffi.Pointer<Utf8> Function();
typedef GetRiskySkillsDart = ffi.Pointer<Utf8> Function();

typedef ListBundledReActSkillsC = ffi.Pointer<Utf8> Function();
typedef ListBundledReActSkillsDart = ffi.Pointer<Utf8> Function();

typedef NotifyPluginAppExitC =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>, ffi.Pointer<Utf8>);
typedef NotifyPluginAppExitDart =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>, ffi.Pointer<Utf8>);

typedef RestoreBotDefaultStateC =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>, ffi.Pointer<Utf8>);
typedef RestoreBotDefaultStateDart =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>, ffi.Pointer<Utf8>);

// Model config FFI signatures
typedef SaveSecurityModelConfigC =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);
typedef SaveSecurityModelConfigDart =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);

typedef GetSecurityModelConfigC = ffi.Pointer<Utf8> Function();
typedef GetSecurityModelConfigDart = ffi.Pointer<Utf8> Function();

typedef SaveBotModelConfigC = ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);
typedef SaveBotModelConfigDart = ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);

typedef GetBotModelConfigC = ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);
typedef GetBotModelConfigDart = ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);

typedef DeleteBotModelConfigC = ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);
typedef DeleteBotModelConfigDart =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);

typedef FreeStringC = ffi.Void Function(ffi.Pointer<Utf8>);
typedef FreeStringDart = void Function(ffi.Pointer<Utf8>);

class _PluginLifecycleFFI {
  _PluginLifecycleFFI._();

  static String notifyPluginAppExitInIsolate(
    String libPath,
    String assetName,
    String assetID,
  ) {
    final dylib = ffi.DynamicLibrary.open(libPath);
    final func = dylib
        .lookupFunction<NotifyPluginAppExitC, NotifyPluginAppExitDart>(
          'NotifyPluginAppExitFFI',
        );
    final freeString = dylib.lookupFunction<FreeStringC, FreeStringDart>(
      'FreeString',
    );

    final assetNamePtr = assetName.toNativeUtf8();
    final assetIDPtr = assetID.toNativeUtf8();
    final resultPtr = func(assetNamePtr, assetIDPtr);
    malloc.free(assetNamePtr);
    malloc.free(assetIDPtr);

    final result = resultPtr.toDartString();
    freeString(resultPtr);
    return result;
  }

  static String restoreBotDefaultStateInIsolate(
    String libPath,
    String assetName,
    String assetID,
  ) {
    final dylib = ffi.DynamicLibrary.open(libPath);
    final func = dylib
        .lookupFunction<RestoreBotDefaultStateC, RestoreBotDefaultStateDart>(
          'RestoreBotDefaultStateFFI',
        );
    final freeString = dylib.lookupFunction<FreeStringC, FreeStringDart>(
      'FreeString',
    );

    final assetNamePtr = assetName.toNativeUtf8();
    final assetIDPtr = assetID.toNativeUtf8();
    final resultPtr = func(assetNamePtr, assetIDPtr);
    malloc.free(assetNamePtr);
    malloc.free(assetIDPtr);

    final result = resultPtr.toDartString();
    freeString(resultPtr);
    return result;
  }
}

/// 插件服务：管理插件专有的业务逻辑（扫描/识别/风险评估/模型配置等）
///
/// dylib加载和Go全局运行时（日志/DB）由 [NativeLibraryService] 管理。
/// 本服务在 [initializePlugin] 时仅处理插件专有的配置（SetConfigPath / SetAppStoreBuild）。
class PluginService {
  static final PluginService _instance = PluginService._internal();

  factory PluginService() => _instance;

  PluginService._internal();

  /// 标记插件是否已完成初始化
  static bool _initialized = false;

  /// 从NativeLibraryService获取dylib
  ffi.DynamicLibrary? get dylib => NativeLibraryService().dylib;

  /// 从NativeLibraryService获取FreeString函数
  FreeStringDart? get freeString => NativeLibraryService().freeString;

  /// 插件是否已初始化
  bool get isInitialized => _initialized;

  /// 初始化插件：仅处理插件专有配置
  ///
  /// 前置条件：NativeLibraryService 已初始化完成
  Future<void> initializePlugin() async {
    if (_initialized) return;

    final nativeLib = NativeLibraryService();
    if (!nativeLib.isInitialized || nativeLib.dylib == null) {
      appLogger.warning(
        '[Plugin] NativeLibraryService not initialized, cannot init plugin',
      );
      return;
    }

    final lib = nativeLib.dylib!;
    final freeStr = nativeLib.freeString!;

    // 仅处理插件专有配置
    _initConfigPath(lib, freeStr);
    _initAppStoreBuild(lib, freeStr);

    _initialized = true;
    appLogger.info('[Plugin] Plugin initialized successfully');
  }

  /// 关闭插件：仅处理插件专有清理
  ///
  /// Go DB的关闭由NativeLibraryService负责
  Future<void> closePlugin() async {
    if (!_initialized) return;
    _initialized = false;
    appLogger.info('[Plugin] Plugin closed');
  }

  /// 设置授权路径（仅macOS App Store版本需要）
  void _initConfigPath(ffi.DynamicLibrary dylib, FreeStringDart freeStr) {
    if (!Platform.isMacOS || !BuildConfig.requiresDirectoryAuth) return;

    try {
      final authorizedPath = BookmarkService().authorizedPath;
      if (authorizedPath == null) {
        appLogger.warning('[Plugin] Authorized path is null');
        return;
      }

      final setConfigPath = dylib
          .lookupFunction<SetConfigPathC, SetConfigPathDart>(
            'SetConfigPathFFI',
          );
      final pathPtr = authorizedPath.toNativeUtf8();
      final resultPtr = setConfigPath(pathPtr);
      final result = resultPtr.toDartString();
      freeStr(resultPtr);
      malloc.free(pathPtr);
      appLogger.info('[Plugin] Config path set: $result');
    } catch (e) {
      appLogger.debug('[Plugin] SetConfigPathFFI not available: $e');
    }
  }

  /// 设置App Store版本标志
  void _initAppStoreBuild(ffi.DynamicLibrary dylib, FreeStringDart freeStr) {
    if (!BuildConfig.isAppStore) return;

    try {
      final setAppStoreBuild = dylib
          .lookupFunction<SetAppStoreBuildC, SetAppStoreBuildDart>(
            'SetAppStoreBuildFFI',
          );
      final resultPtr = setAppStoreBuild(1);
      final result = resultPtr.toDartString();
      freeStr(resultPtr);
      appLogger.info('[Plugin] AppStore build flag set: $result');
    } catch (e) {
      appLogger.debug('[Plugin] SetAppStoreBuildFFI not available: $e');
    }
  }

  /// Returns whether the target plugin requires explicit bot model config.
  ///
  /// Fallback is `true` for safety when plugin metadata cannot be read.
  Future<bool> requiresBotModelConfig(String assetName) async {
    final normalizedAssetName = assetName.trim().toLowerCase();
    if (normalizedAssetName.isEmpty) {
      return true;
    }

    final response = await _withPlugin('GetPluginsFFI', (lib) {
      final getPlugins = lib.lookupFunction<GetPluginsC, GetPluginsDart>(
        'GetPluginsFFI',
      );
      final resultPtr = getPlugins();
      final result = resultPtr.toDartString();
      freeString!(resultPtr);
      return jsonDecode(result) as Map<String, dynamic>;
    });

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

  /// 通过FFI获取内置ReAct安全技能列表（name + description）
  List<Map<String, dynamic>> listBundledReActSkills() {
    final lib = dylib;
    if (lib == null) {
      appLogger.warning(
        '[Plugin] Plugin not initialized, cannot list bundled skills',
      );
      return const [];
    }

    try {
      final listSkills = lib
          .lookupFunction<ListBundledReActSkillsC, ListBundledReActSkillsDart>(
            'ListBundledReActSkillsFFI',
          );
      final resultPtr = listSkills();
      final result = resultPtr.toDartString();
      freeString!(resultPtr);

      final decoded = jsonDecode(result);
      if (decoded is! Map<String, dynamic>) return const [];
      if (decoded['success'] != true) return const [];

      final data = decoded['data'];
      if (data is! List) return const [];

      return data.whereType<Map<String, dynamic>>().toList(growable: false);
    } catch (e) {
      appLogger.error('[Plugin] Failed to list bundled ReAct skills', e);
      return const [];
    }
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
    final libPath = NativeLibraryService().libraryPath;
    if (libPath == null) {
      return {'success': false, 'error': 'Plugin library not initialized'};
    }

    try {
      final result = await Isolate.run(() {
        return _PluginLifecycleFFI.notifyPluginAppExitInIsolate(
          libPath,
          assetName,
          assetID,
        );
      });
      return jsonDecode(result) as Map<String, dynamic>;
    } catch (e) {
      appLogger.debug('[Plugin] NotifyPluginAppExitFFI failed: $e');
      return {'success': false, 'error': 'NotifyPluginAppExitFFI failed: $e'};
    }
  }

  Future<Map<String, dynamic>> restoreBotDefaultState(
    String assetName, [
    String assetID = '',
  ]) async {
    final libPath = NativeLibraryService().libraryPath;
    if (libPath == null) {
      return {'success': false, 'error': 'Plugin library not initialized'};
    }

    try {
      final result = await Isolate.run(() {
        return _PluginLifecycleFFI.restoreBotDefaultStateInIsolate(
          libPath,
          assetName,
          assetID,
        );
      });
      return jsonDecode(result) as Map<String, dynamic>;
    } catch (e) {
      appLogger.debug('[Plugin] RestoreBotDefaultStateFFI failed: $e');
      return {
        'success': false,
        'error': 'RestoreBotDefaultStateFFI failed: $e',
      };
    }
  }

  /// 通过FFI调用Go层保存安全模型配置
  Future<Map<String, dynamic>> saveSecurityModelConfig(
    Map<String, dynamic> config,
  ) async {
    return _withPlugin('SaveSecurityModelConfigFFI', (lib) {
      final func = lib
          .lookupFunction<
            SaveSecurityModelConfigC,
            SaveSecurityModelConfigDart
          >('SaveSecurityModelConfigFFI');
      final jsonStr = jsonEncode(config);
      final jsonPtr = jsonStr.toNativeUtf8();
      final resultPtr = func(jsonPtr);
      final result = resultPtr.toDartString();
      freeString!(resultPtr);
      malloc.free(jsonPtr);
      return jsonDecode(result) as Map<String, dynamic>;
    });
  }

  /// 通过FFI调用Go层获取安全模型配置
  Future<Map<String, dynamic>> getSecurityModelConfig() async {
    return _withPlugin('GetSecurityModelConfigFFI', (lib) {
      final func = lib
          .lookupFunction<GetSecurityModelConfigC, GetSecurityModelConfigDart>(
            'GetSecurityModelConfigFFI',
          );
      final resultPtr = func();
      final result = resultPtr.toDartString();
      freeString!(resultPtr);
      return jsonDecode(result) as Map<String, dynamic>;
    });
  }

  /// 通过FFI调用Go层保存Bot模型配置（按资产名称关联）
  Future<Map<String, dynamic>> saveBotModelConfig(
    Map<String, dynamic> config,
  ) async {
    return _withPlugin('SaveBotModelConfigFFI', (lib) {
      final func = lib
          .lookupFunction<SaveBotModelConfigC, SaveBotModelConfigDart>(
            'SaveBotModelConfigFFI',
          );
      final jsonStr = jsonEncode(config);
      final jsonPtr = jsonStr.toNativeUtf8();
      final resultPtr = func(jsonPtr);
      final result = resultPtr.toDartString();
      freeString!(resultPtr);
      malloc.free(jsonPtr);
      return jsonDecode(result) as Map<String, dynamic>;
    });
  }

  /// 通过FFI调用Go层获取指定资产的Bot模型配置
  Future<Map<String, dynamic>> getBotModelConfig(
    String assetName, [
    String assetID = '',
  ]) async {
    return _withPlugin('GetBotModelConfigFFI', (lib) {
      final func = lib
          .lookupFunction<GetBotModelConfigC, GetBotModelConfigDart>(
            'GetBotModelConfigFFI',
          );
      final idPtr = assetID.toNativeUtf8();
      final resultPtr = func(idPtr);
      final result = resultPtr.toDartString();
      freeString!(resultPtr);
      malloc.free(idPtr);
      return jsonDecode(result) as Map<String, dynamic>;
    });
  }

  /// 通过FFI调用Go层删除指定资产的Bot模型配置
  Future<Map<String, dynamic>> deleteBotModelConfig(
    String assetName, [
    String assetID = '',
  ]) async {
    return _withPlugin('DeleteBotModelConfigFFI', (lib) {
      final func = lib
          .lookupFunction<DeleteBotModelConfigC, DeleteBotModelConfigDart>(
            'DeleteBotModelConfigFFI',
          );
      final idPtr = assetID.toNativeUtf8();
      final resultPtr = func(idPtr);
      final result = resultPtr.toDartString();
      freeString!(resultPtr);
      malloc.free(idPtr);
      return jsonDecode(result) as Map<String, dynamic>;
    });
  }

  /// 辅助方法：使用dylib执行回调
  Future<Map<String, dynamic>> _withPlugin(
    String funcName,
    Map<String, dynamic> Function(ffi.DynamicLibrary lib) executor,
  ) async {
    final lib = dylib;
    if (lib == null) {
      return {'success': false, 'error': 'Plugin not initialized'};
    }

    try {
      return executor(lib);
    } catch (e) {
      appLogger.debug('[Plugin] $funcName failed: $e');
      return {'success': false, 'error': '$funcName failed: $e'};
    }
  }

  Future<List<RiskInfo>> assessRisksOnly() async {
    final lib = dylib;
    if (lib == null) {
      appLogger.warning('[Plugin] Plugin not initialized, cannot assess risks');
      return const [];
    }

    final freeStr = freeString!;
    final allRisks = <RiskInfo>[];

    try {
      final assessRisks = lib.lookupFunction<AssessRisksFFIC, AssessRisksFFIDart>(
        'AssessRisksFFI',
      );

      String scannedHashesJson = '[]';
      try {
        final getHashes = lib
            .lookupFunction<GetScannedSkillHashesC, GetScannedSkillHashesDart>(
              'GetScannedSkillHashes',
            );
        final hashesResultPtr = getHashes();
        final hashesResult = hashesResultPtr.toDartString();
        freeStr(hashesResultPtr);
        final hashesResponse = jsonDecode(hashesResult);
        if (hashesResponse['success'] == true && hashesResponse['data'] != null) {
          scannedHashesJson = jsonEncode(hashesResponse['data']);
        }
      } catch (e) {
        appLogger.debug('[Plugin] GetScannedSkillHashes not available: $e');
      }

      final hashesPtr = scannedHashesJson.toNativeUtf8();
      final riskPtr = assessRisks(hashesPtr);
      malloc.free(hashesPtr);

      final riskJsonString = riskPtr.toDartString();
      freeStr(riskPtr);

      final riskResponse = jsonDecode(riskJsonString) as Map<String, dynamic>;
      if (riskResponse['success'] == true && riskResponse['data'] != null) {
        final List<dynamic> jsonList = riskResponse['data'] as List<dynamic>;
        for (var item in jsonList) {
          allRisks.add(_parseRisk(item as Map<String, dynamic>));
        }
      }
    } catch (e) {
      appLogger.error('[Plugin] AssessRisksFFI failed: $e');
    }

    return allRisks;
  }

  Future<ScanResult> scan() async {
    List<Asset> allAssets = [];
    List<RiskInfo> allRisks = [];

    final lib = dylib;
    if (lib == null) {
      appLogger.warning('[Plugin] Plugin not initialized, cannot scan');
      return ScanResult(risks: [], configFound: false);
    }

    final freeStr = freeString!;

    appLogger.info('[Plugin] Starting scan');

    try {
      // 1. 通过PluginManager聚合扫描所有插件的资产
      try {
        appLogger.debug('[Plugin] Calling ScanAssetsFFI...');
        final scanAssets = lib
            .lookupFunction<ScanAssetsFFIC, ScanAssetsFFIDart>('ScanAssetsFFI');
        final resultPtr = scanAssets();
        final resultJson = resultPtr.toDartString();
        freeStr(resultPtr);

        appLogger.info('[Plugin] ScanAssetsFFI result: $resultJson');

        final response = jsonDecode(resultJson) as Map<String, dynamic>;
        if (response['success'] == true && response['data'] != null) {
          final List<dynamic> jsonList = response['data'] as List<dynamic>;
          allAssets.addAll(
            jsonList
                .map((e) => Asset.fromJson(e as Map<String, dynamic>))
                .toList(),
          );
          appLogger.info('[Plugin] Found ${jsonList.length} assets');
        }
      } catch (e) {
        appLogger.error('[Plugin] ScanAssetsFFI failed: $e');
      }

      // 2. 通过PluginManager聚合评估所有插件的风险
      try {
        final assessRisks = lib
            .lookupFunction<AssessRisksFFIC, AssessRisksFFIDart>(
              'AssessRisksFFI',
            );

        // 从Go数据库层获取已扫描的skill哈希
        String scannedHashesJson = '[]';
        try {
          final getHashes = lib
              .lookupFunction<
                GetScannedSkillHashesC,
                GetScannedSkillHashesDart
              >('GetScannedSkillHashes');
          final hashesResultPtr = getHashes();
          final hashesResult = hashesResultPtr.toDartString();
          freeStr(hashesResultPtr);
          final hashesResponse = jsonDecode(hashesResult);
          if (hashesResponse['success'] == true &&
              hashesResponse['data'] != null) {
            scannedHashesJson = jsonEncode(hashesResponse['data']);
          }
        } catch (e) {
          appLogger.debug('[Plugin] GetScannedSkillHashes not available: $e');
        }

        final hashesPtr = scannedHashesJson.toNativeUtf8();
        final riskPtr = assessRisks(hashesPtr);
        malloc.free(hashesPtr);

        final riskJsonString = riskPtr.toDartString();
        freeStr(riskPtr);

        final riskResponse = jsonDecode(riskJsonString) as Map<String, dynamic>;
        if (riskResponse['success'] == true && riskResponse['data'] != null) {
          final List<dynamic> jsonList = riskResponse['data'] as List<dynamic>;
          for (var item in jsonList) {
            allRisks.add(_parseRisk(item as Map<String, dynamic>));
          }
        }
      } catch (e) {
        appLogger.error('[Plugin] AssessRisksFFI failed: $e');
      }
    } catch (e) {
      appLogger.error('[Plugin] Scan error', e);
    }

    appLogger.info(
      '[Plugin] Scan complete: ${allAssets.length} assets, ${allRisks.length} risks',
    );

    // 从资产元数据中提取配置信息
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
    final lib = dylib;
    if (lib == null) {
      return {'success': false, 'error': 'Plugin not initialized'};
    }

    try {
      final mitigateRiskFn = lib
          .lookupFunction<MitigateRiskC, MitigateRiskDart>('MitigateRiskFFI');

      final sourcePlugin = _normalizeSourcePlugin(risk.sourcePlugin);
      if (sourcePlugin == null) {
        return {
          'success': false,
          'error': 'source_plugin is required for mitigation routing',
        };
      }
      final req = {
        'id': risk.id,
        'args': risk.args,
        'form_data': formData,
        'source_plugin': sourcePlugin,
      };

      final reqPtr = jsonEncode(req).toNativeUtf8();
      final resPtr = mitigateRiskFn(reqPtr);
      malloc.free(reqPtr);

      final resJson = resPtr.toDartString();
      freeString!(resPtr);

      return jsonDecode(resJson);
    } catch (e) {
      return {'success': false, 'error': e.toString()};
    }
  }

  RiskInfo _parseRisk(Map<String, dynamic> json) {
    final sourcePlugin = _normalizeSourcePlugin(
      json['source_plugin'] as String?,
    );
    return RiskInfo(
      id: json['id'] ?? 'unknown',
      title: json['title'] ?? 'Unknown Risk',
      description: json['description'] ?? '',
      level: _parseRiskLevel(json['level']),
      icon: _getIconForRisk(json['level']),
      args: json['args'] != null
          ? Map<String, Object>.from(json['args'])
          : null,
      mitigation: json['mitigation'] != null
          ? Mitigation.fromJson(json['mitigation'])
          : null,
      sourcePlugin: sourcePlugin,
    );
  }

  String? _normalizeSourcePlugin(String? sourcePlugin) {
    final normalized = sourcePlugin?.trim();
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
}
