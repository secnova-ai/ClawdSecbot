import '../l10n/app_localizations.dart';
import '../models/truth_record_model.dart';

String buildSecurityDecisionValue(
  AppLocalizations l10n,
  TruthRecordModel group, {
  required bool isZh,
}) {
  final reason = group.decisionReason.trim();
  if (group.decisionStatus.trim().toUpperCase() != 'NEEDS_CONFIRMATION') {
    return reason;
  }

  final criteria = _matchJudgmentCriteria(group);
  if (criteria.isEmpty) {
    return '';
  }

  final label = isZh ? '判断准则' : 'Judgment Criteria';
  final localized = criteria
      .map((item) => _localizeJudgmentCriteria(item, isZh: isZh))
      .toList();
  return '$label: ${localized.join(', ')}';
}

List<String> _matchJudgmentCriteria(TruthRecordModel group) {
  final haystack = _securityEvidenceHaystack(group);
  final criteria = <String>{};

  if (_containsAnyText(haystack, const [
    'upload',
    'send',
    'email',
    'external',
    'transfer',
    'exfiltration',
  ])) {
    criteria.add('data_exfiltration');
  }
  if (_containsAnyText(haystack, const [
    'credential',
    'secret',
    'token',
    'password',
    'private key',
    '/etc/shadow',
    '.ssh/id_rsa',
  ])) {
    criteria.add('sensitive_data');
  }
  if (_containsAnyText(haystack, const [
    'delete',
    'remove',
    'chmod',
    'chown',
    'sudo',
    'rm -rf',
    'script',
    'execute',
  ])) {
    criteria.add('destructive_operation');
  }
  if (_containsAnyText(haystack, const [
    'all files',
    'bulk',
    'recursive',
    'entire',
    'whole workspace',
  ])) {
    criteria.add('bulk_scope');
  }
  if (group.toolCalls.isNotEmpty ||
      _containsAnyText(haystack, const [
        'requires confirmation',
        'need confirmation',
        'not allowed',
        'risk',
      ])) {
    criteria.add('explicit_request');
  }

  return criteria.toList();
}

String _securityEvidenceHaystack(TruthRecordModel group) {
  final parts = <String>[
    group.decisionReason,
    group.primaryContent,
    group.outputContent,
    for (final tc in group.toolCalls) ...[tc.name, tc.arguments, tc.result],
  ];
  return parts.join('\n').toLowerCase();
}

bool _containsAnyText(String haystack, List<String> needles) {
  for (final needle in needles) {
    if (haystack.contains(needle.toLowerCase())) {
      return true;
    }
  }
  return false;
}

String _localizeJudgmentCriteria(String criteria, {required bool isZh}) {
  if (!isZh) {
    switch (criteria) {
      case 'data_exfiltration':
        return 'Data exfiltration patterns are high risk';
      case 'sensitive_data':
        return 'Sensitive data operations require special attention';
      case 'destructive_operation':
        return 'Destructive operations require explicit user intent';
      case 'bulk_scope':
        return "Bulk operation scope must match the user's requested scope";
      case 'explicit_request':
        return "Tool calls deviating from the user's explicit request";
      default:
        return criteria;
    }
  }

  switch (criteria) {
    case 'data_exfiltration':
      return '数据外泄模式属于高风险';
    case 'sensitive_data':
      return '敏感数据操作需要特别关注';
    case 'destructive_operation':
      return '破坏性操作需要明确用户意图';
    case 'bulk_scope':
      return '批量操作范围必须与用户请求范围一致';
    case 'explicit_request':
      return '工具调用偏离用户明确请求';
    default:
      return criteria;
  }
}
