import 'dart:io';

import '../l10n/app_localizations.dart';
import '../models/risk_model.dart';

class RiskLocalization {
  static bool _isZh(AppLocalizations l10n) =>
      l10n.localeName.toLowerCase().startsWith('zh');

  static String riskDescription(RiskInfo risk, AppLocalizations l10n) {
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
        if (Platform.isWindows) {
          return _windowsAclDesc(
            risk,
            l10n,
            defaultLabelEn: 'Config file ACL',
            defaultLabelZh: '配置文件 ACL',
          );
        }
        return l10n.riskConfigPermUnsafeDesc(
          risk.args?['path']?.toString() ?? '',
          risk.args?['current']?.toString() ?? '',
        );
      case 'config_dir_perm_unsafe':
        if (Platform.isWindows) {
          return _windowsAclDesc(
            risk,
            l10n,
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
        if (Platform.isWindows) {
          return _windowsAclDesc(
            risk,
            l10n,
            defaultLabelEn: 'Log directory ACL',
            defaultLabelZh: '日志目录 ACL',
          );
        }
        return l10n.riskLogDirPermUnsafeDesc;
      case 'plaintext_secrets':
        return l10n.riskPlaintextSecretsDesc(
          risk.args?['pattern']?.toString() ?? '',
        );
      case 'skills_not_scanned':
        return l10n.riskSkillsNotScannedDesc(
          risk.args?['count'] as int? ?? 0,
          risk.args?['skills']?.toString() ?? '',
        );
      case 'openclaw_1click_rce_vulnerability':
      case 'nullclaw_1click_rce_vulnerability':
        return l10n.riskOneClickRceDesc(
          risk.args?['current_version']?.toString() ?? 'unknown',
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
      case 'process_running_as_root':
        return l10n.riskProcessRunningAsRootDesc;
      case 'memory_dir_perm_unsafe':
        return l10n.riskMemoryDirPermUnsafeDesc;
      case 'skill_agent_risk':
        return l10n.riskSkillAgentRiskDesc;
      case 'riskSkillSecurityIssue':
        return l10n.riskSkillSecurityIssueDesc(
          risk.args?['skillName']?.toString() ?? '',
          risk.args?['issueCount'] as int? ?? 0,
        );
      default:
        return risk.description;
    }
  }

  static String mitigationTitle(RiskInfo risk, AppLocalizations l10n) {
    if (_isZh(l10n)) {
      switch (risk.id) {
        case 'logging_redact_off':
          return '启用敏感日志脱敏';
        case 'gateway_auth_disabled':
          return risk.sourcePlugin == 'nullclaw' ? '启用 Gateway 配对认证' : '启用网关认证';
        case 'gateway_auth_password_mode':
          return risk.sourcePlugin == 'nullclaw' ? '切换密码认证模式' : '切换弱口令认证模式';
        case 'config_perm_unsafe':
          return '修复配置文件权限';
        case 'config_dir_perm_unsafe':
          return '修复配置目录权限';
        case 'gateway_bind_unsafe':
          return '收紧网关监听地址';
        case 'gateway_weak_password':
          return '修复弱密码认证';
        case 'gateway_weak_token':
          return '轮换弱 Token';
        case 'sandbox_disabled_default':
          return risk.sourcePlugin == 'nullclaw' ? '启用默认沙箱' : '启用默认沙箱模式';
        case 'sandbox_disabled_agent':
          return '启用 Agent 沙箱';
        case 'log_dir_perm_unsafe':
          return '修复日志目录权限';
        case 'plaintext_secrets':
          return risk.sourcePlugin == 'nullclaw' ? '移除明文凭据' : '移除明文密钥';
        case 'skills_not_scanned':
          return '扫描未检测的 Skills';
        case 'skill_agent_risk':
          return '处置高风险 Skill';
        case 'audit_disabled':
          return '启用安全审计日志';
        case 'autonomy_workspace_unrestricted':
          return '限制 Agent 工作区访问范围';
        case 'memory_dir_perm_unsafe':
          return '修复 memory 目录权限';
        case 'process_running_as_root':
          return '停止以 root 身份运行 DinTalClaw';
        case 'terminal_backend_local':
          return '切换终端后端到受隔离环境';
        case 'approvals_mode_disabled':
          return '启用审批确认模式';
        case 'redact_secrets_disabled':
          return '启用敏感信息脱敏';
        case 'model_base_url_public':
          return '限制自定义模型公网地址';
        case 'openclaw_1click_rce_vulnerability':
          return '修复 OpenClaw 1-click RCE 漏洞';
        case 'nullclaw_1click_rce_vulnerability':
          return '修复 NullClaw 1-click RCE 漏洞';
      }
    }
    return risk.mitigation?.title ?? risk.title;
  }

  static String mitigationDescription(RiskInfo risk, AppLocalizations l10n) {
    if (_isZh(l10n)) {
      switch (risk.id) {
        case 'logging_redact_off':
          return '该风险可通过修改日志脱敏配置自动修复，启用后会降低敏感数据出现在日志中的概率。';
        case 'gateway_auth_disabled':
          return risk.sourcePlugin == 'nullclaw'
              ? '该风险可通过启用 Gateway 配对认证自动修复，防止未授权客户端直接接入。'
              : '该风险可通过配置认证方式和凭据自动修复，避免未授权访问网关。';
        case 'gateway_auth_password_mode':
          return '该风险可通过切换到 Token 模式或更新凭据自动修复，推荐优先使用 Token。';
        case 'config_perm_unsafe':
          return '该风险可通过收紧配置文件权限自动修复，减少敏感配置被其他本地用户读取的风险。';
        case 'config_dir_perm_unsafe':
          return '该风险可通过收紧配置目录权限自动修复，限制非授权访问配置目录内容。';
        case 'gateway_bind_unsafe':
          return '该风险可通过改为回环地址监听自动修复，降低对外暴露面。';
        case 'gateway_weak_password':
          return '该风险可通过切换认证方式或更新更强凭据自动修复，推荐使用 Token 模式。';
        case 'gateway_weak_token':
          return '该风险可通过生成更强的 Token 自动修复，降低凭据被猜测或复用的风险。';
        case 'sandbox_disabled_default':
          return '该风险可通过启用默认沙箱模式自动修复，减少高风险操作对宿主系统的直接影响。';
        case 'sandbox_disabled_agent':
          return '该风险可通过为 Agent 启用沙箱自动修复，降低命令执行和文件访问风险。';
        case 'log_dir_perm_unsafe':
          return '该风险可通过收紧日志目录权限自动修复，避免未授权读取、删除或篡改日志。';
        case 'plaintext_secrets':
          return '该风险可通过迁移凭据到环境变量或改为人工复核自动修复，避免明文长期存储。';
        case 'skills_not_scanned':
          return '该风险可通过运行 Skill 安全扫描自动修复，补齐未检测 Skill 的风险信息。';
        case 'skill_agent_risk':
          return '该风险可通过执行 Skill 处置动作自动修复，若 Skill 不可信建议删除。';
        case 'audit_disabled':
          return '该风险可通过启用审计日志自动修复，便于追踪敏感操作和异常行为。';
        case 'autonomy_workspace_unrestricted':
          return '该风险可通过限制 Agent 仅访问工作区自动修复，减少越界读写风险。';
        case 'memory_dir_perm_unsafe':
          return '该风险可通过收紧 memory 目录权限自动修复，降低运行时记忆数据泄露风险。';
        case 'process_running_as_root':
          return '该风险需要人工修复，不提供自动执行按钮。建议改为普通用户运行服务。';
        case 'terminal_backend_local':
          return '该风险可通过将终端执行后端切换到远程受隔离环境修复，避免直接在宿主机执行高风险操作。';
        case 'approvals_mode_disabled':
          return '该风险可通过启用审批模式修复，确保高风险操作需要交互确认后才能执行。';
        case 'redact_secrets_disabled':
          return '该风险可通过启用密钥脱敏修复，减少敏感信息泄露到日志和审计记录的可能。';
        case 'model_base_url_public':
          return '该风险可通过将自定义模型地址改为本地或受限内网地址修复，减少外网暴露面。';
        case 'openclaw_1click_rce_vulnerability':
        case 'nullclaw_1click_rce_vulnerability':
          return '该风险需要人工修复，不提供自动执行按钮。建议优先升级版本并完成网关入口加固。';
      }
    }
    return risk.mitigation?.description ?? riskDescription(risk, l10n);
  }

  static String formLabel(RiskInfo risk, FormItem item, AppLocalizations l10n) {
    if (!_isZh(l10n)) return item.label;
    if (item.key == 'fix_permission') {
      if (!Platform.isWindows) return '修复目录或文件权限';
      switch (risk.id) {
        case 'config_perm_unsafe':
          return '修复配置文件 ACL 权限';
        case 'config_dir_perm_unsafe':
          return '修复配置目录 ACL 权限';
        case 'log_dir_perm_unsafe':
          return '修复日志目录 ACL 权限';
        case 'memory_dir_perm_unsafe':
          return '修复 memory 目录 ACL 权限';
        default:
          return '修复 ACL 权限';
      }
    }

    switch (item.key) {
      case 'redact_sensitive':
        return '启用自动脱敏';
      case 'auth_mode':
        return '认证模式';
      case 'token_value':
        return 'Token';
      case 'switch_to_token':
        return '切换到 Token 模式';
      case 'bind_address':
        return '绑定地址';
      case 'action':
        return '处理方式';
      case 'regenerate_token':
        return '重新生成强 Token';
      case 'sandbox_mode':
        return '沙箱模式';
      case 'env_var_name':
        return '环境变量名称';
      case 'scan_skills':
        return '扫描未检测的 Skills';
      case 'delete_skill':
        return '删除该 Skill';
      case 'enable_pairing':
        return '启用 Gateway 配对认证';
      case 'enable_audit':
        return '启用安全审计日志';
      case 'workspace_only':
        return '限制为仅访问工作区内文件';
      default:
        return item.label;
    }
  }

  static String formOption(
    RiskInfo risk,
    FormItem item,
    String option,
    AppLocalizations l10n,
  ) {
    if (!_isZh(l10n)) return option;
    switch (option) {
      case 'token':
        return 'Token';
      case 'password':
        return '密码';
      case 'switch_to_token':
        return '切换到 Token 模式';
      case 'use_stronger_password':
        return '使用更强密码';
      case 'use_env_var':
        return '迁移到环境变量';
      case 'manual_review':
        return '人工复核';
      case 'auto':
        return '自动';
      case 'docker':
        return 'Docker';
      case 'gvisor':
        return 'gVisor';
      case 'loopback':
        return '回环地址';
      default:
        return option;
    }
  }

  static String suggestionCategory(
    RiskInfo risk,
    SuggestionGroup group,
    AppLocalizations l10n,
  ) {
    if (!_isZh(l10n)) return group.category;
    switch (group.category) {
      case 'Immediate actions':
        return '立即措施';
      case 'Short term hardening':
        return '短期加固';
      case 'Long term governance':
        return '长期治理';
      case 'Hardening actions':
        return '加固措施';
      default:
        return group.category;
    }
  }

  static String suggestionAction(
    RiskInfo risk,
    SuggestionItem item,
    AppLocalizations l10n,
  ) {
    if (!_isZh(l10n)) return item.action;
    switch (item.action) {
      case 'Check the current version and upgrade':
        return '确认当前版本并升级';
      case 'Remove gatewayUrl override entry points':
      case 'Remove gatewayUrl override paths':
        return '移除 gatewayUrl 动态覆盖入口';
      case 'Add WebSocket origin validation':
        return '增加 WebSocket Origin 校验';
      case 'Harden token storage':
      case 'Improve credential storage':
        return '改进凭据存储策略';
      case 'Add security response headers':
      case 'Add security headers':
        return '补充安全响应头';
      case 'Allowlist gateway destinations':
      case 'Allowlist trusted gateway destinations':
        return '建立网关地址白名单';
      case 'Alert on sensitive config changes':
        return '增加高风险配置变更告警';
      case 'Establish recurring security review':
        return '建立定期安全审计';
      case 'Strengthen runtime monitoring':
        return '加强运行时监控';
      case 'Train teams on secure gateway design':
      case 'Document secure gateway practices':
        return '完善安全开发规范';
      case 'Restart the service as a non root user':
        return '切换为普通用户运行';
      case 'Review startup scripts and service units':
        return '检查启动脚本与服务配置';
      case 'Minimize writable paths':
        return '最小化目录写权限';
      default:
        return item.action;
    }
  }

  static String suggestionDetail(
    RiskInfo risk,
    SuggestionItem item,
    AppLocalizations l10n,
  ) {
    if (!_isZh(l10n)) return item.detail;
    switch (item.detail) {
      case 'Run the version command and upgrade to a fixed OpenClaw release as soon as possible.':
      case 'Run the version command and upgrade to a fixed NullClaw release.':
        return '执行版本检查命令，并尽快升级到包含修复的安全版本。';
      case 'Review UI and startup flows and remove URL parameter paths that can override gatewayUrl.':
      case 'Review startup and UI flows and remove any route that lets external URL input override gatewayUrl.':
        return '审查 UI 和启动流程，移除通过外部 URL 输入覆盖 gatewayUrl 的路径。';
      case 'Enforce an allowlist for Origin during gateway connection authorization.':
      case 'Require origin allowlisting during gateway authorization.':
        return '在网关鉴权流程中对 Origin 做白名单校验，只允许受信任来源建立连接。';
      case 'Avoid long lived plaintext browser storage for sensitive tokens and move to safer storage controls.':
      case 'Avoid storing long lived sensitive tokens in plaintext browser or local storage flows.':
        return '避免在浏览器或本地存储中长期明文保存敏感 Token，改用更安全的存储方式。';
      case 'Apply CSP, frame protections, and transport security headers to the management UI and gateway endpoints.':
      case 'Apply CSP, frame protections, and strict transport settings to management and gateway responses.':
        return '为管理界面和网关响应增加 CSP、Frame 保护和传输安全相关响应头。';
      case 'Restrict the system so it can only connect to approved gateway URLs.':
      case 'Restrict runtime connections to approved gateway addresses only.':
        return '仅允许系统连接到受信任的网关地址，阻止跳转到未知目标。';
      case 'Generate a visible warning when gatewayUrl or related high risk settings change.':
      case 'Warn users whenever gatewayUrl or similar high risk configuration changes.':
        return '当 gatewayUrl 或类似高风险配置发生变化时，向用户显示明确告警。';
      case 'Add version review, code review, and targeted security verification to the release process.':
      case 'Add version review, code review, and focused security verification to release workflows.':
        return '将版本审计、代码审查和针对性安全验证纳入发布流程。';
      case 'Monitor unusual gateway connections, unusual origins, and suspicious credential usage patterns.':
      case 'Monitor unusual gateway connections, unusual origins, and suspicious credential use.':
        return '监控异常网关连接、异常来源和异常凭据使用行为。';
      case 'Document and train around URL trust boundaries, credential management, and gateway authorization design.':
      case 'Standardize team guidance for URL trust boundaries, gateway authorization, and credential handling.':
        return '围绕 URL 信任边界、网关鉴权和凭据管理建立统一规范并开展培训。';
      case 'Stop the current root owned process and relaunch DinTalClaw with a dedicated low privilege account.':
        return '停止当前以 root 运行的进程，并使用受限普通用户重新启动 DinTalClaw。';
      case 'Verify that systemd units, container entry points, or shell scripts do not explicitly launch DinTalClaw as root.':
        return '确认 systemd、容器入口或启动脚本未显式以 root 身份启动 DinTalClaw。';
      case 'Grant the runtime account only the minimum write access needed for config, logs, and working directories.':
        return '为运行账户只授予配置、日志和工作目录所需的最小写权限。';
      default:
        return item.detail;
    }
  }

  static String _windowsAclDesc(
    RiskInfo risk,
    AppLocalizations l10n, {
    required String defaultLabelEn,
    required String defaultLabelZh,
  }) {
    final args = risk.args;
    if (args == null) return risk.description;
    final isZh = _isZh(l10n);
    final path = args['path']?.toString() ?? '';
    final summary = args['acl_summary']?.toString() ?? '';
    final label = isZh ? defaultLabelZh : defaultLabelEn;
    if (path.isEmpty && summary.isEmpty) return risk.description;
    if (path.isEmpty) return '$label: $summary';
    if (summary.isEmpty) return '$label: $path';
    return '$label: $path ($summary)';
  }
}
