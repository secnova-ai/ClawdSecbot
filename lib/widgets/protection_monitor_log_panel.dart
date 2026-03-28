import 'package:flutter/material.dart';
import 'package:lucide_icons/lucide_icons.dart';
import '../l10n/app_localizations.dart';
import '../models/protection_analysis_model.dart';
import '../models/request_log_group_model.dart';
import '../utils/app_fonts.dart';

/// 防护监控日志面板
/// 支持分组视图和原始视图两种展示模式
class ProtectionMonitorLogPanel extends StatefulWidget {
  final List<LogEntry> logs;
  final bool useGroupedView;
  final Map<String, RequestLogGroup> requestGroups;
  final List<String> requestOrder;
  final String currentBotModelName;
  final ScrollController logScrollController;
  final ScrollController horizontalScrollController;
  final ValueChanged<bool> onViewModeChanged;
  final VoidCallback onClearLogs;
  final void Function(String text, AppLocalizations l10n) onCopyText;
  final VoidCallback onScrollToBottom;

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
    return Padding(
      padding: const EdgeInsets.all(12),
      child: Row(
        children: [
          const Icon(LucideIcons.terminal, color: Color(0xFF6366F1), size: 16),
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
          IconButton(
            icon: const Icon(LucideIcons.copy, size: 16),
            color: _selectedIndices.isNotEmpty
                ? Colors.white54
                : Colors.white24,
            tooltip: l10n.localeName.startsWith('zh')
                ? '复制选中'
                : 'Copy Selected',
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
            tooltip: l10n.localeName.startsWith('zh') ? '复制全部' : 'Copy All',
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
                padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
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
                l10n.localeName.startsWith('zh') ? '清晰视图' : 'Grouped',
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
                l10n.localeName.startsWith('zh') ? '原始视图' : 'Raw',
                style: AppFonts.inter(fontSize: 11, color: Colors.white),
              ),
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildContent(AppLocalizations l10n) {
    if (widget.logs.isEmpty) {
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

    return LayoutBuilder(
      builder: (context, constraints) {
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
          child: widget.useGroupedView
              ? Scrollbar(
                  controller: widget.logScrollController,
                  thumbVisibility: true,
                  trackVisibility: true,
                  notificationPredicate: (notif) => notif.depth == 1,
                  child: Scrollbar(
                    controller: widget.horizontalScrollController,
                    thumbVisibility: true,
                    trackVisibility: true,
                    child: SingleChildScrollView(
                      controller: widget.horizontalScrollController,
                      scrollDirection: Axis.horizontal,
                      child: Container(
                        constraints: BoxConstraints(
                          minWidth: constraints.maxWidth,
                        ),
                        width: constraints.maxWidth > 2000
                            ? constraints.maxWidth
                            : 2000,
                        child: _buildGroupedView(l10n),
                      ),
                    ),
                  ),
                )
              : Scrollbar(
                  controller: widget.logScrollController,
                  thumbVisibility: true,
                  trackVisibility: true,
                  notificationPredicate: (notif) => notif.depth == 1,
                  child: Scrollbar(
                    controller: widget.horizontalScrollController,
                    thumbVisibility: true,
                    trackVisibility: true,
                    child: SingleChildScrollView(
                      controller: widget.horizontalScrollController,
                      scrollDirection: Axis.horizontal,
                      child: Container(
                        constraints: BoxConstraints(
                          minWidth: constraints.maxWidth,
                        ),
                        width: constraints.maxWidth > 3000
                            ? constraints.maxWidth
                            : 3000,
                        child: _buildRawView(l10n),
                      ),
                    ),
                  ),
                ),
        );
      },
    );
  }

  Widget _buildGroupedView(AppLocalizations l10n) {
    return ListView.builder(
      controller: widget.logScrollController,
      padding: const EdgeInsets.all(12),
      itemCount: widget.requestOrder.length,
      itemBuilder: (context, index) {
        // 与原始视图保持一致：按时间正序展示，最新记录位于底部。
        final reqId = widget.requestOrder[index];
        final group = widget.requestGroups[reqId];
        if (group == null) {
          return const SizedBox.shrink();
        }
        return _buildGroupedCard(group, l10n);
      },
    );
  }

  Widget _buildRawView(AppLocalizations l10n) {
    return ListView.builder(
      controller: widget.logScrollController,
      padding: const EdgeInsets.all(12),
      itemCount: widget.logs.length,
      itemExtent: 20.0,
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
                    child: Text(
                      l10n.localeName.startsWith('zh') ? '复制本行' : 'Copy Line',
                    ),
                  ),
                  if (_selectedIndices.isNotEmpty)
                    PopupMenuItem(
                      value: 'copy_selected',
                      child: Text(
                        l10n.localeName.startsWith('zh')
                            ? '复制选中'
                            : 'Copy Selected',
                      ),
                    ),
                  PopupMenuItem(
                    value: 'copy_all',
                    child: Text(
                      l10n.localeName.startsWith('zh') ? '复制全部' : 'Copy All',
                    ),
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
            child: SizedBox(
              height: 20.0,
              child: Row(
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
                          height: 1.0,
                          color: Color(logEntry.color),
                        ),
                        softWrap: false,
                        overflow: TextOverflow.visible,
                        maxLines: 1,
                      ),
                    ),
                  ),
                  SizedBox(
                    width: 22,
                    height: 20,
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

  /// 构建分组请求卡片
  Widget _buildGroupedCard(RequestLogGroup group, AppLocalizations l10n) {
    final subtitleBase = widget.currentBotModelName.isNotEmpty
        ? widget.currentBotModelName
        : (group.model.isNotEmpty ? group.model : '-');
    final subtitle = subtitleBase;
    final ts = _formatTime(group.startTime);
    final contentText = group.responseContent.isNotEmpty
        ? group.responseContent
        : (group.streamContent.isNotEmpty
              ? group.streamContent
              : (group.securityMessage.isNotEmpty
                    ? group.securityMessage
                    : (group.completionTokens > 0 ? '...' : '')));
    final hasContent = contentText.isNotEmpty;
    // 构造 Token 统计字符串: 输入/输出/总计
    final tokenStr = group.totalTokens > 0
        ? '${group.promptTokens} / ${group.completionTokens} / ${group.totalTokens}'
        : '';

    // 只展示最新一轮交互: 从尾部倒推,跳过 tool 消息找到最近的 assistant/user
    final nonSystemMessages = group.messages
        .where((m) => m.role != 'system')
        .toList();
    int latestStart = 0;
    if (nonSystemMessages.length > 1) {
      int i = nonSystemMessages.length - 1;
      while (i > 0 && nonSystemMessages[i].role == 'tool') {
        i--;
      }
      latestStart = i;
    }
    final relevantMessages = nonSystemMessages.sublist(latestStart);
    final userAndAssistantMessages =
        relevantMessages.where((m) => m.role != 'tool').toList();
    final toolMessageCount =
        relevantMessages.where((m) => m.role == 'tool').length;
    // 风险边框颜色映射
    final Color borderColor;
    if (group.decisionBlocked) {
      // 已拦截 → 红色
      borderColor = const Color(0xFFEF4444).withValues(alpha: 0.4);
    } else if (group.decisionStatus.isNotEmpty &&
        group.decisionStatus != 'ALLOWED') {
      // 有风险但未拦截（审计模式/警告）→ 黄色
      borderColor = const Color(0xFFF59E0B).withValues(alpha: 0.3);
    } else {
      // 正常/已允许 → 默认
      borderColor = Colors.white.withValues(alpha: 0.08);
    }

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
            children: [
              const Icon(
                LucideIcons.workflow,
                color: Color(0xFF6366F1),
                size: 16,
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
                      subtitle,
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
                      ],
                    ),
                  ],
                ),
              ),
            ],
          ),
          const SizedBox(height: 8),
          if (userAndAssistantMessages.isNotEmpty || toolMessageCount > 0)
            Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                // user / assistant 消息正常展示
                ...userAndAssistantMessages.take(3).map((m) {
                  final roleColor = m.role == 'user'
                      ? const Color(0xFF22C55E)
                      : (m.role == 'assistant'
                            ? const Color(0xFF6366F1)
                            : Colors.white70);
                  var content = m.content.replaceAll(
                    RegExp(r'^\[.*?\]\s*'),
                    '',
                  );
                  // assistant 消息内容为空时(仅含 tool_calls),用工具名填充
                  if (content.trim().isEmpty && m.role == 'assistant') {
                    if (group.toolNames.isNotEmpty) {
                      content = '→ ${group.toolNames.join(', ')}';
                    } else if (group.toolCallCount > 0) {
                      content = '→ ${group.toolCallCount} tool call(s)';
                    } else {
                      content = '...';
                    }
                  }
                  return Padding(
                    padding: const EdgeInsets.only(bottom: 4),
                    child: Row(
                      children: [
                        Container(
                          width: 56,
                          alignment: Alignment.centerLeft,
                          child: Text(
                            m.role,
                            style: AppFonts.inter(
                              fontSize: 11,
                              color: roleColor,
                            ),
                          ),
                        ),
                        Expanded(
                          child: Text(
                            content,
                            style: AppFonts.inter(
                              fontSize: 11,
                              color: Colors.white,
                            ),
                            maxLines: 1,
                            overflow: TextOverflow.ellipsis,
                          ),
                        ),
                      ],
                    ),
                  );
                }),
                // tool 消息折叠为汇总行 + 可选的工具结果摘要
                if (toolMessageCount > 0)
                  _buildToolResultSummaryRow(
                    toolMessageCount,
                    group.toolResultSummaries,
                    l10n,
                  ),
              ],
            ),
          if (hasContent)
            Padding(
              padding: const EdgeInsets.only(bottom: 8),
              child: Row(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Container(
                    width: 56,
                    padding: const EdgeInsets.only(top: 2),
                    alignment: Alignment.topLeft,
                    child: Text(
                      'Assistant',
                      style: AppFonts.inter(
                        fontSize: 11,
                        color: const Color(0xFF6366F1),
                      ),
                    ),
                  ),
                  Expanded(
                    child: Container(
                      padding: const EdgeInsets.all(8),
                      decoration: BoxDecoration(
                        color: Colors.white.withValues(alpha: 0.04),
                        borderRadius: BorderRadius.circular(6),
                      ),
                      child: Text(
                        contentText,
                        style: AppFonts.inter(
                          fontSize: 11,
                          color: Colors.white,
                        ),
                        maxLines: 4,
                        overflow: TextOverflow.ellipsis,
                      ),
                    ),
                  ),
                ],
              ),
            ),
          if (group.finishReason.isNotEmpty &&
              group.finishReason != 'tool_calls')
            Padding(
              padding: const EdgeInsets.only(top: 6),
              child: Row(
                children: [
                  const Icon(
                    LucideIcons.flag,
                    size: 14,
                    color: Color(0xFFF59E0B),
                  ),
                  const SizedBox(width: 6),
                  Text(
                    group.finishReason,
                    style: AppFonts.inter(fontSize: 11, color: Colors.white70),
                  ),
                ],
              ),
            ),
          const SizedBox(height: 12),
          Divider(height: 1, color: Colors.white.withValues(alpha: 0.1)),
          const SizedBox(height: 8),
          Row(
            children: [
              // 决策状态 Badge（仅非 ALLOWED 时显示）
              if (group.decisionStatus.isNotEmpty &&
                  group.decisionStatus != 'ALLOWED') ...[
                Builder(
                  builder: (_) {
                    final Color badgeColor;
                    final IconData badgeIcon;
                    if (group.decisionBlocked) {
                      badgeColor = const Color(0xFFEF4444);
                      badgeIcon = LucideIcons.shieldOff;
                    } else {
                      badgeColor = const Color(0xFFF59E0B);
                      badgeIcon = LucideIcons.alertTriangle;
                    }
                    return Container(
                      padding: const EdgeInsets.symmetric(
                        horizontal: 10,
                        vertical: 6,
                      ),
                      decoration: BoxDecoration(
                        color: badgeColor.withValues(alpha: 0.15),
                        borderRadius: BorderRadius.circular(6),
                        border: Border.all(
                          color: badgeColor.withValues(alpha: 0.3),
                        ),
                      ),
                      child: Row(
                        mainAxisSize: MainAxisSize.min,
                        children: [
                          Icon(badgeIcon, size: 14, color: badgeColor),
                          const SizedBox(width: 8),
                          Text(
                            group.decisionStatus,
                            style: AppFonts.inter(
                              fontSize: 11,
                              fontWeight: FontWeight.w600,
                              color: badgeColor,
                            ),
                          ),
                          if (group.decisionReason.isNotEmpty) ...[
                            const SizedBox(width: 8),
                            ConstrainedBox(
                              constraints: const BoxConstraints(maxWidth: 200),
                              child: Text(
                                group.decisionReason,
                                style: AppFonts.inter(
                                  fontSize: 11,
                                  color: Colors.white70,
                                ),
                                maxLines: 1,
                                overflow: TextOverflow.ellipsis,
                              ),
                            ),
                          ],
                        ],
                      ),
                    );
                  },
                ),
                const SizedBox(width: 12),
              ],
              if (group.toolCallCount > 0 || group.toolNames.isNotEmpty)
                Container(
                  padding: const EdgeInsets.symmetric(
                    horizontal: 10,
                    vertical: 6,
                  ),
                  decoration: BoxDecoration(
                    color: const Color(0xFFEC4899).withValues(alpha: 0.15),
                    borderRadius: BorderRadius.circular(6),
                    border: Border.all(
                      color: const Color(0xFFEC4899).withValues(alpha: 0.3),
                    ),
                  ),
                  child: Row(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      const Icon(
                        LucideIcons.wrench,
                        size: 14,
                        color: Color(0xFFEC4899),
                      ),
                      const SizedBox(width: 8),
                      Text(
                        'Tool Calls',
                        style: AppFonts.inter(
                          fontSize: 11,
                          fontWeight: FontWeight.w600,
                          color: const Color(0xFFEC4899),
                        ),
                      ),
                      if (group.toolCallCount > 0) ...[
                        const SizedBox(width: 8),
                        Container(
                          padding: const EdgeInsets.symmetric(
                            horizontal: 6,
                            vertical: 1,
                          ),
                          decoration: BoxDecoration(
                            color: const Color(
                              0xFFEC4899,
                            ).withValues(alpha: 0.2),
                            borderRadius: BorderRadius.circular(4),
                          ),
                          child: Text(
                            '${group.toolCallCount}',
                            style: AppFonts.firaCode(
                              fontSize: 11,
                              fontWeight: FontWeight.bold,
                              color: const Color(0xFFEC4899),
                            ),
                          ),
                        ),
                      ],
                      if (group.toolNames.isNotEmpty) ...[
                        const SizedBox(width: 8),
                        Text(
                          group.toolNames.join(', '),
                          style: AppFonts.inter(
                            fontSize: 11,
                            color: Colors.white70,
                          ),
                          overflow: TextOverflow.ellipsis,
                        ),
                      ],
                    ],
                  ),
                ),
              if ((group.toolCallCount > 0 || group.toolNames.isNotEmpty) &&
                  tokenStr.isNotEmpty)
                const SizedBox(width: 12),
              if (tokenStr.isNotEmpty)
                Container(
                  padding: const EdgeInsets.symmetric(
                    horizontal: 10,
                    vertical: 6,
                  ),
                  decoration: BoxDecoration(
                    color: const Color(0xFF10B981).withValues(alpha: 0.15),
                    borderRadius: BorderRadius.circular(6),
                    border: Border.all(
                      color: const Color(0xFF10B981).withValues(alpha: 0.3),
                    ),
                  ),
                  child: Row(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      const Icon(
                        LucideIcons.gauge,
                        size: 14,
                        color: Color(0xFF10B981),
                      ),
                      const SizedBox(width: 8),
                      Text(
                        'Tokens',
                        style: AppFonts.inter(
                          fontSize: 11,
                          fontWeight: FontWeight.w600,
                          color: const Color(0xFF10B981),
                        ),
                      ),
                      const SizedBox(width: 8),
                      Text(
                        tokenStr,
                        style: AppFonts.firaCode(
                          fontSize: 11,
                          color: Colors.white,
                        ),
                      ),
                    ],
                  ),
                ),
            ],
          ),
        ],
      ),
    );
  }

  /// 构建工具结果折叠汇总行
  Widget _buildToolResultSummaryRow(
    int toolCount,
    List<String> summaries,
    AppLocalizations l10n,
  ) {
    final isZh = l10n.localeName.startsWith('zh');
    final label = isZh ? '$toolCount 条工具结果' : '$toolCount tool result(s)';
    final previewText = summaries.isNotEmpty
        ? summaries.first
        : (isZh ? '(工具输出)' : '(tool output)');
    final displayPreview = previewText.length > 100
        ? '${previewText.substring(0, 100)}...'
        : previewText;
    return Padding(
      padding: const EdgeInsets.only(bottom: 4),
      child: Row(
        children: [
          Container(
            width: 56,
            alignment: Alignment.centerLeft,
            child: Row(
              children: [
                const Icon(
                  LucideIcons.wrench,
                  size: 12,
                  color: Color(0xFFEC4899),
                ),
                const SizedBox(width: 4),
                Text(
                  'tool',
                  style: AppFonts.inter(
                    fontSize: 11,
                    color: const Color(0xFFEC4899),
                  ),
                ),
              ],
            ),
          ),
          Container(
            padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 1),
            decoration: BoxDecoration(
              color: const Color(0xFFEC4899).withValues(alpha: 0.12),
              borderRadius: BorderRadius.circular(4),
            ),
            child: Text(
              label,
              style: AppFonts.inter(
                fontSize: 10,
                fontWeight: FontWeight.w500,
                color: const Color(0xFFEC4899),
              ),
            ),
          ),
          const SizedBox(width: 8),
          Expanded(
            child: Text(
              displayPreview,
              style: AppFonts.inter(fontSize: 11, color: Colors.white54),
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
            ),
          ),
        ],
      ),
    );
  }
}
