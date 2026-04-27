import 'package:flutter/material.dart';
import 'package:lucide_icons/lucide_icons.dart';

import '../l10n/app_localizations.dart';
import '../services/model_catalog_service.dart';
import '../utils/app_fonts.dart';

/// 模型 id 输入区：支持刷新远程/静态推荐列表与快捷 chip 选择。
class ModelIdPicker extends StatefulWidget {
  /// 创建模型 id 选择器。
  const ModelIdPicker({
    super.key,
    required this.controller,
    required this.providerId,
    required this.baseUrl,
    required this.apiKey,
    required this.label,
    required this.hint,
    required this.icon,
    this.useFiraCode = true,
    this.enabled = true,
  });

  /// 模型 id 文本。
  final TextEditingController controller;

  /// 当前 provider id（与 Go 元数据一致）。
  final String providerId;

  /// 读取当前 base URL。
  final String Function() baseUrl;

  /// 读取当前 API Key。
  final String Function() apiKey;

  /// 字段标签。
  final String label;

  /// 输入提示。
  final String hint;

  /// 前缀图标。
  final IconData icon;

  /// Bot 表单使用 Fira Code；安全模型表单使用 Inter。
  final bool useFiraCode;

  /// 是否可编辑。
  final bool enabled;

  @override
  State<ModelIdPicker> createState() => _ModelIdPickerState();
}

class _ModelIdPickerState extends State<ModelIdPicker> {
  List<String> _options = [];
  bool _loading = false;
  int _refreshSeq = 0;

  TextStyle get _fieldStyle => widget.useFiraCode
      ? AppFonts.firaCode(fontSize: 13, color: Colors.white)
      : AppFonts.inter(fontSize: 14, color: Colors.white);

  TextStyle get _hintStyle => widget.useFiraCode
      ? AppFonts.firaCode(fontSize: 13, color: Colors.white30)
      : AppFonts.inter(fontSize: 13, color: Colors.white30);

