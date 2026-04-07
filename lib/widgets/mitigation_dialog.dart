import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'dart:io';
import '../utils/app_fonts.dart';
import '../models/risk_model.dart';
import '../services/plugin_service.dart';

class MitigationDialog extends StatefulWidget {
  final RiskInfo risk;

  const MitigationDialog({super.key, required this.risk});

  @override
  State<MitigationDialog> createState() => _MitigationDialogState();
}

class _MitigationDialogState extends State<MitigationDialog> {
  final _formKey = GlobalKey<FormState>();
  final Map<String, dynamic> _formData = {};
  bool _isLoading = false;
  String? _error;

  @override
  void initState() {
    super.initState();
    // Check if this is a skills_not_scanned risk - handle specially
    if (widget.risk.id == 'skills_not_scanned') {
      WidgetsBinding.instance.addPostFrameCallback((_) {
        _redirectToSkillScan();
      });
      return;
    }
    // Initialize default values
    if (widget.risk.mitigation != null) {
      for (var item in widget.risk.mitigation!.formSchema) {
        if (item.defaultValue != null) {
          _formData[item.key] = item.defaultValue;
        }
      }
    }
  }

  void _redirectToSkillScan() {
    // Return special result to indicate skill scan is needed
    Navigator.of(
      context,
    ).pop({'action': 'skill_scan', 'asset_name': widget.risk.sourcePlugin});
  }

