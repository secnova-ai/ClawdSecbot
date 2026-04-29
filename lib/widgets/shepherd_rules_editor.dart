import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:lucide_icons/lucide_icons.dart';

import '../l10n/app_localizations.dart';
import '../models/shepherd_rule_model.dart';
import '../utils/app_fonts.dart';

class ShepherdRulesEditor extends StatefulWidget {
  final List<ShepherdSemanticRule> rules;
  final ValueChanged<List<ShepherdSemanticRule>> onChanged;

  const ShepherdRulesEditor({
    super.key,
    required this.rules,
    required this.onChanged,
  });

  @override
  State<ShepherdRulesEditor> createState() => _ShepherdRulesEditorState();
}

class _ShepherdRulesEditorState extends State<ShepherdRulesEditor> {
  final TextEditingController _controller = TextEditingController();

  static const Map<String, String> _zhSemanticRuleLabels = {
    'delete': '删除',
    'remove': '移除',
    'drop': '删除（数据库）',
    'truncate': '清空（数据库）',
    'update': '更新',
    'write': '写入',
    'edit': '编辑',
    'modify': '修改',
    'execute': '执行',
    'exec': '执行',
    'run': '运行',
    'install': '安装',
    'uninstall': '卸载',
    'chmod': '修改权限',
    'chown': '修改属主',
    'sudo': '提权执行',
    'kill': '终止进程',
    'shutdown': '关机',
    'reboot': '重启',
    'format': '格式化',
    'rm': '删除文件',
    'mv': '移动文件',
    'cp': '复制文件',
    'curl': '网络请求',
    'wget': '网络下载',
    'ssh': '远程连接',
    'scp': '远程传输',
    'powershell': 'PowerShell 执行',
    'bash': 'Bash 执行',
    'cmd': '命令行执行',
  };

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context)!;
    final customRules = widget.rules.where((rule) => rule.isCustom).toList();
    final builtinRules = widget.rules.where((rule) => !rule.isCustom).toList();

    return Container(
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: Colors.white.withValues(alpha: 0.03),
        borderRadius: BorderRadius.circular(8),
        border: Border.all(color: Colors.white.withValues(alpha: 0.1)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          _buildHeader(l10n),
          const SizedBox(height: 14),
          _buildInput(l10n),
          const SizedBox(height: 14),
          if (customRules.isNotEmpty) ...[
            _buildSectionLabel(l10n.shepherdRulesTitle),
            const SizedBox(height: 8),
            ...customRules.map(_buildRuleRow),
            const SizedBox(height: 12),
          ],
          _buildSectionLabel(l10n.securitySkillsTitle),
          const SizedBox(height: 8),
          ...builtinRules.map(_buildRuleRow),
        ],
      ),
    );
  }

  Widget _buildHeader(AppLocalizations l10n) {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const Icon(LucideIcons.shieldAlert, color: Color(0xFF6366F1), size: 18),
        const SizedBox(width: 8),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                l10n.shepherdRulesTitle,
                style: AppFonts.inter(
                  fontSize: 14,
                  fontWeight: FontWeight.w600,
                  color: Colors.white,
                ),
              ),
              const SizedBox(height: 2),
              Text(
                l10n.shepherdRulesDesc,
                style: AppFonts.inter(fontSize: 12, color: Colors.white54),
              ),
            ],
          ),
        ),
      ],
    );
  }

  Widget _buildInput(AppLocalizations l10n) {
    return Row(
      children: [
        Expanded(
          child: SizedBox(
            height: 38,
            child: TextField(
              controller: _controller,
              style: AppFonts.firaCode(fontSize: 12, color: Colors.white),
              decoration: InputDecoration(
                hintText: l10n.shepherdSensitivePlaceholder,
                hintStyle: AppFonts.inter(fontSize: 11, color: Colors.white38),
                filled: true,
                fillColor: Colors.white.withValues(alpha: 0.05),
                enabledBorder: OutlineInputBorder(
                  borderRadius: BorderRadius.circular(6),
                  borderSide: BorderSide(
                    color: Colors.white.withValues(alpha: 0.1),
                  ),
                ),
                focusedBorder: OutlineInputBorder(
                  borderRadius: BorderRadius.circular(6),
                  borderSide: const BorderSide(color: Color(0xFF6366F1)),
                ),
                contentPadding: const EdgeInsets.symmetric(
                  horizontal: 10,
                  vertical: 10,
                ),
              ),
              onSubmitted: (_) => _addRule(),
            ),
          ),
        ),
        const SizedBox(width: 8),
        Tooltip(
          message: l10n.localeName.startsWith('zh') ? '添加' : 'Add',
          child: IconButton(
            onPressed: _addRule,
            icon: const Icon(LucideIcons.plus, size: 16),
            style: IconButton.styleFrom(
              backgroundColor: const Color(0xFF6366F1),
              foregroundColor: Colors.white,
              fixedSize: const Size(38, 38),
              shape: RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(6),
              ),
            ),
          ),
        ),
      ],
    );
  }

  Widget _buildSectionLabel(String text) {
    return Text(
      text,
      style: AppFonts.inter(
        fontSize: 12,
        fontWeight: FontWeight.w600,
        color: Colors.white70,
      ),
    );
  }

  Widget _buildRuleRow(ShepherdSemanticRule rule) {
    final l10n = AppLocalizations.of(context)!;
    final isZh = l10n.localeName.startsWith('zh');
    final displayText = _localizeRuleDescription(rule.description, l10n);
    final index = widget.rules.indexWhere((item) => item.id == rule.id);

    return Container(
      margin: const EdgeInsets.only(bottom: 8),
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 8),
      decoration: BoxDecoration(
        color: rule.enabled
            ? const Color(0xFF111827).withValues(alpha: 0.72)
            : Colors.white.withValues(alpha: 0.03),
        borderRadius: BorderRadius.circular(8),
        border: Border.all(
          color: rule.enabled
              ? const Color(0xFF6366F1).withValues(alpha: 0.32)
              : Colors.white.withValues(alpha: 0.08),
        ),
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Switch(
            value: rule.enabled,
            onChanged: index < 0
                ? null
                : (value) => _replaceRule(index, rule.copyWith(enabled: value)),
            activeThumbColor: const Color(0xFF6366F1),
          ),
          const SizedBox(width: 8),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  displayText,
                  style: AppFonts.inter(
                    fontSize: 12,
                    color: rule.enabled ? Colors.white : Colors.white54,
                  ),
                ),
                const SizedBox(height: 6),
                Wrap(
                  spacing: 6,
                  runSpacing: 6,
                  children: [
                    _buildBadge(
                      rule.isCustom
                          ? (isZh ? '自定义' : 'Custom')
                          : (isZh ? '内置' : 'Built-in'),
                    ),
                    _buildBadge(_localizeAction(rule.action, isZh)),
                    _buildBadge(_localizeRisk(rule.riskType, isZh)),
                    for (final stage in rule.appliesTo)
                      _buildBadge(_localizeStage(stage, isZh)),
                  ],
                ),
              ],
            ),
          ),
          if (rule.isCustom)
            Tooltip(
              message: MaterialLocalizations.of(context).deleteButtonTooltip,
              child: IconButton(
                onPressed: index < 0 ? null : () => _removeRule(index),
                icon: const Icon(LucideIcons.trash2, size: 15),
                color: Colors.white54,
                constraints: const BoxConstraints.tightFor(
                  width: 32,
                  height: 32,
                ),
                padding: EdgeInsets.zero,
              ),
            ),
        ],
      ),
    );
  }

  Widget _buildBadge(String text) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 7, vertical: 3),
      decoration: BoxDecoration(
        color: Colors.white.withValues(alpha: 0.06),
        borderRadius: BorderRadius.circular(4),
        border: Border.all(color: Colors.white.withValues(alpha: 0.08)),
      ),
      child: Text(
        text,
        style: AppFonts.inter(fontSize: 10, color: Colors.white60),
      ),
    );
  }

  void _addRule() {
    final description = _controller.text.trim();
    if (description.isEmpty) return;
    final exists = widget.rules.any(
      (rule) =>
          rule.description.trim().toLowerCase() == description.toLowerCase(),
    );
    if (exists) return;

    _controller.clear();
    _emit([
      ...widget.rules,
      ShepherdSemanticRule(
        id: 'custom_${DateTime.now().microsecondsSinceEpoch}',
        scope: 'custom',
        description: description,
      ),
    ]);
  }

  void _replaceRule(int index, ShepherdSemanticRule rule) {
    final next = [...widget.rules];
    next[index] = rule;
    _emit(next);
  }

  void _removeRule(int index) {
    final next = [...widget.rules]..removeAt(index);
    _emit(next);
  }

  void _emit(List<ShepherdSemanticRule> rules) {
    HapticFeedback.selectionClick();
    widget.onChanged(rules);
  }

  String _localizeRuleDescription(String rawRule, AppLocalizations l10n) {
    if (!l10n.localeName.startsWith('zh')) {
      return rawRule;
    }
    final normalized = rawRule.trim().toLowerCase();
    if (normalized.isEmpty) {
      return rawRule;
    }

    if (normalized.startsWith(
      'writing to or modifying critical system files or files outside the project workspace',
    )) {
      return '写入或修改关键系统文件，或修改项目工作区之外的文件需要确认（说明：在项目内创建/编辑文件不需要确认）';
    }
    if (normalized.startsWith(
      'sending emails, messages, or notifications to external recipients',
    )) {
      return '向外部接收方发送邮件、消息或通知需要确认';
    }
    if (normalized.startsWith('executing dangerous shell commands')) {
      return '执行潜在危险的 Shell 命令需要确认（如 rm -rf /、chmod、systemctl）';
    }
    if (normalized.startsWith('changing global system settings') ||
        normalized.startsWith(
          'changing global system settings or configuration',
        )) {
      return '修改全局系统设置或配置需要确认';
    }
    if (normalized.startsWith(
      'payment, purchase, subscription, billing, or money transfer operations',
    )) {
      return '支付、购买、订阅、账单或转账操作需要用户确认';
    }

    return _zhSemanticRuleLabels[normalized] ?? rawRule;
  }

  String _localizeStage(String stage, bool isZh) {
    switch (stage) {
      case 'user_input':
        return isZh ? '用户输入' : 'User input';
      case 'tool_call':
        return isZh ? '工具调用' : 'Tool call';
      case 'tool_call_result':
        return isZh ? '工具结果' : 'Tool result';
      case 'final_result':
        return isZh ? '最终输出' : 'Final result';
      default:
        return stage;
    }
  }

  String _localizeAction(String action, bool isZh) {
    switch (action.toLowerCase()) {
      case 'block':
        return isZh ? '阻断' : 'Block';
      case 'allow':
        return isZh ? '允许' : 'Allow';
      case 'redact':
        return isZh ? '脱敏' : 'Redact';
      default:
        return isZh ? '需要确认' : 'Confirm';
    }
  }

  String _localizeRisk(String riskType, bool isZh) {
    if (!isZh) return riskType;
    switch (riskType.toUpperCase()) {
      case 'PROMPT_INJECTION_DIRECT':
        return '直接提示注入';
      case 'PROMPT_INJECTION_INDIRECT':
        return '间接提示注入';
      case 'SENSITIVE_DATA_EXFILTRATION':
        return '敏感数据外泄';
      case 'UNEXPECTED_CODE_EXECUTION':
        return '非预期代码执行';
      case 'PRIVILEGE_ABUSE':
        return '权限滥用';
      default:
        return '高风险操作';
    }
  }
}