  Future<void> _onRefresh() async {
    final l10n = AppLocalizations.of(context)!;
    final seq = ++_refreshSeq;
    setState(() {
      _loading = true;
    });
    final result = await ModelCatalogService.instance.fetchModels(
      provider: widget.providerId,
      baseUrl: widget.baseUrl(),
      apiKey: widget.apiKey(),
    );
    if (!mounted || seq != _refreshSeq) {
      return;
    }
    setState(() {
      _loading = false;
      if (result.success) {
        _options = result.models;
      }
    });
    if (!result.success) {
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(
            l10n.modelConfigModelListFailed(result.error),
          ),
        ),
      );
      return;
    }
    if (result.message.isNotEmpty && result.models.isEmpty) {
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(result.message)),
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context)!;
    final labelWidget = Text(
      widget.label,
      style: AppFonts.inter(
        fontSize: widget.useFiraCode ? 12 : 12,
        fontWeight: FontWeight.w500,
        color: Colors.white70,
      ),
    );

    final field = TextField(
      controller: widget.controller,
      enabled: widget.enabled,
      style: _fieldStyle,
      decoration: widget.useFiraCode
          ? InputDecoration(
              hintText: widget.hint,
              hintStyle: _hintStyle,
              prefixIcon: Icon(widget.icon, color: Colors.white54, size: 18),
              suffixIcon: _loading
                  ? const Padding(
                      padding: EdgeInsets.all(12),
                      child: SizedBox(
                        width: 18,
                        height: 18,
                        child: CircularProgressIndicator(strokeWidth: 2),
                      ),
                    )
                  : IconButton(
                      tooltip: l10n.modelConfigRefreshModelList,
                      icon: const Icon(
                        LucideIcons.refreshCw,
                        color: Colors.white54,
                        size: 18,
                      ),
                      onPressed: widget.enabled ? _onRefresh : null,
                    ),
              filled: true,
              fillColor: const Color(0xFF1E1E2E),
              border: OutlineInputBorder(
                borderRadius: BorderRadius.circular(8),
                borderSide: BorderSide.none,
              ),
              focusedBorder: OutlineInputBorder(
                borderRadius: BorderRadius.circular(8),
                borderSide: const BorderSide(
                  color: Color(0xFF10B981),
                  width: 1.5,
                ),
              ),
              contentPadding: const EdgeInsets.symmetric(
                horizontal: 16,
                vertical: 14,
              ),
            )
          : InputDecoration(
              labelText: widget.label,
              labelStyle: AppFonts.inter(fontSize: 12, color: Colors.white54),
              hintText: widget.hint,
              hintStyle: _hintStyle,
              prefixIcon: Icon(widget.icon, color: Colors.white54, size: 18),
              suffixIcon: _loading
                  ? const Padding(
                      padding: EdgeInsets.all(12),
                      child: SizedBox(
                        width: 18,
                        height: 18,
                        child: CircularProgressIndicator(strokeWidth: 2),
                      ),
                    )
                  : IconButton(
                      tooltip: l10n.modelConfigRefreshModelList,
                      icon: const Icon(
                        LucideIcons.refreshCw,
                        color: Colors.white54,
                        size: 18,
                      ),
                      onPressed: widget.enabled ? _onRefresh : null,
                    ),
              filled: true,
              fillColor: const Color(0xFF1E1E2E),
              border: OutlineInputBorder(
                borderRadius: BorderRadius.circular(8),
                borderSide: BorderSide.none,
              ),
              focusedBorder: OutlineInputBorder(
                borderRadius: BorderRadius.circular(8),
                borderSide: const BorderSide(
                  color: Color(0xFF6366F1),
                  width: 1.5,
                ),
              ),
              contentPadding: const EdgeInsets.symmetric(
                horizontal: 16,
                vertical: 14,
              ),
            ),
    );

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        if (widget.useFiraCode) ...[
          labelWidget,
          const SizedBox(height: 6),
        ],
        field,
        if (_options.isNotEmpty) ...[
          const SizedBox(height: 8),
          Text(
            l10n.modelConfigUseRecommendedModels,
            style: AppFonts.inter(fontSize: 11, color: Colors.white38),
          ),
          const SizedBox(height: 6),
          SizedBox(
            height: 36,
            child: ListView(
              scrollDirection: Axis.horizontal,
              children: _options
                  .take(32)
                  .map(
                    (m) => Padding(
                      padding: const EdgeInsets.only(right: 8),
                      child: ActionChip(
                        label: Text(
                          m,
                          style: AppFonts.firaCode(
                            fontSize: 11,
                            color: Colors.white,
                          ),
                          overflow: TextOverflow.ellipsis,
                        ),
                        backgroundColor: const Color(0xFF1E1E2E),
                        onPressed: widget.enabled
                            ? () {
                                widget.controller.text = m;
                                widget.controller.selection =
                                    TextSelection.collapsed(offset: m.length);
                                setState(() {});
                              }
                            : null,
                      ),
                    ),
                  )
                  .toList(),
            ),
          ),
        ],
        const SizedBox(height: 8),
        Container(
          padding: const EdgeInsets.all(12),
          decoration: BoxDecoration(
            color: const Color(0xFF6366F1).withValues(alpha: 0.1),
            borderRadius: BorderRadius.circular(8),
            border: Border.all(
              color: const Color(0xFF6366F1).withValues(alpha: 0.3),
            ),
          ),
          child: Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const Icon(
                LucideIcons.info,
                color: Color(0xFF6366F1),
                size: 16,
              ),
              const SizedBox(width: 8),
              Expanded(
                child: Text(
                  l10n.modelConfigRefreshModelListRequirement,
                  style: AppFonts.inter(fontSize: 12, color: Colors.white70),
                ),
              ),
            ],
          ),
        ),
      ],
    );
  }
}
