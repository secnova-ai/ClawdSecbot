import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:math' as math;
import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter/services.dart';
import 'package:file_picker/file_picker.dart';
import 'package:path_provider/path_provider.dart';
import '../utils/app_fonts.dart';
import 'package:lucide_icons/lucide_icons.dart';
import 'package:window_manager/window_manager.dart';
import '../l10n/app_localizations.dart';
import '../models/audit_log_model.dart';
import '../models/security_event_model.dart';
import '../services/audit_log_database_service.dart';
import '../services/security_event_database_service.dart';
import '../services/protection_service.dart';
import '../utils/audit_log_export_helper.dart';
import '../utils/app_logger.dart';
import '../utils/window_animation_helper.dart';
import '../widgets/hide_window_shortcut.dart';

const _appBackground = Color(0xFF0F0F23);

/// Audit Log Window App for multi-window support
class AuditLogWindowApp extends StatefulWidget {
  final String windowId;
  final String locale;
  final String initialAssetName;
  final String initialAssetID;

  const AuditLogWindowApp({
    super.key,
    required this.windowId,
    this.locale = 'en',
    this.initialAssetName = '',
    this.initialAssetID = '',
  });

  @override
  State<AuditLogWindowApp> createState() => _AuditLogWindowAppState();
}

class _AuditLogWindowAppState extends State<AuditLogWindowApp> {
  bool _isWindowShown = false;

  @override
  void initState() {
    super.initState();
    _showWindowAfterFirstFrame();
  }

  /// 在首帧完成栅格化后显示审计窗口，减少启动闪烁。
  void _showWindowAfterFirstFrame() {
    Future<void>(() async {
      await WidgetsBinding.instance.waitUntilFirstFrameRasterized;
      if (!mounted || _isWindowShown) return;
      _isWindowShown = true;
      await WindowAnimationHelper.showWithAnimation();
    });
  }

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Audit Log',
      debugShowCheckedModeBanner: false,
      locale: Locale(widget.locale),
      localizationsDelegates: const [
        AppLocalizations.delegate,
        GlobalMaterialLocalizations.delegate,
        GlobalWidgetsLocalizations.delegate,
        GlobalCupertinoLocalizations.delegate,
      ],
      supportedLocales: const [Locale('zh'), Locale('en')],
      theme: ThemeData(
        useMaterial3: true,
        colorScheme: ColorScheme.fromSeed(
          seedColor: const Color(0xFF6366F1),
          brightness: Brightness.dark,
        ),
        scaffoldBackgroundColor: _appBackground,
        textTheme: AppFonts.interTextTheme(ThemeData.dark().textTheme),
      ),
      // 在 MaterialApp 级别定义快捷键，确保全局生效
      shortcuts: {
        LogicalKeySet(LogicalKeyboardKey.meta, LogicalKeyboardKey.keyW):
            const HideWindowIntent(),
      },
      actions: {
        HideWindowIntent: CallbackAction<HideWindowIntent>(
          onInvoke: (_) {
            WindowAnimationHelper.hideWithAnimation();
            return null;
          },
        ),
      },
      home: AuditLogWindow(
        windowId: widget.windowId,
        initialAssetName: widget.initialAssetName,
        initialAssetID: widget.initialAssetID,
      ),
    );
  }
}

/// Audit Log Window
class AuditLogWindow extends StatefulWidget {
  final String windowId;
  final String initialAssetName;
  final String initialAssetID;

  const AuditLogWindow({
    super.key,
    required this.windowId,
    this.initialAssetName = '',
    this.initialAssetID = '',
  });

  @override
  State<AuditLogWindow> createState() => _AuditLogWindowState();
}

