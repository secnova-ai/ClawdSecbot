import 'package:flutter/material.dart';
import 'package:flutter_animate/flutter_animate.dart';
import 'package:intl/intl.dart';
import 'package:lucide_icons/lucide_icons.dart';
import 'dart:convert';
import '../l10n/app_localizations.dart';
import '../models/asset_model.dart';
import '../models/risk_model.dart';
import '../services/app_settings_database_service.dart';
import '../utils/app_fonts.dart';
import '../utils/runtime_platform.dart';
import 'bot_icon_picker_dialog.dart';

enum RescanAction { securityDiscovery, fullScan }

/// 扫描结果展示组件
/// 用于展示扫描完成后的资产列表和风险信息
class ScanResultView extends StatelessWidget {
  final ScanResult result;
  final Set<String> protectedAssets;
  final bool isRestoringProtection;
  final RescanAction selectedRescanAction;
  final ValueChanged<RescanAction> onRescanActionChanged;
  final VoidCallback onRescan;
  final VoidCallback onViewSkillScanResults;
  final void Function(Asset asset, {required bool isEditMode})
  onShowProtectionConfig;
  final void Function(Asset asset) onShowProtectionMonitor;
  final Future<void> Function(Asset asset) onStopProtection;
  final Set<String> stoppingProtectionAssets;
  final void Function(RiskInfo risk) onShowMitigation;
  final Future<void> Function(RiskInfo risk) onDeleteRiskSkill;

