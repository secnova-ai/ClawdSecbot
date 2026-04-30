import 'package:bot_sec_manager/models/audit_log_model.dart';
import 'package:bot_sec_manager/models/security_event_model.dart';
import 'package:bot_sec_manager/utils/audit_log_export_helper.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  AuditLog buildLog() {
    return AuditLog(
      id: 'log-001',
      timestamp: DateTime(2026, 1, 15, 10, 30),
      requestId: 'req-001',
      assetName: 'openclaw',
      assetID: 'openclaw:abc123',
      model: 'gpt-4',
      requestContent: 'test',
      toolCalls: [],
      hasRisk: true,
      riskLevel: 'HIGH',
      riskReason: 'tool execution',
      action: 'WARN',
      durationMs: 100,
    );
  }

  group('buildAuditLogMarkdownContent', () {
    test('generates markdown with all sections for Chinese locale', () {
      final md = buildAuditLogMarkdownContent(
        isZh: true,
        log: buildLog(),
        relatedEvents: [],
        rawText: 'raw content',
        actionText: 'action content',
        eventText: 'event content',
      );

      expect(md, contains('# 审计日志详情导出'));
      expect(md, contains('## Meta'));
      expect(md, contains('log-001'));
      expect(md, contains('req-001'));
      expect(md, contains('openclaw'));
      expect(md, contains('gpt-4'));
      expect(md, contains('HIGH'));
      expect(md, contains('## 原始'));
      expect(md, contains('raw content'));
      expect(md, contains('## 动作'));
      expect(md, contains('action content'));
      expect(md, contains('## 事件'));
      expect(md, contains('暂无关联安全事件'));
    });

    test('generates markdown with English locale', () {
      final md = buildAuditLogMarkdownContent(
        isZh: false,
        log: buildLog(),
        relatedEvents: [],
        rawText: 'raw',
        actionText: 'action',
        eventText: 'event',
      );

      expect(md, contains('# Audit Log Detail Export'));
      expect(md, contains('## Raw'));
      expect(md, contains('## Actions'));
      expect(md, contains('## Events'));
      expect(md, contains('No related security events'));
    });

    test('includes related events when provided', () {
      final md = buildAuditLogMarkdownContent(
        isZh: false,
        log: buildLog(),
        relatedEvents: [
          SecurityEvent(
            id: 'evt-1',
            timestamp: DateTime(2026, 1, 15),
            eventType: 'blocked',
            actionDesc: 'blocked action',
            riskType: 'high',
            detail: 'detail',
            source: 'heuristic',
          ),
        ],
        rawText: 'raw',
        actionText: 'action',
        eventText: 'event text here',
      );

      expect(md, contains('event text here'));
    });

    test('escapes triple backticks in content', () {
      final md = buildAuditLogMarkdownContent(
        isZh: false,
        log: buildLog(),
        relatedEvents: [],
        rawText: 'code: ```block```',
        actionText: 'action',
        eventText: 'event',
      );

      expect(md, isNot(contains('```block```')));
      expect(md, contains(r'\`\`\`block\`\`\`'));
    });

    test('normalizes line endings', () {
      final md = buildAuditLogMarkdownContent(
        isZh: false,
        log: buildLog(),
        relatedEvents: [],
        rawText: 'line1\r\nline2\rline3',
        actionText: 'a',
        eventText: 'e',
      );

      expect(md, isNot(contains('\r')));
      expect(md, contains('line1\nline2\nline3'));
    });
  });
}
