import '../l10n/app_localizations.dart';
import '../models/risk_model.dart';
import '../utils/runtime_platform.dart';

String localizeScanRiskTitle(RiskInfo risk, AppLocalizations l10n) {
  switch (risk.id) {
    case 'riskNonLoopbackBinding':
    case 'gateway_bind_unsafe':
      return l10n.riskNonLoopbackBinding;
    case 'riskNoAuth':
    case 'gateway_auth_disabled':
      return l10n.riskNoAuth;
    case 'gateway_auth_password_mode':
      return l10n.riskGatewayAuthPasswordMode;
    case 'riskWeakPassword':
    case 'gateway_weak_password':
      return l10n.riskWeakPassword;
    case 'gateway_weak_token':
      return l10n.riskGatewayWeakToken;
    case 'riskAllPluginsAllowed':
      return l10n.riskAllPluginsAllowed;
    case 'riskControlUiEnabled':
      return l10n.riskControlUiEnabled;
    case 'riskRunningAsRoot':
      return l10n.riskRunningAsRoot;
    case 'config_perm_unsafe':
      return l10n.riskConfigPermUnsafe;
    case 'config_dir_perm_unsafe':
      return l10n.riskConfigDirPermUnsafe;
    case 'sandbox_disabled_default':
      return l10n.riskSandboxDisabledDefault;
    case 'sandbox_disabled_agent':
      return l10n.riskSandboxDisabledAgent;
    case 'logging_redact_off':
      return l10n.riskLoggingRedactOff;
    case 'audit_disabled':
      return l10n.riskAuditDisabled;
    case 'autonomy_workspace_unrestricted':
      return l10n.riskAutonomyWorkspaceUnrestricted;
    case 'log_dir_perm_unsafe':
      return l10n.riskLogDirPermUnsafe;
    case 'plaintext_secrets':
      return l10n.riskPlaintextSecrets;
    case 'memory_dir_perm_unsafe':
      return l10n.riskMemoryDirPermUnsafe;
    case 'process_running_as_root':
      return l10n.riskProcessRunningAsRoot;
    case 'skill_agent_risk':
      return l10n.riskSkillAgentRisk;
    case 'skills_not_scanned':
      return l10n.riskSkillsNotScanned;
    case 'openclaw_insecure_or_dangerous_flags':
      return l10n.riskOpenclawInsecureOrDangerousFlags;
    case 'openclaw_config_patch_outdated':
      return l10n.riskOpenclawConfigPatchOutdated;
    case 'terminal_backend_local':
      return l10n.riskTerminalBackendLocal;
    case 'approvals_mode_disabled':
      return l10n.riskApprovalsModeDisabled;
    case 'redact_secrets_disabled':
      return l10n.riskRedactSecretsDisabled;
    case 'model_base_url_public':
      return l10n.riskModelBaseUrlPublic;
    case 'riskSkillSecurityIssue':
      return l10n.riskSkillSecurityIssue(
        risk.args?['skillName']?.toString() ?? risk.title,
      );
    default:
      return risk.displayTitle(l10n.localeName);
  }
}

