import 'package:flutter/material.dart';
import 'package:lucide_icons/lucide_icons.dart';

import '../l10n/app_localizations.dart';
import '../services/provider_service.dart';
import '../utils/app_fonts.dart';

/// 供应商选择器：摘要按钮 + 搜索 + 分组列表（底部弹层）。
class ModelProviderSelector extends StatefulWidget {
  /// 创建供应商选择器。
  const ModelProviderSelector({
    super.key,
    required this.providers,
    required this.selectedName,
    required this.onProviderSelected,
    required this.iconForName,
    this.readOnly = false,
    this.accentColor = const Color(0xFF10B981),
  });

  /// 可选供应商列表。
  final List<ProviderInfo> providers;

  /// 当前选中的 provider id。
  final String selectedName;

  /// 选中回调。
  final ValueChanged<ProviderInfo> onProviderSelected;

  /// 将 Go 下发的 icon 字符串映射为 IconData。
  final IconData Function(String iconName) iconForName;

  /// 是否禁用交互（只读表单）。
  final bool readOnly;

  /// 选中态强调色（Bot 与安全模型可区分）。
  final Color accentColor;

  @override
  State<ModelProviderSelector> createState() => _ModelProviderSelectorState();
}

class _ModelProviderSelectorState extends State<ModelProviderSelector> {
  /// 与 Go `ProviderInfo.group` 一致：recommended / china / global / local。
  static const List<String> _groupOrder = [
    'recommended',
    'china',
    'global',
    'local',
    '',
  ];

  String _groupTitle(AppLocalizations l10n, String group) {
    switch (group) {
      case 'recommended':
        return l10n.modelConfigGroupRecommended;
      case 'china':
        return l10n.modelConfigGroupChina;
      case 'global':
        return l10n.modelConfigGroupGlobal;
      case 'local':
        return l10n.modelConfigGroupLocal;
      case 'compatible':
        return l10n.modelConfigGroupCompatible;
      default:
        return l10n.modelConfigGroupOther;
    }
  }

  int _groupRank(String group) {
    final idx = _groupOrder.indexOf(group);
    return idx < 0 ? _groupOrder.length : idx;
  }

  Map<String, List<ProviderInfo>> _groupProviders() {
    final map = <String, List<ProviderInfo>>{};
    for (final p in widget.providers) {
      final g = p.group.isEmpty ? '' : p.group;
      map.putIfAbsent(g, () => []).add(p);
    }
    for (final e in map.values) {
      e.sort((a, b) => a.displayName.compareTo(b.displayName));
    }
    return map;
  }

  ProviderInfo? _selectedInfo() {
    for (final p in widget.providers) {
      if (p.name == widget.selectedName) {
        return p;
      }
    }
    return null;
  }

  Future<void> _openSheet() async {
    if (widget.readOnly || widget.providers.isEmpty) {
      return;
    }
    final l10n = AppLocalizations.of(context)!;
    final grouped = _groupProviders();
    final keys = grouped.keys.toList()
      ..sort((a, b) => _groupRank(a).compareTo(_groupRank(b)));

    await showModalBottomSheet<void>(
      context: context,
      isScrollControlled: true,
      backgroundColor: const Color(0xFF11111A),
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(12)),
      ),
      builder: (ctx) {
        return _ProviderPickerSheet(
          groupedKeys: keys,
          grouped: grouped,
          groupTitle: (g) => _groupTitle(l10n, g),
          searchHint: l10n.modelConfigSearchProvider,
          sheetTitle: l10n.modelConfigSelectProviderTitle,
          iconForName: widget.iconForName,
          accentColor: widget.accentColor,
          initialSelected: widget.selectedName,
          onPick: (p) {
            Navigator.of(ctx).pop();
            widget.onProviderSelected(p);
          },
        );
      },
    );
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context)!;
    final selected = _selectedInfo();
    final label = selected?.displayName ?? widget.selectedName;

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          l10n.modelConfigProvider,
          style: AppFonts.inter(
            fontSize: 13,
            fontWeight: FontWeight.w500,
            color: Colors.white70,
          ),
        ),
        const SizedBox(height: 8),
        Material(
          color: Colors.transparent,
          child: InkWell(
            onTap: widget.readOnly ? null : _openSheet,
            borderRadius: BorderRadius.circular(8),
            child: AnimatedContainer(
              duration: const Duration(milliseconds: 200),
              padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
              decoration: BoxDecoration(
                color: const Color(0xFF1E1E2E),
                borderRadius: BorderRadius.circular(8),
                border: Border.all(color: Colors.white.withValues(alpha: 0.1)),
              ),
              child: Row(
                children: [
                  Icon(
                    selected != null
                        ? widget.iconForName(selected.icon)
                        : LucideIcons.sparkles,
                    size: 18,
                    color: Colors.white70,
                  ),
                  const SizedBox(width: 10),
                  Expanded(
                    child: Text(
                      label,
                      style: AppFonts.inter(
                        fontSize: 14,
                        fontWeight: FontWeight.w600,
                        color: Colors.white,
                      ),
                      overflow: TextOverflow.ellipsis,
                    ),
                  ),
                  if (!widget.readOnly)
                    Icon(
                      LucideIcons.chevronsUpDown,
                      size: 18,
                      color: Colors.white54,
                    ),
                ],
              ),
            ),
          ),
        ),
      ],
    );
  }
}

