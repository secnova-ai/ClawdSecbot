// ignore_for_file: deprecated_member_use, avoid_web_libraries_in_flutter

import 'dart:async';
import 'dart:convert';
import 'dart:html' as html;

import 'botsec_transport.dart';

/// Web transport that calls Go web bridge over HTTP.
///
/// This implementation uses synchronous XHR to keep the same sync call shape
/// as FFI transport, so existing service facades can be reused gradually.
class HttpTransportWeb extends BotsecTransport {
  HttpTransportWeb({required String apiBaseUrl, bool isBootstrapped = false})
    : _apiBaseUrl = _normalizeBaseUrl(apiBaseUrl),
      _isBootstrapped = isBootstrapped;

  String _apiBaseUrl;
  bool _isBootstrapped;

  String get apiBaseUrl => _apiBaseUrl;

  @override
  bool get isReady => _apiBaseUrl.isNotEmpty && _isBootstrapped;

  void updateApiBaseUrl(String apiBaseUrl) {
    _apiBaseUrl = _normalizeBaseUrl(apiBaseUrl);
  }

  Map<String, dynamic> bootstrapInit({
    required String workspaceDirPrefix,
    required String homeDir,
    required String currentVersion,
  }) {
    final body = jsonEncode({
      'workspace_dir_prefix': workspaceDirPrefix,
      'home_dir': homeDir,
      'current_version': currentVersion,
    });
    final raw = _postRaw('/api/v1/bootstrap/init', body);
    final decoded = _decodeEnvelope(raw, 'bootstrap/init');
    if (decoded['success'] == true) {
      _isBootstrapped = true;
    }
    return decoded;
  }

  Map<String, dynamic> health() {
    final raw = _getRaw('/health');
    return _decodeEnvelope(raw, 'health');
  }

  Map<String, dynamic> claimUiSession({
    required String clientID,
    String clientLabel = '',
  }) {
    final body = jsonEncode({
      'client_id': clientID,
      'client_label': clientLabel,
    });
    final raw = _postRaw('/api/v1/session/claim', body);
    return _decodeEnvelope(raw, 'session/claim');
  }

  Map<String, dynamic> heartbeatUiSession({
    required String clientID,
    String clientLabel = '',
  }) {
    final body = jsonEncode({
      'client_id': clientID,
      'client_label': clientLabel,
    });
    final raw = _postRaw('/api/v1/session/heartbeat', body);
    return _decodeEnvelope(raw, 'session/heartbeat');
  }

  Map<String, dynamic> releaseUiSession({
    required String clientID,
    String clientLabel = '',
  }) {
    final body = jsonEncode({
      'client_id': clientID,
      'client_label': clientLabel,
    });
    final raw = _postRaw('/api/v1/session/release', body);
    return _decodeEnvelope(raw, 'session/release');
  }

  String callRaw(
    String method, {
    List<String> strings = const [],
    List<int> ints = const [],
  }) {
    final payload = jsonEncode({'strings': strings, 'ints': ints});
    return _postRaw('/api/v1/rpc/$method', payload);
  }

  @override
  String callRawNoArg(String method) {
    return callRaw(method);
  }

  @override
  Future<String> callRawNoArgAsync(String method) {
    final payload = jsonEncode({
      'strings': const <String>[],
      'ints': const <int>[],
    });
    return _postRawAsync('/api/v1/rpc/$method', payload);
  }

  @override
  String callRawOneArg(String method, String arg) {
    return callRaw(method, strings: [arg]);
  }

  @override
  Future<String> callRawOneArgAsync(String method, String arg) {
    final payload = jsonEncode({
      'strings': [arg],
      'ints': const <int>[],
    });
    return _postRawAsync('/api/v1/rpc/$method', payload);
  }

  @override
  String callRawTwoArgs(String method, String arg1, String arg2) {
    return callRaw(method, strings: [arg1, arg2]);
  }

  @override
  Future<String> callRawTwoArgsAsync(String method, String arg1, String arg2) {
    final payload = jsonEncode({
      'strings': [arg1, arg2],
      'ints': const <int>[],
    });
    return _postRawAsync('/api/v1/rpc/$method', payload);
  }

  @override
  String callRawOneInt(String method, int arg) {
    return callRaw(method, ints: [arg]);
  }

  @override
  String callRawOneArgOneInt(String method, String arg, int value) {
    return callRaw(method, strings: [arg], ints: [value]);
  }

  @override
  String callRawThreeInts(String method, int arg1, int arg2, int arg3) {
    return callRaw(method, ints: [arg1, arg2, arg3]);
  }

  String _getRaw(String path) {
    try {
      final req = html.HttpRequest();
      req.open('GET', '$_apiBaseUrl$path', async: false);
      req.withCredentials = false;
      req.send();
      final status = req.status ?? 0;
      final body = req.responseText ?? '';
      if (status >= 200 && status < 300 && body.isNotEmpty) {
        return body;
      }
      return _errorJson('GET $path failed: HTTP $status');
    } catch (e) {
      return _errorJson('GET $path failed: $e');
    }
  }

