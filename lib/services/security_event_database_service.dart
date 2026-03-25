import 'dart:convert';
import 'dart:ffi' as ffi;
import 'package:ffi/ffi.dart';
import '../models/security_event_model.dart';
import '../utils/app_logger.dart';
import 'native_library_service.dart';

// FFI type definitions
typedef _OneArgC = ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);
typedef _OneArgDart = ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);

typedef _NoArgC = ffi.Pointer<Utf8> Function();
typedef _NoArgDart = ffi.Pointer<Utf8> Function();

/// 安全事件 FFI 持久化门面：通过 FFI 委托 Go 层进行数据持久化，Flutter 不直接操作 DB。
class SecurityEventDatabaseService {
  static final SecurityEventDatabaseService _instance =
      SecurityEventDatabaseService._internal();

  factory SecurityEventDatabaseService() => _instance;

  SecurityEventDatabaseService._internal();

  ffi.DynamicLibrary? get _dylib => NativeLibraryService().dylib;
  FreeStringDart? get _freeString => NativeLibraryService().freeString;

  /// 批量保存安全事件
  Future<void> saveSecurityEventsBatch(List<SecurityEvent> events) async {
    if (events.isEmpty) return;

    final eventsList = events.map((e) => e.toJson()).toList();
    final result = _callFFI(
      'SaveSecurityEventsBatchFFI',
      jsonEncode(eventsList),
    );
    if (result['success'] != true) {
      appLogger.warning(
        '[SecurityEventDB] Batch save failed: ${result['error']}',
      );
    }
  }

  /// 查询安全事件
  Future<List<SecurityEvent>> getSecurityEvents({
    int limit = 100,
    int offset = 0,
    String assetID = '',
  }) async {
    final result = _callFFI(
      'GetSecurityEventsFFI',
      jsonEncode({'limit': limit, 'offset': offset, 'asset_id': assetID}),
    );
    if (result['success'] != true) return [];

    final data = result['data'];
    if (data == null || data is! List) return [];

    return data.map((item) {
      return SecurityEvent.fromJson(item as Map<String, dynamic>);
    }).toList();
  }

  /// 获取安全事件数量
  Future<int> getSecurityEventCount() async {
    final result = _callFFINoArg('GetSecurityEventCountFFI');
    if (result['success'] != true) return 0;
    return result['data'] as int? ?? 0;
  }

  /// 清空所有安全事件
  Future<void> clearAllSecurityEvents() async {
    _callFFINoArg('ClearAllSecurityEventsFFI');
  }

  /// 清空指定资产安全事件（仅按 assetID 过滤）
  Future<void> clearSecurityEvents({String assetID = ''}) async {
    _callFFI('ClearSecurityEventsFFI', jsonEncode({'asset_id': assetID}));
  }

  // --- Helper methods ---

  Map<String, dynamic> _callFFINoArg(String funcName) {
    final dylib = _dylib;
    if (dylib == null || _freeString == null) {
      return {'success': false, 'error': 'Native library not initialized'};
    }

    try {
      final func = dylib.lookupFunction<_NoArgC, _NoArgDart>(funcName);
      final resultPtr = func();
      final result = resultPtr.toDartString();
      _freeString!(resultPtr);
      return jsonDecode(result) as Map<String, dynamic>;
    } catch (e) {
      appLogger.error('[SecurityEventDB] $funcName failed: $e');
      return {'success': false, 'error': '$funcName failed: $e'};
    }
  }

  Map<String, dynamic> _callFFI(String funcName, String jsonStr) {
    final dylib = _dylib;
    if (dylib == null || _freeString == null) {
      return {'success': false, 'error': 'Native library not initialized'};
    }

    try {
      final func = dylib.lookupFunction<_OneArgC, _OneArgDart>(funcName);
      final argPtr = jsonStr.toNativeUtf8();
      final resultPtr = func(argPtr);
      final result = resultPtr.toDartString();
      _freeString!(resultPtr);
      malloc.free(argPtr);
      return jsonDecode(result) as Map<String, dynamic>;
    } catch (e) {
      appLogger.error('[SecurityEventDB] $funcName failed: $e');
      return {'success': false, 'error': '$funcName failed: $e'};
    }
  }
}