  const ScanResultView({
    super.key,
    required this.result,
    required this.protectedAssets,
    required this.isRestoringProtection,
    required this.selectedRescanAction,
    required this.onRescanActionChanged,
    required this.onRescan,
    required this.onViewSkillScanResults,
    required this.onShowProtectionConfig,
    required this.onShowProtectionMonitor,
    required this.onStopProtection,
    required this.stoppingProtectionAssets,
    required this.onShowMitigation,
    required this.onDeleteRiskSkill,
  });

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context)!;
    final scannedAtText = _formatScannedAt(result.scannedAt, l10n);

    return SingleChildScrollView(
      key: const ValueKey('completed'),
      padding: const EdgeInsets.all(20),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          // 扫描完成标题（始终同一行，窄屏时右侧操作区自适应缩放）
          Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Container(
                padding: const EdgeInsets.all(10),
                decoration: BoxDecoration(
                  color: const Color(0xFF22C55E).withValues(alpha: 0.2),
                  borderRadius: BorderRadius.circular(8),
                ),
                child: const Icon(
                  LucideIcons.checkCircle,
                  color: Color(0xFF22C55E),
                  size: 24,
                ),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      l10n.scanComplete,
                      style: AppFonts.inter(
                        fontSize: 18,
                        fontWeight: FontWeight.w600,
                        color: Colors.white,
                      ),
                    ),
                    if (scannedAtText != null) ...[
                      const SizedBox(height: 4),
                      Text(
                        l10n.lastScanTime(scannedAtText),
                        style: AppFonts.inter(
                          fontSize: 11,
                          color: Colors.white38,
                        ),
                      ),
                    ],
                  ],
                ),
              ),
              const SizedBox(width: 12),
              Flexible(
                child: FittedBox(
                  fit: BoxFit.scaleDown,
                  alignment: Alignment.centerRight,
                  child: _buildHeaderActions(l10n),
                ),
              ),
            ],
          ),
          const SizedBox(height: 20),

          // 资产信息
          _buildSectionTitle(l10n.detectedAssets, LucideIcons.box),
          const SizedBox(height: 12),
          if (result.assets.isEmpty)
            Padding(
              padding: const EdgeInsets.only(left: 4),
              child: Text(
                l10n.notFound,
                style: AppFonts.inter(color: Colors.white54, fontSize: 13),
              ),
            )
          else
            ...result.assets.map(
              (asset) => Padding(
                padding: const EdgeInsets.only(bottom: 12),
                child: _AssetCard(
                  key: ValueKey(asset.id),
                  asset: asset,
                  initiallyExpanded: result.assets.length <= 1,
                  isProtected: protectedAssets.contains(asset.id),
                  isRestoringProtection: isRestoringProtection,
                  isStoppingProtection: stoppingProtectionAssets.contains(
                    asset.id,
                  ),
                  onShowProtectionConfig: onShowProtectionConfig,
                  onShowProtectionMonitor: onShowProtectionMonitor,
                  onStopProtection: onStopProtection,
                ).animate().fadeIn().slideX(begin: 0.1, end: 0),
              ),
            ),
          const SizedBox(height: 24),

          // 风险信息
          _buildSectionTitle(
            '${l10n.securityFindings} (${result.risks.length})',
            LucideIcons.alertTriangle,
          ),
          const SizedBox(height: 12),
          if (result.risks.isEmpty)
            _buildNoRisksCard(l10n)
          else
            ...result.risks.asMap().entries.map(
              (entry) => Padding(
                padding: const EdgeInsets.only(bottom: 12),
                child: _buildRiskCard(context, entry.value, l10n)
                    .animate(delay: (100 * entry.key).ms)
                    .fadeIn()
                    .slideX(begin: 0.1, end: 0),
              ),
            ),
        ],
      ),
    ).animate().fadeIn(duration: 400.ms);
  }

  String? _formatScannedAt(DateTime? scannedAt, AppLocalizations l10n) {
    if (scannedAt == null) return null;
    final localTime = scannedAt.toLocal();
    final pattern = l10n.localeName.toLowerCase().startsWith('zh')
        ? 'yyyy-MM-dd HH:mm:ss'
        : 'yyyy-MM-dd HH:mm:ss';
    return DateFormat(pattern).format(localTime);
  }

  Widget _buildSectionTitle(String title, IconData icon) {
    return Row(
      children: [
        Icon(icon, size: 16, color: const Color(0xFF6366F1)),
        const SizedBox(width: 8),
        Text(
          title,
          style: AppFonts.inter(
            fontSize: 14,
            fontWeight: FontWeight.w600,
            color: Colors.white,
          ),
        ),
      ],
    );
  }

  Widget _buildNoRisksCard(AppLocalizations l10n) {
    return Container(
      padding: const EdgeInsets.all(20),
      decoration: BoxDecoration(
        color: const Color(0xFF22C55E).withValues(alpha: 0.1),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(
          color: const Color(0xFF22C55E).withValues(alpha: 0.3),
        ),
      ),
      child: Row(
        children: [
          const Icon(
            LucideIcons.shieldCheck,
            color: Color(0xFF22C55E),
            size: 24,
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  l10n.noSecurityIssues,
                  style: AppFonts.inter(
                    fontSize: 14,
                    fontWeight: FontWeight.w600,
                    color: const Color(0xFF22C55E),
                  ),
                ),
                const SizedBox(height: 4),
                Text(
                  l10n.secureConfigMessage,
                  style: AppFonts.inter(fontSize: 12, color: Colors.white54),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }

  /// 构建标题区域的右侧操作按钮组（保持单行布局）。
  Widget _buildHeaderActions(AppLocalizations l10n) {
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        MouseRegion(
          cursor: SystemMouseCursors.click,
          child: GestureDetector(
            onTap: onViewSkillScanResults,
            child: Container(
              padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
              decoration: BoxDecoration(
                color: Colors.white.withValues(alpha: 0.1),
                borderRadius: BorderRadius.circular(6),
              ),
              child: Row(
                mainAxisSize: MainAxisSize.min,
                children: [
                  const Icon(
                    LucideIcons.fileSearch,
                    color: Colors.white70,
                    size: 14,
                  ),
                  const SizedBox(width: 6),
                  Text(
                    l10n.viewSkillScanResults,
                    style: AppFonts.inter(fontSize: 12, color: Colors.white70),
                  ),
                ],
              ),
            ),
          ),
        ),
        const SizedBox(width: 8),
        Container(
          decoration: BoxDecoration(
            color: Colors.white.withValues(alpha: 0.1),
            borderRadius: BorderRadius.circular(6),
          ),
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              MouseRegion(
                cursor: SystemMouseCursors.click,
                child: GestureDetector(
                  onTap: onRescan,
                  child: Padding(
                    padding: const EdgeInsets.symmetric(
                      horizontal: 12,
                      vertical: 6,
                    ),
                    child: Row(
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        const Icon(
                          LucideIcons.refreshCw,
                          color: Colors.white70,
                          size: 14,
                        ),
                        const SizedBox(width: 6),
                        Text(
                          l10n.rescan,
                          style: AppFonts.inter(
                            fontSize: 12,
                            color: Colors.white70,
                          ),
                        ),
                      ],
                    ),
                  ),
                ),
              ),
              Container(
                width: 1,
                height: 16,
                color: Colors.white.withValues(alpha: 0.22),
              ),
              PopupMenuButton<RescanAction>(
                tooltip: '',
                onSelected: onRescanActionChanged,
                color: const Color(0xFF1F2937),
                elevation: 10,
                offset: const Offset(0, 40),
                shape: RoundedRectangleBorder(
                  borderRadius: BorderRadius.circular(14),
                  side: BorderSide(color: Colors.white.withValues(alpha: 0.2)),
                ),
                itemBuilder: (context) => [
                  PopupMenuItem<RescanAction>(
                    value: RescanAction.securityDiscovery,
                    height: 40,
                    child: _buildRescanMenuItem(
                      text: l10n.rescanSecurityDiscovery,
                      selected:
                          selectedRescanAction ==
                          RescanAction.securityDiscovery,
                    ),
                  ),
                  PopupMenuItem<RescanAction>(
                    value: RescanAction.fullScan,
                    height: 40,
                    child: _buildRescanMenuItem(
                      text: l10n.rescanAll,
                      selected: selectedRescanAction == RescanAction.fullScan,
                    ),
                  ),
                ],
                child: const Padding(
                  padding: EdgeInsets.fromLTRB(8, 6, 8, 6),
                  child: Icon(
                    LucideIcons.chevronDown,
                    color: Colors.white70,
                    size: 14,
                  ),
                ),
              ),
            ],
          ),
        ),
      ],
    );
  }

  Widget _buildRiskCard(
    BuildContext context,
    RiskInfo risk,
    AppLocalizations l10n,
  ) {
    final title = _getRiskTitle(risk, l10n);
    final description = _getRiskDesc(risk, l10n);
    final levelText = _getRiskLevel(risk.level, l10n);
    final assetName = _getRiskAssetName(risk);

    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: risk.color.withValues(alpha: 0.1),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: risk.color.withValues(alpha: 0.3)),
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Container(
            padding: const EdgeInsets.all(8),
            decoration: BoxDecoration(
              color: risk.color.withValues(alpha: 0.2),
              borderRadius: BorderRadius.circular(8),
            ),
            child: Icon(risk.icon, color: risk.color, size: 18),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  children: [
                    Expanded(
                      child: Text(
                        title,
                        style: AppFonts.inter(
                          fontSize: 13,
                          fontWeight: FontWeight.w600,
                          color: Colors.white,
                        ),
                      ),
                    ),
                    Container(
                      padding: const EdgeInsets.symmetric(
                        horizontal: 8,
                        vertical: 2,
                      ),
                      decoration: BoxDecoration(
                        color: risk.color.withValues(alpha: 0.2),
                        borderRadius: BorderRadius.circular(4),
                      ),
                      child: Text(
                        levelText,
                        style: AppFonts.inter(
                          fontSize: 10,
                          fontWeight: FontWeight.w600,
                          color: risk.color,
                        ),
                      ),
                    ),
                  ],
                ),
                if (assetName != null) ...[
                  const SizedBox(height: 6),
                  Container(
                    padding: const EdgeInsets.symmetric(
                      horizontal: 8,
                      vertical: 3,
                    ),
                    decoration: BoxDecoration(
                      color: Colors.white.withValues(alpha: 0.08),
                      borderRadius: BorderRadius.circular(999),
                      border: Border.all(
                        color: Colors.white.withValues(alpha: 0.15),
                      ),
                    ),
                    child: Text(
                      '${l10n.assetName}: ${_getAssetDisplayName(assetName)}',
                      style: AppFonts.inter(
                        fontSize: 10,
                        color: Colors.white70,
                        fontWeight: FontWeight.w500,
                      ),
                    ),
                  ),
                ],
                const SizedBox(height: 6),
                Text(
                  description,
                  style: AppFonts.inter(
                    fontSize: 12,
                    color: Colors.white70,
                    height: 1.4,
                  ),
                ),
                const SizedBox(height: 12),
                if (risk.mitigation != null || _canDeleteRiskSkill(risk))
                  Wrap(
                    spacing: 8,
                    runSpacing: 8,
                    children: [
                      if (risk.mitigation != null)
                        ElevatedButton.icon(
                          onPressed: () => onShowMitigation(risk),
                          icon: const Icon(LucideIcons.wrench, size: 14),
                          label: Text(l10n.mitigate),
                          style: ElevatedButton.styleFrom(
                            backgroundColor: const Color(0xFF6366F1),
                            foregroundColor: Colors.white,
                            textStyle: AppFonts.inter(fontSize: 12),
                            padding: const EdgeInsets.symmetric(
                              horizontal: 12,
                              vertical: 8,
                            ),
                          ),
                        ),
                      if (_canDeleteRiskSkill(risk))
                        ElevatedButton.icon(
                          onPressed: () => onDeleteRiskSkill(risk),
                          icon: const Icon(LucideIcons.trash2, size: 14),
                          label: Text(l10n.deleteRiskSkill),
                          style: ElevatedButton.styleFrom(
                            backgroundColor: const Color(0xFFEF4444),
                            foregroundColor: Colors.white,
                            textStyle: AppFonts.inter(fontSize: 12),
                            padding: const EdgeInsets.symmetric(
                              horizontal: 12,
                              vertical: 8,
                            ),
                          ),
                        ),
                    ],
                  ),
              ],
            ),
          ),
        ],
      ),
    );
  }

  bool _canDeleteRiskSkill(RiskInfo risk) {
    return risk.id == 'riskSkillSecurityIssue';
  }

  /// 构建重新扫描下拉菜单项
  Widget _buildRescanMenuItem({required String text, required bool selected}) {
    return Row(
      children: [
        Icon(
          selected ? LucideIcons.checkCircle2 : LucideIcons.circle,
          size: 14,
          color: selected ? const Color(0xFF6366F1) : Colors.white38,
        ),
        const SizedBox(width: 8),
        Text(
          text,
          style: AppFonts.inter(
            fontSize: 12,
            fontWeight: selected ? FontWeight.w600 : FontWeight.w500,
            color: selected ? Colors.white : Colors.white70,
          ),
        ),
      ],
    );
  }

  String _getRiskTitle(RiskInfo risk, AppLocalizations l10n) {
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
      case 'openclaw_1click_rce_vulnerability':
      case 'nullclaw_1click_rce_vulnerability':
        return risk.displayTitle(l10n.localeName);
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
        return risk.title;
    }
  }

  String _getRiskDesc(RiskInfo risk, AppLocalizations l10n) {
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
            defaultLabelZh: '\u914d\u7f6e\u6587\u4ef6 ACL',
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
            defaultLabelZh: '\u914d\u7f6e\u76ee\u5f55 ACL',
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
            defaultLabelZh: '\u65e5\u5fd7\u76ee\u5f55 ACL',
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
          risk.args?['count'] as int? ?? 0,
          risk.args?['skills']?.toString() ?? '',
        );
      case 'openclaw_1click_rce_vulnerability':
      case 'nullclaw_1click_rce_vulnerability':
        return risk.displayDescription(l10n.localeName);
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
          risk.args?['issueCount'] as int? ?? 0,
        );
      default:
        return risk.description;
    }
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
    final violationsRaw = args['acl_violations'];

    String violations = '';
    if (violationsRaw is List) {
      violations = violationsRaw
          .map((e) => e.toString())
          .where((e) => e.isNotEmpty)
          .join('; ');
    } else {
      violations = violationsRaw?.toString() ?? '';
    }

    final details = <String>[];
    if (path.isNotEmpty) {
      details.add(isZh ? '\u8def\u5f84: $path' : 'Path: $path');
    }
    if (summary.isNotEmpty) {
      details.add(isZh ? '\u6458\u8981: $summary' : 'Summary: $summary');
    }
    if (violations.isNotEmpty) {
      details.add(
        isZh
            ? '\u8fdd\u89c4\u4e3b\u4f53: $violations'
            : 'Violations: $violations',
      );
    }

    if (details.isEmpty) return fallback;
    if (isZh) {
      return '$defaultLabelZh \u6743\u9650\u4e0d\u5b89\u5168\u3002${details.join('\uff1b')}\u3002';
    }
    return '$defaultLabelEn is unsafe. ${details.join(' | ')}.';
  }

  String _translateAclSummary(String summary, bool isZh) {
    if (!isZh) return summary;
    switch (summary.toLowerCase()) {
      case 'acl safe':
        return 'ACL \u5b89\u5168';
      case 'acl has non-whitelisted principal access':
        return '\u5b58\u5728\u975e\u767d\u540d\u5355\u4e3b\u4f53\u8bbf\u95ee\u6743\u9650';
      case 'acl check failed':
        return 'ACL \u68c0\u67e5\u5931\u8d25';
      default:
        return summary;
    }
  }

  String _getRiskLevel(RiskLevel level, AppLocalizations l10n) {
    switch (level) {
      case RiskLevel.low:
        return l10n.riskLevelLow;
      case RiskLevel.medium:
        return l10n.riskLevelMedium;
      case RiskLevel.high:
        return l10n.riskLevelHigh;
      case RiskLevel.critical:
        return l10n.riskLevelCritical;
    }
  }

  String? _getRiskAssetName(RiskInfo risk) {
    final fromArgs = (risk.args?['asset_name'] ?? risk.args?['assetName'])
        ?.toString()
        .trim();
    if (fromArgs != null && fromArgs.isNotEmpty) {
      return fromArgs;
    }
    final fromSourcePlugin = risk.sourcePlugin?.trim();
    if (fromSourcePlugin != null && fromSourcePlugin.isNotEmpty) {
      return fromSourcePlugin;
    }
    return null;
  }

  String _getAssetDisplayName(String name) {
    const displayNames = {'dintalclaw': '政务龙虾'};
    return displayNames[name] ?? name;
  }
}