  String _postRaw(String path, String body) {
    if (_apiBaseUrl.isEmpty) {
      return _errorJson('empty api base url');
    }

    final candidates = _candidateBaseUrls(_apiBaseUrl);
    String? lastError;

    for (final base in candidates) {
      final raw = _postRawOnce(base, path, body);
      if (!_isConnectivityFailure(raw)) {
        if (base != _apiBaseUrl) {
          _apiBaseUrl = base;
        }
        return raw;
      }
      lastError = raw;
    }

    return lastError ?? _errorJson('POST $path failed: unknown error');
  }

  Future<String> _postRawAsync(String path, String body) async {
    if (_apiBaseUrl.isEmpty) {
      return _errorJson('empty api base url');
    }

    final candidates = _candidateBaseUrls(_apiBaseUrl);
    String? lastError;

    for (final base in candidates) {
      final raw = await _postRawOnceAsync(base, path, body);
      if (!_isConnectivityFailure(raw)) {
        if (base != _apiBaseUrl) {
          _apiBaseUrl = base;
        }
        return raw;
      }
      lastError = raw;
    }

    return lastError ?? _errorJson('POST $path failed: unknown error');
  }

  String _postRawOnce(String baseUrl, String path, String body) {
    try {
      final req = html.HttpRequest();
      req.open('POST', '$baseUrl$path', async: false);
      req.withCredentials = false;
      // Keep request "simple" to avoid browser preflight edge-cases.
      req.send(body);
      final status = req.status ?? 0;
      final respBody = req.responseText ?? '';
      if (status >= 200 && status < 300 && respBody.isNotEmpty) {
        return respBody;
      }
      if (respBody.isNotEmpty) {
        return respBody;
      }
      return _errorJson('POST $path failed: HTTP $status');
    } catch (e) {
      return _errorJson('POST $path failed: $e');
    }
  }

  Future<String> _postRawOnceAsync(String baseUrl, String path, String body) {
    final completer = Completer<String>();
    try {
      final req = html.HttpRequest();
      req.open('POST', '$baseUrl$path', async: true);
      req.withCredentials = false;
      req.timeout = 60000;
      void complete(String raw) {
        if (!completer.isCompleted) {
          completer.complete(raw);
        }
      }

      req.onLoadEnd.listen((_) {
        final status = req.status ?? 0;
        final respBody = req.responseText ?? '';
        if (status >= 200 && status < 300 && respBody.isNotEmpty) {
          complete(respBody);
          return;
        }
        if (respBody.isNotEmpty) {
          complete(respBody);
          return;
        }
        complete(_errorJson('POST $path failed: HTTP $status'));
      });
      req.onError.listen((_) {
        complete(_errorJson('POST $path failed: XMLHttpRequest error'));
      });
      req.onTimeout.listen((_) {
        complete(_errorJson('POST $path failed: timeout'));
      });
      req.send(body);
    } catch (e) {
      if (!completer.isCompleted) {
        completer.complete(_errorJson('POST $path failed: $e'));
      }
    }
    return completer.future;
  }

  List<String> _candidateBaseUrls(String baseUrl) {
    final trimmed = _normalizeBaseUrl(baseUrl);
    if (trimmed.isEmpty) {
      return <String>[trimmed];
    }

    final uri = Uri.tryParse(trimmed);
    if (uri == null || uri.host.isEmpty) {
      return <String>[trimmed];
    }

    final candidates = <String>[trimmed];
    if (uri.host == '127.0.0.1') {
      candidates.add(_swapHost(uri, 'localhost'));
    } else if (uri.host.toLowerCase() == 'localhost') {
      candidates.add(_swapHost(uri, '127.0.0.1'));
    }
    return candidates;
  }

  String _swapHost(Uri source, String host) {
    if (source.hasPort) {
      return Uri(
        scheme: source.scheme,
        userInfo: source.userInfo,
        host: host,
        port: source.port,
      ).toString();
    }
    return Uri(
      scheme: source.scheme,
      userInfo: source.userInfo,
      host: host,
    ).toString();
  }

  bool _isConnectivityFailure(String raw) {
    try {
      final decoded = jsonDecode(raw);
      if (decoded is! Map<String, dynamic>) {
        return false;
      }
      if (decoded['success'] == true) {
        return false;
      }
      final error = decoded['error']?.toString().toLowerCase() ?? '';
      return error.contains('http 0') ||
          error.contains('failed to connect') ||
          error.contains('networkerror') ||
          error.contains('xmlhttprequest') ||
          error.contains('connection refused');
    } catch (_) {
      return false;
    }
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

  String _errorJson(String message) {
    final escaped = message.replaceAll('"', '\\"');
    return '{"success":false,"error":"$escaped"}';
  }

  static String _normalizeBaseUrl(String input) {
    final trimmed = input.trim();
    if (trimmed.isEmpty) {
      return '';
    }
    return trimmed.endsWith('/')
        ? trimmed.substring(0, trimmed.length - 1)
        : trimmed;
  }
}
