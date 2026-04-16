import 'botsec_transport.dart';

class _NoopTransport extends BotsecTransport {
  @override
  bool get isReady => false;

  @override
  String callRawNoArg(String method) => _err(method);

  @override
  String callRawOneArg(String method, String arg) => _err(method);

  @override
  String callRawTwoArgs(String method, String arg1, String arg2) =>
      _err(method);

  @override
  String callRawOneInt(String method, int arg) => _err(method);

  @override
  String callRawOneArgOneInt(String method, String arg, int value) =>
      _err(method);

  @override
  String callRawThreeInts(String method, int arg1, int arg2, int arg3) =>
      _err(method);

  String _err(String method) {
    return '{"success":false,"error":"Transport not initialized for $method"}';
  }
}

BotsecTransport createDefaultTransport() => _NoopTransport();
