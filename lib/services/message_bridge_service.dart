import 'dart:async';
import 'dart:convert';
import 'dart:ffi' as ffi;
import 'dart:isolate';

import 'package:ffi/ffi.dart';
import '../models/version_info.dart';
import '../utils/app_logger.dart';
import 'native_library_service.dart' hide FreeStringDart;

/// 供 [Isolate.run] 在后台 isolate 中解析桥接 JSON, 避免大 payload 阻塞 UI isolate.
Map<String, dynamic> _decodeBridgeMessageForIsolate(String trimmed) {
  return jsonDecode(trimmed) as Map<String, dynamic>;
}

// FFI 类型定义 - 回调相关
typedef DartMessageCallbackNative = ffi.Void Function(ffi.Pointer<Utf8>);
typedef RegisterMessageCallbackC =
    ffi.Pointer<Utf8> Function(
      ffi.Pointer<ffi.NativeFunction<DartMessageCallbackNative>>,
    );
typedef RegisterMessageCallbackDart =
    ffi.Pointer<Utf8> Function(
      ffi.Pointer<ffi.NativeFunction<DartMessageCallbackNative>>,
    );

typedef UnregisterMessageCallbackC = ffi.Pointer<Utf8> Function();
typedef UnregisterMessageCallbackDart = ffi.Pointer<Utf8> Function();

typedef IsCallbackBridgeRunningC = ffi.Int32 Function();
typedef IsCallbackBridgeRunningDart = int Function();

typedef FreeStringC = ffi.Void Function(ffi.Pointer<Utf8>);
typedef FreeStringDart = void Function(ffi.Pointer<Utf8>);

/// 消息类型
enum BridgeMessageType {
  log,
  metrics,
  status,
  versionUpdate,
  securityEvent,
  truthRecord,
}

/// 桥接消息结构
class BridgeMessage {
  final BridgeMessageType type;
  final DateTime timestamp;
  final Map<String, dynamic> payload;

  BridgeMessage({
    required this.type,
    required this.timestamp,
    required this.payload,
  });

  factory BridgeMessage.fromJson(Map<String, dynamic> json) {
    return BridgeMessage(
      type: _parseMessageType(json['type']),
      timestamp: DateTime.fromMillisecondsSinceEpoch(json['timestamp'] ?? 0),
      payload: json['payload'] ?? {},
    );
  }

  Map<String, dynamic> toJson() => {
    'type': type.name,
    'timestamp': timestamp.millisecondsSinceEpoch,
    'payload': payload,
  };

  static BridgeMessageType _parseMessageType(String? typeStr) {
    switch (typeStr) {
      case 'log':
        return BridgeMessageType.log;
      case 'metrics':
        return BridgeMessageType.metrics;
      case 'status':
        return BridgeMessageType.status;
      case 'version_update':
        return BridgeMessageType.versionUpdate;
      case 'security_event':
        return BridgeMessageType.securityEvent;
      case 'truth_record':
        return BridgeMessageType.truthRecord;
      default:
        return BridgeMessageType.log;
    }
  }
}

/// 消息桥接服务 - 使用 FFI 回调实现 Go 与 Dart 之间的实时通信
class MessageBridgeService {
  static final MessageBridgeService _instance =
      MessageBridgeService._internal();
  factory MessageBridgeService() => _instance;
  MessageBridgeService._internal();

  ffi.DynamicLibrary? _dylib;

  /// 释放 Go 侧 C.CString 传入回调的缓冲区(与 RegisterMessageCallback 闭包配对, 避免异步 listener 下 use-after-free).
  FreeStringDart? _bridgeCStringFree;

  // FFI 回调相关
  ReceivePort? _receivePort;
  ffi.NativeCallable<DartMessageCallbackNative>? _nativeCallable;

  final StreamController<BridgeMessage> _messageController =
      StreamController<BridgeMessage>.broadcast();
  final StreamController<Exception> _errorController =
      StreamController<Exception>.broadcast();

  bool _isRunning = false;
  bool _isInitialized = false;
  int _subscriberCount = 0;

  /// 消息流
  Stream<BridgeMessage> get messageStream => _messageController.stream;