/// 单个 Bot 资产卡片，支持折叠/展开
class _AssetCard extends StatefulWidget {
  final Asset asset;
  final bool initiallyExpanded;
  final bool isProtected;
  final bool isRestoringProtection;
  final bool isStoppingProtection;
  final void Function(Asset asset, {required bool isEditMode})
  onShowProtectionConfig;
  final void Function(Asset asset) onShowProtectionMonitor;
  final Future<void> Function(Asset asset) onStopProtection;

  const _AssetCard({
    super.key,
    required this.asset,
    required this.initiallyExpanded,
    required this.isProtected,
    required this.isRestoringProtection,
    required this.isStoppingProtection,
    required this.onShowProtectionConfig,
    required this.onShowProtectionMonitor,
    required this.onStopProtection,
  });

  @override
  State<_AssetCard> createState() => _AssetCardState();
}

class _AssetCardState extends State<_AssetCard> {
  late bool _isExpanded = widget.initiallyExpanded;
  String? _iconName;
  int? _iconColor;

  @override
  void initState() {
    super.initState();
    _loadSavedIcon();
  }

  IconData get _currentIcon =>
      botIconDataMap[_iconName] ??
      (widget.asset.displaySections.isNotEmpty
          ? LucideIcons.fileJson
          : LucideIcons.package);

