import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../l10n/app_localizations.dart';
import '../models/risk_model.dart';
import '../services/plugin_service.dart';
import '../utils/app_fonts.dart';
import '../utils/risk_localization.dart';

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
    if (widget.risk.id == 'skills_not_scanned') {
      WidgetsBinding.instance.addPostFrameCallback((_) {
        _redirectToSkillScan();
      });
      return;
    }

    final mitigation = widget.risk.mitigation;
    if (mitigation != null) {
      for (final item in mitigation.formSchema) {
        if (item.defaultValue != null) {
          _formData[item.key] = item.defaultValue;
        }
      }
    }
  }

  void _redirectToSkillScan() {
    Navigator.of(context).pop({'action': 'skill_scan'});
  }

  Future<void> _submit() async {
    final l10n = AppLocalizations.of(context)!;
    if (!_formKey.currentState!.validate()) return;
    _formKey.currentState!.save();

    setState(() {
      _isLoading = true;
      _error = null;
    });

    try {
      final result = await PluginService().mitigateRisk(widget.risk, _formData);
      if (result['success'] == true) {
        if (!mounted) return;
        Navigator.of(context).pop(true);
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Row(
              children: [
                const Icon(Icons.check_circle, color: Colors.white, size: 24),
                const SizedBox(width: 12),
                Expanded(
                  child: Text(
                    l10n.fixApplied,
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
      } else {
        setState(() {
          _error = result['error']?.toString() ?? 'Unknown error';
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
    final l10n = AppLocalizations.of(context)!;

    if (widget.risk.id == 'skills_not_scanned') {
      return const AlertDialog(
        backgroundColor: Color(0xFF1A1A2E),
        content: SizedBox(
          height: 100,
          child: Center(child: CircularProgressIndicator()),
        ),
      );
    }

    if (widget.risk.mitigation?.type == 'suggestion') {
      return _buildSuggestionDialog(l10n);
    }

    return AlertDialog(
      backgroundColor: const Color(0xFF1A1A2E),
      title: Text(
        l10n.mitigationDialogTitle,
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
                RiskLocalization.mitigationTitle(widget.risk, l10n),
                style: AppFonts.inter(
                  color: Colors.white,
                  fontSize: 16,
                  fontWeight: FontWeight.w600,
                ),
              ),
              const SizedBox(height: 8),
              Text(
                RiskLocalization.mitigationDescription(widget.risk, l10n),
                style: AppFonts.inter(color: Colors.white70, fontSize: 13),
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
                    child: _buildFormItem(l10n, item),
                  );
                }),
              if (widget.risk.mitigation?.formSchema.isEmpty ?? true)
                Text(
                  l10n.mitigationConfirmAutoFix,
                  style: const TextStyle(color: Colors.white70),
                ),
            ],
          ),
        ),
      ),
      actions: [
        TextButton(
          onPressed: _isLoading ? null : () => Navigator.of(context).pop(),
          child: Text(
            l10n.cancel,
            style: const TextStyle(color: Colors.white54),
          ),
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
              : Text(l10n.mitigationExecute),
        ),
      ],
    );
  }

  Widget _buildFormItem(AppLocalizations l10n, FormItem item) {
    final label = RiskLocalization.formLabel(widget.risk, item, l10n);
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
              items: (item.options ?? []).map((option) {
                return DropdownMenuItem<String>(
                  value: option,
                  child: Text(
                    RiskLocalization.formOption(
                      widget.risk,
                      item,
                      option,
                      l10n,
                    ),
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
                  return l10n.mitigationFieldRequired;
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
              return l10n.mitigationFieldRequired;
            }
            if (value != null && value.isNotEmpty) {
              if (item.minLength > 0 && value.length < item.minLength) {
                return l10n.mitigationFieldMinLength(item.minLength);
              }
              if (item.regex != null && item.regex!.isNotEmpty) {
                try {
                  final regExp = RegExp(item.regex!);
                  if (!regExp.hasMatch(value)) {
                    return item.regexMsg ?? l10n.mitigationFieldInvalidFormat;
                  }
                } catch (_) {
                  return l10n.mitigationFieldInvalidRegex;
                }
              }
            }
            return null;
          },
          onSaved: (val) => _formData[item.key] = val,
        );
      default:
        return Text(
          '${l10n.mitigationUnsupportedFieldType}: ${item.type}',
          style: const TextStyle(color: Colors.red),
        );
    }
  }

  Widget _buildSuggestionDialog(AppLocalizations l10n) {
    final mitigation = widget.risk.mitigation!;
    return Dialog(
      backgroundColor: const Color(0xFF1A1A2E),
      child: Container(
        constraints: const BoxConstraints(maxWidth: 800, maxHeight: 600),
        padding: const EdgeInsets.all(24),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
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
                        RiskLocalization.mitigationTitle(widget.risk, l10n),
                        style: AppFonts.inter(
                          color: Colors.white,
                          fontSize: 18,
                          fontWeight: FontWeight.bold,
                        ),
                      ),
                      const SizedBox(height: 4),
                      Text(
                        RiskLocalization.mitigationDescription(
                          widget.risk,
                          l10n,
                        ),
                        style: AppFonts.inter(
                          color: Colors.white70,
                          fontSize: 12,
                        ),
                      ),
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
            Expanded(
              child: SingleChildScrollView(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
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
                        RiskLocalization.riskDescription(widget.risk, l10n),
                        style: AppFonts.inter(
                          color: Colors.white70,
                          fontSize: 14,
                          height: 1.5,
                        ),
                      ),
                    ),
                    const SizedBox(height: 24),
                    if (mitigation.suggestions != null)
                      ...mitigation.suggestions!.map((group) {
                        return Padding(
                          padding: const EdgeInsets.only(bottom: 24),
                          child: _buildSuggestionGroup(l10n, group),
                        );
                      }),
                  ],
                ),
              ),
            ),
            const SizedBox(height: 16),
            Align(
              alignment: Alignment.centerRight,
              child: TextButton(
                onPressed: () => Navigator.of(context).pop(),
                child: Text(
                  l10n.close,
                  style: const TextStyle(color: Colors.white70),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildSuggestionGroup(AppLocalizations l10n, SuggestionGroup group) {
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
              '${group.priority} - ${RiskLocalization.suggestionCategory(widget.risk, group, l10n)}',
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
          return Padding(
            padding: const EdgeInsets.only(bottom: 12),
            child: _buildSuggestionItem(l10n, entry.value, entry.key + 1),
          );
        }),
      ],
    );
  }

  Widget _buildSuggestionItem(
    AppLocalizations l10n,
    SuggestionItem item,
    int index,
  ) {
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: const Color(0xFF2A2A3E),
        borderRadius: BorderRadius.circular(8),
        border: Border.all(color: Colors.white10),
      ),
      child: Row(
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
                  RiskLocalization.suggestionAction(widget.risk, item, l10n),
                  style: AppFonts.inter(
                    color: Colors.white,
                    fontSize: 14,
                    fontWeight: FontWeight.w600,
                  ),
                ),
                const SizedBox(height: 8),
                Text(
                  RiskLocalization.suggestionDetail(widget.risk, item, l10n),
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
                              SnackBar(
                                content: Text(l10n.mitigationCommandCopied),
                                duration: const Duration(seconds: 2),
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
    );
  }
}