  /// 日志流（过滤日志消息）
  Stream<String> get logStream => messageStream
      .where((msg) => msg.type == BridgeMessageType.log)
      .map((msg) => jsonEncode(msg.payload));

  /// 指标流（过滤指标消息）
  Stream<Map<String, dynamic>> get metricsStream => messageStream
      .where((msg) => msg.type == BridgeMessageType.metrics)
      .map((msg) => msg.payload);

  /// 状态流（过滤状态消息）
  Stream<Map<String, dynamic>> get statusStream => messageStream
      .where((msg) => msg.type == BridgeMessageType.status)
      .map((msg) => msg.payload);

  /// 版本更新流（过滤版本更新消息）
  Stream<VersionInfo> get versionUpdateStream => messageStream
      .where((msg) => msg.type == BridgeMessageType.versionUpdate)
      .map((msg) => VersionInfo.fromJson(msg.payload));

  /// 安全事件流（过滤安全事件消息）
  Stream<Map<String, dynamic>> get securityEventStream => messageStream
      .where((msg) => msg.type == BridgeMessageType.securityEvent)
      .map((msg) => msg.payload);

  /// TruthRecord 快照流（分组卡片事实来源）
  Stream<Map<String, dynamic>> get truthRecordStream => messageStream
      .where((msg) => msg.type == BridgeMessageType.truthRecord)
      .map((msg) => msg.payload);

  /// 错误流
  Stream<Exception> get errorStream => _errorController.stream;

  /// 是否正在运行
  bool get isRunning => _isRunning;

  /// 是否已初始化
  bool get isInitialized => _isInitialized;

  /// 初始化消息桥接服务
  Future<bool> initialize() async {
    if (_isInitialized) {
      _subscriberCount++;
      return true;
    }

    try {
      // 从 NativeLibraryService 获取已加载的 dylib
      final nativeLib = NativeLibraryService();
      if (!nativeLib.isInitialized || nativeLib.dylib == null) {
        appLogger.warning(
          '[MessageBridge] NativeLibraryService not initialized',
        );
        return false;
      }

      _dylib = nativeLib.dylib;

      _bridgeCStringFree = _dylib!.lookupFunction<FreeStringC, FreeStringDart>(
        'FreeString',
      );

      // 创建接收端口用于跨线程通信
      _receivePort = ReceivePort();

      // 监听来自回调的消息: 勿在 ReceivePort 回调栈内同步 jsonDecode, 否则会长时间占用 UI isolate, Windows 易显示「未响应」.
      _receivePort!.listen((message) {
        if (message is String) {
          unawaited(_processMessageAsync(message));
        }
      });

      // 创建 NativeCallable.listener - 这是关键!
      // NativeCallable.listener 会在独立的原生线程中执行回调
      _nativeCallable = ffi.NativeCallable<DartMessageCallbackNative>.listener(
        _onNativeMessage,
      );

      // 注册回调到 Go
      final registerCallback = _dylib!
          .lookupFunction<
            RegisterMessageCallbackC,
            RegisterMessageCallbackDart
          >('RegisterMessageCallback');

      final resultPtr = registerCallback(_nativeCallable!.nativeFunction);
      final resultStr = resultPtr.toDartString();
      _bridgeCStringFree!(resultPtr);

      final result = jsonDecode(resultStr);
      if (result['success'] != true) {
        appLogger.error(
          '[MessageBridge] Callback registration failed: ${result['error']}',
        );
        _disposeCallback();
        return false;
      }

      _isRunning = true;
      _isInitialized = true;
      _subscriberCount = 1;
      appLogger.info('[MessageBridge] FFI callback mode initialized');
      return true;
    } catch (e) {
      appLogger.error('[MessageBridge] Initialization error', e);
      _subscriberCount = 0;
      _disposeCallback();
      return false;
    }
  }

  /// 释放回调路径上的 C 字符串(Go 不再 defer free, 必须由 Dart 在拷贝后调用).
  void _freeBridgeCString(ffi.Pointer<Utf8> ptr) {
    if (ptr.address == 0) {
      return;
    }
    _bridgeCStringFree?.call(ptr);
  }