String localizeScanRiskDescription(RiskInfo risk, AppLocalizations l10n) {
  switch (risk.id) {
    case 'riskNonLoopbackBinding':
    case 'gateway_bind_unsafe':
      return l10n.riskNonLoopbackBindingDesc(
        risk.args?['bind']?.toString() ?? '',
      );
    case 'riskNoAuth':
    case 'gateway_auth_disabled':
      return l10n.riskNoAuthDesc;
    case 'gateway_auth_password_mode':
      return l10n.riskGatewayAuthPasswordModeDesc;
    case 'riskWeakPassword':
    case 'gateway_weak_password':
      return l10n.riskWeakPasswordDesc;
    case 'gateway_weak_token':
      return l10n.riskGatewayWeakTokenDesc;
    case 'riskAllPluginsAllowed':
      return l10n.riskAllPluginsAllowedDesc;
    case 'riskControlUiEnabled':
      return l10n.riskControlUiEnabledDesc;
    case 'riskRunningAsRoot':
      return l10n.riskRunningAsRootDesc;
    case 'config_perm_unsafe':
      if (isRuntimeWindows) {
        return _getWindowsAclRiskDesc(
          risk,
          l10n: l10n,
          fallback: risk.description,
          defaultLabelEn: 'Config file ACL',
          defaultLabelZh: '配置文件 ACL',
        );
      }
      return l10n.riskConfigPermUnsafeDesc(
        risk.args?['path']?.toString() ?? '',
        risk.args?['current']?.toString() ?? '',
      );
    case 'config_dir_perm_unsafe':
      if (isRuntimeWindows) {
        return _getWindowsAclRiskDesc(
          risk,
          l10n: l10n,
          fallback: risk.description,
          defaultLabelEn: 'Config directory ACL',
          defaultLabelZh: '配置目录 ACL',
        );
      }
      return l10n.riskConfigDirPermUnsafeDesc(
        risk.args?['path']?.toString() ?? '',
        risk.args?['current']?.toString() ?? '',
      );
    case 'sandbox_disabled_default':
      return l10n.riskSandboxDisabledDefaultDesc;
    case 'sandbox_disabled_agent':
      return l10n.riskSandboxDisabledAgentDesc(
        risk.args?['agent']?.toString() ?? '',
      );
    case 'logging_redact_off':
      return l10n.riskLoggingRedactOffDesc;
    case 'audit_disabled':
      return l10n.riskAuditDisabledDesc;
    case 'autonomy_workspace_unrestricted':
      return l10n.riskAutonomyWorkspaceUnrestrictedDesc;
    case 'log_dir_perm_unsafe':
      if (isRuntimeWindows) {
        return _getWindowsAclRiskDesc(
          risk,
          l10n: l10n,
          fallback: risk.description,
          defaultLabelEn: 'Log directory ACL',
          defaultLabelZh: '日志目录 ACL',
        );
      }
      return l10n.riskLogDirPermUnsafeDesc;
    case 'plaintext_secrets':
      return l10n.riskPlaintextSecretsDesc(
        risk.args?['pattern']?.toString() ?? '',
      );
    case 'memory_dir_perm_unsafe':
      return l10n.riskMemoryDirPermUnsafeDesc;
    case 'process_running_as_root':
      return l10n.riskProcessRunningAsRootDesc;
    case 'skill_agent_risk':
      return l10n.riskSkillAgentRiskDesc;
    case 'skills_not_scanned':
      return l10n.riskSkillsNotScannedDesc(
        _intArg(risk, 'count'),
        risk.args?['skills']?.toString() ?? '',
      );
    case 'openclaw_insecure_or_dangerous_flags':
      return l10n.riskOpenclawInsecureOrDangerousFlagsDesc(
        _joinArgList(risk.args?['flags']),
      );
    case 'openclaw_config_patch_outdated':
      return l10n.riskOpenclawConfigPatchOutdatedDesc(
        risk.args?['current_version']?.toString() ?? '',
        risk.args?['required_version']?.toString() ?? '',
        risk.args?['advisories']?.toString() ?? '',
      );
    case 'terminal_backend_local':
      return l10n.riskTerminalBackendLocalDesc;
    case 'approvals_mode_disabled':
      return l10n.riskApprovalsModeDisabledDesc(
        risk.args?['mode']?.toString() ?? 'off',
      );
    case 'redact_secrets_disabled':
      return l10n.riskRedactSecretsDisabledDesc;
    case 'model_base_url_public':
      return l10n.riskModelBaseUrlPublicDesc(
        risk.args?['base_url']?.toString() ?? '',
      );
    case 'riskSkillSecurityIssue':
      return l10n.riskSkillSecurityIssueDesc(
        risk.args?['skillName']?.toString() ?? '',
        _intArg(risk, 'issueCount'),
      );
    default:
      return risk.displayDescription(l10n.localeName);
  }
}

int _intArg(RiskInfo risk, String key) {
  final raw = risk.args?[key];
  if (raw is int) return raw;
  return int.tryParse(raw?.toString() ?? '') ?? 0;
}

String _joinArgList(Object? raw) {
  if (raw is Iterable) {
    return raw.map((e) => e.toString()).where((e) => e.isNotEmpty).join('; ');
  }
  return raw?.toString() ?? '';
}

String _getWindowsAclRiskDesc(
  RiskInfo risk, {
  required AppLocalizations l10n,
  required String fallback,
  required String defaultLabelEn,
  required String defaultLabelZh,
}) {
  final args = risk.args;
  if (args == null) return fallback;
  final isZh = l10n.localeName.toLowerCase().startsWith('zh');

  final path = args['path']?.toString() ?? '';
  final summaryRaw = args['acl_summary']?.toString() ?? '';
  final summary = _translateAclSummary(summaryRaw, isZh);
  final violations = _joinArgList(args['acl_violations']);

  final details = <String>[];
  if (path.isNotEmpty) {
    details.add(isZh ? '路径: $path' : 'Path: $path');
  }
  if (summary.isNotEmpty) {
    details.add(isZh ? '摘要: $summary' : 'Summary: $summary');
  }
  if (violations.isNotEmpty) {
    details.add(isZh ? '违规主体: $violations' : 'Violations: $violations');
  }

  if (details.isEmpty) return fallback;
  if (isZh) {
    return '$defaultLabelZh 权限不安全。${details.join('；')}。';
  }
  return '$defaultLabelEn is unsafe. ${details.join(' | ')}.';
}

String _translateAclSummary(String summary, bool isZh) {
  if (!isZh) return summary;
  switch (summary.toLowerCase()) {
    case 'acl safe':
      return 'ACL 安全';
    case 'acl has non-whitelisted principal access':
      return '存在非白名单主体访问权限';
    case 'acl check failed':
      return 'ACL 检查失败';
    default:
      return summary;
  }
}
