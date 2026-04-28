// ignore_for_file: deprecated_member_use, avoid_web_libraries_in_flutter

import 'dart:async';
import 'dart:convert';
import 'dart:html' as html;

import '../core_transport/http_transport_web.dart';
import '../models/version_info.dart';
import '../utils/app_logger.dart';

// Keep typedef names for compatibility with import hide clauses.
typedef FreeStringC = void Function();
typedef FreeStringDart = void Function();

enum BridgeMessageType {
  log,
  metrics,
  status,
  versionUpdate,
  securityEvent,
  truthRecord,
}

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

/// Web message bridge service based on SSE endpoint `/api/v1/events`.
class MessageBridgeService {
  static final MessageBridgeService _instance =
      MessageBridgeService._internal();
  factory MessageBridgeService() => _instance;
  MessageBridgeService._internal();

  final StreamController<BridgeMessage> _messageController =
      StreamController<BridgeMessage>.broadcast();
  final StreamController<Exception> _errorController =
      StreamController<Exception>.broadcast();

  html.EventSource? _eventSource;
  bool _isRunning = false;
  bool _isInitialized = false;
  int _subscriberCount = 0;

  Stream<BridgeMessage> get messageStream => _messageController.stream;
  Stream<Exception> get errorStream => _errorController.stream;
  bool get isRunning => _isRunning;
  bool get isInitialized => _isInitialized;

  Stream<String> get logStream => messageStream
      .where((msg) => msg.type == BridgeMessageType.log)
      .map((msg) => jsonEncode(msg.payload));

  Stream<Map<String, dynamic>> get metricsStream => messageStream
      .where((msg) => msg.type == BridgeMessageType.metrics)
      .map((msg) => msg.payload);

  Stream<Map<String, dynamic>> get statusStream => messageStream
      .where((msg) => msg.type == BridgeMessageType.status)
      .map((msg) => msg.payload);

  Stream<VersionInfo> get versionUpdateStream => messageStream
      .where((msg) => msg.type == BridgeMessageType.versionUpdate)
      .map((msg) => VersionInfo.fromJson(msg.payload));

  Stream<Map<String, dynamic>> get securityEventStream => messageStream
      .where((msg) => msg.type == BridgeMessageType.securityEvent)
      .map((msg) => msg.payload);

  Stream<Map<String, dynamic>> get truthRecordStream => messageStream
      .where((msg) => msg.type == BridgeMessageType.truthRecord)
      .map((msg) => msg.payload);

  Future<bool> initialize() async {
    if (_isInitialized) {
      _subscriberCount++;
      return true;
    }

    try {
      final baseUrl = _resolveApiBaseUrl();
      _eventSource = html.EventSource(_eventsUrl(baseUrl));

      _eventSource!.onOpen.listen((_) {
        appLogger.info('[MessageBridgeWeb] SSE connected');
      });
      _eventSource!.onMessage.listen((event) {
        _onEventData(event.data);
      });
      _eventSource!.onError.listen((_) {
        appLogger.warning('[MessageBridgeWeb] SSE disconnected');
      });

      _isRunning = true;
      _isInitialized = true;
      _subscriberCount = 1;
      return true;
    } catch (e) {
      appLogger.error('[MessageBridgeWeb] Initialization error', e);
      _isRunning = false;
      _isInitialized = false;
      _subscriberCount = 0;
      return false;
    }
  }

  void _onEventData(String? raw) {
    if (!_isRunning || raw == null || raw.isEmpty) return;
    try {
      final decoded = jsonDecode(raw);
      if (decoded is! Map<String, dynamic>) {
        return;
      }
      final message = BridgeMessage.fromJson(decoded);
      if (!_messageController.isClosed) {
        _messageController.add(message);
      }
    } catch (_) {
      // Ignore malformed event data.
    }
  }

  bool isGoBridgeRunning() => _isRunning;

  void dispose() {
    if (_subscriberCount > 0) {
      _subscriberCount--;
    }
    if (_subscriberCount > 0) {
      return;
    }
    _isRunning = false;
    _isInitialized = false;
    _eventSource?.close();
    _eventSource = null;
  }

  String _resolveApiBaseUrl() {
    final queryApi = Uri.base.queryParameters['api_base_url']?.trim();
    if (queryApi != null && queryApi.isNotEmpty) {
      return queryApi.endsWith('/')
          ? queryApi.substring(0, queryApi.length - 1)
          : queryApi;
    }

    const envApi = String.fromEnvironment(
      'BOTSEC_WEB_API_BASE_URL',
      defaultValue: '',
    );
    if (envApi.isNotEmpty) {
      return envApi.endsWith('/')
          ? envApi.substring(0, envApi.length - 1)
          : envApi;
    }

    const apiPort = String.fromEnvironment(
      'BOTSEC_WEB_API_PORT',
      defaultValue: '18080',
    );
    final scheme = Uri.base.scheme == 'https' ? 'https' : 'http';
    final host = Uri.base.host.isNotEmpty ? Uri.base.host : '127.0.0.1';
    return '$scheme://$host:$apiPort';
  }

  String _eventsUrl(String baseUrl) {
    var url = '$baseUrl/api/v1/events';
    try {
      final token =
          html.window.sessionStorage[HttpTransportWeb.authTokenStorageKey]
              ?.trim() ??
          '';
      if (token.isEmpty) {
        return url;
      }
      final uri = Uri.parse(url);
      url = uri
          .replace(
            queryParameters: {...uri.queryParameters, 'access_token': token},
          )
          .toString();
    } catch (_) {}
    return url;
  }
}
