import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter_animate/flutter_animate.dart';
import '../utils/app_fonts.dart';
import 'package:lucide_icons/lucide_icons.dart';
import '../models/skill_scan_result_model.dart';
import '../services/skill_security_analyzer_service.dart';
import '../l10n/app_localizations.dart';
import '../utils/app_logger.dart';

class SkillScanDialog extends StatefulWidget {
  final String? assetName;

  const SkillScanDialog({super.key, this.assetName});

  @override
  State<SkillScanDialog> createState() => _SkillScanDialogState();
}

class _SkillScanDialogState extends State<SkillScanDialog> {
  final _service = SkillSecurityAnalyzerService();
  final _scrollController = ScrollController();
  final List<String> _logs = [];
  StreamSubscription<String>? _logSubscription;
  StreamSubscription<BatchScanProgress>? _progressSubscription;

  int _currentIndex = 0;
  int _totalCount = 0;
  String _currentSkill = '';
  bool _scanning = false;
  bool _completed = false;
  String? _error;
  String? _batchID;

  // 从 Go 层批量扫描结果中解析
  final Map<String, SkillAnalysisResult?> _results = {};
  final Map<String, bool> _deleteConfirmed = {};
  final Map<String, bool> _trustConfirmed = {};
  final Map<String, String> _failedSkills = {};
  // Record skill paths for delete operation
  final Map<String, String> _skillPaths = {};
  // Record skill hashes for trust/delete operation
  final Map<String, String> _skillHashes = {};
  // Track which skill cards have expanded issues
  final Set<String> _expandedSkills = {};

