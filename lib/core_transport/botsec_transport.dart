import 'dart:convert';

/// Transport abstraction for invoking Go capabilities from Flutter.
///
/// Desktop uses FFI transport; Web will use HTTP/SSE transport.
abstract class BotsecTransport {
  bool get isReady;

  String callRawNoArg(String method);
  String callRawOneArg(String method, String arg);
  String callRawTwoArgs(String method, String arg1, String arg2);
  String callRawOneInt(String method, int arg);
  String callRawOneArgOneInt(String method, String arg, int value);
  String callRawThreeInts(String method, int arg1, int arg2, int arg3);

  /// 异步原始调用，默认退化为同步调用，仅由 FFI 传输重写为后台 isolate 执行，
  /// 用于避免长耗时 FFI（如 LLM 连通性测试）阻塞 UI isolate。
  Future<String> callRawNoArgAsync(String method) async {
    return callRawNoArg(method);
  }

  Future<String> callRawOneArgAsync(String method, String arg) async {
    return callRawOneArg(method, arg);
  }

  Future<String> callRawTwoArgsAsync(
    String method,
    String arg1,
    String arg2,
  ) async {
    return callRawTwoArgs(method, arg1, arg2);
  }

  Map<String, dynamic> callNoArg(String method) {
    return _decodeEnvelope(callRawNoArg(method), method);
  }

  Future<Map<String, dynamic>> callNoArgAsync(String method) async {
    final raw = await callRawNoArgAsync(method);
    return _decodeEnvelope(raw, method);
  }

  Map<String, dynamic> callOneArg(String method, String arg) {
    return _decodeEnvelope(callRawOneArg(method, arg), method);
  }

  /// 异步封装调用：在后台 isolate 执行原始 FFI 后再解析 JSON 包络。
  Future<Map<String, dynamic>> callOneArgAsync(
    String method,
    String arg,
  ) async {
    final raw = await callRawOneArgAsync(method, arg);
    return _decodeEnvelope(raw, method);
  }

  Map<String, dynamic> callTwoArgs(String method, String arg1, String arg2) {
    return _decodeEnvelope(callRawTwoArgs(method, arg1, arg2), method);
  }

  Future<Map<String, dynamic>> callTwoArgsAsync(
    String method,
    String arg1,
    String arg2,
  ) async {
    final raw = await callRawTwoArgsAsync(method, arg1, arg2);
    return _decodeEnvelope(raw, method);
  }

  Map<String, dynamic> callOneInt(String method, int arg) {
    return _decodeEnvelope(callRawOneInt(method, arg), method);
  }

  Map<String, dynamic> callOneArgOneInt(String method, String arg, int value) {
    return _decodeEnvelope(callRawOneArgOneInt(method, arg, value), method);
  }

  Map<String, dynamic> callThreeInts(
    String method,
    int arg1,
    int arg2,
    int arg3,
  ) {
    return _decodeEnvelope(callRawThreeInts(method, arg1, arg2, arg3), method);
  }

  Map<String, dynamic> _decodeEnvelope(String json, String method) {
    try {
      final decoded = jsonDecode(json);
      if (decoded is Map<String, dynamic>) {
        return decoded;
      }
      return {'success': false, 'error': '$method returned non-object JSON'};
    } catch (e) {
      return {'success': false, 'error': '$method returned invalid JSON: $e'};
    }
  }
}
