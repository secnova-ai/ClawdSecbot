import 'package:flutter/material.dart';
import 'package:lucide_icons/lucide_icons.dart';
import '../l10n/app_localizations.dart';
import '../models/protection_analysis_model.dart';
import '../models/truth_record_model.dart';
import '../utils/app_fonts.dart';

/// 中部工具区跨卡去重上下文，每张卡片一份。
/// 仅影响中部「工具参数」和「工具结果」两个 _buildToolSection 区块。
class _ToolDedupContext {
  /// 经跨卡去重后，本卡可展示的工具结果 ID 集合（仅含非空 ID）
  final Set<String> visibleResultIds;

  /// 是否隐藏中部工具参数区（上一张为「仅参数」且本卡有对应结果时）
  final bool hideToolArgs;

  /// 是否隐藏中部工具结果区（本卡为「仅工具参数」模式时）
  final bool hideToolResults;

  const _ToolDedupContext({
    this.visibleResultIds = const {},
    this.hideToolArgs = false,
    this.hideToolResults = false,
  });
}

class ProtectionMonitorLogPanel extends StatefulWidget {
  final List<LogEntry> logs;
  final bool useGroupedView;
  final Map<String, TruthRecordModel> requestGroups;
  final List<String> requestOrder;
  final String currentBotModelName;
  final ScrollController logScrollController;
  final ScrollController horizontalScrollController;
  final ValueChanged<bool> onViewModeChanged;
  final VoidCallback onClearLogs;
  final void Function(String text, AppLocalizations l10n) onCopyText;
  final VoidCallback onScrollToBottom;

  /// 日志面板是否处于放大状态(隐藏趋势图后扩展)
  final bool isExpanded;

  /// 切换日志面板放大/还原
  final VoidCallback? onToggleExpand;

  /// bot 资产配置的默认模型名,当日志未携带模型信息时作为兜底显示
  final String defaultModelName;

  const ProtectionMonitorLogPanel({
    super.key,
    required this.logs,
    required this.useGroupedView,
    required this.requestGroups,
    required this.requestOrder,
    this.currentBotModelName = '',
    required this.logScrollController,
    required this.horizontalScrollController,
    required this.onViewModeChanged,
    required this.onClearLogs,
    required this.onCopyText,
    required this.onScrollToBottom,
    this.isExpanded = false,
    this.onToggleExpand,
    this.defaultModelName = '',
  });

  @override
  State<ProtectionMonitorLogPanel> createState() =>
      _ProtectionMonitorLogPanelState();
}

class _ProtectionMonitorLogPanelState extends State<ProtectionMonitorLogPanel> {
  int? _hoverLogIndex;
  final Set<int> _selectedIndices = {};
  bool _isSelecting = false;
  int? _selectionStartIndex;

  @override
  void didUpdateWidget(ProtectionMonitorLogPanel oldWidget) {
    super.didUpdateWidget(oldWidget);
    // Clear row-level selection state when switching between grouped/raw views
    // to prevent stale blue left-border highlights from bleeding into the new view.
    if (oldWidget.useGroupedView != widget.useGroupedView) {
      _selectedIndices.clear();
      _isSelecting = false;
      _selectionStartIndex = null;
      _hoverLogIndex = null;
    }
  }

  String _joinSelectedLines() {
    final indices = _selectedIndices.toList()..sort();
    return indices.map((i) => widget.logs[i].text).join('\n');
  }

  String _formatTime(DateTime time) {
    final y = time.year.toString();
    final m = time.month.toString().padLeft(2, '0');
    final d = time.day.toString().padLeft(2, '0');
    final hh = time.hour.toString().padLeft(2, '0');
    final mm = time.minute.toString().padLeft(2, '0');
    final ss = time.second.toString().padLeft(2, '0');
    return '$y-$m-$d $hh:$mm:$ss';
  }

  /// 清洗卡片展示文本：去除 Sender 元数据前缀，或提取 `final` 标签区间。
  String _sanitizeCardContent(String text) {
    // 1. 优先处理 <final> ... </final> 结构
    String trimmed = text.trimLeft();
    if (trimmed.startsWith('<final>')) {
      final finalTagLength = '<final>'.length;
      final contentStart = trimmed.indexOf('<final>') + finalTagLength;

      final endTagIndex = trimmed.indexOf('</final>', contentStart);
      if (endTagIndex != -1) {
        final inner = trimmed.substring(contentStart, endTagIndex).trimLeft();
        return inner;
      } else {
        return trimmed.substring(contentStart).trimLeft();
      }
    }

    // 2. 处理 Sender (untrusted metadata): ... ```...``` ... 结构
    const senderPrefix = 'Sender (untrusted metadata):';
    if (trimmed.startsWith(senderPrefix)) {
      final fenceStart = trimmed.indexOf('```');
      if (fenceStart < 0) return text;
      final fenceEnd = trimmed.indexOf('```', fenceStart + 3);
      if (fenceEnd < 0) return text;
      final content = trimmed.substring(fenceEnd + 3).trimLeft();
      if (content.isNotEmpty) {
        return content;
      }
      return text;
    }

    // 3. 默认情况：去除前后空白后返回
    return text.trim();
  }

  // ==================== 资产特有消息预处理 ====================

  /// 按资产类型对原始消息内容做预处理，提取实际展示文本。
  /// 不同资产的消息格式差异在此集中处理，避免侵入通用逻辑。
  String _preprocessMessageForAsset(
    String content,
    String assetName,
    String role,
  ) {
    final lowerAsset = assetName.toLowerCase();

    if (lowerAsset.contains('dintalclaw')) {
      return _preprocessDinTalClaw(content, role);
    }
    if (lowerAsset.contains('qclaw')) {
      return _preprocessQClaw(content, role);
    }

    // 其他资产类型在此扩展，例如：
    // if (lowerAsset.contains('other_asset')) {
    //   return _preprocessOtherAsset(content, role);
    // }

    return content;
  }

  // ---- QClaw 消息预处理 ----

