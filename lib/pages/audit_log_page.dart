import 'dart:async';
import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:lucide_icons/lucide_icons.dart';

import '../l10n/app_localizations.dart';
import '../models/audit_log_model.dart';
import '../models/security_event_model.dart';
import '../services/audit_log_database_service.dart';
import '../services/protection_service.dart';
import '../services/security_event_database_service.dart';
import '../utils/app_fonts.dart';
import '../utils/app_logger.dart';
import '../utils/audit_log_export_helper.dart';
import '../utils/runtime_platform.dart';

part 'audit_log_page_detail_ext.dart';

const _appBackground = Color(0xFF0F0F23);

class AuditLogPage extends StatefulWidget {
  final String windowId;
  final String initialAssetName;
  final String initialAssetID;
  final Future<void> Function()? onRequestMinimize;
  final Future<void> Function()? onRequestToggleMaximize;
  final Future<void> Function()? onRequestClose;
  final Future<void> Function()? onRequestStartDragging;
  final Future<String?> Function({
    required String fileName,
    required String content,
  })?
  onExportMarkdown;
  final bool initialMaximized;

  const AuditLogPage({
    super.key,
    required this.windowId,
    this.initialAssetName = '',
    this.initialAssetID = '',
    this.onRequestMinimize,
    this.onRequestToggleMaximize,
    this.onRequestClose,
    this.onRequestStartDragging,
    this.onExportMarkdown,
    this.initialMaximized = false,
  });

  @override
  State<AuditLogPage> createState() => _AuditLogPageState();
}