class _AuditLogWindowState extends State<AuditLogWindow> with WindowListener {
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
    _selectedAssetName = widget.initialAssetName.trim();
    _selectedAssetID = widget.initialAssetID.trim();
    try {
      windowManager.addListener(this);
      if (Platform.isLinux || Platform.isWindows) {
        windowManager.setPreventClose(true).catchError((_) {});
      }
      if (Platform.isWindows) {
        _syncMaximizedState();
      }
    } catch (_) {
      // 子窗口 window_manager 可能未完全注册，降级处理
    }
    _loadLogs();
    _loadStatistics();
    _loadAssetTabs();
    // Auto refresh every 5 seconds
    _refreshTimer = Timer.periodic(const Duration(seconds: 5), (_) {
      _syncAndRefresh();
    });
  }

  @override
  Future<void> onWindowClose() async {
    if (Platform.isLinux || Platform.isWindows) {
      await WindowAnimationHelper.hideWithAnimation();
      return;
    }
  }

  @override
  void onWindowFocus() {
    if (Platform.isWindows) {
      _syncMaximizedState();
    }
  }

  @override
  void onWindowMaximize() {
    if (!Platform.isWindows || !mounted) return;
    setState(() => _isMaximized = true);
  }

  @override
  void onWindowUnmaximize() {
    if (!Platform.isWindows || !mounted) return;
    setState(() => _isMaximized = false);
  }

  @override
  void dispose() {
    try {
      windowManager.removeListener(this);
    } catch (_) {}
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

  Future<void> _syncMaximizedState() async {
    try {
      final maximized = await windowManager.isMaximized();
      if (!mounted || _isMaximized == maximized) return;
      setState(() => _isMaximized = maximized);
    } catch (_) {}
  }

  Future<void> _toggleMaximize() async {
    try {
      final maximized = await windowManager.isMaximized();
      if (maximized) {
        await windowManager.unmaximize();
      } else {
        await windowManager.maximize();
      }
      if (mounted) {
        setState(() => _isMaximized = !maximized);
      }
    } catch (_) {}
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

  Future<void> _exportSelectedLogs() async {
    if (_selectedLogsForExport.isEmpty) return;
    final l10n = AppLocalizations.of(context);
    try {
      final selectedLogs = _selectedLogsForExport.values.toList()
        ..sort((a, b) {
          final byTime = b.timestamp.compareTo(a.timestamp);
          if (byTime != 0) return byTime;
          return b.requestId.compareTo(a.requestId);
        });

      final outputPath = await _resolveExportPath(
        fileName:
            'audit_logs_batch_${DateTime.now().millisecondsSinceEpoch}.md',
      );
      if (outputPath == null || outputPath.trim().isEmpty) return;

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

      final file = File(outputPath);
      await file.writeAsString(
        buffer.toString(),
        encoding: const Utf8Codec(allowMalformed: true),
        flush: true,
      );
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(
            _isZh
                ? '已批量导出 ${selectedLogs.length} 条日志到 ${file.path}'
                : 'Exported ${selectedLogs.length} logs to ${file.path}',
          ),
          duration: const Duration(seconds: 4),
        ),
      );
    } catch (e, st) {
      appLogger.error('[AuditLogWindow] Batch export failed', e, st);
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
    final isWindows = Platform.isWindows;
    final isLinux = Platform.isLinux;

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
                      try {
                        windowManager.startDragging();
                      } catch (_) {}
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
          if (Platform.isWindows) ...[
            const SizedBox(width: 8),
            _buildWindowButton(
              icon: LucideIcons.minus,
              onTap: () => windowManager.minimize(),
            ),
            const SizedBox(width: 8),
            _buildWindowButton(
              icon: _isMaximized ? Icons.filter_none : Icons.crop_square,
              onTap: _toggleMaximize,
            ),
            const SizedBox(width: 8),
            _buildWindowButton(
              icon: LucideIcons.x,
              onTap: () => WindowAnimationHelper.hideWithAnimation(),
              isClose: true,
            ),
          ],
          if (Platform.isMacOS) ...[
            const SizedBox(width: 8),
            _buildWindowButton(
              icon: LucideIcons.minus,
              onTap: () => windowManager.minimize(),
            ),
            const SizedBox(width: 8),
            _buildWindowButton(
              icon: LucideIcons.x,
              onTap: () => WindowAnimationHelper.hideWithAnimation(),
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
    if (log.messages.isEmpty) return log.requestContent;
    return _buildRawMessagesWithToolFallback(log)
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
    final l10n = AppLocalizations.of(context);
    try {
      final safeId = _safeFileNameSegment(log.id);
      final outputPath = await _resolveExportPath(
        fileName:
            'audit_log_${safeId}_${DateTime.now().millisecondsSinceEpoch}.md',
      );
      if (outputPath == null || outputPath.trim().isEmpty) return;

      final markdownContent = buildAuditLogMarkdownContent(
        isZh: _isZh,
        log: log,
        relatedEvents: _relatedEvents,
        rawText: _buildRawSectionText(log),
        actionText: _buildActionSectionText(log),
        eventText: _buildEventSectionText(_relatedEvents),
      );
      final file = File(outputPath);
      await file.writeAsString(
        markdownContent,
        encoding: const Utf8Codec(allowMalformed: true),
        flush: true,
      );
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(
            _isZh ? '已导出到 ${file.path}' : 'Exported to ${file.path}',
          ),
          duration: const Duration(seconds: 4),
        ),
      );
    } catch (e, st) {
      appLogger.error('[AuditLogWindow] Single export failed', e, st);
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

  /// 解析导出路径：优先使用文件选择对话框，失败时自动降级到本地默认目录。
  /// 该兜底用于 Linux 未安装 zenity 等依赖时，避免导出直接失败。
  Future<String?> _resolveExportPath({required String fileName}) async {
    final normalizedFileName = _ensureMarkdownExtension(fileName.trim());
    try {
      final savePath = await FilePicker.platform.saveFile(
        dialogTitle: _isZh ? '选择导出位置' : 'Choose export location',
        fileName: normalizedFileName,
        type: FileType.custom,
        allowedExtensions: const ['md'],
      );
      if (savePath == null || savePath.trim().isEmpty) {
        return null;
      }
      return _ensureMarkdownExtension(savePath.trim());
    } catch (e, st) {
      appLogger.warning(
        '[AuditLogWindow] save dialog unavailable, fallback to local path: $e',
      );
      appLogger.debug('[AuditLogWindow] save dialog stacktrace: $st');
      final fallbackDir = await _resolveFallbackExportDirectory();
      if (fallbackDir == null) {
        return null;
      }
      return _ensureMarkdownExtension(
        '${fallbackDir.path}${Platform.pathSeparator}$normalizedFileName',
      );
    }
  }

  /// 获取导出兜底目录：优先下载目录，其次主目录，最后应用文档目录。
  Future<Directory?> _resolveFallbackExportDirectory() async {
    try {
      final downloads = await getDownloadsDirectory();
      if (downloads != null) {
        await downloads.create(recursive: true);
        return downloads;
      }
    } catch (e) {
      appLogger.warning(
        '[AuditLogWindow] getDownloadsDirectory failed: $e',
      );
    }

    try {
      final home = Platform.environment['HOME'];
      if (home != null && home.trim().isNotEmpty) {
        final dir = Directory(home.trim());
        if (await dir.exists()) {
          return dir;
        }
      }
    } catch (e) {
      appLogger.warning('[AuditLogWindow] resolve HOME failed: $e');
    }

    try {
      final docs = await getApplicationDocumentsDirectory();
      await docs.create(recursive: true);
      return docs;
    } catch (e) {
      appLogger.error(
        '[AuditLogWindow] resolve fallback directory failed',
        e,
      );
      return null;
    }
  }

  /// 将导出文件名中的非法字符替换为下划线，避免跨平台写文件失败。
  String _safeFileNameSegment(String input) {
    final normalized = input.trim();
    if (normalized.isEmpty) return 'log';
    return normalized.replaceAll(RegExp(r'[\\/:*?"<>|]'), '_');
  }

  /// 确保导出路径包含 .md 扩展名，避免部分平台下文件类型不一致。
  String _ensureMarkdownExtension(String path) {
    if (path.toLowerCase().endsWith('.md')) {
      return path;
    }
    return '$path.md';
  }

  List<_RawMessageItem> _buildRawMessagesWithToolFallback(AuditLog log) {
    int toolIndex = 0;
    return log.messages.map((msg) {
      final roleLabel = msg.role.isNotEmpty
          ? '${msg.role[0].toUpperCase()}${msg.role.substring(1)}'
          : 'Unknown';
      var content = msg.content.trim();
      if (msg.role.toLowerCase() == 'assistant' && content.isEmpty) {
        if (toolIndex < log.toolCalls.length) {
          final tc = log.toolCalls[toolIndex];
          final args = tc.arguments.trim().isNotEmpty ? tc.arguments : '{}';
          content = _isZh
              ? '请求工具: ${tc.name}\n参数:\n$args'
              : 'Tool request: ${tc.name}\nArguments:\n$args';
        } else {
          content = _isZh ? '(空内容)' : '(empty content)';
        }
        toolIndex++;
      }
      return _RawMessageItem(roleLabel: roleLabel, content: content);
    }).toList();
  }

  Widget _buildLogDetail() {
    final l10n = AppLocalizations.of(context);
    final log = _selectedLog!;

    return Container(
      margin: const EdgeInsets.fromLTRB(12, 0, 12, 12),
      decoration: BoxDecoration(
        color: Colors.black.withValues(alpha: 0.3),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: Colors.white.withValues(alpha: 0.1)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Container(
            padding: const EdgeInsets.all(16),
            decoration: BoxDecoration(
              border: Border(
                bottom: BorderSide(color: Colors.white.withValues(alpha: 0.1)),
              ),
            ),
            child: Row(
              children: [
                Text(
                  l10n?.auditLogDetail ?? 'Log Detail',
                  style: AppFonts.inter(
                    fontSize: 14,
                    fontWeight: FontWeight.w600,
                    color: Colors.white,
                  ),
                ),
                const Spacer(),
                Tooltip(
                  message: l10n?.auditLogExport ?? 'Export',
                  child: IconButton(
                    icon: const Icon(LucideIcons.download, size: 16),
                    color: Colors.white54,
                    onPressed: _exportLogDetail,
                  ),
                ),
                IconButton(
                  icon: const Icon(LucideIcons.x, size: 16),
                  color: Colors.white54,
                  onPressed: () => _selectLog(null),
                ),
              ],
            ),
          ),
          Expanded(
            child: SingleChildScrollView(
              padding: const EdgeInsets.all(16),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  _buildDetailRow(l10n?.auditLogId ?? 'ID', log.id),
                  _buildDetailRow(
                    l10n?.auditLogTimestamp ?? 'Timestamp',
                    _formatTimestamp(log.timestamp),
                  ),
                  _buildDetailRow(
                    l10n?.auditLogRequestId ?? 'Request ID',
                    log.requestId,
                  ),
                  if (log.model != null)
                    _buildDetailRow(l10n?.auditLogModel ?? 'Model', log.model!),
                  _buildDetailRow(
                    l10n?.auditLogAction ?? 'Action',
                    _getActionText(log.action),
                  ),
                  if (log.hasRisk) ...[
                    _buildDetailRow(
                      l10n?.auditLogRiskLevel ?? 'Risk Level',
                      log.riskLevel ?? 'N/A',
                    ),
                    _buildDetailRow(
                      l10n?.auditLogRiskReason ?? 'Risk Reason',
                      log.riskReason ?? 'N/A',
                    ),
                  ],
                  _buildDetailRow(
                    l10n?.auditLogDuration ?? 'Duration',
                    '${log.durationMs}ms',
                  ),
                  if (log.totalTokens != null)
                    _buildDetailRow(
                      l10n?.auditLogTokens ?? 'Tokens',
                      '${log.totalTokens}',
                    ),

                  const SizedBox(height: 16),
                  _buildSectionTitleWithCopy(
                    _isZh
                        ? '原始${log.messages.isNotEmpty ? " (${log.messages.length})" : ""}'
                        : 'Raw${log.messages.isNotEmpty ? " (${log.messages.length})" : ""}',
                    _buildRawSectionText(log),
                  ),
                  if (log.messages.isNotEmpty) ...[
                    const SizedBox(height: 8),
                    ..._buildRawMessagesWithToolFallback(log).map((item) {
                      return _buildConversationMessage(
                        item.roleLabel,
                        item.content,
                      );
                    }),
                  ] else ...[
                    _buildCodeBlock(log.requestContent),
                  ],

                  if (log.toolCalls.isNotEmpty) ...[
                    const SizedBox(height: 16),
                    _buildSectionTitleWithCopy(
                      _isZh
                          ? '动作 (${log.toolCalls.length})'
                          : 'Actions (${log.toolCalls.length})',
                      _buildActionSectionText(log),
                    ),
                    ...log.toolCalls.map((tc) => _buildToolCallItem(tc)),
                  ],

                  const SizedBox(height: 16),
                  _buildSectionTitleWithCopy(
                    _isZh
                        ? '事件${_relatedEvents.isNotEmpty ? " (${_relatedEvents.length})" : ""}'
                        : 'Events${_relatedEvents.isNotEmpty ? " (${_relatedEvents.length})" : ""}',
                    _relatedEvents.isNotEmpty
                        ? _buildEventSectionText(_relatedEvents)
                        : '',
                  ),
                  if (_relatedEvents.isNotEmpty)
                    ..._relatedEvents.map((evt) => _buildSecurityEventCard(evt))
                  else
                    Padding(
                      padding: const EdgeInsets.only(top: 4),
                      child: Text(
                        _isZh ? '暂无关联安全事件' : 'No related security events',
                        style: AppFonts.inter(
                          fontSize: 12,
                          color: Colors.white38,
                        ),
                      ),
                    ),
                ],
              ),
            ),
          ),
        ],
      ),
    );
  }

  /// 构建单个安全事件卡片
  Widget _buildSecurityEventCard(SecurityEvent evt) {
    final blocked = evt.isBlocked;
    final accent = blocked ? Colors.red : Colors.amber;
    final typeLabel = switch (evt.eventType) {
      'blocked' => 'BLOCKED',
      'tool_execution' => 'TOOL',
      _ => evt.eventType.toUpperCase(),
    };
    return Container(
      margin: const EdgeInsets.only(bottom: 8),
      padding: const EdgeInsets.all(10),
      decoration: BoxDecoration(
        color: accent.withValues(alpha: blocked ? 0.1 : 0.05),
        borderRadius: BorderRadius.circular(8),
        border: Border.all(color: accent.withValues(alpha: 0.3)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Icon(
                blocked ? LucideIcons.shieldAlert : LucideIcons.alertTriangle,
                size: 14,
                color: accent,
              ),
              const SizedBox(width: 6),
              Container(
                padding: const EdgeInsets.symmetric(horizontal: 5, vertical: 1),
                decoration: BoxDecoration(
                  color: accent.withValues(alpha: 0.2),
                  borderRadius: BorderRadius.circular(4),
                ),
                child: Text(
                  typeLabel,
                  style: AppFonts.firaCode(
                    fontSize: 9,
                    fontWeight: FontWeight.w600,
                    color: accent,
                  ),
                ),
              ),
              if (evt.riskType.isNotEmpty) ...[
                const SizedBox(width: 6),
                Flexible(
                  child: Text(
                    evt.riskType,
                    style: AppFonts.firaCode(
                      fontSize: 10,
                      color: Colors.white54,
                    ),
                    overflow: TextOverflow.ellipsis,
                  ),
                ),
              ],
              const Spacer(),
              Text(
                evt.source,
                style: AppFonts.firaCode(fontSize: 9, color: Colors.white38),
              ),
            ],
          ),
          const SizedBox(height: 6),
          SelectableText(
            evt.actionDesc,
            style: AppFonts.inter(fontSize: 11, color: Colors.white70),
          ),
          if (evt.detail.isNotEmpty) ...[
            const SizedBox(height: 4),
            SelectableText(
              evt.detail,
              style: AppFonts.firaCode(fontSize: 10, color: Colors.white38),
            ),
          ],
        ],
      ),
    );
  }

  String _getActionText(String action) {
    final l10n = AppLocalizations.of(context);
    switch (action.toUpperCase()) {
      case 'BLOCK':
      case 'HARD_BLOCK':
        return l10n?.auditLogActionBlock ?? 'Blocked';
      case 'WARN':
        return l10n?.auditLogActionWarn ?? 'Risk';
      case 'ALLOW':
      default:
        return l10n?.auditLogActionAllow ?? 'Allowed';
    }
  }

  Widget _buildDetailRow(String label, String value) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 8),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          SizedBox(
            width: 100,
            child: Text(
              label,
              style: AppFonts.inter(fontSize: 12, color: Colors.white54),
            ),
          ),
          Expanded(
            child: Text(
              value,
              style: AppFonts.firaCode(fontSize: 12, color: Colors.white),
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildSectionTitleWithCopy(String title, String copyText) {
    final l10n = AppLocalizations.of(context);
    return Padding(
      padding: const EdgeInsets.only(bottom: 8),
      child: Row(
        children: [
          Text(
            title,
            style: AppFonts.inter(
              fontSize: 13,
              fontWeight: FontWeight.w600,
              color: const Color(0xFF6366F1),
            ),
          ),
          const SizedBox(width: 8),
          IconButton(
            icon: const Icon(LucideIcons.copy, size: 16),
            color: Colors.white54,
            tooltip: l10n?.appStoreGuideCopy ?? '复制',
            onPressed: () => _copyText(copyText),
          ),
        ],
      ),
    );
  }

  Widget _buildCodeBlock(String content) {
    return Container(
      width: double.infinity,
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: Colors.black.withValues(alpha: 0.5),
        borderRadius: BorderRadius.circular(8),
      ),
      child: SelectableText(
        content,
        style: AppFonts.firaCode(fontSize: 11, color: Colors.white70),
      ),
    );
  }

  bool get _isZh =>
      (AppLocalizations.of(context)?.localeName ?? '').startsWith('zh');

  /// 构建单条对话消息展示（审计详情中的完整对话区）
  Widget _buildConversationMessage(String roleLabel, String content) {
    final Color roleColor;
    switch (roleLabel.toLowerCase()) {
      case 'user':
        roleColor = const Color(0xFF22C55E);
        break;
      case 'assistant':
        roleColor = const Color(0xFF6366F1);
        break;
      case 'tool':
        roleColor = const Color(0xFFEC4899);
        break;
      default:
        roleColor = Colors.white70;
    }
    return Container(
      margin: const EdgeInsets.only(bottom: 6),
      padding: const EdgeInsets.all(10),
      decoration: BoxDecoration(
        color: Colors.black.withValues(alpha: 0.3),
        borderRadius: BorderRadius.circular(6),
        border: Border.all(color: roleColor.withValues(alpha: 0.2)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            roleLabel,
            style: AppFonts.inter(
              fontSize: 11,
              fontWeight: FontWeight.w600,
              color: roleColor,
            ),
          ),
          const SizedBox(height: 4),
          SelectableText(
            content,
            style: AppFonts.firaCode(fontSize: 11, color: Colors.white70),
          ),
        ],
      ),
    );
  }

  Widget _buildToolCallItem(AuditToolCall tc) {
    final l10n = AppLocalizations.of(context);
    return Container(
      margin: const EdgeInsets.only(bottom: 8),
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: tc.isSensitive
            ? Colors.red.withValues(alpha: 0.1)
            : Colors.black.withValues(alpha: 0.3),
        borderRadius: BorderRadius.circular(8),
        border: Border.all(
          color: tc.isSensitive
              ? Colors.red.withValues(alpha: 0.3)
              : Colors.white.withValues(alpha: 0.1),
        ),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Icon(
                LucideIcons.wrench,
                size: 14,
                color: tc.isSensitive ? Colors.red : const Color(0xFF6366F1),
              ),
              const SizedBox(width: 6),
              Text(
                tc.name,
                style: AppFonts.firaCode(
                  fontSize: 12,
                  fontWeight: FontWeight.w600,
                  color: Colors.white,
                ),
              ),
              if (tc.isSensitive) ...[
                const SizedBox(width: 8),
                Container(
                  padding: const EdgeInsets.symmetric(
                    horizontal: 4,
                    vertical: 1,
                  ),
                  decoration: BoxDecoration(
                    color: Colors.red.withValues(alpha: 0.2),
                    borderRadius: BorderRadius.circular(4),
                  ),
                  child: Text(
                    l10n?.auditLogSensitive ?? 'SENSITIVE',
                    style: AppFonts.firaCode(fontSize: 9, color: Colors.red),
                  ),
                ),
              ],
            ],
          ),
          if (tc.arguments.isNotEmpty) ...[
            const SizedBox(height: 8),
            Row(
              children: [
                Text(
                  '${l10n?.auditLogToolArguments ?? "Arguments"}:',
                  style: AppFonts.inter(fontSize: 10, color: Colors.white54),
                ),
                const SizedBox(width: 6),
                IconButton(
                  icon: const Icon(LucideIcons.copy, size: 14),
                  color: Colors.white54,
                  tooltip: l10n?.appStoreGuideCopy ?? '复制',
                  onPressed: () => _copyText(tc.arguments),
                ),
              ],
            ),
            const SizedBox(height: 4),
            _buildCodeBlock(tc.arguments),
          ],
          if (tc.result != null && tc.result!.isNotEmpty) ...[
            const SizedBox(height: 8),
            Row(
              children: [
                Text(
                  '${l10n?.auditLogToolResult ?? "Result"}:',
                  style: AppFonts.inter(fontSize: 10, color: Colors.white54),
                ),
                const SizedBox(width: 6),
                IconButton(
                  icon: const Icon(LucideIcons.copy, size: 14),
                  color: Colors.white54,
                  tooltip: l10n?.appStoreGuideCopy ?? '复制',
                  onPressed: () => _copyText(tc.result!),
                ),
              ],
            ),
            const SizedBox(height: 4),
            _buildCodeBlock(tc.result!),
          ],
        ],
      ),
    );
  }

  /// 底栏: 总条数、当前页条目区间与翻页控制(单页时按钮禁用但仍展示数量).
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
