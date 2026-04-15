import 'dart:async';
import 'dart:convert';
import 'dart:ffi' as ffi;
import 'dart:isolate';

import 'package:ffi/ffi.dart';

import '../utils/app_logger.dart';
import 'native_library_service.dart';

typedef _RegisterAppShutdownCallbackC =
    ffi.Pointer<Utf8> Function(
      ffi.Pointer<ffi.NativeFunction<_AppShutdownCallbackNative>>,
    );
typedef _RegisterAppShutdownCallbackDart =
    ffi.Pointer<Utf8> Function(
      ffi.Pointer<ffi.NativeFunction<_AppShutdownCallbackNative>>,
    );

typedef _UnregisterAppShutdownCallbackC = ffi.Pointer<Utf8> Function();
typedef _UnregisterAppShutdownCallbackDart = ffi.Pointer<Utf8> Function();

typedef _AppShutdownCallbackNative = ffi.Void Function(ffi.Pointer<Utf8>);

class ApiShutdownRequest {
  const ApiShutdownRequest({required this.restoreConfig});

  final bool restoreConfig;

  factory ApiShutdownRequest.fromJson(Map<String, dynamic> json) {
    return ApiShutdownRequest(restoreConfig: json['restoreConfig'] == true);
  }
}

class ApiShutdownService {
  ApiShutdownService._();

  static final ApiShutdownService _instance = ApiShutdownService._();

  factory ApiShutdownService() => _instance;

  final StreamController<ApiShutdownRequest> _controller =
      StreamController<ApiShutdownRequest>.broadcast();

  ffi.NativeCallable<_AppShutdownCallbackNative>? _nativeCallable;
  ReceivePort? _receivePort;
  bool _running = false;

  Stream<ApiShutdownRequest> get requests => _controller.stream;

  Future<bool> register() async {
    if (_running) {
      return true;
    }

    final dylib = NativeLibraryService().dylib;
    final freeString = NativeLibraryService().freeString;
    if (dylib == null || freeString == null) {
      appLogger.warning('[ApiShutdown] Native library not initialized');
      return false;
    }

    try {
      _receivePort = ReceivePort();
      _receivePort!.listen((dynamic message) async {
        if (!_running || message is! String) {
          return;
        }
        try {
          final decoded = jsonDecode(message) as Map<String, dynamic>;
          _controller.add(ApiShutdownRequest.fromJson(decoded));
        } catch (e) {
          appLogger.warning(
            '[ApiShutdown] Failed to decode shutdown payload: $e',
          );
        }
      });

      _nativeCallable = ffi.NativeCallable<_AppShutdownCallbackNative>.listener(
        _onNativeShutdown,
      );

      final registerFn = dylib
          .lookupFunction<
            _RegisterAppShutdownCallbackC,
            _RegisterAppShutdownCallbackDart
          >('RegisterAppShutdownCallback');
      final resultPtr = registerFn(_nativeCallable!.nativeFunction);
      final result =
          jsonDecode(resultPtr.toDartString()) as Map<String, dynamic>;
      freeString(resultPtr);

      if (result['success'] != true) {
        appLogger.error(
          '[ApiShutdown] Callback registration failed: ${result['error']}',
        );
        _disposeNativeState();
        return false;
      }

      _running = true;
      appLogger.info('[ApiShutdown] Callback registration completed');
      return true;
    } catch (e) {
      appLogger.error('[ApiShutdown] Registration failed', e);
      _disposeNativeState();
      return false;
    }
  }

  void _onNativeShutdown(ffi.Pointer<Utf8> payloadPtr) {
    try {
      if (!_running) {
        return;
      }
      final port = _receivePort;
      if (port == null) {
        return;
      }
      port.sendPort.send(payloadPtr.toDartString());
    } finally {
      final freeString = NativeLibraryService().freeString;
      if (freeString != null && payloadPtr.address != 0) {
        freeString(payloadPtr);
      }
    }
  }

  Future<void> dispose() async {
    if (!_running) {
      _disposeNativeState();
      return;
    }

    final dylib = NativeLibraryService().dylib;
    final freeString = NativeLibraryService().freeString;
    if (dylib != null && freeString != null) {
      try {
        final unregisterFn = dylib
            .lookupFunction<
              _UnregisterAppShutdownCallbackC,
              _UnregisterAppShutdownCallbackDart
            >('UnregisterAppShutdownCallback');
        final resultPtr = unregisterFn();
        freeString(resultPtr);
      } catch (e) {
        appLogger.warning('[ApiShutdown] Failed to unregister callback: $e');
      }
    }

    _running = false;
    _disposeNativeState();
  }

  void _disposeNativeState() {
    _nativeCallable?.close();
    _nativeCallable = null;
    _receivePort?.close();
    _receivePort = null;
  }
}
