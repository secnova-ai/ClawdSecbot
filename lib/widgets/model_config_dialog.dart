import 'package:flutter/material.dart';
import 'security_model_config_form.dart';
import '../l10n/app_localizations.dart';
import '../utils/app_fonts.dart';
import 'package:lucide_icons/lucide_icons.dart';

/// 安全模型配置对话框
/// 用于 ShepherdGate 风险检测的 LLM 配置
class ModelConfigDialog extends StatefulWidget {
  /// Creates a model configuration dialog.
  const ModelConfigDialog({super.key});

  @override
  State<ModelConfigDialog> createState() => _ModelConfigDialogState();
}

class _ModelConfigDialogState extends State<ModelConfigDialog> {
  final GlobalKey<SecurityModelConfigFormState> _formKey =
      GlobalKey<SecurityModelConfigFormState>();
  bool _saving = false;

  Future<void> _handleSave() async {
    if (_saving) return;
    setState(() {
      _saving = true;
    });

    final success = await _formKey.currentState?.saveConfig();
    if (success == true) {
      if (mounted && Navigator.of(context).canPop()) {
        Navigator.of(context).pop(true);
      }
    } else {
      if (mounted) {
        setState(() {
          _saving = false;
        });
      }
    }
  }

  void _handleCancel() {
    if (_saving) return;
    if (Navigator.of(context).canPop()) {
      Navigator.of(context).pop();
    }
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context)!;
    return Dialog(
      backgroundColor: const Color(0xFF1A1A2E),
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
      child: Container(
        width: 450,
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            // Header
            Row(
              children: [
                Container(
                  padding: const EdgeInsets.all(8),
                  decoration: BoxDecoration(
                    color: const Color(0xFF6366F1).withValues(alpha: 0.2),
                    borderRadius: BorderRadius.circular(8),
                  ),
                  child: const Icon(
                    LucideIcons.shield,
                    color: Color(0xFF6366F1),
                    size: 20,
                  ),
                ),
                const SizedBox(width: 12),
                Expanded(
                  child: Text(
                    l10n.modelConfigTitle,
                    style: AppFonts.inter(
                      fontSize: 18,
                      fontWeight: FontWeight.w600,
                      color: Colors.white,
                    ),
                  ),
                ),
                IconButton(
                  icon: const Icon(
                    LucideIcons.x,
                    color: Colors.white54,
                    size: 20,
                  ),
                  onPressed: _saving ? null : _handleCancel,
                ),
              ],
            ),
            const SizedBox(height: 16),
            // Form
            SecurityModelConfigForm(key: _formKey),
            const SizedBox(height: 20),
            // Actions
            Row(
              mainAxisAlignment: MainAxisAlignment.end,
              children: [
                TextButton(
                  onPressed: _saving ? null : _handleCancel,
                  child: Text(
                    l10n.cancel,
                    style: AppFonts.inter(
                      fontSize: 14,
                      color: _saving ? Colors.white24 : Colors.white54,
                    ),
                  ),
                ),
                const SizedBox(width: 12),
                ElevatedButton(
                  onPressed: _saving ? null : _handleSave,
                  style: ElevatedButton.styleFrom(
                    backgroundColor: const Color(0xFF6366F1),
                    foregroundColor: Colors.white,
                    padding: const EdgeInsets.symmetric(
                      horizontal: 24,
                      vertical: 12,
                    ),
                    shape: RoundedRectangleBorder(
                      borderRadius: BorderRadius.circular(8),
                    ),
                  ),
                  child: _saving
                      ? Row(
                          mainAxisSize: MainAxisSize.min,
                          children: [
                            const SizedBox(
                              width: 14,
                              height: 14,
                              child: CircularProgressIndicator(
                                strokeWidth: 2,
                                color: Colors.white,
                              ),
                            ),
                            const SizedBox(width: 10),
                            Text(
                              l10n.modelConfigSaving,
                              style: AppFonts.inter(fontSize: 14),
                            ),
                          ],
                        )
                      : Text(
                          l10n.modelConfigSave,
                          style: AppFonts.inter(
                            fontSize: 14,
                            fontWeight: FontWeight.w600,
                          ),
                        ),
                ),
              ],
            ),
          ],
        ),
      ),
    );
  }
}
