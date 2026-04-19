import 'dart:convert';
import 'dart:isolate';

import 'native_library_service.dart';
import 'protection_proxy_ffi.dart';

class ProtectionProxyExecutor {
  ProtectionProxyExecutor._();

  static Future<Map<String, dynamic>> start(String configJSON) async {
    final libPath = NativeLibraryService().libraryPath;
    if (libPath == null || libPath.trim().isEmpty) {
      return {'success': false, 'error': 'Native library path not initialized'};
    }

    final resultRaw = await Isolate.run(
      () =>
          ProtectionProxyFFI.startProtectionProxyInIsolate(libPath, configJSON),
    );
    return jsonDecode(resultRaw) as Map<String, dynamic>;
  }

  static Future<Map<String, dynamic>> stop({required String assetID}) async {
    final libPath = NativeLibraryService().libraryPath;
    if (libPath == null || libPath.trim().isEmpty) {
      return {'success': false, 'error': 'Native library path not initialized'};
    }

    final useAssetScope = assetID.trim().isNotEmpty;
    final scopedAssetID = assetID.trim();
    final resultRaw = await Isolate.run(() {
      if (useAssetScope) {
        return ProtectionProxyFFI.stopProtectionProxyByAssetInIsolate(
          libPath,
          scopedAssetID,
        );
      }
      return ProtectionProxyFFI.stopProtectionProxyInIsolate(libPath);
    });
    return jsonDecode(resultRaw) as Map<String, dynamic>;
  }

  static Future<Map<String, dynamic>> status({required String assetID}) async {
    final libPath = NativeLibraryService().libraryPath;
    if (libPath == null || libPath.trim().isEmpty) {
      return {'running': false, 'error': 'Native library path not initialized'};
    }

    final useAssetScope = assetID.trim().isNotEmpty;
    final scopedAssetID = assetID.trim();
    final resultRaw = await Isolate.run(() {
      if (useAssetScope) {
        return ProtectionProxyFFI.getProtectionProxyStatusByAssetInIsolate(
          libPath,
          scopedAssetID,
        );
      }
      return ProtectionProxyFFI.getProtectionProxyStatusInIsolate(libPath);
    });
    return jsonDecode(resultRaw) as Map<String, dynamic>;
  }
}