class _ProviderPickerSheet extends StatefulWidget {
  const _ProviderPickerSheet({
    required this.groupedKeys,
    required this.grouped,
    required this.groupTitle,
    required this.searchHint,
    required this.sheetTitle,
    required this.iconForName,
    required this.accentColor,
    required this.initialSelected,
    required this.onPick,
  });

  final List<String> groupedKeys;
  final Map<String, List<ProviderInfo>> grouped;
  final String Function(String group) groupTitle;
  final String searchHint;
  final String sheetTitle;
  final IconData Function(String iconName) iconForName;
  final Color accentColor;
  final String initialSelected;
  final ValueChanged<ProviderInfo> onPick;

  @override
  State<_ProviderPickerSheet> createState() => _ProviderPickerSheetState();
}

class _ProviderPickerSheetState extends State<_ProviderPickerSheet> {
  final TextEditingController _search = TextEditingController();

  @override
  void dispose() {
    _search.dispose();
    super.dispose();
  }

  bool _match(ProviderInfo p, String q) {
    if (q.isEmpty) {
      return true;
    }
    final n = p.name.toLowerCase();
    final d = p.displayName.toLowerCase();
    return n.contains(q) || d.contains(q);
  }

  @override
  Widget build(BuildContext context) {
    final q = _search.text.trim().toLowerCase();
    return Padding(
      padding: EdgeInsets.only(
        left: 16,
        right: 16,
        top: 12,
        bottom: MediaQuery.of(context).viewInsets.bottom + 16,
      ),
      child: DraggableScrollableSheet(
        expand: false,
        initialChildSize: 0.75,
        minChildSize: 0.45,
        maxChildSize: 0.95,
        builder: (context, scrollController) {
          return Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Center(
                child: Container(
                  width: 40,
                  height: 4,
                  decoration: BoxDecoration(
                    color: Colors.white24,
                    borderRadius: BorderRadius.circular(2),
                  ),
                ),
              ),
              const SizedBox(height: 12),
              Text(
                widget.sheetTitle,
                style: AppFonts.inter(
                  fontSize: 16,
                  fontWeight: FontWeight.w600,
                  color: Colors.white,
                ),
              ),
              const SizedBox(height: 10),
              TextField(
                controller: _search,
                onChanged: (_) => setState(() {}),
                style: AppFonts.inter(fontSize: 14, color: Colors.white),
                decoration: InputDecoration(
                  hintText: widget.searchHint,
                  hintStyle: AppFonts.inter(fontSize: 14, color: Colors.white30),
                  prefixIcon: const Icon(LucideIcons.search, color: Colors.white54),
                  filled: true,
                  fillColor: const Color(0xFF1E1E2E),
                  border: OutlineInputBorder(
                    borderRadius: BorderRadius.circular(8),
                    borderSide: BorderSide.none,
                  ),
                  contentPadding: const EdgeInsets.symmetric(
                    horizontal: 12,
                    vertical: 10,
                  ),
                ),
              ),
              const SizedBox(height: 8),
              Expanded(
                child: ListView.builder(
                  controller: scrollController,
                  itemCount: widget.groupedKeys.length,
                  itemBuilder: (context, index) {
                    final key = widget.groupedKeys[index];
                    final list = widget.grouped[key] ?? const [];
                    final visible = list.where((p) => _match(p, q)).toList();
                    if (visible.isEmpty) {
                      return const SizedBox.shrink();
                    }
                    return Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Padding(
                          padding: const EdgeInsets.symmetric(vertical: 8),
                          child: Text(
                            widget.groupTitle(key),
                            style: AppFonts.inter(
                              fontSize: 12,
                              fontWeight: FontWeight.w600,
                              color: Colors.white54,
                            ),
                          ),
                        ),
                        ...visible.map((p) {
                          final selected = p.name == widget.initialSelected;
                          return ListTile(
                            dense: true,
                            leading: Icon(
                              widget.iconForName(p.icon),
                              color: selected ? widget.accentColor : Colors.white70,
                              size: 20,
                            ),
                            title: Text(
                              p.displayName,
                              style: AppFonts.inter(
                                fontSize: 14,
                                color: Colors.white,
                              ),
                            ),
                            subtitle: Text(
                              p.name,
                              style: AppFonts.firaCode(
                                fontSize: 11,
                                color: Colors.white38,
                              ),
                            ),
                            trailing: selected
                                ? Icon(Icons.check, color: widget.accentColor)
                                : null,
                            onTap: () => widget.onPick(p),
                          );
                        }),
                      ],
                    );
                  },
                ),
              ),
            ],
          );
        },
      ),
    );
  }
}
