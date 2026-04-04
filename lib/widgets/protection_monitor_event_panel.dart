import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:lucide_icons/lucide_icons.dart';
import '../l10n/app_localizations.dart';
import '../models/security_event_model.dart';
import '../utils/app_fonts.dart';

/// 安全事件面板：展示数据库驱动的安全事件记录
class ProtectionMonitorEventPanel extends StatefulWidget {
  final List<SecurityEvent> events;
  final VoidCallback onClearEvents;
  final VoidCallback onRefresh;

  const ProtectionMonitorEventPanel({
    super.key,
    required this.events,
    required this.onClearEvents,
    required this.onRefresh,
  });

  @override
  State<ProtectionMonitorEventPanel> createState() =>
      _ProtectionMonitorEventPanelState();
}

class _ProtectionMonitorEventPanelState
    extends State<ProtectionMonitorEventPanel> {
  final ScrollController _scrollController = ScrollController();
  int? _hoverIndex;

  @override
  void dispose() {
    _scrollController.dispose();
    super.dispose();
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
            Expanded(child: _buildEventList(l10n)),
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
          const Icon(LucideIcons.shield, color: Color(0xFF6366F1), size: 16),
          const SizedBox(width: 8),
          Text(
            l10n.securityEvents,
            style: AppFonts.inter(
              fontSize: 14,
              fontWeight: FontWeight.w600,
              color: Colors.white,
            ),
          ),
          const SizedBox(width: 8),
          Container(
            padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
            decoration: BoxDecoration(
              color: const Color(0xFF6366F1).withValues(alpha: 0.2),
              borderRadius: BorderRadius.circular(10),
            ),
            child: Text(
              '${widget.events.length}',
              style: AppFonts.firaCode(
                fontSize: 11,
                color: const Color(0xFF6366F1),
              ),
            ),
          ),
          const Spacer(),
          IconButton(
            icon: const Icon(LucideIcons.refreshCw, size: 14),
            color: Colors.white38,
            tooltip: l10n.refresh,
            onPressed: widget.onRefresh,
            constraints: const BoxConstraints(minWidth: 28, minHeight: 28),
            padding: EdgeInsets.zero,
            iconSize: 14,
          ),
          IconButton(
            icon: const Icon(LucideIcons.trash2, size: 14),
            color: Colors.white38,
            tooltip: l10n.clearAll,
            onPressed: widget.events.isEmpty ? null : widget.onClearEvents,
            constraints: const BoxConstraints(minWidth: 28, minHeight: 28),
            padding: EdgeInsets.zero,
            iconSize: 14,
          ),
        ],
      ),
    );
  }

  Widget _buildEventList(AppLocalizations l10n) {
    if (widget.events.isEmpty) {
      return Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(
              LucideIcons.shieldCheck,
              color: Colors.white.withValues(alpha: 0.15),
              size: 32,
            ),
            const SizedBox(height: 8),
            Text(
              l10n.noSecurityEvents,
              style: AppFonts.inter(fontSize: 12, color: Colors.white38),
            ),
          ],
        ),
      );
    }

    return ListView.builder(
      controller: _scrollController,
      itemCount: widget.events.length,
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
      itemBuilder: (context, index) {
        final event = widget.events[index];
        return _buildEventCard(event, index, l10n);
      },
    );
  }

  Widget _buildEventCard(
    SecurityEvent event,
    int index,
    AppLocalizations l10n,
  ) {
    final isHovered = _hoverIndex == index;
    final eventColor = _getEventColor(event);
    final eventIcon = _getEventIcon(event);

    return MouseRegion(
      onEnter: (_) => setState(() => _hoverIndex = index),
      onExit: (_) => setState(() => _hoverIndex = null),
      child: GestureDetector(
        onTap: () => _showEventDetail(event, l10n),
        child: Container(
          margin: const EdgeInsets.symmetric(vertical: 2),
          padding: const EdgeInsets.all(8),
          decoration: BoxDecoration(
            color: isHovered
                ? Colors.white.withValues(alpha: 0.08)
                : Colors.white.withValues(alpha: 0.03),
            borderRadius: BorderRadius.circular(6),
            border: Border(left: BorderSide(color: eventColor, width: 2)),
          ),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  Icon(eventIcon, color: eventColor, size: 13),
                  const SizedBox(width: 6),
                  Expanded(
                    child: Text(
                      event.actionDesc.isNotEmpty
                          ? event.actionDesc
                          : _getEventTypeLabel(event, l10n),
                      style: AppFonts.inter(
                        fontSize: 12,
                        color: Colors.white.withValues(alpha: 0.85),
                      ),
                      maxLines: 2,
                      overflow: TextOverflow.ellipsis,
                    ),
                  ),
                  const SizedBox(width: 4),
                  _buildSourceBadge(event),
                ],
              ),
              const SizedBox(height: 4),
              Row(
                children: [
                  if (event.riskType.isNotEmpty) ...[
                    Container(
                      padding: const EdgeInsets.symmetric(
                        horizontal: 5,
                        vertical: 1,
                      ),
                      decoration: BoxDecoration(
                        color: eventColor.withValues(alpha: 0.15),
                        borderRadius: BorderRadius.circular(3),
                      ),
                      child: Text(
                        event.riskType,
                        style: AppFonts.inter(fontSize: 10, color: eventColor),
                      ),
                    ),
                    const SizedBox(width: 6),
                  ],
                  Text(
                    _formatEventTime(event.timestamp),
                    style: AppFonts.firaCode(
                      fontSize: 10,
                      color: Colors.white38,
                    ),
                  ),
                ],
              ),
            ],
          ),
        ),
      ),
    );
  }

  Widget _buildSourceBadge(SecurityEvent event) {
    final isAgent = event.isFromReactAgent;
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 4, vertical: 1),
      decoration: BoxDecoration(
        color: isAgent
            ? const Color(0xFF6366F1).withValues(alpha: 0.2)
            : const Color(0xFFF59E0B).withValues(alpha: 0.2),
        borderRadius: BorderRadius.circular(3),
      ),
      child: Text(
        isAgent ? 'AI' : 'H',
        style: AppFonts.firaCode(
          fontSize: 9,
          fontWeight: FontWeight.w600,
          color: isAgent ? const Color(0xFF6366F1) : const Color(0xFFF59E0B),
        ),
      ),
    );
  }

  Color _getEventColor(SecurityEvent event) {
    if (event.isBlocked) return const Color(0xFFEF4444);
    if (event.isToolExecution) return const Color(0xFF6366F1);
    return const Color(0xFFF59E0B);
  }

  IconData _getEventIcon(SecurityEvent event) {
    if (event.isBlocked) return LucideIcons.shieldOff;
    if (event.isToolExecution) return LucideIcons.wrench;
    return LucideIcons.alertTriangle;
  }

  String _getEventTypeLabel(SecurityEvent event, AppLocalizations l10n) {
    if (event.isBlocked) return l10n.eventBlocked;
    if (event.isToolExecution) return l10n.eventToolExecution;
    return l10n.eventOther;
  }

  String _formatEventTime(DateTime time) {
    final local = time.toLocal();
    final hh = local.hour.toString().padLeft(2, '0');
    final mm = local.minute.toString().padLeft(2, '0');
    final ss = local.second.toString().padLeft(2, '0');
    return '$hh:$mm:$ss';
  }

  void _showEventDetail(SecurityEvent event, AppLocalizations l10n) {
    showDialog(
      context: context,
      builder: (context) =>
          _SecurityEventDetailDialog(event: event, l10n: l10n),
    );
  }
}