  bool _initialized = false;

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    if (!_initialized) {
      _initialized = true;
      _startBatchScan();
    }
  }

  Future<void> _startBatchScan() async {
    setState(() {
      _scanning = true;
      _logs.clear();
    });

    _logSubscription = _service.logStream.listen((log) {
      if (_shouldShowLog(log)) {
        setState(() {
          _logs.add(log);
        });
        _scrollToBottom();
      }
    });

    _progressSubscription = _service.progressStream.listen((progress) {
      setState(() {
        _currentIndex = progress.currentIndex;
        _totalCount = progress.total;
        _currentSkill = progress.currentSkill;
        if (progress.completed) {
          _onBatchCompleted();
        }
      });
    });

    try {
      final result = await _service.startBatchScan(widget.assetName);
      if (result['success'] != true) {
        final errorMsg = result['error'] as String? ?? 'Unknown error';
        if (result['message'] == 'no skills to scan') {
          // 没有需要扫描的技能
          setState(() {
            _scanning = false;
            _completed = true;
            _totalCount = 0;
          });
          return;
        }
        setState(() {
          _scanning = false;
          _error = errorMsg;
        });
        return;
      }

      // Handle "no skills to scan" when success == true but total == 0
      if (result['total'] == 0) {
        setState(() {
          _scanning = false;
          _completed = true;
          _totalCount = 0;
        });
        return;
      }

      _batchID = result['batch_id'] as String;
      _totalCount = result['total'] as int? ?? 0;
    } catch (e) {
      if (!mounted) return;
      final l10n = AppLocalizations.of(context);
      setState(() {
        _scanning = false;
        _error =
            l10n?.skillScanFailedLoadConfig(e.toString()) ??
            'Failed to start scan: $e';
      });
    }
  }

  void _onBatchCompleted() {
    if (_batchID == null || _completed) return;
    _fetchBatchResults();
  }

  Future<void> _fetchBatchResults() async {
    if (_batchID == null) return;

    try {
      final response = await _service.getBatchScanResults(
        _batchID!,
        widget.assetName,
      );
      if (response['success'] == true && response['results'] != null) {
        final results = response['results'] as Map<String, dynamic>;
        for (final entry in results.entries) {
          final skillName = entry.key;
          final data = entry.value as Map<String, dynamic>;
          final skillPath = data['skill_path'] as String? ?? '';
          final batchSkillHash = data['skill_hash'] as String? ?? '';
          _skillPaths[skillName] = skillPath;
          if (batchSkillHash.isNotEmpty) {
            _skillHashes[skillName] = batchSkillHash;
          }

          if (data['success'] == true && data['result'] != null) {
            final resultMap = data['result'] as Map<String, dynamic>;
            final skillHash = resultMap['skill_hash'] as String? ?? '';
            if (skillHash.isNotEmpty) {
              _skillHashes[skillName] = skillHash;
            }
            final analysisResult = SkillAnalysisResult.fromJson(resultMap);
            _results[skillName] = analysisResult;
          } else if (data['error'] != null) {
            _failedSkills[skillName] = data['error'] as String;
          }
        }
      }
    } catch (e) {
      // 结果获取失败不阻塞 UI
    }

    if (mounted) {
      setState(() {
        _scanning = false;
        _completed = true;
      });
    }
  }

  void _scrollToBottom() {
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (_scrollController.hasClients) {
        _scrollController.animateTo(
          _scrollController.position.maxScrollExtent,
          duration: const Duration(milliseconds: 100),
          curve: Curves.easeOut,
        );
      }
    });
  }

  Future<void> _deleteSkill(String skillName) async {
    final skillPath = _skillPaths[skillName];
    final skillHash = _skillHashes[skillName];
    if (skillPath == null ||
        skillPath.isEmpty ||
        skillHash == null ||
        skillHash.isEmpty) {
      appLogger.warning(
        '[SkillScanDialog] Missing skillPath/skillHash for delete: skill=$skillName, hasPath=${(skillPath ?? '').isNotEmpty}, hasHash=${(skillHash ?? '').isNotEmpty}',
      );
      if (mounted) {
        final l10n = AppLocalizations.of(context)!;
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text(l10n.deleteRiskSkillUnavailable)),
        );
      }
      return;
    }
    final deleteResult = await _service.deleteSkill(
      skillPath: skillPath,
      skillHash: skillHash,
    );
    if (deleteResult.success) {
      setState(() {
        _deleteConfirmed[skillName] = true;
      });
      if (mounted && deleteResult.alreadyMissing) {
        final l10n = AppLocalizations.of(context)!;
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text(l10n.deleteRiskSkillAlreadyMissing)),
        );
      }
      return;
    }

    if (mounted) {
      final l10n = AppLocalizations.of(context)!;
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(SnackBar(content: Text(l10n.skillScanFailed)));
    }
  }

  Future<void> _trustSkill(String skillName) async {
    final skillHash = _skillHashes[skillName];
    if (skillHash == null || skillHash.isEmpty) return;
    final success = await _service.trustSkill(skillHash);
    if (success) {
      setState(() {
        _trustConfirmed[skillName] = true;
      });
    }
  }

  @override
  void dispose() {
    _logSubscription?.cancel();
    _progressSubscription?.cancel();
    _scrollController.dispose();
    _service.dispose();
    super.dispose();
  }

  // Filter logs to hide raw JSON/Markdown content during scanning
  bool _shouldShowLog(String log) {
    // Filter out JSON blocks
    if (log.trim().startsWith('{') || log.trim().startsWith('}')) return false;
    if (log.trim().startsWith('"') && log.contains(':')) return false;
    // Filter out markdown code blocks
    if (log.contains('```')) return false;
    // Filter out Analysis Complete markers
    if (log.contains('Analysis Complete')) return false;
    // Filter out pure separator lines
    if (log.trim() == '---') return false;
    // Filter out very long lines (likely raw LLM output)
    if (log.length > 500) return false;
    return true;
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context)!;

    return Dialog(
      backgroundColor: const Color(0xFF1A1A2E),
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
      child: Container(
        width: 700,
        height: 600,
        padding: const EdgeInsets.all(24),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            _buildHeader(l10n),
            const SizedBox(height: 16),
            _buildProgress(),
            const SizedBox(height: 16),
            Expanded(
              child: _completed ? _buildResultsPanel() : _buildLogArea(),
            ),
            const SizedBox(height: 16),
            _buildActions(l10n),
          ],
        ),
      ),
    );
  }

  Widget _buildHeader(AppLocalizations l10n) {
    return Row(
      children: [
        Container(
              padding: const EdgeInsets.all(8),
              decoration: BoxDecoration(
                color: const Color(0xFF6366F1).withValues(alpha: 0.2),
                borderRadius: BorderRadius.circular(8),
              ),
              child: Icon(
                _scanning ? LucideIcons.loader2 : LucideIcons.shieldCheck,
                color: const Color(0xFF6366F1),
                size: 20,
              ),
            )
            .animate(onPlay: (c) => _scanning ? c.repeat() : null)
            .rotate(duration: 1000.ms),
        const SizedBox(width: 12),
        Text(
          l10n.skillScanTitle,
          style: AppFonts.inter(
            fontSize: 18,
            fontWeight: FontWeight.w600,
            color: Colors.white,
          ),
        ),
        const Spacer(),
        if (!_scanning)
          IconButton(
            icon: const Icon(LucideIcons.x, color: Colors.white54, size: 20),
            onPressed: () => Navigator.of(context).pop(_hasRisks),
          ),
      ],
    );
  }

  Widget _buildProgress() {
    final l10n = AppLocalizations.of(context)!;
    final progress = _totalCount > 0
        ? (_currentIndex + (_completed ? 0 : 0)) / _totalCount
        : 0.0;
    // 完成时进度满格
    final displayProgress = _completed ? 1.0 : progress;

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          mainAxisAlignment: MainAxisAlignment.spaceBetween,
          children: [
            Text(
              _scanning
                  ? '${l10n.skillScanScanning}: ${_currentSkill.isNotEmpty ? _currentSkill : "..."}'
                  : _completed
                  ? l10n.skillScanCompleted
                  : l10n.skillScanPreparing,
              style: AppFonts.inter(fontSize: 13, color: Colors.white70),
            ),
            Text(
              _totalCount > 0
                  ? '${_completed ? _totalCount : _currentIndex} / $_totalCount'
                  : '',
              style: AppFonts.firaCode(fontSize: 12, color: Colors.white54),
            ),
          ],
        ),
        const SizedBox(height: 8),
        ClipRRect(
          borderRadius: BorderRadius.circular(4),
          child: LinearProgressIndicator(
            value: displayProgress,
            backgroundColor: Colors.white.withValues(alpha: 0.1),
            valueColor: AlwaysStoppedAnimation<Color>(
              _completed
                  ? (_hasRisks
                        ? const Color(0xFFEF4444)
                        : const Color(0xFF22C55E))
                  : const Color(0xFF6366F1),
            ),
            minHeight: 6,
          ),
        ),
      ],
    );
  }

  Widget _buildLogArea() {
    if (_error != null) {
      return Center(
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            const Icon(LucideIcons.alertCircle, color: Colors.red, size: 48),
            const SizedBox(height: 16),
            Text(
              _error!,
              style: AppFonts.inter(fontSize: 14, color: Colors.red),
              textAlign: TextAlign.center,
            ),
          ],
        ),
      );
    }

    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: Colors.black.withValues(alpha: 0.3),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: Colors.white.withValues(alpha: 0.1)),
      ),
      child: ListView.builder(
        controller: _scrollController,
        itemCount: _logs.length,
        itemBuilder: (context, index) {
          final log = _logs[index];
          Color textColor = Colors.white70;

          if (log.startsWith('\u26a0\ufe0f') || log.contains('RISK')) {
            textColor = const Color(0xFFEF4444);
          } else if (log.startsWith('\u2705')) {
            textColor = const Color(0xFF22C55E);
          } else if (log.startsWith('\u274c') || log.startsWith('Error')) {
            textColor = Colors.red;
          } else if (log.startsWith('[Tool')) {
            textColor = const Color(0xFF6366F1);
          }

          return Padding(
            padding: const EdgeInsets.symmetric(vertical: 2),
            child: Text(
              log,
              style: AppFonts.firaCode(fontSize: 11, color: textColor),
            ),
          );
        },
      ),
    );
  }

  Widget _buildResultsPanel() {
    final l10n = AppLocalizations.of(context)!;

    // No skills scanned case
    if (_totalCount == 0) {
      return Center(
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Icon(LucideIcons.folderSearch, color: Colors.white38, size: 48),
            const SizedBox(height: 16),
            Text(
              l10n.skillScanNoSkills,
              style: AppFonts.inter(fontSize: 14, color: Colors.white54),
            ),
          ],
        ),
      );
    }

    final safeSkills = _results.entries
        .where((e) => e.value != null && e.value!.safe)
        .toList();
    final riskySkills = _results.entries
        .where((e) => e.value != null && !e.value!.safe)
        .toList();

    final List<Widget> cards = [];

    // Add failed skill cards first
    for (final entry in _failedSkills.entries) {
      cards.add(_buildFailedSkillCard(entry.key, entry.value));
    }

    // Add risky skill cards
    for (final entry in riskySkills) {
      cards.add(_buildRiskySkillCard(entry.key, entry.value!, l10n));
    }

    // Add safe skill cards
    for (final entry in safeSkills) {
      cards.add(_buildSafeSkillCard(entry.key, entry.value!, l10n));
    }

    return ListView.separated(
      itemCount: cards.length,
      separatorBuilder: (_, _) => const SizedBox(height: 12),
      itemBuilder: (context, index) => cards[index]
          .animate()
          .fadeIn(duration: 300.ms, delay: (index * 50).ms)
          .slideY(begin: 0.1, end: 0, duration: 300.ms, delay: (index * 50).ms),
    );
  }

  Widget _buildSafeSkillCard(
    String skillName,
    SkillAnalysisResult result,
    AppLocalizations l10n,
  ) {
    final skillPath = _skillPaths[skillName] ?? '';
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: const Color(0xFF22C55E).withValues(alpha: 0.1),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(
          color: const Color(0xFF22C55E).withValues(alpha: 0.3),
        ),
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Icon(
            LucideIcons.shieldCheck,
            color: Color(0xFF22C55E),
            size: 20,
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  children: [
                    Expanded(
                      child: Text(
                        skillName,
                        style: AppFonts.inter(
                          fontSize: 14,
                          fontWeight: FontWeight.w600,
                          color: Colors.white,
                        ),
                      ),
                    ),
                    Container(
                      padding: const EdgeInsets.symmetric(
                        horizontal: 8,
                        vertical: 2,
                      ),
                      decoration: BoxDecoration(
                        color: const Color(0xFF22C55E).withValues(alpha: 0.2),
                        borderRadius: BorderRadius.circular(4),
                      ),
                      child: Text(
                        l10n.skillScanSafe,
                        style: AppFonts.inter(
                          fontSize: 11,
                          fontWeight: FontWeight.w500,
                          color: const Color(0xFF22C55E),
                        ),
                      ),
                    ),
                  ],
                ),
                _buildSkillPathLine(skillPath),
                if (result.summary.isNotEmpty) ...[
                  const SizedBox(height: 8),
                  Text(
                    result.summary,
                    style: AppFonts.inter(fontSize: 12, color: Colors.white54),
                  ),
                ],
              ],
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildRiskySkillCard(
    String skillName,
    SkillAnalysisResult result,
    AppLocalizations l10n,
  ) {
    final deleted = _deleteConfirmed[skillName] == true;
    final trusted = _trustConfirmed[skillName] == true;
    final isExpanded = _expandedSkills.contains(skillName);
    final skillPath = _skillPaths[skillName] ?? '';

    final riskColor = _getRiskLevelColor(result.riskLevel);

    // Show muted state for both deleted and trusted
    final isMuted = deleted || trusted;

    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: isMuted
            ? Colors.white.withValues(alpha: 0.05)
            : trusted
            ? const Color(0xFF3B82F6).withValues(alpha: 0.1)
            : const Color(0xFFEF4444).withValues(alpha: 0.1),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(
          color: isMuted
              ? Colors.white.withValues(alpha: 0.1)
              : trusted
              ? const Color(0xFF3B82F6).withValues(alpha: 0.3)
              : const Color(0xFFEF4444).withValues(alpha: 0.3),
        ),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          // Header row
          Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Icon(
                deleted
                    ? LucideIcons.checkCircle
                    : trusted
                    ? LucideIcons.shieldCheck
                    : LucideIcons.alertTriangle,
                color: deleted
                    ? const Color(0xFF22C55E)
                    : trusted
                    ? const Color(0xFF3B82F6)
                    : const Color(0xFFEF4444),
                size: 20,
              ),
              const SizedBox(width: 12),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Row(
                      children: [
                        Expanded(
                          child: Text(
                            skillName,
                            style: AppFonts.inter(
                              fontSize: 14,
                              fontWeight: FontWeight.w600,
                              color: isMuted ? Colors.white54 : Colors.white,
                              decoration: deleted
                                  ? TextDecoration.lineThrough
                                  : null,
                            ),
                          ),
                        ),
                        if (!isMuted)
                          Container(
                            padding: const EdgeInsets.symmetric(
                              horizontal: 8,
                              vertical: 2,
                            ),
                            decoration: BoxDecoration(
                              color: riskColor.withValues(alpha: 0.2),
                              borderRadius: BorderRadius.circular(4),
                            ),
                            child: Text(
                              _getRiskLevelText(result.riskLevel, l10n),
                              style: AppFonts.inter(
                                fontSize: 11,
                                fontWeight: FontWeight.w500,
                                color: riskColor,
                              ),
                            ),
                          ),
                      ],
                    ),
                    if (deleted)
                      Text(
                        l10n.skillScanDeleted,
                        style: AppFonts.inter(
                          fontSize: 11,
                          color: Colors.white38,
                        ),
                      )
                    else if (trusted)
                      Text(
                        l10n.skillScanTrusted,
                        style: AppFonts.inter(
                          fontSize: 11,
                          color: const Color(0xFF3B82F6),
                        ),
                      )
                    else ...[
                      _buildSkillPathLine(skillPath),
                      const SizedBox(height: 4),
                      Text(
                        result.summary,
                        style: AppFonts.inter(
                          fontSize: 12,
                          color: Colors.white70,
                        ),
                      ),
                    ],
                  ],
                ),
              ),
            ],
          ),
          // Issues section (collapsible)
          if (!isMuted && result.issues.isNotEmpty) ...[
            const SizedBox(height: 12),
            InkWell(
              onTap: () {
                setState(() {
                  if (isExpanded) {
                    _expandedSkills.remove(skillName);
                  } else {
                    _expandedSkills.add(skillName);
                  }
                });
              },
              borderRadius: BorderRadius.circular(8),
              child: Padding(
                padding: const EdgeInsets.symmetric(vertical: 4),
                child: Row(
                  children: [
                    Icon(
                      isExpanded
                          ? LucideIcons.chevronDown
                          : LucideIcons.chevronRight,
                      size: 16,
                      color: Colors.white54,
                    ),
                    const SizedBox(width: 4),
                    Text(
                      '${result.issues.length} ${l10n.skillScanIssues}',
                      style: AppFonts.inter(
                        fontSize: 12,
                        color: Colors.white54,
                      ),
                    ),
                  ],
                ),
              ),
            ),
            if (isExpanded) ...[
              const SizedBox(height: 8),
              ...result.issues.map(
                (issue) => _buildIssueItem(issue, l10n, skillPath),
              ),
            ],
          ],
          // Action buttons (Trust and Delete)
          if (!isMuted) ...[
            const SizedBox(height: 12),
            Row(
              mainAxisAlignment: MainAxisAlignment.end,
              children: [
                OutlinedButton.icon(
                  onPressed: () => _showTrustConfirmation(skillName),
                  icon: const Icon(LucideIcons.shieldCheck, size: 14),
                  label: Text(l10n.skillScanTrust),
                  style: OutlinedButton.styleFrom(
                    foregroundColor: const Color(0xFF3B82F6),
                    side: const BorderSide(color: Color(0xFF3B82F6)),
                    padding: const EdgeInsets.symmetric(
                      horizontal: 12,
                      vertical: 8,
                    ),
                    textStyle: AppFonts.inter(fontSize: 12),
                  ),
                ),
                const SizedBox(width: 8),
                ElevatedButton.icon(
                  onPressed: () => _showDeleteConfirmation(skillName),
                  icon: const Icon(LucideIcons.trash2, size: 14),
                  label: Text(l10n.skillScanDelete),
                  style: ElevatedButton.styleFrom(
                    backgroundColor: const Color(0xFFEF4444),
                    foregroundColor: Colors.white,
                    padding: const EdgeInsets.symmetric(
                      horizontal: 12,
                      vertical: 8,
                    ),
                    textStyle: AppFonts.inter(fontSize: 12),
                  ),
                ),
              ],
            ),
          ],
        ],
      ),
    );
  }

  Widget _buildSkillPathLine(String skillPath) {
    if (skillPath.isEmpty) {
      return const SizedBox.shrink();
    }
    return Padding(
      padding: const EdgeInsets.only(top: 6),
      child: Row(
        children: [
          const Icon(LucideIcons.folder, size: 12, color: Colors.white38),
          const SizedBox(width: 4),
          Expanded(
            child: Text(
              skillPath,
              style: AppFonts.firaCode(fontSize: 11, color: Colors.white54),
              overflow: TextOverflow.ellipsis,
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildIssueItem(
    SkillSecurityIssue issue,
    AppLocalizations l10n,
    String skillPath,
  ) {
    final issuePath = _resolveIssueDisplayPath(skillPath, issue.file);
    return Container(
      margin: const EdgeInsets.only(bottom: 8),
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: Colors.black.withValues(alpha: 0.2),
        borderRadius: BorderRadius.circular(8),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          // Type and severity badges
          Row(
            children: [
              _buildSeverityBadge(issue.severity),
              const SizedBox(width: 8),
              _buildIssueTypeLabel(issue.type, l10n),
            ],
          ),
          // File path
          if (issuePath.isNotEmpty) ...[
            const SizedBox(height: 8),
            Row(
              children: [
                const Icon(LucideIcons.file, size: 12, color: Colors.white38),
                const SizedBox(width: 4),
                Expanded(
                  child: Text(
                    issuePath,
                    style: AppFonts.firaCode(
                      fontSize: 11,
                      color: Colors.white54,
                    ),
                    overflow: TextOverflow.ellipsis,
                  ),
                ),
              ],
            ),
          ],
          // Description
          const SizedBox(height: 8),
          Text(
            issue.description,
            style: AppFonts.inter(fontSize: 12, color: Colors.white70),
          ),
          // Evidence code block
          if (issue.evidence.isNotEmpty) ...[
            const SizedBox(height: 8),
            Container(
              width: double.infinity,
              padding: const EdgeInsets.all(8),
              decoration: BoxDecoration(
                color: Colors.black.withValues(alpha: 0.4),
                borderRadius: BorderRadius.circular(4),
              ),
              child: Text(
                issue.evidence,
                style: AppFonts.firaCode(fontSize: 10, color: Colors.white60),
              ),
            ),
          ],
        ],
      ),
    );
  }

  String _resolveIssueDisplayPath(String skillPath, String issueFile) {
    final normalizedIssueFile = issueFile.trim();
    if (normalizedIssueFile.isEmpty) {
      return skillPath.trim();
    }
    if (normalizedIssueFile.startsWith('/')) {
      return normalizedIssueFile;
    }
    final normalizedSkillPath = skillPath.trim();
    if (normalizedSkillPath.isEmpty) {
      return normalizedIssueFile;
    }
    final separator = normalizedSkillPath.endsWith('/') ? '' : '/';
    return '$normalizedSkillPath$separator$normalizedIssueFile';
  }

  Widget _buildSeverityBadge(String severity) {
    final color = switch (severity.toLowerCase()) {
      'critical' => const Color(0xFFEF4444),
      'high' => const Color(0xFFF97316),
      'medium' => const Color(0xFFF59E0B),
      'low' => const Color(0xFF3B82F6),
      _ => Colors.white54,
    };

    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.2),
        borderRadius: BorderRadius.circular(4),
        border: Border.all(color: color.withValues(alpha: 0.5)),
      ),
      child: Text(
        severity.toUpperCase(),
        style: AppFonts.inter(
          fontSize: 9,
          fontWeight: FontWeight.w600,
          color: color,
        ),
      ),
    );
  }

  Widget _buildIssueTypeLabel(String type, AppLocalizations l10n) {
    final label = switch (type.toLowerCase()) {
      'prompt_injection' => l10n.skillScanTypePromptInjection,
      'data_theft' => l10n.skillScanTypeDataTheft,
      'code_execution' => l10n.skillScanTypeCodeExecution,
      'social_engineering' => l10n.skillScanTypeSocialEngineering,
      'supply_chain' => l10n.skillScanTypeSupplyChain,
      _ => l10n.skillScanTypeOther,
    };

    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
      decoration: BoxDecoration(
        color: const Color(0xFF6366F1).withValues(alpha: 0.2),
        borderRadius: BorderRadius.circular(4),
      ),
      child: Text(
        label,
        style: AppFonts.inter(fontSize: 10, color: const Color(0xFF6366F1)),
      ),
    );
  }

  Color _getRiskLevelColor(String riskLevel) {
    return switch (riskLevel.toLowerCase()) {
      'critical' => const Color(0xFFEF4444),
      'high' => const Color(0xFFF97316),
      'medium' => const Color(0xFFF59E0B),
      'low' => const Color(0xFF3B82F6),
      _ => Colors.white54,
    };
  }

  String _getRiskLevelText(String riskLevel, AppLocalizations l10n) {
    return switch (riskLevel.toLowerCase()) {
      'critical' => l10n.riskLevelCritical,
      'high' => l10n.riskLevelHigh,
      'medium' => l10n.riskLevelMedium,
      'low' => l10n.riskLevelLow,
      _ => riskLevel,
    };
  }

  Widget _buildFailedSkillCard(String skillName, String errorMsg) {
    final l10n = AppLocalizations.of(context)!;
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: const Color(0xFFF59E0B).withValues(alpha: 0.1),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(
          color: const Color(0xFFF59E0B).withValues(alpha: 0.3),
        ),
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Icon(
            LucideIcons.alertCircle,
            color: Color(0xFFF59E0B),
            size: 20,
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  skillName,
                  style: AppFonts.inter(
                    fontSize: 14,
                    fontWeight: FontWeight.w600,
                    color: Colors.white,
                  ),
                ),
                const SizedBox(height: 4),
                Text(
                  l10n.skillScanFailed,
                  style: AppFonts.inter(
                    fontSize: 11,
                    color: const Color(0xFFF59E0B),
                  ),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }

  void _showDeleteConfirmation(String skillName) {
    final l10n = AppLocalizations.of(context)!;
    showDialog(
      context: context,
      builder: (context) => AlertDialog(
        backgroundColor: const Color(0xFF1A1A2E),
        title: Text(
          l10n.skillScanDeleteTitle,
          style: AppFonts.inter(color: Colors.white),
        ),
        content: Text(
          l10n.skillScanDeleteConfirm(skillName),
          style: AppFonts.inter(color: Colors.white70),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(),
            child: Text(
              l10n.cancel,
              style: AppFonts.inter(color: Colors.white54),
            ),
          ),
          ElevatedButton(
            onPressed: () {
              Navigator.of(context).pop();
              _deleteSkill(skillName);
            },
            style: ElevatedButton.styleFrom(
              backgroundColor: const Color(0xFFEF4444),
            ),
            child: Text(
              l10n.skillScanDelete,
              style: AppFonts.inter(color: Colors.white),
            ),
          ),
        ],
      ),
    );
  }

  void _showTrustConfirmation(String skillName) {
    final l10n = AppLocalizations.of(context)!;
    showDialog(
      context: context,
      builder: (context) => AlertDialog(
        backgroundColor: const Color(0xFF1A1A2E),
        title: Text(
          l10n.skillScanTrustTitle,
          style: AppFonts.inter(color: Colors.white),
        ),
        content: Text(
          l10n.skillScanTrustConfirm(skillName),
          style: AppFonts.inter(color: Colors.white70),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(),
            child: Text(
              l10n.cancel,
              style: AppFonts.inter(color: Colors.white54),
            ),
          ),
          ElevatedButton(
            onPressed: () {
              Navigator.of(context).pop();
              _trustSkill(skillName);
            },
            style: ElevatedButton.styleFrom(
              backgroundColor: const Color(0xFF3B82F6),
            ),
            child: Text(
              l10n.skillScanTrust,
              style: AppFonts.inter(color: Colors.white),
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildActions(AppLocalizations l10n) {
    return Row(
      mainAxisAlignment: MainAxisAlignment.end,
      children: [
        if (_scanning)
          TextButton.icon(
            onPressed: () async {
              await _service.cancelBatchScan();
              if (mounted) {
                Navigator.of(context).pop(false);
              }
            },
            icon: const Icon(LucideIcons.x, size: 16),
            label: Text(l10n.cancel),
            style: TextButton.styleFrom(foregroundColor: Colors.white54),
          )
        else
          ElevatedButton(
            onPressed: () => Navigator.of(context).pop(_hasRisks),
            style: ElevatedButton.styleFrom(
              backgroundColor: const Color(0xFF6366F1),
              foregroundColor: Colors.white,
              padding: const EdgeInsets.symmetric(horizontal: 24, vertical: 12),
            ),
            child: Text(
              l10n.skillScanDone,
              style: AppFonts.inter(fontWeight: FontWeight.w500),
            ),
          ),
      ],
    );
  }

  bool get _hasRisks {
    return _results.values.any((r) => r != null && !r.safe);
  }
}
