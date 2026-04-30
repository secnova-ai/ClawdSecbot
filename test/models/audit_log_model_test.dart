import 'package:bot_sec_manager/models/audit_log_model.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  group('AuditMessage', () {
    test('serializes to and from JSON round-trip', () {
      final original = AuditMessage(
        index: 0,
        role: 'user',
        content: 'Hello, world!',
      );
      final json = original.toJson();
      final restored = AuditMessage.fromJson(json);
      expect(restored.index, 0);
      expect(restored.role, 'user');
      expect(restored.content, 'Hello, world!');
    });

    test('defaults to empty values on missing fields', () {
      final restored = AuditMessage.fromJson({});
      expect(restored.index, 0);
      expect(restored.role, '');
      expect(restored.content, '');
    });
  });

  group('AuditToolCall', () {
    test('serializes to and from JSON round-trip', () {
      final original = AuditToolCall(
        name: 'read_file',
        arguments: '{"path": "/etc/passwd"}',
        result: 'file content',
        isSensitive: true,
      );
      final json = original.toJson();
      final restored = AuditToolCall.fromJson(json);
      expect(restored.name, 'read_file');
      expect(restored.arguments, '{"path": "/etc/passwd"}');
      expect(restored.result, 'file content');
      expect(restored.isSensitive, true);
    });

    test('defaults isSensitive to false', () {
      final restored = AuditToolCall.fromJson({'name': 'test'});
      expect(restored.isSensitive, false);
      expect(restored.arguments, '');
    });
  });

  group('AuditLog', () {
    AuditLog buildLog({String action = 'ALLOW', String? riskLevel}) {
      return AuditLog(
        id: 'log-001',
        timestamp: DateTime(2026, 1, 15, 10, 30),
        requestId: 'req-001',
        assetName: 'openclaw',
        assetID: 'openclaw:abc123',
        model: 'gpt-4',
        requestContent: 'user query',
        toolCalls: [AuditToolCall(name: 'exec', arguments: '{"cmd":"ls"}')],
        outputContent: 'response text',
        hasRisk: riskLevel != null,
        riskLevel: riskLevel,
        riskReason: 'tool execution',
        confidence: 85,
        action: action,
        promptTokens: 100,
        completionTokens: 50,
        totalTokens: 150,
        durationMs: 1200,
        messages: [
          AuditMessage(index: 0, role: 'user', content: 'query'),
          AuditMessage(index: 1, role: 'assistant', content: 'answer'),
        ],
        messageCount: 2,
      );
    }

    test('serializes to and from JSON round-trip', () {
      final original = buildLog(riskLevel: 'DANGEROUS');
      final json = original.toJson();
      final restored = AuditLog.fromJson(json);

      expect(restored.id, 'log-001');
      expect(restored.requestId, 'req-001');
      expect(restored.assetName, 'openclaw');
      expect(restored.assetID, 'openclaw:abc123');
      expect(restored.model, 'gpt-4');
      expect(restored.requestContent, 'user query');
      expect(restored.toolCalls.length, 1);
      expect(restored.toolCalls[0].name, 'exec');
      expect(restored.outputContent, 'response text');
      expect(restored.hasRisk, true);
      expect(restored.riskLevel, 'DANGEROUS');
      expect(restored.riskReason, 'tool execution');
      expect(restored.confidence, 85);
      expect(restored.action, 'ALLOW');
      expect(restored.promptTokens, 100);
      expect(restored.completionTokens, 50);
      expect(restored.totalTokens, 150);
      expect(restored.durationMs, 1200);
      expect(restored.messages.length, 2);
      expect(restored.messages[0].role, 'user');
      expect(restored.messageCount, 2);
    });

    test('riskLevelColor maps correctly', () {
      expect(
        buildLog(riskLevel: 'CRITICAL').riskLevelColor,
        RiskLevelColor.critical,
      );
      expect(
        buildLog(riskLevel: 'DANGEROUS').riskLevelColor,
        RiskLevelColor.dangerous,
      );
      expect(
        buildLog(riskLevel: 'SUSPICIOUS').riskLevelColor,
        RiskLevelColor.suspicious,
      );
      expect(buildLog(riskLevel: 'SAFE').riskLevelColor, RiskLevelColor.safe);
      expect(buildLog(riskLevel: null).riskLevelColor, RiskLevelColor.safe);
    });

    test('actionDisplayText returns correct Chinese text', () {
      expect(buildLog(action: 'BLOCK').actionDisplayText, '已拦截');
      expect(buildLog(action: 'HARD_BLOCK').actionDisplayText, '强制拦截');
      expect(buildLog(action: 'WARN').actionDisplayText, '警告');
      expect(buildLog(action: 'ALLOW').actionDisplayText, '允许');
    });

    test('handles missing optional fields gracefully', () {
      final json = {
        'id': 'id1',
        'timestamp': '2026-01-01T00:00:00Z',
        'request_id': 'r1',
        'request_content': 'text',
        'action': 'ALLOW',
        'duration_ms': 100,
      };
      final log = AuditLog.fromJson(json);
      expect(log.assetName, '');
      expect(log.model, isNull);
      expect(log.toolCalls, isEmpty);
      expect(log.messages, isEmpty);
      expect(log.hasRisk, false);
    });

    test('_parseMessages handles JSON string input', () {
      final json = {
        'id': 'id1',
        'timestamp': '2026-01-01T00:00:00Z',
        'request_id': 'r1',
        'request_content': '',
        'action': 'ALLOW',
        'duration_ms': 0,
        'messages': '[{"index":0,"role":"user","content":"hi"}]',
      };
      final log = AuditLog.fromJson(json);
      expect(log.messages.length, 1);
      expect(log.messages[0].content, 'hi');
    });

    test('_parseMessages handles malformed string gracefully', () {
      final json = {
        'id': 'id1',
        'timestamp': '2026-01-01T00:00:00Z',
        'request_id': 'r1',
        'request_content': '',
        'action': 'ALLOW',
        'duration_ms': 0,
        'messages': 'not-json',
      };
      final log = AuditLog.fromJson(json);
      expect(log.messages, isEmpty);
    });
  });

  group('AuditLogQueryResult', () {
    test('parses from JSON with logs list', () {
      final json = {
        'logs': [
          {
            'id': 'id1',
            'timestamp': '2026-01-01T00:00:00Z',
            'request_id': 'r1',
            'request_content': '',
            'action': 'ALLOW',
            'duration_ms': 0,
          },
        ],
        'total': 1,
      };
      final result = AuditLogQueryResult.fromJson(json);
      expect(result.logs.length, 1);
      expect(result.total, 1);
    });

    test('defaults to empty list when logs is null', () {
      final result = AuditLogQueryResult.fromJson({'total': 0});
      expect(result.logs, isEmpty);
      expect(result.total, 0);
    });
  });
}
