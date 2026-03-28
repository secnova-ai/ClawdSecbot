import 'dart:async';
import 'dart:convert';
import 'dart:ffi' as ffi;
import 'dart:isolate';

import 'package:ffi/ffi.dart';
import '../models/version_info.dart';
import '../utils/app_logger.dart';
import 'native_library_service.dart' hide FreeStringDart;

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
enum BridgeMessageType { log, metrics, status, versionUpdate, securityEvent }

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

      // 创建接收端口用于跨线程通信
      _receivePort = ReceivePort();

      // 监听来自回调的消息
      _receivePort!.listen((message) {
        if (message is String) {
          _processMessage(message);
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

      final freeString = _dylib!.lookupFunction<FreeStringC, FreeStringDart>(
        'FreeString',
      );

      final resultPtr = registerCallback(_nativeCallable!.nativeFunction);
      final resultStr = resultPtr.toDartString();
      freeString(resultPtr);

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
    }
  }

  void _processMessage(String data) {
    try {
      // 验证 JSON 格式
      if (data.isEmpty || data.length < 10) {
        return; // 跳过空消息或过短消息
      }

      // 检查消息完整性(简单启发式:必须以 { 开头,} 结尾)
      final trimmed = data.trim();
      if (!trimmed.startsWith('{') || !trimmed.endsWith('}')) {
        return;
      }

      final json = jsonDecode(data);
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

        final freeString = _dylib!.lookupFunction<FreeStringC, FreeStringDart>(
          'FreeString',
        );

        final resultPtr = unregisterCallback();
        final resultStr = resultPtr.toDartString();
        freeString(resultPtr);
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