  String _preprocessQClaw(String content, String role) {
    if (role.toLowerCase() != 'user') {
      return content;
    }
    return _extractQClawUserBody(content) ?? content;
  }

  String? _extractQClawUserBody(String content) {
    const senderPrefix = 'Sender (untrusted metadata):';
    final trimmed = content.trimLeft();
    final senderIndex = trimmed.indexOf(senderPrefix);
    if (senderIndex < 0) {
      return null;
    }

    final senderBlock = trimmed.substring(senderIndex);
    final fenceStart = senderBlock.indexOf('```');
    if (fenceStart < 0) {
      return null;
    }
    final fenceEnd = senderBlock.indexOf('```', fenceStart + 3);
    if (fenceEnd < 0) {
      return null;
    }

    final body = senderBlock.substring(fenceEnd + 3).trimLeft();
    return body.isNotEmpty ? body : null;
  }

  // ---- DinTalClaw 消息预处理 ----

  static final _dintalclawSectionRe = RegExp(
    r'===\s*(SYSTEM|USER|ASSISTANT)\s*===',
    caseSensitive: false,
  );

  /// DinTalClaw 消息预处理：提取 "=== ROLE ===" 段落中对应角色的实际内容
  String _preprocessDinTalClaw(String content, String role) {
    String result;
    if (!_dintalclawSectionRe.hasMatch(content)) {
      result = content;
    } else {
      final section = role.toLowerCase() == 'assistant' ? 'ASSISTANT' : 'USER';
      result = _extractDinTalClawSection(content, section) ?? content;
    }
    if (role.toLowerCase() == 'user') {
      result = _stripDinTalClawProtocolNoise(result);
    }
    return result;
  }

  /// 去除 DinTalClaw 用户消息中的协议噪声内容
  static final _workingMemoryBlockRe = RegExp(
    r'###\s*\[WORKING MEMORY\][\s\S]*?</history>',
    caseSensitive: false,
  );
  static final _historyBlockRe = RegExp(
    r'<history>[\s\S]*?</history>',
    caseSensitive: false,
  );
  static final _workingMemoryTrailingRe = RegExp(
    r'###\s*\[WORKING MEMORY\][\s\S]*$',
  );
  static final _systemLineRe = RegExp(r'^[ \t]*\[System\].*$', multiLine: true);
  static final _protocolViolationLineRe = RegExp(
    r'^[ \t]*PROTOCOL_VIOLATION.*$',
    multiLine: true,
  );
  static final _excessiveNewlinesRe = RegExp(r'\n{3,}');

  String _stripDinTalClawProtocolNoise(String content) {
    var result = content;
    result = result.replaceAll(_workingMemoryBlockRe, '');
    result = result.replaceAll(_historyBlockRe, '');
    result = result.replaceAll(_workingMemoryTrailingRe, '');
    result = result.replaceAll(_systemLineRe, '');
    result = result.replaceAll(_protocolViolationLineRe, '');
    result = result.replaceAll(_excessiveNewlinesRe, '\n\n');
    return result.trim();
  }

  /// 从 DinTalClaw "=== SECTION ===" 格式中提取指定段内容
  String? _extractDinTalClawSection(String text, String section) {
    final startRe = RegExp('===\\s*$section\\s*===', caseSensitive: false);
    final startMatch = startRe.firstMatch(text);
    if (startMatch == null) return null;
    final contentStart = startMatch.end;
    final nextMatch = _dintalclawSectionRe.firstMatch(
      text.substring(contentStart),
    );
    final body = nextMatch != null
        ? text.substring(contentStart, contentStart + nextMatch.start)
        : text.substring(contentStart);
    final trimmed = body.trim();
    return trimmed.isNotEmpty ? trimmed : null;
  }

  String _formatRoleLabel(String role) {
    switch (role.trim().toLowerCase()) {
      case 'user':
        return 'User';
      case 'assistant':
        return 'Assistant';
      case 'tool':
        return 'Tool';
      case 'tool_request':
        return 'Tool Call';
      case 'tool_result':
        return 'Tool Result';
      case 'system':
        return 'System';
      default:
        final trimmed = role.trim();
        if (trimmed.isEmpty) return '';
        return trimmed[0].toUpperCase() + trimmed.substring(1);
    }
  }

  /// 从消息内容中提取 `<summary>...</summary>` 标签中的摘要文本
  String? _extractSummary(String content) {
    final pattern = RegExp(r'<summary>\s*([\s\S]*?)\s*</summary>');
    final match = pattern.firstMatch(content);
    return match?.group(1)?.trim();
  }

  /// 去除消息内容中的 `<thinking>` / `<summary>` / `<tool_use>` / `<tool_result>` 等元标签，保留可读正文
  String _stripMetaTags(String content) {
    var result = content;
    result = result.replaceAll(
      RegExp(r'<thinking>[\s\S]*?</thinking>', caseSensitive: false),
      '',
    );
    result = result.replaceAll(
      RegExp(r'<summary>[\s\S]*?</summary>', caseSensitive: false),
      '',
    );
    result = result.replaceAll(
      RegExp(r'<tool_use>[\s\S]*?</tool_use>', caseSensitive: false),
      '',
    );
    result = result.replaceAll(
      RegExp(r'<tool_result>[\s\S]*?</tool_result>', caseSensitive: false),
      '',
    );
    return result.trim();
  }

  /// 将 Markdown 风格的列表内容（- **Field**: Value）解析为结构化字段列表
  List<({String field, String value})> _parseStructuredFields(String content) {
    final fields = <({String field, String value})>[];
    final lines = content.split('\n');
    for (final line in lines) {
      final match = RegExp(
        r'^[-*]\s+\*\*(.+?)\*\*\s*[:：]\s*(.+)$',
      ).firstMatch(line.trim());
      if (match != null) {
        fields.add((
          field: match.group(1)!.trim(),
          value: match.group(2)!.trim(),
        ));
      }
    }
    return fields;
  }

  bool _isZh(AppLocalizations l10n) => l10n.localeName.startsWith('zh');

