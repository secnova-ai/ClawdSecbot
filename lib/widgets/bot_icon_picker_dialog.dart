import 'package:flutter/material.dart';
import 'package:lucide_icons/lucide_icons.dart';
import '../utils/app_fonts.dart';
import '../l10n/app_localizations.dart';

/// 图标名称到 IconData 的映射表，供选择器和卡片共用
const botIconDataMap = <String, IconData>{
  'scissors': LucideIcons.scissors,
  'bug': LucideIcons.bug,
  'bot': LucideIcons.bot,
  'shield': LucideIcons.shield,
  'globe': LucideIcons.globe,
  'server': LucideIcons.server,
  'zap': LucideIcons.zap,
  'star': LucideIcons.star,
  'flame': LucideIcons.flame,
  'cloud': LucideIcons.cloud,
  'box': LucideIcons.box,
  'heart': LucideIcons.heart,
  'target': LucideIcons.target,
  'anchor': LucideIcons.anchor,
  'compass': LucideIcons.compass,
  'crown': LucideIcons.crown,
  'diamond': LucideIcons.diamond,
  'gem': LucideIcons.gem,
  'rocket': LucideIcons.rocket,
  'swords': LucideIcons.swords,
  'eye': LucideIcons.eye,
  'lock': LucideIcons.lock,
  'key': LucideIcons.key,
  'cpu': LucideIcons.cpu,
  'file-json': LucideIcons.fileJson,
  'package': LucideIcons.package,
  'terminal': LucideIcons.terminal,
  'wifi': LucideIcons.wifi,
};

/// 预设颜色列表
const _presetColors = <int>[
  0xFFEF4444, // 红
  0xFFF97316, // 橙
  0xFFF59E0B, // 黄
  0xFF22C55E, // 绿
  0xFF3B82F6, // 蓝
  0xFF6366F1, // 紫
  0xFFEC4899, // 粉
  0xFFFFFFFF, // 白
];

/// 图标选择结果
class BotIconSelection {
  final String iconName;
  final int colorValue;
  const BotIconSelection({required this.iconName, required this.colorValue});
}

/// Bot 图标选择器对话框
class BotIconPickerDialog extends StatefulWidget {
  final String? currentIcon;
  final int? currentColor;

  const BotIconPickerDialog({
    super.key,
    this.currentIcon,
    this.currentColor,
  });

  @override
  State<BotIconPickerDialog> createState() => _BotIconPickerDialogState();
}

class _BotIconPickerDialogState extends State<BotIconPickerDialog> {
  late String _selectedIcon;
  late int _selectedColor;

  @override
  void initState() {
    super.initState();
    _selectedIcon = widget.currentIcon ?? 'package';
    _selectedColor = widget.currentColor ?? 0xFF6366F1;
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context)!;
    final isZh = l10n.localeName.startsWith('zh');