class _AuditLogPageState extends State<AuditLogPage>
    with WidgetsBindingObserver {
  final ProtectionService _agentService = ProtectionService();
  final AuditLogDatabaseService _auditLogDatabaseService =
      AuditLogDatabaseService();
  final SecurityEventDatabaseService _securityEventDatabaseService =
      SecurityEventDatabaseService();
  final TextEditingController _searchController = TextEditingController();
  final ScrollController _scrollController = ScrollController();

  /// 已生效的全文搜索关键词(仅回车后更新, 与输入框内容可暂时不一致).
  String _appliedSearchQuery = '';

  /// 用户按回车提交搜索后, 列表查询进行中的标记(用于搜索框旁旋转指示).
  bool _searchSubmitLoading = false;

  List<AuditLog> _logs = [];
  int _totalCount = 0;
  bool _isLoading = false;
  bool _riskOnly = false;
  int _currentPage = 0;
  final int _pageSize = 10;
  Timer? _refreshTimer;
  AuditLog? _selectedLog;
  final Map<String, AuditLog> _selectedLogsForExport = <String, AuditLog>{};
  List<SecurityEvent> _relatedEvents = [];
  Map<String, dynamic> _statistics = {};
  bool _isMaximized = false;
  List<_AuditAssetFilterTab> _assetTabs = const [
    _AuditAssetFilterTab(label: 'All Bots', assetName: '', assetID: ''),
  ];
  String _selectedAssetName = '';
  String _selectedAssetID = '';

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    _selectedAssetName = widget.initialAssetName.trim();
    _selectedAssetID = widget.initialAssetID.trim();
    _isMaximized = widget.initialMaximized;
    _loadLogs();
    _loadStatistics();
    _loadAssetTabs();
    // Auto refresh every 5 seconds
    _refreshTimer = Timer.periodic(const Duration(seconds: 5), (_) {
      _syncAndRefresh();
    });
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state != AppLifecycleState.resumed) return;
    unawaited(_syncAndRefresh());
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    _refreshTimer?.cancel();
    _searchController.dispose();
    _scrollController.dispose();
    super.dispose();
  }

  Future<void> _syncAndRefresh() async {
    // Sync pending logs from Go buffer to SQLite
    await _agentService.syncPendingAuditLogs();
    await _loadAssetTabs();
    await _loadLogs();
    await _loadStatistics();
  }

  Future<void> _refreshFromToolbar() async {
    _selectLog(null);
    _clearSelectedLogs();
    await _syncAndRefresh();
  }

  bool _isLogChecked(AuditLog log) =>
      _selectedLogsForExport.containsKey(log.id);

  void _toggleLogSelection(AuditLog log) {
    setState(() {
      if (_selectedLogsForExport.containsKey(log.id)) {
        _selectedLogsForExport.remove(log.id);
      } else {
        _selectedLogsForExport[log.id] = log;
      }
    });
  }

  void _clearSelectedLogs() {
    if (_selectedLogsForExport.isEmpty) return;
    setState(() => _selectedLogsForExport.clear());
  }

  Future<void> _loadAssetTabs() async {
    final tabs = <_AuditAssetFilterTab>[
      const _AuditAssetFilterTab(label: 'All Bots', assetName: '', assetID: ''),
    ];
    final assets = await _auditLogDatabaseService.getAuditLogAssets();
    for (final asset in assets) {
      final exists = tabs.any(
        (tab) =>
            tab.assetName == asset['asset_name'] &&
            tab.assetID == asset['asset_id'],
      );
      if (exists) continue;
      tabs.add(
        _AuditAssetFilterTab(
          label: asset['asset_name'] ?? '',
          assetName: asset['asset_name'] ?? '',
          assetID: asset['asset_id'] ?? '',
        ),
      );
    }
    final hasCurrent = tabs.any(
      (tab) =>
          tab.assetName == _selectedAssetName &&
          tab.assetID == _selectedAssetID,
    );

    setState(() {
      _assetTabs = tabs;
      if (!hasCurrent) {
        _selectedAssetName = '';
        _selectedAssetID = '';
      }
    });
  }

  Future<void> _toggleMaximize() async {
    final callback = widget.onRequestToggleMaximize;
    if (callback == null) return;
    await callback();
    if (!mounted) return;
    setState(() => _isMaximized = !_isMaximized);
  }

  /// 左侧日志列表按时间降序排列, 最近的在最上方(与分页 offset 语义一致).
  void _sortAuditLogsNewestFirst(List<AuditLog> logs) {
    logs.sort((a, b) {
      final byTime = b.timestamp.compareTo(a.timestamp);
      if (byTime != 0) {
        return byTime;
      }
      return b.requestId.compareTo(a.requestId);
    });
  }

  /// 加载当前页的审计日志列表; [triggeredBySearch] 为 true 时在搜索框侧显示查询动画.
  Future<void> _loadLogs({bool triggeredBySearch = false}) async {
    if (!mounted) return;
    setState(() {
      _isLoading = true;
      if (triggeredBySearch) {
        _searchSubmitLoading = true;
      } else {
        _searchSubmitLoading = false;
      }
    });

    try {
      final logs = await _agentService.getAuditLogs(
        limit: _pageSize,
        offset: _currentPage * _pageSize,
        riskOnly: _riskOnly,
        assetName: _selectedAssetName,
        assetID: _selectedAssetID,
        searchQuery: _appliedSearchQuery.isNotEmpty
            ? _appliedSearchQuery
            : null,
      );

      _sortAuditLogsNewestFirst(logs);

      final count = await _agentService.getAuditLogCount(
        riskOnly: _riskOnly,
        assetName: _selectedAssetName,
        assetID: _selectedAssetID,
        searchQuery: _appliedSearchQuery.isNotEmpty
            ? _appliedSearchQuery
            : null,
      );

      if (!mounted) return;
      setState(() {
        _logs = logs;
        for (final log in logs) {
          if (_selectedLogsForExport.containsKey(log.id)) {
            _selectedLogsForExport[log.id] = log;
          }
        }
        _totalCount = count;
        _isLoading = false;
        _searchSubmitLoading = false;
      });
    } catch (_) {
      if (!mounted) return;
      setState(() {
        _isLoading = false;
        _searchSubmitLoading = false;
      });
    }
  }

  Future<void> _loadStatistics() async {
    final stats = await _agentService.getAuditLogStatistics(
      assetName: _selectedAssetName,
      assetID: _selectedAssetID,
    );
    setState(() => _statistics = stats);
  }

  /// 将搜索框当前内容提交为查询条件并回到第一页重新加载列表(仅回车触发).
  void _submitSearch() {
    final trimmed = _searchController.text.trim();
    setState(() {
      _appliedSearchQuery = trimmed;
      _currentPage = 0;
      _selectedLogsForExport.clear();
    });
    _loadLogs(triggeredBySearch: true);
  }

  void _onRiskFilterChanged(bool value) {
    setState(() {
      _riskOnly = value;
      _currentPage = 0;
      _selectedLogsForExport.clear();
    });
    _loadLogs();
    _loadStatistics();
  }

  void _onAssetTabChanged(_AuditAssetFilterTab tab) {
    setState(() {
      _selectedAssetName = tab.assetName;
      _selectedAssetID = tab.assetID;
      _currentPage = 0;
      _selectedLogsForExport.clear();
    });
    _loadLogs();
    _loadStatistics();
  }

  void _onPageChanged(int page) {
    setState(() => _currentPage = page);
    _loadLogs();
  }

  void _clearAllLogs() {
    final l10n = AppLocalizations.of(context);
    final hasAssetFilter =
        _selectedAssetName.isNotEmpty || _selectedAssetID.isNotEmpty;
    final currentTabLabel = _assetTabs
        .where(
          (tab) =>
              tab.assetName == _selectedAssetName &&
              tab.assetID == _selectedAssetID,
        )
        .map((tab) => tab.label)
        .cast<String?>()
        .firstWhere(
          (label) => label != null && label.isNotEmpty,
          orElse: () {
            return _selectedAssetName.isNotEmpty ? _selectedAssetName : null;
          },
        );
    final confirmTitle = hasAssetFilter
        ? '${l10n?.auditLogClear ?? 'Clear'} ${currentTabLabel ?? ''}'.trim()
        : (l10n?.auditLogClearConfirmTitle ?? 'Clear All Logs');
    final confirmMessage = hasAssetFilter
        ? '确定要清空当前标签页“${currentTabLabel ?? _selectedAssetName}”的审计日志吗？此操作无法撤销。'
        : (l10n?.auditLogClearConfirmMessage ??
              'Are you sure you want to clear all audit logs? This action cannot be undone.');
    showDialog(
      context: context,
      builder: (ctx) => AlertDialog(
        backgroundColor: const Color(0xFF1A1A2E),
        title: Text(confirmTitle, style: const TextStyle(color: Colors.white)),
        content: Text(
          confirmMessage,
          style: const TextStyle(color: Colors.white70),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(ctx),
            child: Text(l10n?.auditLogCancel ?? 'Cancel'),
          ),
          TextButton(
            onPressed: () async {
              final navigator = Navigator.of(ctx);
              await _agentService.clearAuditLogs(
                assetName: _selectedAssetName,
                assetID: _selectedAssetID,
              );
              _agentService.clearAuditLogsBufferWithFilter(
                assetName: _selectedAssetName,
                assetID: _selectedAssetID,
              );
              navigator.pop();
              if (!mounted) return;
              _selectLog(null);
              _clearSelectedLogs();
              await _loadAssetTabs();
              await _loadLogs();
              await _loadStatistics();
            },
            child: Text(
              l10n?.auditLogClear ?? 'Clear',
              style: const TextStyle(color: Colors.red),
            ),
          ),
        ],
      ),
    );
  }

  /// 选中一条审计日志，同时按 request_id 加载关联安全事件
  void _selectLog(AuditLog? log) {
    setState(() {
      _selectedLog = log;
      _relatedEvents = [];
    });
    if (log != null && log.requestId.isNotEmpty) {
      _securityEventDatabaseService
          .getSecurityEventsByRequestID(log.requestId)
          .then((events) {
            if (mounted && _selectedLog?.requestId == log.requestId) {
              setState(() => _relatedEvents = events);
            }
          });
    }
  }

  void _copyText(String text) {
    final l10n = AppLocalizations.of(context);
    Clipboard.setData(ClipboardData(text: text));
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        content: Text(l10n?.appStoreGuideCopied ?? '已复制到剪贴板'),
        duration: const Duration(seconds: 2),
      ),
    );
  }

  Future<void> _exportMarkdownContent({
    required String fileName,
    required String content,
    required String successMessageZh,
    required String successMessageEn,
  }) async {
    final l10n = AppLocalizations.of(context);
    final export = widget.onExportMarkdown;
    if (export == null) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(
            _isZh
                ? '当前环境暂不支持文件导出，请在桌面端操作。'
                : 'File export is not available in this environment. Use desktop app.',
          ),
          duration: const Duration(seconds: 3),
        ),
      );
      return;
    }

    try {
      final exportedPath = await export(fileName: fileName, content: content);
      if (exportedPath == null || exportedPath.trim().isEmpty || !mounted) {
        return;
      }
      final message = _isZh ? successMessageZh : successMessageEn;
      final text = message.replaceAll('{path}', exportedPath.trim());
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(text), duration: const Duration(seconds: 4)),
      );
    } catch (e, st) {
      appLogger.error('[AuditLogWindow] Export failed', e, st);
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(
            _isZh
                ? '${l10n?.auditLogExportFailed ?? "导出失败"}: $e'
                : '${l10n?.auditLogExportFailed ?? "Export failed"}: $e',
          ),
          duration: const Duration(seconds: 3),
        ),
      );
    }
  }

  Future<void> _exportSelectedLogs() async {
    if (_selectedLogsForExport.isEmpty) return;
    try {
      final selectedLogs = _selectedLogsForExport.values.toList()
        ..sort((a, b) {
          final byTime = b.timestamp.compareTo(a.timestamp);
          if (byTime != 0) return byTime;
          return b.requestId.compareTo(a.requestId);
        });
      final fileName =
          'audit_logs_batch_${DateTime.now().millisecondsSinceEpoch}.md';

      final contents = await Future.wait(
        selectedLogs.map((log) async {
          final relatedEvents = log.requestId.isEmpty
              ? <SecurityEvent>[]
              : await _securityEventDatabaseService
                    .getSecurityEventsByRequestID(log.requestId);
          return buildAuditLogMarkdownContent(
            isZh: _isZh,
            log: log,
            relatedEvents: relatedEvents,
            rawText: _buildRawSectionText(log),
            actionText: _buildActionSectionText(log),
            eventText: _buildEventSectionText(relatedEvents),
          );
        }),
      );

      final exportTime = DateTime.now().toIso8601String();
      final buffer = StringBuffer()
        ..writeln(_isZh ? '# 审计日志批量导出' : '# Audit Log Batch Export')
        ..writeln()
        ..writeln(
          _isZh
              ? '- 导出条数: ${selectedLogs.length}'
              : '- Exported logs: ${selectedLogs.length}',
        )
        ..writeln(_isZh ? '- 导出时间: $exportTime' : '- Exported at: $exportTime');

      for (final content in contents) {
        buffer
          ..writeln()
          ..writeln('---')
          ..writeln()
          ..write(content);
      }
      await _exportMarkdownContent(
        fileName: fileName,
        content: buffer.toString(),
        successMessageZh: '已批量导出 ${selectedLogs.length} 条日志到 {path}',
        successMessageEn: 'Exported ${selectedLogs.length} logs to {path}',
      );
    } catch (e, st) {
      appLogger.error('[AuditLogWindow] Batch export prepare failed', e, st);
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(_isZh ? '导出失败: $e' : 'Export failed: $e'),
          duration: const Duration(seconds: 3),
        ),
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: _appBackground,
      body: LayoutBuilder(
        builder: (context, constraints) {
          if (constraints.maxHeight < 100 || constraints.maxWidth < 100) {
            return const SizedBox.shrink();
          }
          return SelectionArea(
            child: Container(
              decoration: BoxDecoration(
                gradient: LinearGradient(
                  begin: Alignment.topLeft,
                  end: Alignment.bottomRight,
                  colors: [
                    const Color(0xFF0F0F23),
                    const Color(0xFF1A1A2E),
                    const Color(0xFF16213E),
                  ],
                ),
              ),
              child: Column(
                children: [
                  _buildTitleBar(),
                  _buildStatisticsBar(),
                  _buildToolbar(),
                  Expanded(
                    child: Row(
                      children: [
                        Expanded(flex: 2, child: _buildLogList()),
                        if (_selectedLog != null)
                          Expanded(flex: 3, child: _buildLogDetail()),
                      ],
                    ),
                  ),
                  _buildPagination(),
                ],
              ),
            ),
          );
        },
      ),
    );
  }

  Widget _buildTitleBar() {
    final l10n = AppLocalizations.of(context);
    final isWindows = isRuntimeWindows;
    final isLinux = isRuntimeLinux;

    return Container(
      height: 48,
      decoration: BoxDecoration(
        color: Colors.black.withValues(alpha: 0.3),
        border: Border(
          bottom: BorderSide(color: Colors.white.withValues(alpha: 0.1)),
        ),
      ),
      child: Row(
        children: [
          Expanded(
            child: GestureDetector(
              onPanStart: isLinux
                  ? null
                  : (_) {
                      unawaited(widget.onRequestStartDragging?.call());
                    },
              behavior: HitTestBehavior.translucent,
              child: Padding(
                padding: isWindows || isLinux
                    ? const EdgeInsets.only(left: 16)
                    : const EdgeInsets.only(left: 78),
                child: Row(
                  children: [
                    Container(
                      padding: const EdgeInsets.all(6),
                      decoration: BoxDecoration(
                        gradient: const LinearGradient(
                          colors: [Color(0xFF6366F1), Color(0xFF8B5CF6)],
                        ),
                        borderRadius: BorderRadius.circular(8),
                      ),
                      child: const Icon(
                        LucideIcons.fileText,
                        color: Colors.white,
                        size: 16,
                      ),
                    ),
                    const SizedBox(width: 10),
                    Text(
                      l10n?.auditLogTitle ?? 'Audit Log',
                      style: AppFonts.inter(
                        fontSize: 14,
                        fontWeight: FontWeight.w600,
                        color: Colors.white,
                      ),
                    ),
                  ],
                ),
              ),
            ),
          ),
          IconButton(
            icon: const Icon(LucideIcons.download, size: 16),
            color: _selectedLogsForExport.isEmpty
                ? Colors.white30
                : Colors.white70,
            onPressed: _selectedLogsForExport.isEmpty
                ? null
                : _exportSelectedLogs,
            tooltip: _isZh
                ? '批量导出已选日志（${_selectedLogsForExport.length}）'
                : 'Export selected logs (${_selectedLogsForExport.length})',
          ),
          IconButton(
            icon: const Icon(LucideIcons.refreshCw, size: 16),
            color: Colors.white70,
            onPressed: _refreshFromToolbar,
            tooltip: l10n?.auditLogRefresh ?? 'Refresh',
          ),
          IconButton(
            icon: const Icon(LucideIcons.trash2, size: 16),
            color: Colors.white70,
            onPressed: _clearAllLogs,
            tooltip:
                (_selectedAssetName.isNotEmpty || _selectedAssetID.isNotEmpty)
                ? (l10n?.auditLogClear ?? 'Clear')
                : (l10n?.auditLogClearAll ?? 'Clear All'),
          ),
          if (isRuntimeWindows) ...[
            const SizedBox(width: 8),
            _buildWindowButton(
              icon: LucideIcons.minus,
              onTap: () => unawaited(widget.onRequestMinimize?.call()),
            ),
            const SizedBox(width: 8),
            _buildWindowButton(
              icon: _isMaximized ? Icons.filter_none : Icons.crop_square,
              onTap: () => unawaited(_toggleMaximize()),
            ),
            const SizedBox(width: 8),
            _buildWindowButton(
              icon: LucideIcons.x,
              onTap: () => unawaited(widget.onRequestClose?.call()),
              isClose: true,
            ),
          ],
          if (isRuntimeMacOS) ...[
            const SizedBox(width: 8),
            _buildWindowButton(
              icon: LucideIcons.minus,
              onTap: () => unawaited(widget.onRequestMinimize?.call()),
            ),
            const SizedBox(width: 8),
            _buildWindowButton(
              icon: LucideIcons.x,
              onTap: () => unawaited(widget.onRequestClose?.call()),
              isClose: true,
            ),
          ],
          if (!isRuntimeWindows &&
              !isRuntimeMacOS &&
              widget.onRequestClose != null) ...[
            const SizedBox(width: 8),
            _buildWindowButton(
              icon: LucideIcons.x,
              onTap: () => unawaited(widget.onRequestClose?.call()),
              isClose: true,
            ),
          ],
          const SizedBox(width: 16),
        ],
      ),
    );
  }

  Widget _buildWindowButton({
    required IconData icon,
    required VoidCallback onTap,
    bool isClose = false,
  }) {
    return MouseRegion(
      cursor: SystemMouseCursors.click,
      child: GestureDetector(
        onTap: onTap,
        child: Container(
          width: 28,
          height: 28,
          decoration: BoxDecoration(
            color: isClose
                ? Colors.red.withValues(alpha: 0.2)
                : Colors.white.withValues(alpha: 0.1),
            borderRadius: BorderRadius.circular(6),
          ),
          child: Icon(
            icon,
            size: 14,
            color: isClose ? Colors.red.shade300 : Colors.white70,
          ),
        ),
      ),
    );
  }

  Widget _buildStatisticsBar() {
    final l10n = AppLocalizations.of(context);
    // final total = _statistics['total'] ?? 0;
    // In new definition, "Risk" label corresponds to Warned count (Audit Mode)
    // We display riskCount for the risk label
    final blockedCount = _statistics['blocked_count'] ?? 0;
    final riskCount = _statistics['risk_count'] ?? 0;
    final allowedCount = _statistics['allowed_count'] ?? 0;

    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
      decoration: BoxDecoration(
        color: Colors.black.withValues(alpha: 0.2),
        border: Border(
          bottom: BorderSide(color: Colors.white.withValues(alpha: 0.1)),
        ),
      ),
      child: Row(
        children: [
          _buildStatItem(
            l10n?.auditLogBlocked ?? 'Blocked',
            blockedCount,
            Colors.red,
          ),
          const SizedBox(width: 24),
          _buildStatItem(
            l10n?.auditLogRisk ?? 'Risk',
            riskCount,
            Colors.orange,
          ),
          const SizedBox(width: 24),
          _buildStatItem(
            l10n?.auditLogAllowed ?? 'Allowed',
            allowedCount,
            Colors.green,
          ),
        ],
      ),
    );
  }

  Widget _buildStatItem(String label, int value, Color color) {
    return Row(
      children: [
        Container(
          width: 8,
          height: 8,
          decoration: BoxDecoration(color: color, shape: BoxShape.circle),
        ),
        const SizedBox(width: 6),
        Text(
          '$label: ',
          style: AppFonts.inter(fontSize: 12, color: Colors.white54),
        ),
        Text(
          '$value',
          style: AppFonts.inter(
            fontSize: 12,
            fontWeight: FontWeight.w600,
            color: Colors.white,
          ),
        ),
      ],
    );
  }

  Widget _buildToolbar() {
    final l10n = AppLocalizations.of(context);
    return Container(
      padding: const EdgeInsets.all(12),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          SingleChildScrollView(
            scrollDirection: Axis.horizontal,
            child: Row(
              children: _assetTabs.map((tab) {
                final selected =
                    _selectedAssetName == tab.assetName &&
                    _selectedAssetID == tab.assetID;
                return Padding(
                  padding: const EdgeInsets.only(right: 8),
                  child: ChoiceChip(
                    label: Text(tab.label, style: AppFonts.inter(fontSize: 12)),
                    selected: selected,
                    onSelected: (_) => _onAssetTabChanged(tab),
                    selectedColor: const Color(
                      0xFF6366F1,
                    ).withValues(alpha: 0.3),
                    checkmarkColor: const Color(0xFF6366F1),
                  ),
                );
              }).toList(),
            ),
          ),
          const SizedBox(height: 10),
          Row(
            crossAxisAlignment: CrossAxisAlignment.center,
            children: [
              Expanded(
                child: TextField(
                  controller: _searchController,
                  textInputAction: TextInputAction.search,
                  onSubmitted: (_) => _submitSearch(),
                  style: AppFonts.inter(fontSize: 13, color: Colors.white),
                  minLines: 1,
                  maxLines: 1,
                  decoration: InputDecoration(
                    hintText:
                        '${l10n?.auditLogSearchHint ?? 'Search request, reply, risk, messages & tools...'} '
                        '${l10n?.auditLogSearchSubmitHint ?? 'Press Enter to search'}',
                    hintStyle: AppFonts.inter(
                      fontSize: 12,
                      color: Colors.white38,
                      height: 1.25,
                    ),
                    prefixIcon: Tooltip(
                      message:
                          l10n?.auditLogSearchTooltip ??
                          'Matches substrings in request body, output, risk reason, and raw messages/tool_calls JSON.',
                      child: const Icon(
                        LucideIcons.search,
                        size: 16,
                        color: Colors.white38,
                      ),
                    ),
                    suffixIcon: _searchSubmitLoading
                        ? const Padding(
                            padding: EdgeInsets.all(12),
                            child: SizedBox(
                              width: 18,
                              height: 18,
                              child: CircularProgressIndicator(
                                strokeWidth: 2,
                                color: Color(0xFF6366F1),
                              ),
                            ),
                          )
                        : null,
                    filled: true,
                    fillColor: Colors.black.withValues(alpha: 0.3),
                    border: OutlineInputBorder(
                      borderRadius: BorderRadius.circular(8),
                      borderSide: BorderSide.none,
                    ),
                    contentPadding: const EdgeInsets.symmetric(
                      horizontal: 12,
                      vertical: 12,
                    ),
                  ),
                ),
              ),
              const SizedBox(width: 12),
              FilterChip(
                label: Text(
                  l10n?.auditLogRiskOnly ?? 'Risk Only',
                  style: AppFonts.inter(fontSize: 12),
                ),
                selected: _riskOnly,
                onSelected: _onRiskFilterChanged,
                selectedColor: const Color(0xFF6366F1).withValues(alpha: 0.3),
                checkmarkColor: const Color(0xFF6366F1),
              ),
              if (_selectedLogsForExport.isNotEmpty) ...[
                const SizedBox(width: 12),
                Container(
                  padding: const EdgeInsets.symmetric(
                    horizontal: 10,
                    vertical: 8,
                  ),
                  decoration: BoxDecoration(
                    color: const Color(0xFF6366F1).withValues(alpha: 0.16),
                    borderRadius: BorderRadius.circular(8),
                    border: Border.all(
                      color: const Color(0xFF6366F1).withValues(alpha: 0.35),
                    ),
                  ),
                  child: Row(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Icon(
                        LucideIcons.checkSquare,
                        size: 14,
                        color: const Color(0xFF818CF8),
                      ),
                      const SizedBox(width: 6),
                      Text(
                        _isZh
                            ? '已选 ${_selectedLogsForExport.length} 条'
                            : '${_selectedLogsForExport.length} selected',
                        style: AppFonts.inter(
                          fontSize: 12,
                          fontWeight: FontWeight.w600,
                          color: Colors.white,
                        ),
                      ),
                      const SizedBox(width: 8),
                      GestureDetector(
                        onTap: _clearSelectedLogs,
                        child: Text(
                          _isZh ? '清空' : 'Clear',
                          style: AppFonts.inter(
                            fontSize: 12,
                            color: const Color(0xFF93C5FD),
                          ),
                        ),
                      ),
                    ],
                  ),
                ),
              ],
            ],
          ),
        ],
      ),
    );
  }

  Widget _buildLogList() {
    final l10n = AppLocalizations.of(context);
    if (_isLoading) {
      return const Center(
        child: CircularProgressIndicator(color: Color(0xFF6366F1)),
      );
    }

    if (_logs.isEmpty) {
      return Center(
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Icon(
              LucideIcons.fileText,
              size: 48,
              color: Colors.white.withValues(alpha: 0.2),
            ),
            const SizedBox(height: 16),
            Text(
              l10n?.auditLogNoLogs ?? 'No audit logs',
              style: AppFonts.inter(fontSize: 14, color: Colors.white54),
            ),
          ],
        ),
      );
    }

    return ListView.builder(
      controller: _scrollController,
      itemCount: _logs.length,
      padding: const EdgeInsets.symmetric(horizontal: 12),
      itemBuilder: (context, index) {
        final log = _logs[index];
        final isSelected = _selectedLog?.id == log.id;
        final isChecked = _isLogChecked(log);
        final offset = _currentPage * _pageSize + index;
        final serialNumber = math.max(1, _totalCount - offset);

        return Padding(
          padding: const EdgeInsets.only(bottom: 8),
          child: Row(
            crossAxisAlignment: CrossAxisAlignment.center,
            children: [
              GestureDetector(
                onTap: () => _toggleLogSelection(log),
                behavior: HitTestBehavior.opaque,
                child: Container(
                  width: 28,
                  height: 28,
                  alignment: Alignment.center,
                  decoration: BoxDecoration(
                    color: isChecked
                        ? const Color(0xFF6366F1).withValues(alpha: 0.24)
                        : Colors.white.withValues(alpha: 0.06),
                    borderRadius: BorderRadius.circular(8),
                    border: Border.all(
                      color: isChecked
                          ? const Color(0xFF6366F1).withValues(alpha: 0.7)
                          : Colors.white.withValues(alpha: 0.12),
                    ),
                  ),
                  child: Text(
                    '$serialNumber',
                    textAlign: TextAlign.center,
                    style: AppFonts.firaCode(
                      fontSize: 10,
                      fontWeight: isChecked ? FontWeight.w700 : FontWeight.w500,
                      color: isChecked
                          ? Colors.white
                          : Colors.white.withValues(alpha: 0.55),
                    ),
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                  ),
                ),
              ),
              const SizedBox(width: 8),
              Expanded(
                child: GestureDetector(
                  onTap: () => _selectLog(log),
                  behavior: HitTestBehavior.opaque,
                  child: Container(
                    padding: const EdgeInsets.all(12),
                    decoration: BoxDecoration(
                      color: isSelected
                          ? const Color(0xFF6366F1).withValues(alpha: 0.2)
                          : Colors.black.withValues(alpha: 0.3),
                      borderRadius: BorderRadius.circular(8),
                      border: Border.all(
                        color: isSelected
                            ? const Color(0xFF6366F1).withValues(alpha: 0.5)
                            : isChecked
                            ? const Color(0xFF6366F1).withValues(alpha: 0.25)
                            : Colors.white.withValues(alpha: 0.1),
                      ),
                    ),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Row(
                          children: [
                            _buildActionBadge(log.action),
                            const SizedBox(width: 8),
                            if (log.hasRisk) _buildRiskBadge(log.riskLevel),
                            if (isChecked) ...[
                              const SizedBox(width: 8),
                              Icon(
                                LucideIcons.checkSquare,
                                size: 12,
                                color: const Color(0xFF818CF8),
                              ),
                            ],
                            const Spacer(),
                            Text(
                              _formatTimestamp(log.timestamp),
                              style: AppFonts.firaCode(
                                fontSize: 10,
                                color: Colors.white38,
                              ),
                            ),
                            const SizedBox(width: 4),
                            IconButton(
                              icon: const Icon(LucideIcons.copy, size: 14),
                              color: Colors.white54,
                              tooltip:
                                  AppLocalizations.of(
                                    context,
                                  )?.appStoreGuideCopy ??
                                  '复制',
                              onPressed: () => _copyText(log.requestContent),
                            ),
                          ],
                        ),
                        const SizedBox(height: 8),
                        Text(
                          log.requestContent.length > 100
                              ? '${log.requestContent.substring(0, 100)}...'
                              : log.requestContent,
                          style: AppFonts.inter(
                            fontSize: 12,
                            color: Colors.white70,
                          ),
                          maxLines: 2,
                          overflow: TextOverflow.ellipsis,
                        ),
                        if (log.toolCalls.isNotEmpty) ...[
                          const SizedBox(height: 6),
                          Row(
                            children: [
                              Icon(
                                LucideIcons.wrench,
                                size: 12,
                                color: Colors.white38,
                              ),
                              const SizedBox(width: 4),
                              Text(
                                '${log.toolCalls.length} tool calls',
                                style: AppFonts.inter(
                                  fontSize: 11,
                                  color: Colors.white38,
                                ),
                              ),
                            ],
                          ),
                        ],
                      ],
                    ),
                  ),
                ),
              ),
            ],
          ),
        );
      },
    );
  }

  Widget _buildActionBadge(String action) {
    Color color;
    switch (action.toUpperCase()) {
      case 'BLOCK':
      case 'HARD_BLOCK':
        color = Colors.red;
        break;
      case 'WARN':
        color = Colors.orange;
        break;
      default:
        color = Colors.green;
    }

    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.2),
        borderRadius: BorderRadius.circular(4),
      ),
      child: Text(
        _getActionText(action),
        style: AppFonts.firaCode(
          fontSize: 10,
          fontWeight: FontWeight.w600,
          color: color,
        ),
      ),
    );
  }

  Widget _buildRiskBadge(String? riskLevel) {
    Color color;
    switch (riskLevel?.toUpperCase()) {
      case 'CRITICAL':
        color = Colors.red.shade900;
        break;
      case 'DANGEROUS':
        color = Colors.red;
        break;
      case 'SUSPICIOUS':
      case 'NEED_CONFIRMATION':
        color = Colors.orange;
        break;
      default:
        color = Colors.green;
    }

    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.2),
        borderRadius: BorderRadius.circular(4),
      ),
      child: Text(
        riskLevel ?? 'SAFE',
        style: AppFonts.firaCode(
          fontSize: 10,
          fontWeight: FontWeight.w600,
          color: color,
        ),
      ),
    );
  }

  String _buildRawSectionText(AuditLog log) {
    final rawMessages = _buildRawMessagesWithToolFallback(log);
    if (rawMessages.isEmpty) return log.requestContent;
    return rawMessages
        .map((item) {
          return '[${item.roleLabel}]\n${item.content}';
        })
        .join('\n\n');
  }

  String _buildActionSectionText(AuditLog log) => log.toolCalls
      .map(
        (tc) =>
            'Tool: ${tc.name}${tc.isSensitive ? " [SENSITIVE]" : ""}${tc.arguments.isNotEmpty ? "\nArguments: ${tc.arguments}" : ""}',
      )
      .join('\n\n');

  String _buildEventSectionText(List<SecurityEvent> events) => events
      .map((e) {
        final parts = ['[${e.eventType}] ${e.actionDesc}'];
        if (e.riskType.isNotEmpty) parts.add('Risk: ${e.riskType}');
        if (e.detail.isNotEmpty) parts.add('Detail: ${e.detail}');
        parts.add(
          'Source: ${e.source}  Time: ${e.timestamp.toIso8601String()}',
        );
        return parts.join('\n');
      })
      .join('\n\n');

  Future<void> _exportLogDetail() async {
    final log = _selectedLog;
    if (log == null) return;
    try {
      final safeId = _safeFileNameSegment(log.id);
      final fileName =
          'audit_log_${safeId}_${DateTime.now().millisecondsSinceEpoch}.md';

      final markdownContent = buildAuditLogMarkdownContent(
        isZh: _isZh,
        log: log,
        relatedEvents: _relatedEvents,
        rawText: _buildRawSectionText(log),
        actionText: _buildActionSectionText(log),
        eventText: _buildEventSectionText(_relatedEvents),
      );
      await _exportMarkdownContent(
        fileName: fileName,
        content: markdownContent,
        successMessageZh: '已导出到 {path}',
        successMessageEn: 'Exported to {path}',
      );
    } catch (e, st) {
      appLogger.error('[AuditLogWindow] Single export prepare failed', e, st);
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(_isZh ? '导出失败: $e' : 'Export failed: $e'),
          duration: const Duration(seconds: 3),
        ),
      );
    }
  }

  /// Bottom bar: totals, current page info, and pagination controls.
  Widget _buildPagination() {
    final l10n = AppLocalizations.of(context);
    final totalPages = _totalCount == 0
        ? 1
        : ((_totalCount + _pageSize - 1) ~/ _pageSize);
    final latestCount = _totalCount == 0
        ? 0
        : math.min((_currentPage + 1) * _pageSize, _totalCount);

    final countLine = _isZh
        ? '共$_totalCount条，最新$latestCount条'
        : '$_totalCount total, latest $latestCount';

    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
      decoration: BoxDecoration(
        border: Border(
          top: BorderSide(color: Colors.white.withValues(alpha: 0.1)),
        ),
      ),
      child: Row(
        children: [
          Expanded(
            child: Text(
              countLine,
              style: AppFonts.inter(fontSize: 12, color: Colors.white54),
              maxLines: 2,
              overflow: TextOverflow.ellipsis,
            ),
          ),
          IconButton(
            icon: const Icon(LucideIcons.chevronLeft, size: 16),
            color: _currentPage > 0 && _totalCount > 0
                ? Colors.white
                : Colors.white38,
            onPressed: _currentPage > 0 && _totalCount > 0
                ? () => _onPageChanged(_currentPage - 1)
                : null,
          ),
          Text(
            l10n?.auditLogPageInfo(_currentPage + 1, totalPages) ??
                'Page ${_currentPage + 1} of $totalPages',
            style: AppFonts.inter(fontSize: 12, color: Colors.white54),
          ),
          IconButton(
            icon: const Icon(LucideIcons.chevronRight, size: 16),
            color: _currentPage < totalPages - 1 && _totalCount > 0
                ? Colors.white
                : Colors.white38,
            onPressed: _currentPage < totalPages - 1 && _totalCount > 0
                ? () => _onPageChanged(_currentPage + 1)
                : null,
          ),
        ],
      ),
    );
  }

  /// Format timestamp using local time zone.
  String _formatTimestamp(DateTime timestamp) {
    final localTime = timestamp.toLocal();
    return '${localTime.year}-${localTime.month.toString().padLeft(2, '0')}-${localTime.day.toString().padLeft(2, '0')} '
        '${localTime.hour.toString().padLeft(2, '0')}:${localTime.minute.toString().padLeft(2, '0')}:${localTime.second.toString().padLeft(2, '0')}';
  }
}

class _AuditAssetFilterTab {
  final String label;
  final String assetName;
  final String assetID;

  const _AuditAssetFilterTab({
    required this.label,
    required this.assetName,
    required this.assetID,
  });
}

class _RawMessageItem {
  final String roleLabel;
  final String content;

  const _RawMessageItem({required this.roleLabel, required this.content});
}