  Color get _currentIconColor => Color(_iconColor ?? 0xFF6366F1);

  Future<void> _loadSavedIcon() async {
    final saved = await AppSettingsDatabaseService().getSetting(
      'asset_icon_${widget.asset.id}',
    );
    if (saved.isNotEmpty && mounted) {
      try {
        final data = jsonDecode(saved) as Map<String, dynamic>;
        setState(() {
          _iconName = data['icon'] as String?;
          _iconColor = data['color'] as int?;
        });
      } catch (_) {}
    }
  }

  Future<void> _showIconPicker() async {
    final result = await showDialog<BotIconSelection>(
      context: context,
      builder: (_) =>
          BotIconPickerDialog(currentIcon: _iconName, currentColor: _iconColor),
    );
    if (result != null && mounted) {
      setState(() {
        _iconName = result.iconName;
        _iconColor = result.colorValue;
      });
      await AppSettingsDatabaseService().saveSetting(
        'asset_icon_${widget.asset.id}',
        jsonEncode({'icon': result.iconName, 'color': result.colorValue}),
      );
    }
  }

  void _toggleExpand() {
    setState(() {
      _isExpanded = !_isExpanded;
    });
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context)!;
    final asset = widget.asset;

    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: const Color(0xFF6366F1).withValues(alpha: 0.1),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(
          color: const Color(0xFF6366F1).withValues(alpha: 0.3),
        ),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          // Header row - 始终可见，点击切换折叠/展开
          MouseRegion(
            cursor: SystemMouseCursors.click,
            child: GestureDetector(
              onTap: _toggleExpand,
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Row(
                    children: [
                      MouseRegion(
                        cursor: SystemMouseCursors.click,
                        child: GestureDetector(
                          onTap: _showIconPicker,
                          child: Container(
                            padding: const EdgeInsets.all(8),
                            decoration: BoxDecoration(
                              color: _currentIconColor.withValues(alpha: 0.2),
                              borderRadius: BorderRadius.circular(8),
                            ),
                            child: Icon(
                              _currentIcon,
                              color: _currentIconColor,
                              size: 18,
                            ),
                          ),
                        ),
                      ),
                      const SizedBox(width: 12),
                      Expanded(
                        child: Text.rich(
                          _buildAssetTitleSpan(asset),
                          style: AppFonts.inter(
                            fontSize: 14,
                            fontWeight: FontWeight.w600,
                            color: Colors.white,
                          ),
                          maxLines: 2,
                          overflow: TextOverflow.ellipsis,
                        ),
                      ),
                      const SizedBox(width: 8),
                      _buildProtectionBadge(l10n),
                      const SizedBox(width: 8),
                      Container(
                        padding: const EdgeInsets.symmetric(
                          horizontal: 8,
                          vertical: 2,
                        ),
                        decoration: BoxDecoration(
                          color: Colors.white.withValues(alpha: 0.1),
                          borderRadius: BorderRadius.circular(4),
                        ),
                        child: Text(
                          _buildAssetTypeBadgeText(
                            asset,
                            l10n.localeName.startsWith('zh'),
                          ),
                          style: AppFonts.firaCode(
                            fontSize: 10,
                            color: Colors.white70,
                          ),
                        ),
                      ),
                      const SizedBox(width: 8),
                      Icon(
                        _isExpanded
                            ? LucideIcons.chevronDown
                            : LucideIcons.chevronRight,
                        size: 16,
                        color: Colors.white54,
                      ),
                    ],
                  ),
                  if (asset.ports.isNotEmpty) ...[
                    const SizedBox(height: 4),
                    Padding(
                      padding: const EdgeInsets.only(left: 46),
                      child: Text(
                        _buildBindPortSummary(asset),
                        style: AppFonts.firaCode(
                          fontSize: 10,
                          color: Colors.white54,
                        ),
                        maxLines: 2,
                        overflow: TextOverflow.ellipsis,
                      ),
                    ),
                  ],
                ],
              ),
            ),
          ),
          // Body - 展开/折叠动画
          AnimatedSize(
            duration: const Duration(milliseconds: 200),
            curve: Curves.easeInOut,
            clipBehavior: Clip.hardEdge,
            child: _isExpanded
                ? Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      const SizedBox(height: 12),
                      if (asset.serviceName.isNotEmpty)
                        _buildConfigRow(
                          l10n.serviceName,
                          asset.serviceName,
                          Colors.white70,
                        ),
                      if (asset.processPaths.isNotEmpty &&
                          !_hasRuntimeImagePathSection(asset))
                        _buildConfigRow(
                          l10n.processPaths,
                          asset.processPaths.join(', '),
                          Colors.white70,
                        ),
                      // Display structured config sections from the plugin
                      if (asset.displaySections.isNotEmpty) ...[
                        const SizedBox(height: 8),
                        const Divider(color: Colors.white12),
                        const SizedBox(height: 8),
                        _buildDisplaySections(asset.displaySections),
                      ],
                      // 所有资产都显示防护按钮
                      const SizedBox(height: 12),
                      _buildProtectionButton(
                        context,
                        asset,
                        widget.isProtected,
                        widget.isStoppingProtection,
                        l10n,
                      ),
                    ],
                  )
                : const SizedBox.shrink(),
          ),
        ],
      ),
    );
  }

  /// 将资产内部名称映射为用户友好的展示名称
  String _getAssetDisplayName(String name) {
    const displayNames = {'dintalclaw': '政务龙虾'};
    return displayNames[name] ?? name;
  }

  InlineSpan _buildAssetTitleSpan(Asset asset) {
    final displayName = _getAssetDisplayName(asset.name);
    final version = asset.version.trim();
    if (version.isEmpty) {
      return TextSpan(text: displayName);
    }
    return TextSpan(
      children: [
        TextSpan(text: displayName),
        TextSpan(
          text: ' | ',
          style: AppFonts.inter(
            fontSize: 14,
            fontWeight: FontWeight.w600,
            color: Colors.white38,
          ),
        ),
        TextSpan(text: version),
      ],
    );
  }

  /// 根据资产状态生成折叠态右上角类型徽章文案
  String _buildAssetTypeBadgeText(Asset asset, bool isZh) {
    if (asset.name != 'dintalclaw') {
      return asset.type;
    }
    final status = _getDintalclawAssetStatus(asset);
    switch (status) {
      case 'frontend_mode_running':
        return isZh ? '前端' : 'Frontend';
      case 'browser_mode_running':
        return isZh ? '浏览器' : 'Browser';
      case 'cli_mode_running':
        return isZh ? '命令行' : 'CLI';
      case 'installed_not_running':
        return isZh ? '未运行' : 'Stopped';
      default:
        return asset.type;
    }
  }

  /// 从资产展示区块中提取 dintalclaw 运行状态 key
  String _getDintalclawAssetStatus(Asset asset) {
    for (final section in asset.displaySections) {
      if (section.title != 'Asset Status') continue;
      for (final item in section.items) {
        if (item.label == 'Status') {
          return item.value;
        }
      }
    }
    return '';
  }

  /// 判断资产是否处于运行中状态，仅运行中的资产才能一键防护
  bool _isAssetRunning(Asset asset) {
    final status = asset.metadata['asset_status'] ?? '';
    if (status.isEmpty) return true;
    return status.endsWith('_running');
  }

  /// 构建折叠态的绑定地址+端口摘要，如 "127.0.0.1:18789"
  /// 优先从 displaySections 的 Listener/Bind 值读取
  String _buildBindPortSummary(Asset asset) {
    // dintalclaw uses "Listener Address" items with full "addr:port" values
    final listenerValues = <String>[];
    for (final section in asset.displaySections) {
      for (final item in section.items) {
        if ((item.label == 'Listener' || item.label == 'Listener Address') &&
            item.value.isNotEmpty &&
            item.value != 'N/A (not running)' &&
            item.value != 'N/A') {
          listenerValues.add(item.value);
        }
      }
    }
    if (listenerValues.isNotEmpty) {
      return listenerValues.join(', ');
    }

    String bind = '';
    for (final section in asset.displaySections) {
      for (final item in section.items) {
        if (item.label == 'Bind' && item.value.isNotEmpty) {
          bind = item.value;
          break;
        }
      }
      if (bind.isNotEmpty) break;
    }
    if (bind.isNotEmpty) {
      return asset.ports.map((p) => '$bind:$p').join(', ');
    }
    return asset.ports.map((p) => ':$p').join(', ');
  }

  bool _hasRuntimeImagePathSection(Asset asset) {
    for (final section in asset.displaySections) {
      if (section.title != 'Runtime') continue;
      for (final item in section.items) {
        if (item.label == 'Image Path' && item.value.trim().isNotEmpty) {
          return true;
        }
      }
    }
    return false;
  }

  Widget _buildProtectionBadge(AppLocalizations l10n) {
    final isProtected = widget.isProtected;
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
      decoration: BoxDecoration(
        color: isProtected
            ? const Color(0xFF22C55E).withValues(alpha: 0.2)
            : Colors.white.withValues(alpha: 0.1),
        borderRadius: BorderRadius.circular(4),
      ),
      child: Text(
        isProtected ? l10n.protectionActive : l10n.protectionInactive,
        style: AppFonts.inter(
          fontSize: 10,
          color: isProtected ? const Color(0xFF22C55E) : Colors.white54,
        ),
      ),
    );
  }

  Widget _buildDisplaySections(List<DisplaySection> sections) {
    final l10n = AppLocalizations.of(context)!;
    final isZh = l10n.localeName.startsWith('zh');
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        for (int i = 0; i < sections.length; i++) ...[
          if (i > 0) const SizedBox(height: 12),
          _buildConfigSectionHeader(
            _localizePluginText(sections[i].title, isZh),
            _mapPluginSectionIcon(sections[i].icon),
          ),
          const SizedBox(height: 8),
          for (final item in sections[i].items)
            _buildConfigDetailRow(
              _localizePluginText(item.label, isZh),
              _localizeDisplayValue(item.value, isZh),
              _statusColor(item.status),
            ),
        ],
      ],
    );
  }

  /// 将插件返回的英文标题/标签翻译为中文
  String _localizeText(String text, bool isZh) {
    if (!isZh) return text;
    const zhMap = {
      // Section titles
      'Gateway Configuration': '网关配置',
      'Sandbox': '沙箱',
      'Logging': '日志',
      'Config': '配置',
      'Asset Status': '资产状态',
      'Basic Info': '基本信息',
      'Process Info': '进程信息',
      // Item labels
      'Bind': '绑定地址',
      'Port': '端口',
      'Auth': '认证',
      'Mode': '模式',
      'Redact': '脱敏',
      'Path': '路径',
      'Runtime Listeners': '运行时监听',
      'Listener': '监听地址',
      'Listener Address': '监听地址',
      'Installation': '安装信息',
      'Root': '安装根目录',
      'Status': '状态',
      'Version': '版本号',
      'Install Path': '安装路径',
      'Config File': '配置文件',
      'Log Path': '日志路径',
      'Process Name': '进程名称',
      'Audit': '审计',
      'Host': '主机',
      'Pairing Required': '配对认证',
      'Allow Public Bind': '允许公网绑定',
      'Backend': '后端',
      'Workspace Only': '仅工作区',
      'Enabled': '启用状态',
    };
    return zhMap[text] ?? text;
  }

  /// 将插件返回的英文值翻译为中文
  String _localizeDisplayValue(String value, bool isZh) {
    if (!isZh) return value;
    const zhMap = {
      'Enabled': '已启用',
      'Disabled': '已禁用',
      'Token': 'Token 认证',
      'Password': '密码认证',
      'none': '未启用',
      'on': '已开启',
      'off': '已关闭',
      'frontend_mode_running': '前端模式运行中',
      'browser_mode_running': '浏览器模式运行中',
      'cli_mode_running': '命令行模式运行中',
      'installed_not_running': '已安装未运行',
    };
    return zhMap[value] ?? value;
  }

  IconData _mapSectionIcon(String iconName) {
    switch (iconName) {
      case 'globe':
        return LucideIcons.globe;
      case 'box':
        return LucideIcons.box;
      case 'file-text':
        return LucideIcons.fileText;
      case 'file':
        return LucideIcons.file;
      case 'shield':
        return LucideIcons.shield;
      case 'key':
        return LucideIcons.key;
      case 'lock':
        return LucideIcons.lock;
      case 'network':
        return LucideIcons.network;
      case 'settings':
        return LucideIcons.settings;
      case 'radio':
        return LucideIcons.radio;
      case 'folder':
        return LucideIcons.folder;
      default:
        return LucideIcons.info;
    }
  }

  String _localizePluginText(String text, bool isZh) {
    final localized = _localizeText(text, isZh);
    if (!isZh || localized != text) return localized;
    const zhMap = {
      'Gateway Configuration': '网关配置',
      'Sandbox': '沙箱',
      'Logging': '日志',
      'Config': '配置',
      'Runtime': '运行时',
      'Asset Status': '资产状态',
      'Basic Info': '基本信息',
      'Process Info': '进程信息',
      'Bind': '绑定地址',
      'Port': '端口',
      'Auth': '认证',
      'Mode': '模式',
      'Redact': '脱敏',
      'Path': '路径',
      'Runtime Listeners': '运行时监听',
      'Listener': '监听地址',
      'Listener Address': '监听地址',
      'Installation': '安装信息',
      'Root': '安装根目录',
      'Status': '状态',
      'Version': '版本号',
      'Install Path': '安装路径',
      'Config File': '配置文件',
      'Log Path': '日志路径',
      'Process Name': '进程名称',
      'PID': '进程 ID',
      'Image Path': '可执行路径',
      'Audit': '审计',
      'Host': '主机',
      'Pairing Required': '配对认证',
      'Allow Public Bind': '允许公网绑定',
      'Backend': '后端',
      'Workspace Only': '仅工作区',
      'Enabled': '启用状态',
    };
    return zhMap[text] ?? text;
  }

  IconData _mapPluginSectionIcon(String iconName) {
    if (iconName == 'monitor') {
      return LucideIcons.monitor;
    }
    return _mapSectionIcon(iconName);
  }

  Color _statusColor(String status) {
    switch (status) {
      case 'safe':
        return const Color(0xFF22C55E);
      case 'danger':
        return const Color(0xFFEF4444);
      case 'warning':
        return const Color(0xFFF59E0B);
      case 'neutral':
      default:
        return Colors.white70;
    }
  }

  Widget _buildConfigSectionHeader(String title, IconData icon) {
    return Row(
      children: [
        Icon(icon, size: 12, color: Colors.white54),
        const SizedBox(width: 6),
        Text(
          title,
          style: AppFonts.inter(
            fontSize: 11,
            fontWeight: FontWeight.w600,
            color: Colors.white54,
          ),
        ),
      ],
    );
  }

  Widget _buildConfigDetailRow(String label, String value, Color valueColor) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 4),
      child: Row(
        children: [
          SizedBox(
            width: 100,
            child: Text(
              label,
              style: AppFonts.inter(fontSize: 11, color: Colors.white38),
            ),
          ),
          Expanded(
            child: Text(
              value,
              style: AppFonts.firaCode(fontSize: 11, color: valueColor),
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildProtectionButton(
    BuildContext context,
    Asset asset,
    bool isProtected,
    bool isStopping,
    AppLocalizations l10n,
  ) {
    if (isProtected) {
      // 已防护资产：显示防护监控和配置按钮
      final isLoading = widget.isRestoringProtection || isStopping;
      return Wrap(
        spacing: 8,
        runSpacing: 8,
        children: [
          MouseRegion(
            cursor: isLoading
                ? SystemMouseCursors.basic
                : SystemMouseCursors.click,
            child: GestureDetector(
              onTap: isLoading
                  ? null
                  : () => widget.onShowProtectionMonitor(asset),
              child: Container(
                padding: const EdgeInsets.symmetric(
                  horizontal: 12,
                  vertical: 8,
                ),
                decoration: BoxDecoration(
                  gradient: LinearGradient(
                    colors: isLoading
                        ? [
                            const Color(0xFF22C55E).withValues(alpha: 0.5),
                            const Color(0xFF16A34A).withValues(alpha: 0.5),
                          ]
                        : [const Color(0xFF22C55E), const Color(0xFF16A34A)],
                  ),
                  borderRadius: BorderRadius.circular(8),
                  boxShadow: isLoading
                      ? []
                      : [
                          BoxShadow(
                            color: const Color(
                              0xFF22C55E,
                            ).withValues(alpha: 0.3),
                            blurRadius: 8,
                            offset: const Offset(0, 2),
                          ),
                        ],
                ),
                child: Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    if (isLoading)
                      const SizedBox(
                        width: 14,
                        height: 14,
                        child: CircularProgressIndicator(
                          color: Colors.white,
                          strokeWidth: 2,
                        ),
                      )
                    else
                      const Icon(
                        LucideIcons.shieldCheck,
                        color: Colors.white,
                        size: 14,
                      ),
                    const SizedBox(width: 6),
                    Text(
                      isLoading
                          ? l10n.protectionStarting
                          : l10n.protectionMonitor,
                      style: AppFonts.inter(
                        fontSize: 12,
                        fontWeight: FontWeight.w600,
                        color: Colors.white,
                      ),
                    ),
                  ],
                ),
              ),
            ),
          ),
          MouseRegion(
            cursor: isLoading
                ? SystemMouseCursors.basic
                : SystemMouseCursors.click,
            child: GestureDetector(
              onTap: isLoading ? null : () => widget.onStopProtection(asset),
              child: Opacity(
                opacity: isLoading && !isStopping ? 0.4 : 1.0,
                child: Container(
                  padding: const EdgeInsets.symmetric(
                    horizontal: 10,
                    vertical: 8,
                  ),
                  decoration: BoxDecoration(
                    color: const Color(
                      0xFFEF4444,
                    ).withValues(alpha: isLoading && !isStopping ? 0.10 : 0.16),
                    borderRadius: BorderRadius.circular(8),
                    border: Border.all(
                      color: const Color(0xFFEF4444).withValues(alpha: 0.35),
                    ),
                  ),
                  child: Row(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      if (isStopping)
                        const SizedBox(
                          width: 14,
                          height: 14,
                          child: CircularProgressIndicator(
                            color: Color(0xFFFCA5A5),
                            strokeWidth: 2,
                          ),
                        )
                      else
                        const Icon(
                          LucideIcons.power,
                          color: Color(0xFFFCA5A5),
                          size: 14,
                        ),
                      const SizedBox(width: 4),
                      Text(
                        isStopping
                            ? l10n.protectionStopping
                            : l10n.stopProtection,
                        style: AppFonts.inter(
                          fontSize: 12,
                          color: const Color(0xFFFCA5A5),
                          fontWeight: FontWeight.w600,
                        ),
                      ),
                    ],
                  ),
                ),
              ),
            ),
          ),
          // 配置按钮（恢复防护期间禁用）
          MouseRegion(
            cursor: isLoading
                ? SystemMouseCursors.basic
                : SystemMouseCursors.click,
            child: GestureDetector(
              onTap: isLoading
                  ? null
                  : () =>
                        widget.onShowProtectionConfig(asset, isEditMode: true),
              child: Opacity(
                opacity: isLoading ? 0.4 : 1.0,
                child: Container(
                  padding: const EdgeInsets.symmetric(
                    horizontal: 10,
                    vertical: 8,
                  ),
                  decoration: BoxDecoration(
                    color: Colors.white.withValues(alpha: 0.1),
                    borderRadius: BorderRadius.circular(8),
                    border: Border.all(
                      color: Colors.white.withValues(alpha: 0.2),
                    ),
                  ),
                  child: Row(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      const Icon(
                        LucideIcons.settings,
                        color: Colors.white70,
                        size: 14,
                      ),
                      const SizedBox(width: 4),
                      Text(
                        l10n.protectionConfigBtn,
                        style: AppFonts.inter(
                          fontSize: 12,
                          color: Colors.white70,
                        ),
                      ),
                    ],
                  ),
                ),
              ),
            ),
          ),
        ],
      );
    } else {
      // 未防护资产：显示一键防护按钮（仅运行中可点击）
      final canProtect = _isAssetRunning(asset);
      return Tooltip(
        message: canProtect ? '' : l10n.protectionAssetNotRunning,
        child: MouseRegion(
          cursor: canProtect
              ? SystemMouseCursors.click
              : SystemMouseCursors.basic,
          child: GestureDetector(
            onTap: canProtect
                ? () => widget.onShowProtectionConfig(asset, isEditMode: false)
                : null,
            child: Opacity(
              opacity: canProtect ? 1.0 : 0.4,
              child: Container(
                padding: const EdgeInsets.symmetric(
                  horizontal: 12,
                  vertical: 8,
                ),
                decoration: BoxDecoration(
                  gradient: const LinearGradient(
                    colors: [Color(0xFF6366F1), Color(0xFF8B5CF6)],
                  ),
                  borderRadius: BorderRadius.circular(8),
                  boxShadow: canProtect
                      ? [
                          BoxShadow(
                            color: const Color(
                              0xFF6366F1,
                            ).withValues(alpha: 0.3),
                            blurRadius: 8,
                            offset: const Offset(0, 2),
                          ),
                        ]
                      : [],
                ),
                child: Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    const Icon(
                      LucideIcons.shield,
                      color: Colors.white,
                      size: 14,
                    ),
                    const SizedBox(width: 6),
                    Text(
                      l10n.oneClickProtection,
                      style: AppFonts.inter(
                        fontSize: 12,
                        fontWeight: FontWeight.w600,
                        color: Colors.white,
                      ),
                    ),
                  ],
                ),
              ),
            ),
          ),
        ),
      );
    }
  }

  Widget _buildConfigRow(String label, String value, Color valueColor) {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        SizedBox(
          width: 80,
          child: Text(
            label,
            style: AppFonts.inter(fontSize: 12, color: Colors.white54),
          ),
        ),
        Expanded(
          child: Text(
            value,
            style: AppFonts.firaCode(fontSize: 12, color: valueColor),
          ),
        ),
      ],
    );
  }
}
