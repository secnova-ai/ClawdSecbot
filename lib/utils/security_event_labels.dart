import '../l10n/app_localizations.dart';

bool _isZh(AppLocalizations l10n) =>
    l10n.localeName.toLowerCase().startsWith('zh');

String localizeSecurityEventType(String raw, AppLocalizations l10n) {
  switch (raw.trim().toLowerCase()) {
    case 'blocked':
      return l10n.eventBlocked;
    case 'needs_confirmation':
      return l10n.riskTypeNeedsConfirmation;
    case 'tool_execution':
      return l10n.eventToolExecution;
    case 'rewritten':
      return _isZh(l10n) ? '已重写' : 'Rewritten';
    case 'redacted':
      return _isZh(l10n) ? '已脱敏' : 'Redacted';
    case 'allowed':
      return _isZh(l10n) ? '已允许' : 'Allowed';
    case 'warning':
      return l10n.eventTypeWarning;
    case 'other':
      return l10n.eventOther;
    default:
      return raw;
  }
}

String localizeSecurityRiskType(String raw, AppLocalizations l10n) {
  final isZh = _isZh(l10n);
  switch (raw.trim().toUpperCase()) {
    case 'PROMPT_INJECTION_DIRECT':
      return isZh ? '直接提示词注入' : 'Direct Prompt Injection';
    case 'PROMPT_INJECTION_INDIRECT':
      return isZh ? '间接提示词注入' : 'Indirect Prompt Injection';
    case 'SENSITIVE_DATA_EXFILTRATION':
      return isZh ? '敏感数据外泄' : 'Sensitive Data Exfiltration';
    case 'HIGH_RISK_OPERATION':
      return isZh ? '高危操作' : 'High-Risk Operation';
    case 'PRIVILEGE_ABUSE':
      return isZh ? '权限滥用' : 'Privilege Abuse';
    case 'UNEXPECTED_CODE_EXECUTION':
      return isZh ? '非预期代码执行' : 'Unexpected Code Execution';
    case 'CONTEXT_POISONING':
      return isZh ? '上下文污染' : 'Context Poisoning';
    case 'SUPPLY_CHAIN_RISK':
      return isZh ? '供应链风险' : 'Supply Chain Risk';
    case 'HUMAN_TRUST_EXPLOITATION':
      return isZh ? '人类信任利用' : 'Human Trust Exploitation';
    case 'CASCADING_FAILURE':
      return isZh ? '级联故障风险' : 'Cascading Failure Risk';
    case 'QUOTA':
      return l10n.riskTypeQuota;
    case 'SANDBOX_BLOCKED':
      return l10n.riskTypeSandboxBlocked;
    case 'NEEDS_CONFIRMATION':
      return l10n.riskTypeNeedsConfirmation;
    default:
      return raw;
  }
}

String localizeSecurityActionDesc(String raw, AppLocalizations l10n) {
  final trimmed = raw.trim();
  if (trimmed.isEmpty) return trimmed;
  final isZh = _isZh(l10n);
  switch (trimmed) {
    case 'Historical blocked tool result rewritten':
      return isZh ? '历史已拦截工具结果已重写' : 'Historical blocked tool result rewritten';
    case 'Conversation token quota exceeded':
      return isZh ? '会话 Token 配额已超限' : 'Conversation token quota exceeded';
    case 'Daily token quota exceeded':
      return isZh ? '每日 Token 配额已超限' : 'Daily token quota exceeded';
    case 'Final result references quarantined tool result':
      return isZh
          ? '最终输出引用了已隔离的工具结果'
          : 'Final result references quarantined tool result';
    case 'Final result sensitive data redacted':
      return isZh ? '最终输出中的敏感数据已脱敏' : 'Final result sensitive data redacted';
    case 'Risk detected by security detector':
      return isZh ? '安全检测器发现风险' : 'Risk detected by security detector';
    case 'User input risk detected by ShepherdGate semantic analysis':
      return isZh
          ? 'ShepherdGate 语义分析发现用户输入风险'
          : 'User input risk detected by ShepherdGate semantic analysis';
    case 'Tool call risk detected by ShepherdGate ReAct analysis':
      return isZh
          ? 'ShepherdGate ReAct 分析发现工具调用风险'
          : 'Tool call risk detected by ShepherdGate ReAct analysis';
    case 'Guard output format violation requires human confirmation.':
      return isZh
          ? '安全模型输出格式异常，需要人工确认'
          : 'Guard output format violation requires human confirmation.';
    case 'Tool call matches user-defined semantic rule':
      return isZh
          ? '工具调用命中用户自定义语义规则'
          : 'Tool call matches user-defined semantic rule';
    case 'Final output violates user rule':
      return isZh ? '最终输出违反用户自定义规则' : 'Final output violates user rule';
    case 'Direct prompt injection in user input':
      return isZh ? '用户输入存在直接提示词注入' : 'Direct prompt injection in user input';
    default:
      return trimmed;
  }
}