  String _cardText(AppLocalizations l10n, String key) {
    final zh = _isZh(l10n);
    switch (key) {
      case 'grouped':
        return zh ? '\u5206\u7ec4\u89c6\u56fe' : 'Grouped';
      case 'raw':
        return zh ? '\u539f\u59cb\u89c6\u56fe' : 'Raw';
      case 'copySelected':
        return zh ? '\u590d\u5236\u9009\u4e2d' : 'Copy Selected';
      case 'copyAll':
        return zh ? '\u590d\u5236\u5168\u90e8' : 'Copy All';
      case 'copyLine':
        return zh ? '\u590d\u5236\u672c\u884c' : 'Copy Line';
      case 'phase':
        return zh ? '\u9636\u6bb5' : 'Phase';
      case 'complete':
        return zh ? '\u5df2\u5b8c\u6210' : 'Complete';
      case 'summary':
        return zh ? '\u5bf9\u8bdd\u6458\u8981' : 'Conversation Summary';
      case 'toolArgs':
        return zh ? '\u5de5\u5177\u53c2\u6570' : 'Tool Arguments';
      case 'tokens':
        return 'Tokens';
      case 'toolCalls':
        return zh ? '\u5de5\u5177\u8c03\u7528' : 'Tool Calls';
      case 'securityCheck':
        return zh ? '\u5b89\u5168\u68c0\u67e5' : 'Security Check';
      case 'noText':
        return zh
            ? '\u672c\u8f6e\u53ea\u751f\u6210\u4e86\u5de5\u5177\u8c03\u7528\uff0c\u6ca1\u6709\u81ea\u7136\u8bed\u8a00\u6587\u672c\u3002'
            : 'This turn only generated tool calls, with no natural-language text.';
      case 'unavailable':
        return zh
            ? '\u5185\u5bb9\u4e0d\u53ef\u7528\u3002'
            : 'Content unavailable.';
      case 'previewOnly':
        return zh ? '\u4ec5\u5c55\u793a\u9884\u89c8' : 'Preview only';
      case 'captured':
        return zh ? '\u5df2\u91c7\u96c6' : 'Captured';
      case 'chars':
        return zh ? '\u5b57\u7b26' : 'chars';
      case 'toolRequest':
        return zh ? '\u8bf7\u6c42\u5de5\u5177' : 'Requested tools';
      case 'toolArgsInline':
        return zh ? '\u53c2\u6570' : 'Args';
      case 'assistantResponse':
        return zh ? '\u52a9\u624b\u56de\u590d' : 'Assistant Response';
      case 'toolResult':
        return zh ? '\u5de5\u5177\u7ed3\u679c' : 'Tool Result';
      case 'requestUseTool':
        return zh ? '\u8bf7\u6c42\u4f7f\u7528\u5de5\u5177' : 'Requesting tools';
      case 'securityWarning':
        return zh ? '\u5b89\u5168\u544a\u8b66' : 'Security Warning';
      case 'noTextTitle':
        return zh ? '\u4ec5\u5de5\u5177\u8c03\u7528' : 'Tool Calls Only';
      case 'content':
        return zh ? '\u5185\u5bb9' : 'Content';
      default:
        return key;
    }
  }

  Widget _buildSectionTitle(
    String title, {
    required IconData icon,
    Color? color,
  }) {
    final accent = color ?? Colors.white70;
    return Row(
      children: [
        Icon(icon, size: 13, color: accent),
        const SizedBox(width: 6),
        Text(
          title,
          style: AppFonts.inter(
            fontSize: 11,
            fontWeight: FontWeight.w600,
            color: accent,
          ),
        ),
      ],
    );
  }

