import 'dart:convert';
import 'dart:ffi' as ffi;
import 'package:ffi/ffi.dart';
import '../utils/app_logger.dart';
import '../utils/locale_utils.dart';
import 'native_library_service.dart';

// FFI 类型定义
typedef _OneArgC = ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);
typedef _OneArgDart = ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);
typedef _NoArgC = ffi.Pointer<Utf8> Function();
typedef _NoArgDart = ffi.Pointer<Utf8> Function();

/// 应用设置 FFI 持久化服务：通过 FFI 委托 Go 层进行数据持久化
///
/// 封装 SaveAppSettingFFI 和 GetAppSettingFFI 调用，提供应用设置的读写接口。
class AppSettingsDatabaseService {
  static const String scheduledScanIntervalKey =
      'scheduled_scan_interval_seconds';
  static final AppSettingsDatabaseService _instance =
      AppSettingsDatabaseService._internal();

  factory AppSettingsDatabaseService() => _instance;

  AppSettingsDatabaseService._internal();

  static const String _apiServerEnabledKey = 'api_server_enabled';

  ffi.DynamicLibrary? get _dylib => NativeLibraryService().dylib;
  FreeStringDart? get _freeString => NativeLibraryService().freeString;

  /// 保存设置项
  ///
  /// [key] 设置键名
  /// [value] 设置值
  Future<bool> saveSetting(String key, String value) async {
    if (_dylib == null || _freeString == null) {
      appLogger.error('[AppSettingsDB] Native library not initialized');
      return false;
    }

    try {
      final func = _dylib!.lookupFunction<_OneArgC, _OneArgDart>(
        'SaveAppSettingFFI',
      );
      final jsonStr = jsonEncode({'key': key, 'value': value});
      final argPtr = jsonStr.toNativeUtf8();
      final resultPtr = func(argPtr);
      final result = resultPtr.toDartString();
      _freeString!(resultPtr);
      malloc.free(argPtr);

      final response = jsonDecode(result) as Map<String, dynamic>;
      if (response['success'] == true) {
        appLogger.info('[AppSettingsDB] Setting saved: key=$key');
        return true;
      }
      appLogger.error(
        '[AppSettingsDB] Failed to save setting: ${response['error']}',
      );
      return false;
    } catch (e) {
      appLogger.error('[AppSettingsDB] Failed to save setting: $key', e);
      return false;
    }
  }

  /// 获取设置项
  ///
  /// [key] 设置键名
  /// 返回设置值，如果不存在返回空字符串
  Future<String> getSetting(String key) async {
    if (_dylib == null || _freeString == null) {
      appLogger.error('[AppSettingsDB] Native library not initialized');
      return '';
    }

    try {
      final func = _dylib!.lookupFunction<_OneArgC, _OneArgDart>(
        'GetAppSettingFFI',
      );
      final argPtr = key.toNativeUtf8();
      final resultPtr = func(argPtr);
      final result = resultPtr.toDartString();
      _freeString!(resultPtr);
      malloc.free(argPtr);

      final response = jsonDecode(result) as Map<String, dynamic>;
      if (response['success'] == true) {
        return (response['data'] as String?) ?? '';
      }
      appLogger.error(
        '[AppSettingsDB] Failed to get setting: ${response['error']}',
      );
      return '';
    } catch (e) {
      appLogger.error('[AppSettingsDB] Failed to get setting: $key', e);
      return '';
    }
  }

  // ==================== 便捷方法 ====================

