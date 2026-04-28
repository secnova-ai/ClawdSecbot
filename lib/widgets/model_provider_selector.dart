import 'package:flutter/material.dart';
import 'package:lucide_icons/lucide_icons.dart';

import '../l10n/app_localizations.dart';
import '../services/provider_service.dart';
import '../utils/app_fonts.dart';

/// Provider selector: anchored searchable dropdown with grouped options.
class ModelProviderSelector extends StatefulWidget {
  /// Creates a provider selector.
  const ModelProviderSelector({
    super.key,
    required this.providers,
    required this.selectedName,
    required this.onProviderSelected,
    required this.iconForName,
    this.readOnly = false,
    this.accentColor = const Color(0xFF10B981),
  });

  /// Available providers.
  final List<ProviderInfo> providers;

  /// Current selected provider id.
  final String selectedName;

  /// Selection callback.
  final ValueChanged<ProviderInfo> onProviderSelected;

  /// Maps Go-provided icon names to Flutter icons.
  final IconData Function(String iconName) iconForName;

  /// Whether interactions are disabled.
  final bool readOnly;

  /// Selected state accent color.
  final Color accentColor;

  @override
  State<ModelProviderSelector> createState() => _ModelProviderSelectorState();
}

class _ModelProviderSelectorState extends State<ModelProviderSelector> {
  static const List<String> _groupOrder = [
    'recommended',
    'china',
    'global',
    'local',
    'compatible',
    '',
  ];

  final LayerLink _layerLink = LayerLink();
  final GlobalKey _fieldKey = GlobalKey();
  final TextEditingController _searchController = TextEditingController();
  final FocusNode _searchFocusNode = FocusNode();
  OverlayEntry? _overlayEntry;

  @override
  void initState() {
    super.initState();
    _searchController.addListener(() => _overlayEntry?.markNeedsBuild());
  }

  @override
  void didUpdateWidget(covariant ModelProviderSelector oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (widget.readOnly || widget.providers.isEmpty) {
      _closeDropdown();
      return;
    }
    _overlayEntry?.markNeedsBuild();
  }

  @override
  void dispose() {
    _closeDropdown(rebuild: false);
    _searchController.dispose();
    _searchFocusNode.dispose();
    super.dispose();
  }

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

  Map<String, List<ProviderInfo>> _groupProviders({String query = ''}) {
    final q = query.trim().toLowerCase();
    final map = <String, List<ProviderInfo>>{};
    for (final p in widget.providers) {
      if (q.isNotEmpty && !_matchesProvider(p, q)) {
        continue;
      }
      final g = p.group.isEmpty ? '' : p.group;
      map.putIfAbsent(g, () => []).add(p);
    }
    for (final e in map.values) {
      e.sort((a, b) => a.displayName.compareTo(b.displayName));
    }
    return map;
  }

  bool _matchesProvider(ProviderInfo p, String query) {
    final name = p.name.toLowerCase();
    final display = p.displayName.toLowerCase();
    final group = p.group.toLowerCase();
    return name.contains(query) ||
        display.contains(query) ||
        group.contains(query);
  }

  ProviderInfo? _selectedInfo() {
    for (final p in widget.providers) {
      if (p.name == widget.selectedName) {
        return p;
      }
    }
    return null;
  }

  double _fieldWidth() {
    final context = _fieldKey.currentContext;
    final box = context?.findRenderObject() as RenderBox?;
    return box?.size.width ?? 360;
  }

  void _toggleDropdown() {
    if (_overlayEntry == null) {
      _openDropdown();
      return;
    }
    _closeDropdown();
  }