    return Dialog(
      backgroundColor: const Color(0xFF1A1A2E),
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
      child: Container(
        width: 400,
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            // 标题行 + 预览
            Row(
              children: [
                Text(
                  isZh ? '选择图标' : 'Choose Icon',
                  style: AppFonts.inter(
                    fontSize: 16,
                    fontWeight: FontWeight.w600,
                    color: Colors.white,
                  ),
                ),
                const Spacer(),
                // 实时预览
                Container(
                  padding: const EdgeInsets.all(10),
                  decoration: BoxDecoration(
                    color: Color(_selectedColor).withValues(alpha: 0.2),
                    borderRadius: BorderRadius.circular(10),
                    border: Border.all(
                      color: Color(_selectedColor).withValues(alpha: 0.4),
                    ),
                  ),
                  child: Icon(
                    botIconDataMap[_selectedIcon] ?? LucideIcons.package,
                    color: Color(_selectedColor),
                    size: 22,
                  ),
                ),
              ],
            ),
            const SizedBox(height: 20),

            // 图标网格
            Text(
              isZh ? '图标' : 'Icon',
              style: AppFonts.inter(
                fontSize: 12,
                fontWeight: FontWeight.w500,
                color: Colors.white54,
              ),
            ),
            const SizedBox(height: 8),
            Wrap(
              spacing: 6,
              runSpacing: 6,
              children: botIconDataMap.entries.map((entry) {
                final isSelected = entry.key == _selectedIcon;
                return MouseRegion(
                  cursor: SystemMouseCursors.click,
                  child: GestureDetector(
                    onTap: () => setState(() => _selectedIcon = entry.key),
                    child: AnimatedContainer(
                      duration: const Duration(milliseconds: 150),
                      width: 40,
                      height: 40,
                      decoration: BoxDecoration(
                        color: isSelected
                            ? Color(_selectedColor).withValues(alpha: 0.2)
                            : Colors.white.withValues(alpha: 0.05),
                        borderRadius: BorderRadius.circular(8),
                        border: Border.all(
                          color: isSelected
                              ? Color(_selectedColor)
                              : Colors.white.withValues(alpha: 0.1),
                          width: isSelected ? 1.5 : 1,
                        ),
                      ),
                      child: Icon(
                        entry.value,
                        size: 18,
                        color: isSelected
                            ? Color(_selectedColor)
                            : Colors.white54,
                      ),
                    ),
                  ),
                );
              }).toList(),
            ),
            const SizedBox(height: 20),

            // 颜色选择
            Text(
              isZh ? '颜色' : 'Color',
              style: AppFonts.inter(
                fontSize: 12,
                fontWeight: FontWeight.w500,
                color: Colors.white54,
              ),
            ),
            const SizedBox(height: 8),
            Row(
              children: _presetColors.map((colorValue) {
                final isSelected = colorValue == _selectedColor;
                return Padding(
                  padding: const EdgeInsets.only(right: 8),
                  child: MouseRegion(
                    cursor: SystemMouseCursors.click,
                    child: GestureDetector(
                      onTap: () => setState(() => _selectedColor = colorValue),
                      child: AnimatedContainer(
                        duration: const Duration(milliseconds: 150),
                        width: 32,
                        height: 32,
                        decoration: BoxDecoration(
                          color: Color(colorValue),
                          shape: BoxShape.circle,
                          border: Border.all(
                            color: isSelected
                                ? Colors.white
                                : Colors.transparent,
                            width: 2.5,
                          ),
                          boxShadow: isSelected
                              ? [
                                  BoxShadow(
                                    color: Color(colorValue)
                                        .withValues(alpha: 0.4),
                                    blurRadius: 8,
                                  ),
                                ]
                              : null,
                        ),
                      ),
                    ),
                  ),
                );
              }).toList(),
            ),
            const SizedBox(height: 24),

            // 按钮行
            Row(
              mainAxisAlignment: MainAxisAlignment.end,
              children: [
                TextButton(
                  onPressed: () => Navigator.pop(context),
                  child: Text(
                    isZh ? '取消' : 'Cancel',
                    style: AppFonts.inter(fontSize: 13, color: Colors.white54),
                  ),
                ),
                const SizedBox(width: 12),
                ElevatedButton(
                  onPressed: () => Navigator.pop(
                    context,
                    BotIconSelection(
                      iconName: _selectedIcon,
                      colorValue: _selectedColor,
                    ),
                  ),
                  style: ElevatedButton.styleFrom(
                    backgroundColor: const Color(0xFF6366F1),
                    foregroundColor: Colors.white,
                    shape: RoundedRectangleBorder(
                      borderRadius: BorderRadius.circular(8),
                    ),
                    padding: const EdgeInsets.symmetric(
                      horizontal: 16,
                      vertical: 10,
                    ),
                  ),
                  child: Text(
                    isZh ? '确认' : 'Confirm',
                    style: AppFonts.inter(
                      fontSize: 13,
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