/// 安全事件详情弹窗
class _SecurityEventDetailDialog extends StatefulWidget {
  final SecurityEvent event;
  final AppLocalizations l10n;

  const _SecurityEventDetailDialog({required this.event, required this.l10n});

  @override
  State<_SecurityEventDetailDialog> createState() =>
      _SecurityEventDetailDialogState();
}

class _SecurityEventDetailDialogState
    extends State<_SecurityEventDetailDialog> {
  final ScrollController _dialogScrollController = ScrollController();

  @override
  void dispose() {
    _dialogScrollController.dispose();
    super.dispose();
  }

  SecurityEvent get event => widget.event;
  AppLocalizations get l10n => widget.l10n;

  @override
  Widget build(BuildContext context) {
    final eventColor = event.isBlocked
        ? const Color(0xFFEF4444)
        : event.isToolExecution
        ? const Color(0xFF6366F1)
        : const Color(0xFFF59E0B);

    return Dialog(
      backgroundColor: const Color(0xFF1A1A2E),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(12),
        side: BorderSide(color: eventColor.withValues(alpha: 0.3)),
      ),
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 480, maxHeight: 400),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 20, 20, 0),
              child: Row(
                children: [
                  Icon(
                    event.isBlocked
                        ? LucideIcons.shieldOff
                        : event.isToolExecution
                        ? LucideIcons.wrench
                        : LucideIcons.alertTriangle,
                    color: eventColor,
                    size: 20,
                  ),
                  const SizedBox(width: 10),
                  Expanded(
                    child: Text(
                      l10n.securityEventDetail,
                      style: AppFonts.inter(
                        fontSize: 16,
                        fontWeight: FontWeight.w600,
                        color: Colors.white,
                      ),
                    ),
                  ),
                  IconButton(
                    icon: const Icon(LucideIcons.x, size: 16),
                    color: Colors.white54,
                    onPressed: () => Navigator.of(context).pop(),
                  ),
                ],
              ),
            ),
            const SizedBox(height: 16),
            Flexible(
              child: ScrollbarTheme(
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
                  controller: _dialogScrollController,
                  thumbVisibility: true,
                  trackVisibility: true,
                  child: SingleChildScrollView(
                    controller: _dialogScrollController,
                    padding: const EdgeInsets.symmetric(horizontal: 20),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        _buildDetailRow(
                          l10n.eventTime,
                          _formatFullTime(event.timestamp),
                        ),
                        if (event.actionDesc.isNotEmpty)
                          _buildDetailRow(
                              l10n.eventActionDesc, event.actionDesc),
                        if (event.riskType.isNotEmpty)
                          _buildDetailRow(
                              l10n.eventRiskType, event.riskType),
                        _buildDetailRow(
                          l10n.eventSource,
                          event.isFromReactAgent
                              ? l10n.eventSourceAgent
                              : l10n.eventSourceHeuristic,
                        ),
                        _buildDetailRow(l10n.eventType, event.eventType),
                        if (event.detail.isNotEmpty)
                          _buildDetailRow(l10n.eventDetail, event.detail),
                        _buildDetailRow('ID', event.id),
                      ],
                    ),
                  ),
                ),
              ),
            ),
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 12, 20, 20),
              child: Align(
                alignment: Alignment.centerRight,
                child: TextButton.icon(
                  icon: const Icon(LucideIcons.copy, size: 14),
                  label: Text(l10n.copyEventInfo),
                  onPressed: () {
                    final text = _buildCopyText();
                    Clipboard.setData(ClipboardData(text: text));
                    ScaffoldMessenger.of(context).showSnackBar(
                      SnackBar(
                        content: Text(l10n.appStoreGuideCopied),
                        duration: const Duration(seconds: 2),
                      ),
                    );
                  },
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildDetailRow(String label, String value) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 10),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            label,
            style: AppFonts.inter(fontSize: 11, color: Colors.white38),
          ),
          const SizedBox(height: 2),
          Text(
            value,
            style: AppFonts.inter(
              fontSize: 13,
              color: Colors.white.withValues(alpha: 0.85),
            ),
          ),
        ],
      ),
    );
  }

  String _formatFullTime(DateTime time) {
    final y = time.year.toString();
    final m = time.month.toString().padLeft(2, '0');
    final d = time.day.toString().padLeft(2, '0');
    final hh = time.hour.toString().padLeft(2, '0');
    final mm = time.minute.toString().padLeft(2, '0');
    final ss = time.second.toString().padLeft(2, '0');
    return '$y-$m-$d $hh:$mm:$ss';
  }

  String _buildCopyText() {
    final buf = StringBuffer();
    buf.writeln('Security Event: ${event.id}');
    buf.writeln('Time: ${_formatFullTime(event.timestamp)}');
    buf.writeln('Type: ${event.eventType}');
    if (event.actionDesc.isNotEmpty) buf.writeln('Action: ${event.actionDesc}');
    if (event.riskType.isNotEmpty) buf.writeln('Risk: ${event.riskType}');
    buf.writeln('Source: ${event.source}');
    if (event.detail.isNotEmpty) buf.writeln('Detail: ${event.detail}');
    return buf.toString();
  }
}