  /// 设置应用语言，并同步到 Go 运行时。
  Future<bool> setLanguage(String language) async {
    if (_dylib == null || _freeString == null) {
      appLogger.error('[AppSettingsDB] Native library not initialized');
      return false;
    }

    try {
      final normalizedLanguage = LocaleUtils.normalizeLanguageCode(language);
      final func = _dylib!.lookupFunction<_OneArgC, _OneArgDart>(
        'SetLanguageFFI',
      );
      final argPtr = normalizedLanguage.toNativeUtf8();
      final resultPtr = func(argPtr);
      final result = resultPtr.toDartString();
      _freeString!(resultPtr);
      malloc.free(argPtr);

      final response = jsonDecode(result) as Map<String, dynamic>;
      if (response['success'] == true) {
        appLogger.info('[AppSettingsDB] Language updated: $normalizedLanguage');
        return true;
      }
      appLogger.error(
        '[AppSettingsDB] Failed to update language: ${response['error']}',
      );
      return false;
    } catch (e) {
      appLogger.error('[AppSettingsDB] Failed to update language', e);
      return false;
    }
  }

  /// 获取应用语言设置。
  Future<String> getLanguage() async {
    if (_dylib == null || _freeString == null) {
      appLogger.error('[AppSettingsDB] Native library not initialized');
      return '';
    }

    try {
      final func = _dylib!.lookupFunction<_NoArgC, _NoArgDart>(
        'GetLanguageFFI',
      );
      final resultPtr = func();
      final result = resultPtr.toDartString();
      _freeString!(resultPtr);

      final response = jsonDecode(result) as Map<String, dynamic>;
      if (response['success'] != true) {
        appLogger.error(
          '[AppSettingsDB] Failed to get language: ${response['error']}',
        );
        return '';
      }

      final value = (response['data'] as String?) ?? '';
      if (value.isEmpty) {
        return '';
      }
      return LocaleUtils.normalizeLanguageCode(value);
    } catch (e) {
      appLogger.error('[AppSettingsDB] Failed to get language', e);
      return '';
    }
  }

  /// 判断是否为首次启动
  ///
  /// 如果标记不存在或为 "true"，则返回 true（首次启动）
  /// 如果标记为 "false"，则返回 false（非首次启动）
  Future<bool> isFirstLaunch() async {
    final value = await getSetting('is_first_launch');
    // 空值或 "true" 视为首次启动
    return value.isEmpty || value == 'true';
  }

  /// 标记首次启动完成
  ///
  /// 设置 is_first_launch = "false"
  Future<bool> setFirstLaunchCompleted() async {
    return await saveSetting('is_first_launch', 'false');
  }

  /// Get scheduled scan interval in seconds.
  ///
  /// Returns 0 when the setting is missing, invalid, or disabled.
  Future<int> getScheduledScanIntervalSeconds() async {
    final value = await getSetting(scheduledScanIntervalKey);
    if (value.isEmpty) {
      return 0;
    }

    final seconds = int.tryParse(value);
    if (seconds == null || seconds < 0) {
      appLogger.warning(
        '[AppSettingsDB] Invalid scheduled scan interval value: $value',
      );
      return 0;
    }

    return seconds;
  }

  /// Persist scheduled scan interval in seconds.
  ///
  /// 0 disables scheduled scan.
  Future<bool> setScheduledScanIntervalSeconds(int seconds) async {
    if (seconds < 0) {
      appLogger.error(
        '[AppSettingsDB] Refusing to save negative scheduled scan interval: $seconds',
      );
      return false;
    }

    return await saveSetting(scheduledScanIntervalKey, seconds.toString());
  }

  /// 设置 API 服务开关状态。
  Future<bool> setApiServerEnabled(bool enabled) async {
    return await saveSetting(_apiServerEnabledKey, enabled ? 'true' : 'false');
  }

  /// 获取 API 服务开关状态，默认开启。
  ///
  /// 当首次启动未写入配置时，会自动写入默认值，保证后续读取一致。
  Future<bool> getApiServerEnabled({bool defaultValue = true}) async {
    final value = await getSetting(_apiServerEnabledKey);
    if (value.isEmpty) {
      await setApiServerEnabled(defaultValue);
      return defaultValue;
    }

    final normalized = value.trim().toLowerCase();
    return normalized == 'true' || normalized == '1';
  }
}