  /// 构建工具参数/工具结果区块（复用结构）。
  Widget _buildToolSection(
    String title,
    List<String> items,
    IconData icon,
    Color color,
  ) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const SizedBox(height: 12),
        _buildSectionTitle(title, icon: icon, color: color),
        const SizedBox(height: 8),
        Container(
          width: double.infinity,
          padding: const EdgeInsets.all(10),
          decoration: BoxDecoration(
            color: Colors.white.withValues(alpha: 0.03),
            borderRadius: BorderRadius.circular(8),
            border: Border.all(color: color.withValues(alpha: 0.18)),
          ),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: items.map((item) {
              return Padding(
                padding: const EdgeInsets.only(bottom: 6),
                child: Text(
                  item,
                  style: AppFonts.firaCode(fontSize: 10, color: Colors.white70),
                  maxLines: 3,
                  overflow: TextOverflow.ellipsis,
                ),
              );
            }).toList(),
          ),
        ),
      ],
    );
  }

  Widget _buildMetaChip({
    required IconData icon,
    required Color color,
    required String label,
    String? value,
  }) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.14),
        borderRadius: BorderRadius.circular(6),
        border: Border.all(color: color.withValues(alpha: 0.3)),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(icon, size: 14, color: color),
          const SizedBox(width: 8),
          Text(
            label,
            style: AppFonts.inter(
              fontSize: 11,
              fontWeight: FontWeight.w600,
              color: color,
            ),
          ),
          if (value != null && value.isNotEmpty) ...[
            const SizedBox(width: 8),
            ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 220),
              child: Text(
                value,
                style: AppFonts.inter(fontSize: 11, color: Colors.white70),
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
              ),
            ),
          ],
        ],
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context)!;

    return RepaintBoundary(
      child: Container(
        decoration: BoxDecoration(
          color: Colors.white.withValues(alpha: 0.05),
          borderRadius: BorderRadius.circular(12),
          border: Border.all(color: Colors.white.withValues(alpha: 0.1)),
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            _buildHeader(l10n),
            Divider(height: 1, color: Colors.white.withValues(alpha: 0.1)),
            Expanded(child: _buildContent(l10n)),
          ],
        ),
      ),
    );
  }

  Widget _buildHeader(AppLocalizations l10n) {
    return GestureDetector(
      onDoubleTap: widget.onToggleExpand,
      child: Padding(
        padding: const EdgeInsets.all(12),
        child: Row(
          children: [
            const Icon(
              LucideIcons.terminal,
              color: Color(0xFF6366F1),
              size: 16,
            ),
            const SizedBox(width: 8),
            Text(
              l10n.analysisLogs,
              style: AppFonts.inter(
                fontSize: 14,
                fontWeight: FontWeight.w600,
                color: Colors.white,
              ),
            ),
            const Spacer(),
            _buildViewToggle(l10n),
            const SizedBox(width: 8),
            if (widget.onToggleExpand != null)
              IconButton(
                icon: Icon(
                  widget.isExpanded
                      ? LucideIcons.minimize2
                      : LucideIcons.maximize2,
                  size: 14,
                ),
                color: Colors.white54,
                tooltip: widget.isExpanded ? 'Restore' : 'Expand',
                onPressed: widget.onToggleExpand,
                constraints: const BoxConstraints(minWidth: 28, minHeight: 28),
                padding: EdgeInsets.zero,
                iconSize: 14,
              ),
            IconButton(
              icon: const Icon(LucideIcons.copy, size: 16),
              color: _selectedIndices.isNotEmpty
                  ? Colors.white54
                  : Colors.white24,
              tooltip: _cardText(l10n, 'copySelected'),
              onPressed: _selectedIndices.isNotEmpty
                  ? () {
                      final text = _joinSelectedLines();
                      widget.onCopyText(text, l10n);
                    }
                  : null,
            ),
            IconButton(
              icon: const Icon(LucideIcons.copy, size: 16),
              color: Colors.white54,
              tooltip: _cardText(l10n, 'copyAll'),
              onPressed: () {
                final allText = widget.logs.map((e) => e.text).join('\n');
                widget.onCopyText(allText, l10n);
              },
            ),
            MouseRegion(
              cursor: SystemMouseCursors.click,
              child: GestureDetector(
                onTap: widget.onClearLogs,
                child: Container(
                  padding: const EdgeInsets.symmetric(
                    horizontal: 8,
                    vertical: 4,
                  ),
                  decoration: BoxDecoration(
                    color: Colors.white.withValues(alpha: 0.1),
                    borderRadius: BorderRadius.circular(4),
                  ),
                  child: Text(
                    l10n.clear,
                    style: AppFonts.inter(fontSize: 11, color: Colors.white54),
                  ),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildViewToggle(AppLocalizations l10n) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
      decoration: BoxDecoration(
        color: Colors.white.withValues(alpha: 0.08),
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: Colors.white.withValues(alpha: 0.1)),
      ),
      child: Row(
        children: [
          GestureDetector(
            onTap: () {
              widget.onViewModeChanged(true);
            },
            child: Container(
              padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
              decoration: BoxDecoration(
                color: widget.useGroupedView
                    ? const Color(0xFF6366F1).withValues(alpha: 0.3)
                    : Colors.transparent,
                borderRadius: BorderRadius.circular(12),
              ),
              child: Text(
                _cardText(l10n, 'grouped'),
                style: AppFonts.inter(fontSize: 11, color: Colors.white),
              ),
            ),
          ),
          const SizedBox(width: 4),
          GestureDetector(
            onTap: () {
              widget.onViewModeChanged(false);
              widget.onScrollToBottom();
            },
            child: Container(
              padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
              decoration: BoxDecoration(
                color: !widget.useGroupedView
                    ? const Color(0xFF6366F1).withValues(alpha: 0.3)
                    : Colors.transparent,
                borderRadius: BorderRadius.circular(12),
              ),
              child: Text(
                _cardText(l10n, 'raw'),
                style: AppFonts.inter(fontSize: 11, color: Colors.white),
              ),
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildContent(AppLocalizations l10n) {
    if (widget.useGroupedView) {
      final hasGroupedEntries = widget.requestOrder.any(
        (reqId) => widget.requestGroups.containsKey(reqId),
      );
      if (!hasGroupedEntries) {
        return Center(
          child: Column(
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              Icon(LucideIcons.scrollText, color: Colors.white24, size: 32),
              const SizedBox(height: 8),
              Text(
                l10n.waitingLogs,
                style: AppFonts.inter(fontSize: 12, color: Colors.white38),
              ),
            ],
          ),
        );
      }
    } else if (widget.logs.isEmpty) {
      return Center(
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Icon(LucideIcons.scrollText, color: Colors.white24, size: 32),
            const SizedBox(height: 8),
            Text(
              l10n.waitingLogs,
              style: AppFonts.inter(fontSize: 12, color: Colors.white38),
            ),
          ],
        ),
      );
    }

    return ScrollbarTheme(
      data: ScrollbarThemeData(
        thumbColor: WidgetStateProperty.all(
          Colors.white.withValues(alpha: 0.3),
        ),
        trackColor: WidgetStateProperty.all(
          Colors.white.withValues(alpha: 0.05),
        ),
        trackBorderColor: WidgetStateProperty.all(
          Colors.white.withValues(alpha: 0.08),
        ),
        thickness: WidgetStateProperty.all(8.0),
        radius: const Radius.circular(4),
      ),
      child: Scrollbar(
        controller: widget.logScrollController,
        thumbVisibility: true,
        trackVisibility: true,
        child: widget.useGroupedView
            ? _buildGroupedView(l10n)
            : _buildRawView(l10n),
      ),
    );
  }

  /// 中部工具区跨卡去重预处理：遍历所有卡片，计算每张卡片的工具区展示策略。
  /// 不影响对话摘要、meta chip 等其它区域。
  Map<String, _ToolDedupContext> _computeToolDedupContexts() {
    final contexts = <String, _ToolDedupContext>{};
    final shownResultIds = <String>{};
    bool prevIsToolArgsOnly = false;
    Set<String> prevResponseToolIds = {};

    for (final reqId in widget.requestOrder) {
      final group = widget.requestGroups[reqId];
      if (group == null) continue;
      final isDinTalClaw = group.assetName.toLowerCase().contains('dintalclaw');

      final responseToolCalls = group.toolCalls
          .where((tc) => tc.source == 'response')
          .toList();
      final latestHistoryResults = group.toolCalls
          .where(
            (tc) =>
                tc.source == 'history' &&
                tc.latestRound &&
                tc.result.isNotEmpty,
          )
          .toList();

      // DinTalClaw 使用内嵌工具协议，工具调用 ID 在多轮消息中更容易出现同形态序列。
      // 为避免“跨卡去重”误杀有效内容，这里直接按原始结果全量展示，不做隐藏策略。
      if (isDinTalClaw) {
        contexts[reqId] = _ToolDedupContext(
          visibleResultIds: latestHistoryResults
              .map((tc) => tc.id)
              .where((id) => id.isNotEmpty)
              .toSet(),
          hideToolArgs: false,
          hideToolResults: false,
        );
        prevIsToolArgsOnly = false;
        prevResponseToolIds = {};
        continue;
      }

      // 跨卡去重：仅对非空 ID 的工具结果做去重；空 ID 不参与去重以避免误过滤
      final visibleResultIds = <String>{};
      for (final tc in latestHistoryResults) {
        if (tc.id.isEmpty) continue;
        if (!shownResultIds.contains(tc.id)) {
          visibleResultIds.add(tc.id);
        }
      }

      final hasVisibleResults = latestHistoryResults.any(
        (tc) => tc.id.isEmpty || visibleResultIds.contains(tc.id),
      );

      // 上一张为「仅参数」且本卡有对应结果时，隐藏本卡中部工具参数区
      bool hideToolArgs = false;
      if (prevIsToolArgsOnly &&
          prevResponseToolIds.isNotEmpty &&
          hasVisibleResults) {
        final hasMatchingResults = latestHistoryResults.any(
          (tc) => tc.id.isNotEmpty && prevResponseToolIds.contains(tc.id),
        );
        hideToolArgs = hasMatchingResults;
      }

      // 本卡为「仅工具参数」≈ 无文本响应 + 有 response 工具调用
      final isToolArgsOnly =
          group.primaryContentType == 'no_text_response' &&
          responseToolCalls.isNotEmpty;

      contexts[reqId] = _ToolDedupContext(
        visibleResultIds: visibleResultIds,
        hideToolArgs: hideToolArgs,
        hideToolResults: isToolArgsOnly,
      );

      for (final tc in latestHistoryResults) {
        if (tc.id.isNotEmpty && visibleResultIds.contains(tc.id)) {
          shownResultIds.add(tc.id);
        }
      }

      prevIsToolArgsOnly = isToolArgsOnly;
      prevResponseToolIds = responseToolCalls
          .map((tc) => tc.id)
          .where((id) => id.isNotEmpty)
          .toSet();
    }

    return contexts;
  }

  Widget _buildGroupedView(AppLocalizations l10n) {
    final toolDedupContexts = _computeToolDedupContexts();
    return ListView.builder(
      controller: widget.logScrollController,
      padding: const EdgeInsets.all(12),
      itemCount: widget.requestOrder.length,
      itemBuilder: (context, index) {
        final reqId = widget.requestOrder[index];
        final group = widget.requestGroups[reqId];
        if (group == null) {
          return const SizedBox.shrink();
        }
        return _buildGroupedCard(group, l10n, toolDedupContexts[reqId]);
      },
    );
  }

  Widget _buildRawView(AppLocalizations l10n) {
    return ListView.builder(
      controller: widget.logScrollController,
      padding: const EdgeInsets.all(12),
      itemCount: widget.logs.length,
      cacheExtent: 500.0,
      itemBuilder: (context, index) {
        final logEntry = widget.logs[index];
        final isSelected = _selectedIndices.contains(index);
        return MouseRegion(
          onEnter: (_) => setState(() {
            _hoverLogIndex = index;
            if (_isSelecting && _selectionStartIndex != null) {
              final start = _selectionStartIndex!;
              final end = index;
              _selectedIndices
                ..clear()
                ..addAll(
                  List<int>.generate(
                    (end - start).abs() + 1,
                    (i) => start < end ? start + i : start - i,
                  ),
                );
            }
          }),
          onExit: (_) => setState(() {
            _hoverLogIndex = null;
          }),
          child: GestureDetector(
            onPanStart: (_) {
              setState(() {
                _isSelecting = true;
                _selectionStartIndex = index;
                _selectedIndices
                  ..clear()
                  ..add(index);
              });
            },
            onPanEnd: (_) {
              setState(() {
                _isSelecting = false;
                _selectionStartIndex = null;
              });
            },
            onTap: () {
              setState(() {
                _isSelecting = false;
                _selectionStartIndex = null;
                _selectedIndices.clear();
                _hoverLogIndex = null;
              });
            },
            onSecondaryTapDown: (details) async {
              final pos = details.globalPosition;
              final selected = await showMenu<String>(
                context: context,
                position: RelativeRect.fromLTRB(pos.dx, pos.dy, pos.dx, pos.dy),
                items: [
                  PopupMenuItem(
                    value: 'copy_line',
                    child: Text(_cardText(l10n, 'copyLine')),
                  ),
                  if (_selectedIndices.isNotEmpty)
                    PopupMenuItem(
                      value: 'copy_selected',
                      child: Text(_cardText(l10n, 'copySelected')),
                    ),
                  PopupMenuItem(
                    value: 'copy_all',
                    child: Text(_cardText(l10n, 'copyAll')),
                  ),
                ],
              );
              if (selected == 'copy_line') {
                widget.onCopyText(logEntry.text, l10n);
              } else if (selected == 'copy_selected') {
                final text = _joinSelectedLines();
                widget.onCopyText(text, l10n);
              } else if (selected == 'copy_all') {
                final allText = widget.logs.map((e) => e.text).join('\n');
                widget.onCopyText(allText, l10n);
              }
            },
            child: Padding(
              padding: const EdgeInsets.symmetric(vertical: 1),
              child: Row(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Expanded(
                    child: Container(
                      decoration: BoxDecoration(
                        color: isSelected
                            ? const Color(0xFF6366F1).withValues(alpha: 0.22)
                            : Colors.transparent,
                        border: isSelected
                            ? const Border(
                                left: BorderSide(
                                  color: Color(0xFF6366F1),
                                  width: 3,
                                ),
                              )
                            : null,
                      ),
                      child: Text(
                        logEntry.text,
                        style: AppFonts.firaCode(
                          fontSize: 12,
                          height: 1.4,
                          color: Color(logEntry.color),
                        ),
                        softWrap: true,
                      ),
                    ),
                  ),
                  SizedBox(
                    width: 22,
                    child: Opacity(
                      opacity: _hoverLogIndex == index ? 1.0 : 0.0,
                      child: IgnorePointer(
                        ignoring: _hoverLogIndex != index,
                        child: IconButton(
                          icon: const Icon(LucideIcons.copy, size: 14),
                          color: Colors.white38,
                          tooltip: l10n.appStoreGuideCopy,
                          padding: EdgeInsets.zero,
                          constraints: const BoxConstraints.tightFor(
                            width: 20,
                            height: 20,
                          ),
                          visualDensity: VisualDensity.compact,
                          onPressed: () =>
                              widget.onCopyText(logEntry.text, l10n),
                        ),
                      ),
                    ),
                  ),
                ],
              ),
            ),
          ),
        );
      },
    );
  }

  /// 构建分组请求卡片 (TruthRecord 快照)
  Widget _buildGroupedCard(
    TruthRecordModel group,
    AppLocalizations l10n, [
    _ToolDedupContext? toolDedup,
  ]) {
    final subtitleModel = group.model.isNotEmpty
        ? group.model
        : (widget.currentBotModelName.isNotEmpty
              ? widget.currentBotModelName
              : (widget.defaultModelName.isNotEmpty
                    ? widget.defaultModelName
                    : '-'));
    final assetLabel = group.assetName.isNotEmpty
        ? group.assetName
        : (group.assetID.isNotEmpty ? group.assetID : 'Proxy');
    final ts = _formatTime(group.startedAt);
    final contentText = group.primaryContent.isNotEmpty
        ? _sanitizeCardContent(group.primaryContent)
        : (group.primaryContentType == 'no_text_response'
              ? _cardText(l10n, 'noText')
              : (group.primaryContentType == 'unavailable'
                    ? _cardText(l10n, 'unavailable')
                    : ''));
    final hasContent = contentText.isNotEmpty;
    final tokenStr = group.totalTokens > 0
        ? '${group.promptTokens} / ${group.completionTokens} / ${group.totalTokens}'
        : '';

    // === 对话摘要：展示 user/assistant/tool 的有效文本 ===
    var displayMessages = group.messages.where((m) {
      final role = m.role.toLowerCase();
      if (role == 'system') return false;
      if (role == 'tool' && m.content.trim().isEmpty) return false;
      if (role == 'assistant' && m.content.trim().isEmpty) return false;
      return true;
    }).toList();
    final lastUserIdx = displayMessages.lastIndexWhere(
      (m) => m.role.toLowerCase() == 'user',
    );
    if (lastUserIdx >= 0) {
      displayMessages = displayMessages.sublist(lastUserIdx);
    }
    const maxSummaryMessages = 7;
    final visibleMessages = displayMessages.length <= maxSummaryMessages
        ? displayMessages
        : displayMessages.sublist(displayMessages.length - maxSummaryMessages);

    // === 工具区域 ===
    final responseToolCalls = group.toolCalls
        .where((tc) => tc.source == 'response')
        .toList();

    final assetName = group.assetName;
    final lowerAssetName = assetName.toLowerCase();

    final summaryItems =
        <
          ({
            String role,
            String content,
            List<({String field, String value})> fields,
          })
        >[];
    for (final message in visibleMessages) {
      final rawContent = message.content.replaceAll(RegExp(r'^\[.*?\]\s*'), '');
      final role = message.role.toLowerCase();
      // 资产特有预处理（DinTalClaw 提取段落标记、其他资产可扩展）
      final preprocessed = _preprocessMessageForAsset(
        rawContent,
        assetName,
        role,
      );

      if (role == 'user') {
        final stripped = _stripMetaTags(preprocessed);
        final cleaned = _sanitizeCardContent(stripped).trim();
        if (cleaned.isNotEmpty) {
          summaryItems.add((role: 'user', content: cleaned, fields: const []));
        }
      } else if (role == 'assistant') {
        final summaryText = _extractSummary(preprocessed);
        final body = _stripMetaTags(preprocessed);
        final sanitized = _sanitizeCardContent(body);
        final fields = _parseStructuredFields(sanitized);
        final displayText = summaryText ?? sanitized;
        final bodyWithoutFields = fields.isNotEmpty
            ? sanitized
                  .split('\n')
                  .where((line) {
                    return !RegExp(
                      r'^[-*]\s+\*\*.+?\*\*\s*[:：]',
                    ).hasMatch(line.trim());
                  })
                  .join('\n')
                  .trim()
            : '';
        final finalContent = summaryText != null && bodyWithoutFields.isNotEmpty
            ? '$displayText\n$bodyWithoutFields'
            : displayText;
        if (finalContent.trim().isNotEmpty) {
          summaryItems.add((
            role: 'assistant',
            content: finalContent,
            fields: fields,
          ));
        }
      } else if (role == 'tool') {
        final cleaned = _sanitizeCardContent(rawContent).trim();
        summaryItems.add((
          role: 'tool_result',
          content: cleaned,
          fields: const [],
        ));
      } else {
        final cleaned = _sanitizeCardContent(rawContent).trim();
        summaryItems.add((
          role: message.role,
          content: cleaned,
          fields: const [],
        ));
      }
    }

    // 兜底：若摘要中无 assistant 消息但 primaryContent 有值，补入摘要
    if (hasContent &&
        group.primaryContentType.trim().toLowerCase() == 'assistant_response' &&
        !summaryItems.any(
          (item) =>
              item.role.toLowerCase() == 'assistant' &&
              item.content.trim().isNotEmpty,
        )) {
      final fields = _parseStructuredFields(contentText);
      summaryItems.add((
        role: 'assistant',
        content: contentText,
        fields: fields,
      ));
    }

    // 有工具调用时在摘要中插入 tool_request 条目（排除内嵌协议的合成条目，它们仅在工具区展示）
    if (responseToolCalls.isNotEmpty) {
      final standardToolCalls = responseToolCalls
          .where((tc) => !tc.id.startsWith('inline_'))
          .toList();
      if (standardToolCalls.isNotEmpty) {
        final toolEntries = standardToolCalls.map((tc) {
          final argsSummary = tc.arguments.isNotEmpty
              ? _sanitizeCardContent(tc.arguments)
              : '';
          final display = argsSummary.isNotEmpty
              ? '${tc.name}: $argsSummary'
              : tc.name;
          return (
            role: 'tool_request',
            content: display,
            fields: const <({String field, String value})>[],
          );
        }).toList();
        summaryItems.addAll(toolEntries);
      }
    }

    // 资产特有过滤：对已在专用工具区域展示参数/结果的资产，
    // 摘要区不再重复展示 tool_request / tool_result。
    if (lowerAssetName.contains('dintalclaw') ||
        lowerAssetName.contains('qclaw')) {
      summaryItems.removeWhere(
        (item) => item.role == 'tool_request' || item.role == 'tool_result',
      );
    }
    final toolArgs = responseToolCalls
        .where((tc) => tc.arguments.isNotEmpty)
        .map((tc) => '${tc.name}: ${_sanitizeCardContent(tc.arguments)}')
        .take(5)
        .toList();
    // 工具结果：来自 bot 发送的最新一轮工具执行结果（source=history, latest_round=true）
    final latestHistoryResults = group.toolCalls
        .where(
          (tc) =>
              tc.source == 'history' && tc.latestRound && tc.result.isNotEmpty,
        )
        .toList();
    // 中部工具结果经跨卡去重：空 ID 始终展示，非空 ID 仅在首次出现时展示
    final dedupedResults = toolDedup != null
        ? latestHistoryResults
              .where(
                (tc) =>
                    tc.id.isEmpty || toolDedup.visibleResultIds.contains(tc.id),
              )
              .toList()
        : latestHistoryResults;
    final toolResults = dedupedResults
        .map((tc) => '${tc.name}: ${_sanitizeCardContent(tc.result)}')
        .take(5)
        .toList();
    // 底部 meta 仍使用未去重的完整列表（保持其它区域不变）
    final currentToolCalls = [...responseToolCalls, ...latestHistoryResults];

    final Color borderColor;
    if (group.decisionBlocked) {
      borderColor = const Color(0xFFEF4444).withValues(alpha: 0.4);
    } else if (group.decisionStatus.isNotEmpty &&
        !_isNormalDecision(group.decisionStatus)) {
      borderColor = const Color(0xFFF59E0B).withValues(alpha: 0.3);
    } else {
      borderColor = Colors.white.withValues(alpha: 0.08);
    }
    final (statusText, statusColor) = _statusPresentationForView(group);

    return Container(
      margin: const EdgeInsets.symmetric(vertical: 8),
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: Colors.black.withValues(alpha: 0.3),
        borderRadius: BorderRadius.circular(8),
        border: Border.all(color: borderColor),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const Padding(
                padding: EdgeInsets.only(top: 2),
                child: Icon(
                  LucideIcons.workflow,
                  color: Color(0xFF6366F1),
                  size: 16,
                ),
              ),
              const SizedBox(width: 8),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    if (!group.requestId.startsWith('req')) ...[
                      Text(
                        group.requestId,
                        style: AppFonts.firaCode(
                          fontSize: 12,
                          color: Colors.white,
                        ),
                      ),
                      const SizedBox(height: 2),
                    ],
                    Text(
                      '$assetLabel / $subtitleModel',
                      style: AppFonts.inter(
                        fontSize: 11,
                        color: Colors.white70,
                      ),
                    ),
                    const SizedBox(height: 2),
                    Row(
                      children: [
                        const Icon(
                          LucideIcons.clock,
                          size: 12,
                          color: Colors.white54,
                        ),
                        const SizedBox(width: 6),
                        Text(
                          ts,
                          style: AppFonts.firaCode(
                            fontSize: 10,
                            color: Colors.white70,
                          ),
                        ),
                        const SizedBox(width: 8),
                        _buildInlineStatusChip(statusText, statusColor),
                      ],
                    ),
                  ],
                ),
              ),
            ],
          ),
          const SizedBox(height: 10),
          if (summaryItems.isNotEmpty) ...[
            const SizedBox(height: 12),
            _buildSectionTitle(
              _cardText(l10n, 'summary'),
              icon: LucideIcons.messagesSquare,
            ),
            const SizedBox(height: 8),
            Container(
              width: double.infinity,
              padding: const EdgeInsets.fromLTRB(10, 10, 10, 6),
              decoration: BoxDecoration(
                color: Colors.white.withValues(alpha: 0.03),
                borderRadius: BorderRadius.circular(8),
                border: Border.all(color: Colors.white.withValues(alpha: 0.06)),
              ),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: summaryItems.map((item) {
                  final role = item.role.toLowerCase();
                  final Color roleColor;
                  if (role == 'user') {
                    roleColor = const Color(0xFF22C55E);
                  } else if (role == 'assistant') {
                    roleColor = const Color(0xFF6366F1);
                  } else if (role == 'tool_request') {
                    roleColor = const Color(0xFFEC4899);
                  } else if (role == 'tool_result') {
                    roleColor = const Color(0xFF14B8A6);
                  } else if (role == 'tool') {
                    roleColor = const Color(0xFFEC4899);
                  } else {
                    roleColor = Colors.white70;
                  }
                  return Padding(
                    padding: const EdgeInsets.only(bottom: 6),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Row(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            SizedBox(
                              width: 80,
                              child: Text(
                                _formatRoleLabel(item.role),
                                style: AppFonts.inter(
                                  fontSize: 11,
                                  fontWeight: FontWeight.w600,
                                  color: roleColor,
                                ),
                              ),
                            ),
                            Expanded(
                              child: Text(
                                item.content,
                                style: AppFonts.inter(
                                  fontSize: 11,
                                  color: Colors.white,
                                ),
                                maxLines: role == 'assistant' ? 4 : 3,
                                overflow: TextOverflow.ellipsis,
                              ),
                            ),
                          ],
                        ),
                        if (item.fields.isNotEmpty) ...[
                          const SizedBox(height: 4),
                          Padding(
                            padding: const EdgeInsets.only(left: 80),
                            child: Container(
                              width: double.infinity,
                              padding: const EdgeInsets.all(8),
                              decoration: BoxDecoration(
                                color: roleColor.withValues(alpha: 0.06),
                                borderRadius: BorderRadius.circular(6),
                                border: Border.all(
                                  color: roleColor.withValues(alpha: 0.15),
                                ),
                              ),
                              child: Column(
                                crossAxisAlignment: CrossAxisAlignment.start,
                                children: item.fields.map((f) {
                                  return Padding(
                                    padding: const EdgeInsets.only(bottom: 2),
                                    child: Row(
                                      crossAxisAlignment:
                                          CrossAxisAlignment.start,
                                      children: [
                                        Text(
                                          '${f.field}: ',
                                          style: AppFonts.firaCode(
                                            fontSize: 10,
                                            fontWeight: FontWeight.w600,
                                            color: roleColor.withValues(
                                              alpha: 0.8,
                                            ),
                                          ),
                                        ),
                                        Expanded(
                                          child: Text(
                                            f.value,
                                            style: AppFonts.firaCode(
                                              fontSize: 10,
                                              color: Colors.white70,
                                            ),
                                            maxLines: 2,
                                            overflow: TextOverflow.ellipsis,
                                          ),
                                        ),
                                      ],
                                    ),
                                  );
                                }).toList(),
                              ),
                            ),
                          ),
                        ],
                      ],
                    ),
                  );
                }).toList(),
              ),
            ),
          ],
          if (toolArgs.isNotEmpty && !(toolDedup?.hideToolArgs ?? false))
            _buildToolSection(
              _cardText(l10n, 'toolArgs'),
              toolArgs,
              LucideIcons.fileJson2,
              const Color(0xFFEC4899),
            ),
          if (toolResults.isNotEmpty && !(toolDedup?.hideToolResults ?? false))
            _buildToolSection(
              _cardText(l10n, 'toolResult'),
              toolResults,
              LucideIcons.fileOutput,
              const Color(0xFFEC4899),
            ),
          const SizedBox(height: 12),
          Divider(height: 1, color: Colors.white.withValues(alpha: 0.1)),
          const SizedBox(height: 8),
          Wrap(
            spacing: 12,
            runSpacing: 10,
            children: [
              if (group.decisionStatus.isNotEmpty &&
                  !_isNormalDecision(group.decisionStatus))
                _buildMetaChip(
                  icon: group.decisionBlocked
                      ? LucideIcons.shieldOff
                      : LucideIcons.alertTriangle,
                  color: group.decisionBlocked
                      ? const Color(0xFFEF4444)
                      : const Color(0xFFF59E0B),
                  label: group.decisionStatus,
                  value: group.decisionReason,
                ),
              if (currentToolCalls.isNotEmpty)
                _buildMetaChip(
                  icon: LucideIcons.wrench,
                  color: const Color(0xFFEC4899),
                  label:
                      '${_cardText(l10n, 'toolCalls')} ${currentToolCalls.length}',
                  value: currentToolCalls
                      .map((tc) => tc.name)
                      .toSet()
                      .join(', '),
                ),
              if (tokenStr.isNotEmpty)
                _buildMetaChip(
                  icon: LucideIcons.gauge,
                  color: const Color(0xFF10B981),
                  label: _cardText(l10n, 'tokens'),
                  value: tokenStr,
                ),
            ],
          ),
        ],
      ),
    );
  }

  /// ALLOW/ALLOWED/COMPLETED 均属于正常决策，不应以警告形式展示。
  static bool _isNormalDecision(String status) {
    final s = status.trim().toUpperCase();
    return s == 'ALLOW' || s == 'ALLOWED' || s == 'COMPLETED';
  }

  /// 由 [TruthRecordModel] 推导卡片角标状态与颜色。
  (String, Color) _statusPresentationForView(TruthRecordModel group) {
    if (group.decisionBlocked) {
      return ('BLOCKED', const Color(0xFFEF4444));
    }
    final ds = group.decisionStatus.trim();
    if (ds.isNotEmpty && !_isNormalDecision(ds) && ds != 'QUOTA_EXCEEDED') {
      return (ds.toUpperCase(), const Color(0xFFF59E0B));
    }
    final phase = group.phase.trim().toLowerCase();
    if (group.isComplete || phase == 'completed') {
      return ('COMPLETED', const Color(0xFF10B981));
    }
    if (phase == 'stopped') {
      return ('STOPPED', const Color(0xFF94A3B8));
    }
    // 仅当本轮有新的 response 工具调用时才显示 TOOL_CALLING
    final hasResponseToolCalls = group.toolCalls.any(
      (tc) => tc.source == 'response',
    );
    if (hasResponseToolCalls) {
      return ('TOOL_CALLING', const Color(0xFFEC4899));
    }
    if (phase == 'starting') {
      return ('STARTING', const Color(0xFF3B82F6));
    }
    return (
      phase.isEmpty ? 'ACTIVE' : phase.toUpperCase(),
      const Color(0xFF94A3B8),
    );
  }

  Widget _buildInlineStatusChip(String value, Color color) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.12),
        borderRadius: BorderRadius.circular(4),
        border: Border.all(color: color.withValues(alpha: 0.25)),
      ),
      child: Text(value, style: AppFonts.firaCode(fontSize: 10, color: color)),
    );
  }
}
