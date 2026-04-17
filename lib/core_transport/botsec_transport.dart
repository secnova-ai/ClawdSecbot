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

  Map<String, dynamic> callNoArg(String method) {
    return _decodeEnvelope(callRawNoArg(method), method);
  }

  Map<String, dynamic> callOneArg(String method, String arg) {
    return _decodeEnvelope(callRawOneArg(method, arg), method);
  }

  Map<String, dynamic> callTwoArgs(String method, String arg1, String arg2) {
    return _decodeEnvelope(callRawTwoArgs(method, arg1, arg2), method);
  }

  Map<String, dynamic> callOneInt(String method, int arg) {
    return _decodeEnvelope(callRawOneInt(method, arg), method);
  }

  Map<String, dynamic> callOneArgOneInt(
    String method,
    String arg,
    int value,
  ) {
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
      return {
        'success': false,
        'error': '$method returned non-object JSON',
      };
    } catch (e) {
      return {
        'success': false,
        'error': '$method returned invalid JSON: $e',
      };
    }
  }
}
