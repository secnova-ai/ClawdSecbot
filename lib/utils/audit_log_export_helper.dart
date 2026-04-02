import '../models/audit_log_model.dart';
import '../models/security_event_model.dart';

/// 生成审计日志详情的 Markdown 文本（.md 导出）。
String buildAuditLogMarkdownContent({
  required bool isZh,
  required AuditLog log,
  required List<SecurityEvent> relatedEvents,
  required String rawText,
  required String actionText,
  required String eventText,
}) {
  final title = isZh ? '审计日志详情导出' : 'Audit Log Detail Export';
  final safeRaw = _escapeMarkdown(rawText);
  final safeAction = _escapeMarkdown(actionText);
  final safeEvent = _escapeMarkdown(
    relatedEvents.isEmpty
        ? (isZh ? '暂无关联安全事件' : 'No related security events')
        : eventText,
  );
  return '''
# $title

## Meta
- ID: ${_escapeMarkdown(log.id)}
- Request ID: ${_escapeMarkdown(log.requestId)}
- Timestamp: ${_escapeMarkdown(log.timestamp.toIso8601String())}
- Asset: ${_escapeMarkdown(log.assetName)} (${_escapeMarkdown(log.assetID)})
- Model: ${_escapeMarkdown(log.model ?? '')}
- Action: ${_escapeMarkdown(log.action)}
- Risk Level: ${_escapeMarkdown(log.riskLevel ?? '')}
- Risk Reason: ${_escapeMarkdown(log.riskReason ?? '')}

## ${isZh ? '原始' : 'Raw'}
```text
$safeRaw
```

## ${isZh ? '动作' : 'Actions'}
```text
$safeAction
```

## ${isZh ? '事件' : 'Events'}
```text
$safeEvent
```
''';
}

/// Markdown 转义，避免与格式语法冲突。
String _escapeMarkdown(String input) {
  return input
      .replaceAll('```', '\\`\\`\\`')
      .replaceAll('\r\n', '\n')
      .replaceAll('\r', '\n');
}