  Future<void> _submit() async {
    if (!_formKey.currentState!.validate()) return;
    _formKey.currentState!.save();

    setState(() {
      _isLoading = true;
      _error = null;
    });

    try {
      final result = await PluginService().mitigateRisk(widget.risk, _formData);
      if (result['success'] == true) {
        if (mounted) {
          Navigator.of(context).pop(true); // Return success
          ScaffoldMessenger.of(context).showSnackBar(
            SnackBar(
              content: Row(
                children: [
                  const Icon(Icons.check_circle, color: Colors.white, size: 24),
                  const SizedBox(width: 12),
                  Expanded(
                    child: Text(
                      '修复成功',
                      style: AppFonts.inter(
                        color: Colors.white,
                        fontSize: 14,
                        fontWeight: FontWeight.w500,
                      ),
                    ),
                  ),
                ],
              ),
              backgroundColor: const Color(0xFF10B981),
              behavior: SnackBarBehavior.floating,
              shape: RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(12),
              ),
              margin: const EdgeInsets.all(16),
              elevation: 6,
              duration: const Duration(seconds: 3),
            ),
          );
        }
      } else {
        setState(() {
          _error = result['error'] ?? 'Unknown error';
        });
      }
    } catch (e) {
      setState(() {
        _error = e.toString();
      });
    } finally {
      if (mounted) {
        setState(() {
          _isLoading = false;
        });
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    // For skills_not_scanned, show loading while redirecting
    if (widget.risk.id == 'skills_not_scanned') {
      return AlertDialog(
        backgroundColor: const Color(0xFF1A1A2E),
        content: const SizedBox(
          height: 100,
          child: Center(child: CircularProgressIndicator()),
        ),
      );
    }

    // For suggestion type, show suggestions instead of form
    if (widget.risk.mitigation?.type == 'suggestion') {
      return _buildSuggestionDialog();
    }

    return AlertDialog(
      backgroundColor: const Color(0xFF1A1A2E),
      title: Text(
        '风险处置',
        style: AppFonts.inter(color: Colors.white, fontWeight: FontWeight.bold),
      ),
      content: SingleChildScrollView(
        child: Form(
          key: _formKey,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                widget.risk.title,
                style: AppFonts.inter(color: Colors.white70, fontSize: 14),
              ),
              const SizedBox(height: 16),
              if (_error != null)
                Container(
                  padding: const EdgeInsets.all(8),
                  margin: const EdgeInsets.only(bottom: 16),
                  decoration: BoxDecoration(
                    color: Colors.red.withValues(alpha: 0.1),
                    borderRadius: BorderRadius.circular(8),
                    border: Border.all(
                      color: Colors.red.withValues(alpha: 0.3),
                    ),
                  ),
                  child: Text(
                    _error!,
                    style: const TextStyle(color: Colors.red),
                  ),
                ),
              if (widget.risk.mitigation != null)
                ...widget.risk.mitigation!.formSchema.map((item) {
                  return Padding(
                    padding: const EdgeInsets.only(bottom: 16),
                    child: _buildFormItem(item),
                  );
                }),
              if (widget.risk.mitigation?.formSchema.isEmpty ?? true)
                const Text(
                  '确定要执行自动修复吗？',
                  style: TextStyle(color: Colors.white70),
                ),
            ],
          ),
        ),
      ),
      actions: [
        TextButton(
          onPressed: _isLoading ? null : () => Navigator.of(context).pop(),
          child: const Text('取消', style: TextStyle(color: Colors.white54)),
        ),
        ElevatedButton(
          onPressed: _isLoading ? null : _submit,
          style: ElevatedButton.styleFrom(
            backgroundColor: const Color(0xFF6366F1),
            foregroundColor: Colors.white,
          ),
          child: _isLoading
              ? const SizedBox(
                  width: 16,
                  height: 16,
                  child: CircularProgressIndicator(
                    strokeWidth: 2,
                    color: Colors.white,
                  ),
                )
              : const Text('执行修复'),
        ),
      ],
    );
  }

  Widget _buildFormItem(FormItem item) {
    final label = _displayFormLabel(item);
    switch (item.type) {
      case 'boolean':
        return CheckboxListTile(
          title: Text(label, style: const TextStyle(color: Colors.white)),
          value: _formData[item.key] ?? false,
          onChanged: (val) {
            setState(() {
              _formData[item.key] = val;
            });
          },
          checkColor: Colors.black,
          activeColor: const Color(0xFF6366F1),
          contentPadding: EdgeInsets.zero,
          controlAffinity: ListTileControlAffinity.leading,
        );
      case 'select':
        return Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              label,
              style: const TextStyle(color: Colors.white70, fontSize: 14),
            ),
            const SizedBox(height: 8),
            DropdownButtonFormField<String>(
              initialValue: _formData[item.key]?.toString(),
              style: const TextStyle(color: Colors.white),
              dropdownColor: const Color(0xFF2A2A3E),
              decoration: InputDecoration(
                enabledBorder: const OutlineInputBorder(
                  borderSide: BorderSide(color: Colors.white24),
                ),
                focusedBorder: const OutlineInputBorder(
                  borderSide: BorderSide(color: Color(0xFF6366F1)),
                ),
                errorBorder: const OutlineInputBorder(
                  borderSide: BorderSide(color: Colors.red),
                ),
                focusedErrorBorder: const OutlineInputBorder(
                  borderSide: BorderSide(color: Colors.red),
                ),
                contentPadding: const EdgeInsets.symmetric(
                  horizontal: 12,
                  vertical: 8,
                ),
              ),
              items: (item.options ?? []).map<DropdownMenuItem<String>>((
                option,
              ) {
                return DropdownMenuItem<String>(
                  value: option,
                  child: Text(
                    option,
                    style: const TextStyle(color: Colors.white),
                  ),
                );
              }).toList(),
              onChanged: (val) {
                setState(() {
                  _formData[item.key] = val;
                });
              },
              validator: (value) {
                if (item.required && (value == null || value.isEmpty)) {
                  return '此项必填';
                }
                return null;
              },
              onSaved: (val) => _formData[item.key] = val,
            ),
          ],
        );
      case 'text':
      case 'password':
        return TextFormField(
          initialValue: _formData[item.key]?.toString(),
          style: const TextStyle(color: Colors.white),
          obscureText: item.type == 'password',
          decoration: InputDecoration(
            labelText: label,
            labelStyle: const TextStyle(color: Colors.white70),
            enabledBorder: const OutlineInputBorder(
              borderSide: BorderSide(color: Colors.white24),
            ),
            focusedBorder: const OutlineInputBorder(
              borderSide: BorderSide(color: Color(0xFF6366F1)),
            ),
            errorBorder: const OutlineInputBorder(
              borderSide: BorderSide(color: Colors.red),
            ),
            focusedErrorBorder: const OutlineInputBorder(
              borderSide: BorderSide(color: Colors.red),
            ),
          ),
          validator: (value) {
            if (item.required && (value == null || value.isEmpty)) {
              return '此项必填';
            }
            if (value != null && value.isNotEmpty) {
              if (item.minLength > 0 && value.length < item.minLength) {
                return '最小长度为 ${item.minLength}';
              }
              if (item.regex != null && item.regex!.isNotEmpty) {
                try {
                  final regExp = RegExp(item.regex!);
                  if (!regExp.hasMatch(value)) {
                    return item.regexMsg ?? '格式不正确';
                  }
                } catch (e) {
                  return '无效的正则规则';
                }
              }
            }
            return null;
          },
          onSaved: (val) => _formData[item.key] = val,
        );
      default:
        return Text(
          '不支持的字段类型: ${item.type}',
          style: const TextStyle(color: Colors.red),
        );
    }
  }

  String _displayFormLabel(FormItem item) {
    final locale = Localizations.localeOf(context).languageCode.toLowerCase();
    final isZh = locale == 'zh';
    if (!isZh) return item.label;

    if (item.key == 'fix_permission') {
      if (!Platform.isWindows) {
        return '修复目录/文件权限（Unix: chmod）';
      }
      switch (widget.risk.id) {
        case 'config_perm_unsafe':
          return '修复配置文件 ACL 权限';
        case 'config_dir_perm_unsafe':
          return '修复配置目录 ACL 权限';
        case 'log_dir_perm_unsafe':
          return '修复日志目录 ACL 权限';
        default:
          return '修复 ACL 权限';
      }
    }

    return item.label;
  }

  Widget _buildSuggestionDialog() {
    final mitigation = widget.risk.mitigation!;
    return Dialog(
      backgroundColor: const Color(0xFF1A1A2E),
      child: Container(
        constraints: const BoxConstraints(maxWidth: 800, maxHeight: 600),
        padding: const EdgeInsets.all(24),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            // Header
            Row(
              children: [
                Container(
                  padding: const EdgeInsets.all(8),
                  decoration: BoxDecoration(
                    color: Colors.red.withValues(alpha: 0.1),
                    borderRadius: BorderRadius.circular(8),
                  ),
                  child: const Icon(
                    Icons.warning_amber_rounded,
                    color: Colors.red,
                    size: 24,
                  ),
                ),
                const SizedBox(width: 12),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        mitigation.title ?? '安全建议',
                        style: AppFonts.inter(
                          color: Colors.white,
                          fontSize: 18,
                          fontWeight: FontWeight.bold,
                        ),
                      ),
                      if (mitigation.description != null) ...[
                        const SizedBox(height: 4),
                        Text(
                          mitigation.description!,
                          style: AppFonts.inter(
                            color: Colors.white70,
                            fontSize: 12,
                          ),
                        ),
                      ],
                    ],
                  ),
                ),
                IconButton(
                  icon: const Icon(Icons.close, color: Colors.white70),
                  onPressed: () => Navigator.of(context).pop(),
                ),
              ],
            ),
            const SizedBox(height: 24),
            // Content
            Expanded(
              child: SingleChildScrollView(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    // Risk description
                    Container(
                      padding: const EdgeInsets.all(16),
                      decoration: BoxDecoration(
                        color: Colors.red.withValues(alpha: 0.05),
                        borderRadius: BorderRadius.circular(12),
                        border: Border.all(
                          color: Colors.red.withValues(alpha: 0.2),
                        ),
                      ),
                      child: Text(
                        widget.risk.description,
                        style: AppFonts.inter(
                          color: Colors.white70,
                          fontSize: 14,
                          height: 1.5,
                        ),
                      ),
                    ),
                    const SizedBox(height: 24),
                    // Suggestions
                    if (mitigation.suggestions != null)
                      ...mitigation.suggestions!.map((group) {
                        return Padding(
                          padding: const EdgeInsets.only(bottom: 24),
                          child: _buildSuggestionGroup(group),
                        );
                      }),
                  ],
                ),
              ),
            ),
            const SizedBox(height: 16),
            // Footer
            Align(
              alignment: Alignment.centerRight,
              child: TextButton(
                onPressed: () => Navigator.of(context).pop(),
                child: const Text(
                  '关闭',
                  style: TextStyle(color: Colors.white70),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildSuggestionGroup(SuggestionGroup group) {
    Color priorityColor;
    IconData priorityIcon;
    switch (group.priority) {
      case 'P0':
        priorityColor = Colors.red;
        priorityIcon = Icons.warning_amber_rounded;
        break;
      case 'P1':
        priorityColor = Colors.orange;
        priorityIcon = Icons.info_outline;
        break;
      case 'P2':
        priorityColor = Colors.blue;
        priorityIcon = Icons.lightbulb_outline;
        break;
      default:
        priorityColor = Colors.grey;
        priorityIcon = Icons.circle;
    }

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          children: [
            Icon(priorityIcon, color: priorityColor, size: 20),
            const SizedBox(width: 8),
            Text(
              '${group.priority} - ${group.category}',
              style: AppFonts.inter(
                color: priorityColor,
                fontSize: 15,
                fontWeight: FontWeight.bold,
              ),
            ),
          ],
        ),
        const SizedBox(height: 12),
        ...group.items.asMap().entries.map((entry) {
          final index = entry.key;
          final item = entry.value;
          return Padding(
            padding: const EdgeInsets.only(bottom: 12),
            child: _buildSuggestionItem(item, index + 1),
          );
        }),
      ],
    );
  }

  Widget _buildSuggestionItem(SuggestionItem item, int index) {
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: const Color(0xFF2A2A3E),
        borderRadius: BorderRadius.circular(8),
        border: Border.all(color: Colors.white10),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Container(
                width: 24,
                height: 24,
                alignment: Alignment.center,
                decoration: BoxDecoration(
                  color: const Color(0xFF6366F1),
                  borderRadius: BorderRadius.circular(12),
                ),
                child: Text(
                  '$index',
                  style: AppFonts.inter(
                    color: Colors.white,
                    fontSize: 12,
                    fontWeight: FontWeight.bold,
                  ),
                ),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      item.action,
                      style: AppFonts.inter(
                        color: Colors.white,
                        fontSize: 14,
                        fontWeight: FontWeight.w600,
                      ),
                    ),
                    const SizedBox(height: 8),
                    Text(
                      item.detail,
                      style: AppFonts.inter(
                        color: Colors.white70,
                        fontSize: 13,
                        height: 1.5,
                      ),
                    ),
                    if (item.command != null) ...[
                      const SizedBox(height: 12),
                      Container(
                        padding: const EdgeInsets.all(12),
                        decoration: BoxDecoration(
                          color: Colors.black.withValues(alpha: 0.3),
                          borderRadius: BorderRadius.circular(6),
                        ),
                        child: Row(
                          children: [
                            Expanded(
                              child: Text(
                                item.command!,
                                style: AppFonts.robotoMono(
                                  color: const Color(0xFF10B981),
                                  fontSize: 12,
                                ),
                              ),
                            ),
                            IconButton(
                              icon: const Icon(Icons.copy, size: 16),
                              color: Colors.white54,
                              onPressed: () {
                                Clipboard.setData(
                                  ClipboardData(text: item.command!),
                                );
                                ScaffoldMessenger.of(context).showSnackBar(
                                  const SnackBar(
                                    content: Text('命令已复制到剪贴板'),
                                    duration: Duration(seconds: 2),
                                  ),
                                );
                              },
                            ),
                          ],
                        ),
                      ),
                    ],
                  ],
                ),
              ),
            ],
          ),
        ],
      ),
    );
  }
}