  void _openDropdown() {
    if (widget.readOnly || widget.providers.isEmpty || _overlayEntry != null) {
      return;
    }
    _searchController.clear();
    _overlayEntry = OverlayEntry(builder: _buildDropdownOverlay);
    Overlay.of(context).insert(_overlayEntry!);
    setState(() {});
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted && _overlayEntry != null) {
        _searchFocusNode.requestFocus();
      }
    });
  }

  void _closeDropdown({bool rebuild = true}) {
    final entry = _overlayEntry;
    if (entry == null) {
      return;
    }
    _overlayEntry = null;
    entry.remove();
    if (mounted && rebuild) {
      setState(() {});
    }
  }

  void _selectProvider(ProviderInfo provider) {
    _closeDropdown();
    widget.onProviderSelected(provider);
  }

  Widget _buildDropdownOverlay(BuildContext overlayContext) {
    final l10n = AppLocalizations.of(context)!;
    final width = _fieldWidth();
    final grouped = _groupProviders(query: _searchController.text);
    final keys = grouped.keys.toList()
      ..sort((a, b) => _groupRank(a).compareTo(_groupRank(b)));

    return Stack(
      children: [
        Positioned.fill(
          child: GestureDetector(
            behavior: HitTestBehavior.translucent,
            onTap: _closeDropdown,
          ),
        ),
        CompositedTransformFollower(
          link: _layerLink,
          showWhenUnlinked: false,
          targetAnchor: Alignment.bottomLeft,
          followerAnchor: Alignment.topLeft,
          offset: const Offset(0, 8),
          child: Material(
            color: Colors.transparent,
            child: ConstrainedBox(
              constraints: BoxConstraints.tightFor(
                width: width,
              ).copyWith(maxHeight: 360),
              child: Container(
                decoration: BoxDecoration(
                  color: const Color(0xFF11111A),
                  borderRadius: BorderRadius.circular(10),
                  border: Border.all(
                    color: Colors.white.withValues(alpha: 0.1),
                  ),
                  boxShadow: [
                    BoxShadow(
                      color: Colors.black.withValues(alpha: 0.35),
                      blurRadius: 24,
                      offset: const Offset(0, 14),
                    ),
                  ],
                ),
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Padding(
                      padding: const EdgeInsets.all(10),
                      child: TextField(
                        controller: _searchController,
                        focusNode: _searchFocusNode,
                        style: AppFonts.inter(
                          fontSize: 14,
                          color: Colors.white,
                        ),
                        decoration: InputDecoration(
                          hintText: l10n.modelConfigSearchProvider,
                          hintStyle: AppFonts.inter(
                            fontSize: 14,
                            color: Colors.white30,
                          ),
                          prefixIcon: const Icon(
                            LucideIcons.search,
                            color: Colors.white54,
                            size: 18,
                          ),
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
                    ),
                    Flexible(
                      child: keys.isEmpty
                          ? _buildEmptyResult(l10n)
                          : ListView.builder(
                              padding: const EdgeInsets.only(
                                left: 8,
                                right: 8,
                                bottom: 8,
                              ),
                              shrinkWrap: true,
                              itemCount: keys.length,
                              itemBuilder: (context, index) {
                                final key = keys[index];
                                final list = grouped[key] ?? const [];
                                return _buildProviderGroup(l10n, key, list);
                              },
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
  }

  Widget _buildEmptyResult(AppLocalizations l10n) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 4, 16, 16),
      child: Text(
        '未找到匹配的供应商',
        style: AppFonts.inter(fontSize: 13, color: Colors.white38),
      ),
    );
  }

  Widget _buildProviderGroup(
    AppLocalizations l10n,
    String group,
    List<ProviderInfo> providers,
  ) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(8, 8, 8, 4),
          child: Text(
            _groupTitle(l10n, group),
            style: AppFonts.inter(
              fontSize: 12,
              fontWeight: FontWeight.w600,
              color: Colors.white54,
            ),
          ),
        ),
        ...providers.map(_buildProviderOption),
      ],
    );
  }

  Widget _buildProviderOption(ProviderInfo provider) {
    final selected = provider.name == widget.selectedName;
    return InkWell(
      borderRadius: BorderRadius.circular(8),
      onTap: () => _selectProvider(provider),
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 9),
        decoration: BoxDecoration(
          color: selected
              ? widget.accentColor.withValues(alpha: 0.12)
              : Colors.transparent,
          borderRadius: BorderRadius.circular(8),
        ),
        child: Row(
          children: [
            Icon(
              widget.iconForName(provider.icon),
              color: selected ? widget.accentColor : Colors.white70,
              size: 20,
            ),
            const SizedBox(width: 10),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                mainAxisSize: MainAxisSize.min,
                children: [
                  Text(
                    provider.displayName,
                    style: AppFonts.inter(fontSize: 14, color: Colors.white),
                    overflow: TextOverflow.ellipsis,
                  ),
                  const SizedBox(height: 2),
                  Text(
                    provider.name,
                    style: AppFonts.firaCode(
                      fontSize: 11,
                      color: Colors.white38,
                    ),
                    overflow: TextOverflow.ellipsis,
                  ),
                ],
              ),
            ),
            if (selected)
              Icon(Icons.check, color: widget.accentColor, size: 18),
          ],
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context)!;
    final selected = _selectedInfo();
    final label = selected?.displayName ?? widget.selectedName;
    final isOpen = _overlayEntry != null;

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
        CompositedTransformTarget(
          link: _layerLink,
          child: Material(
            key: _fieldKey,
            color: Colors.transparent,
            child: InkWell(
              onTap: widget.readOnly ? null : _toggleDropdown,
              borderRadius: BorderRadius.circular(8),
              child: AnimatedContainer(
                duration: const Duration(milliseconds: 160),
                padding: const EdgeInsets.symmetric(
                  horizontal: 12,
                  vertical: 10,
                ),
                decoration: BoxDecoration(
                  color: const Color(0xFF1E1E2E),
                  borderRadius: BorderRadius.circular(8),
                  border: Border.all(
                    color: isOpen
                        ? widget.accentColor
                        : Colors.white.withValues(alpha: 0.1),
                  ),
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
                        isOpen
                            ? LucideIcons.chevronUp
                            : LucideIcons.chevronDown,
                        size: 18,
                        color: Colors.white54,
                      ),
                  ],
                ),
              ),
            ),
          ),
        ),
      ],
    );
  }
}
