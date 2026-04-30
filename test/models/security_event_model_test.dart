import 'package:bot_sec_manager/models/security_event_model.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  group('SecurityEvent', () {
    SecurityEvent buildEvent({
      String eventType = 'tool_execution',
      String source = 'react_agent',
    }) {
      return SecurityEvent(
        id: 'evt-001',
        timestamp: DateTime(2026, 3, 10, 14, 30),
        eventType: eventType,
        actionDesc: '执行文件读取操作',
        riskType: '敏感文件访问',
        detail: '尝试读取 /etc/shadow',
        source: source,
        assetName: 'openclaw',
        assetID: 'openclaw:abc123',
        requestID: 'req-001',
      );
    }

    test('serializes to and from JSON round-trip', () {
      final original = buildEvent();
      final json = original.toJson();
      final restored = SecurityEvent.fromJson(json);

      expect(restored.id, 'evt-001');
      expect(restored.eventType, 'tool_execution');
      expect(restored.actionDesc, '执行文件读取操作');
      expect(restored.riskType, '敏感文件访问');
      expect(restored.detail, '尝试读取 /etc/shadow');
      expect(restored.source, 'react_agent');
      expect(restored.assetName, 'openclaw');
      expect(restored.assetID, 'openclaw:abc123');
      expect(restored.requestID, 'req-001');
    });

    test('isBlocked returns true for blocked event type', () {
      expect(buildEvent(eventType: 'blocked').isBlocked, true);
      expect(buildEvent(eventType: 'tool_execution').isBlocked, false);
    });

    test('isToolExecution returns true for tool_execution type', () {
      expect(buildEvent(eventType: 'tool_execution').isToolExecution, true);
      expect(buildEvent(eventType: 'blocked').isToolExecution, false);
    });

    test('isNeedsConfirmation returns true for needs_confirmation type', () {
      expect(
        buildEvent(eventType: 'needs_confirmation').isNeedsConfirmation,
        true,
      );
      expect(buildEvent(eventType: 'blocked').isNeedsConfirmation, false);
    });

    test('isFromReactAgent returns true when source is react_agent', () {
      expect(buildEvent(source: 'react_agent').isFromReactAgent, true);
      expect(buildEvent(source: 'heuristic').isFromReactAgent, false);
    });

    test('isFromHeuristic returns true when source is heuristic', () {
      expect(buildEvent(source: 'heuristic').isFromHeuristic, true);
      expect(buildEvent(source: 'react_agent').isFromHeuristic, false);
    });

    test('handles missing fields with defaults', () {
      final restored = SecurityEvent.fromJson({});
      expect(restored.id, '');
      expect(restored.eventType, 'other');
      expect(restored.actionDesc, '');
      expect(restored.riskType, '');
      expect(restored.assetName, '');
    });

    test('_parseTimestamp handles null gracefully', () {
      final restored = SecurityEvent.fromJson({'id': 'test'});
      expect(restored.timestamp, isA<DateTime>());
    });

    test('_parseTimestamp handles empty string gracefully', () {
      final restored = SecurityEvent.fromJson({'id': 'test', 'timestamp': ''});
      expect(restored.timestamp, isA<DateTime>());
    });

    test('_parseTimestamp parses valid ISO 8601 string', () {
      final restored = SecurityEvent.fromJson({
        'id': 'test',
        'timestamp': '2026-03-10T14:30:00Z',
      });
      expect(restored.timestamp.year, 2026);
      expect(restored.timestamp.month, 3);
    });
  });
}