  /// 原生回调处理函数 - 运行在独立原生线程
  void _onNativeMessage(ffi.Pointer<Utf8> messagePtr) {
    try {
      // 检查是否正在运行,避免在关闭后继续处理消息
      if (!_isRunning) {
        return;
      }

      // 检查 receivePort 是否可用
      final port = _receivePort;
      if (port == null) {
        return;
      }

      // 将 C 字符串转换为 Dart 字符串
      final message = messagePtr.toDartString();

      // 通过 SendPort 发送到主 Isolate
      // 注意: NativeCallable 运行在独立的原生线程,不能直接操作 Dart 对象
      port.sendPort.send(message);
    } catch (e) {
      // 在回调中不能打印日志,因为可能不在主线程
      // 忽略任何异常,防止崩溃
    } finally {
      _freeBridgeCString(messagePtr);
    }
  }

  /// 异步处理 FFI 推送的 JSON: 先让出事件循环, 大报文在独立 isolate 中 jsonDecode.
  Future<void> _processMessageAsync(String data) async {
    await Future<void>.delayed(Duration.zero);
    if (!_isRunning || _messageController.isClosed) {
      return;
    }
    try {
      if (data.isEmpty || data.length < 10) {
        return;
      }

      final trimmed = data.trim();
      if (!trimmed.startsWith('{') || !trimmed.endsWith('}')) {
        return;
      }

      late final Map<String, dynamic> json;
      // TruthRecord 等单条 JSON 常达数万字符, 阈值略低以尽量走后台解析.
      const int heavyJsonChars = 12000;
      if (trimmed.length >= heavyJsonChars) {
        json = await Isolate.run(() => _decodeBridgeMessageForIsolate(trimmed));
      } else {
        json = jsonDecode(trimmed) as Map<String, dynamic>;
      }

      if (!_isRunning || _messageController.isClosed) {
        return;
      }
      final message = BridgeMessage.fromJson(json);
      if (!_messageController.isClosed) {
        _messageController.add(message);
      }
    } on FormatException {
      // 格式化异常通常是由于消息截断或乱码导致,静默跳过
    } catch (e) {
      appLogger.warning('[MessageBridge] Message processing error: $e');
    }
  }

  void _disposeCallback() {
    _nativeCallable?.close();
    _nativeCallable = null;
    _receivePort?.close();
    _receivePort = null;
    _bridgeCStringFree = null;
  }

  /// 检查 Go 层回调桥接器是否运行
  bool isGoBridgeRunning() {
    if (_dylib == null) return false;

    try {
      final isRunning = _dylib!
          .lookupFunction<
            IsCallbackBridgeRunningC,
            IsCallbackBridgeRunningDart
          >('IsCallbackBridgeRunning');
      return isRunning() == 1;
    } catch (e) {
      return false;
    }
  }

  /// 关闭消息桥接服务
  void dispose() {
    if (_subscriberCount > 0) {
      _subscriberCount--;
    }

    if (_subscriberCount > 0) {
      return;
    }

    if (!_isRunning && !_isInitialized) {
      // 已经关闭
      return;
    }

    _isRunning = false;

    // 1. 先注销 Go 端回调 (必须在释放 NativeCallable 之前)
    if (_dylib != null) {
      try {
        final unregisterCallback = _dylib!
            .lookupFunction<
              UnregisterMessageCallbackC,
              UnregisterMessageCallbackDart
            >('UnregisterMessageCallback');

        final resultPtr = unregisterCallback();
        final resultStr = resultPtr.toDartString();
        _bridgeCStringFree?.call(resultPtr);
        appLogger.info('[MessageBridge] Go callback unregistered: $resultStr');

        // 注意: 之前使用同步 sleep(100ms) 等待 Go 端停止回调,
        // 但这会阻塞 Dart 事件循环,导致信号处理(如 SIGINT)无法执行。
        // Go 端的 UnregisterMessageCallback 已经同步等待了工作协程退出,
        // 所以 Dart 侧不需要额外等待。
      } catch (e) {
        appLogger.error('[MessageBridge] Unregister callback error', e);
      }
    }

    // 3. 现在安全地释放 Dart 端资源
    _disposeCallback();

    // 4. 最后关闭状态标志（流控制器保持打开，供后续重新 initialize 复用）
    _isInitialized = false;
    appLogger.info('[MessageBridge] Service closed');
  }
}
