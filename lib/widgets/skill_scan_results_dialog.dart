import 'package:flutter/material.dart';
import 'package:lucide_icons/lucide_icons.dart';
import '../l10n/app_localizations.dart';
import '../services/scan_database_service.dart';
import '../utils/app_fonts.dart';

/// Dialog to display all skill scan results (safe, risky, trusted)
class SkillScanResultsDialog extends StatefulWidget {
  const SkillScanResultsDialog({super.key});

  @override
  State<SkillScanResultsDialog> createState() => _SkillScanResultsDialogState();
}

class _SkillScanResultsDialogState extends State<SkillScanResultsDialog> {
  List<Map<String, dynamic>>? _records;
  bool _loading = true;
  final Set<int> _expandedIndices = {};

  @override
  void initState() {
    super.initState();
    _loadData();
  }

  Future<void> _loadData() async {
    final records = await ScanDatabaseService().getAllSkillScans();
    if (mounted) {
      setState(() {
        _records = records;
        _loading = false;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context)!;

    return Dialog(
      backgroundColor: const Color(0xFF1A1A2E),
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
      child: Container(
        width: 600,
        height: 500,
        padding: const EdgeInsets.all(24),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            _buildHeader(l10n),
            const SizedBox(height: 16),
            Expanded(child: _buildBody(l10n)),
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
          child: const Icon(
            LucideIcons.fileSearch,
            color: Color(0xFF6366F1),
            size: 20,
          ),
        ),
        const SizedBox(width: 12),
        Text(
          l10n.viewSkillScanResultsTitle,
          style: AppFonts.inter(
            fontSize: 18,
            fontWeight: FontWeight.w600,
            color: Colors.white,
          ),
        ),
        const Spacer(),
        IconButton(
          icon: const Icon(LucideIcons.x, color: Colors.white54, size: 20),
          onPressed: () => Navigator.of(context).pop(),
        ),
      ],
    );
  }

  Widget _buildBody(AppLocalizations l10n) {
    if (_loading) {
      return const Center(
        child: CircularProgressIndicator(color: Color(0xFF6366F1)),
      );
    }

    final records = _records;
    if (records == null || records.isEmpty) {
      return Center(
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            const Icon(
              LucideIcons.folderSearch,
              color: Colors.white38,
              size: 48,
            ),
            const SizedBox(height: 16),
            Text(
              l10n.noSkillScanResults,
              style: AppFonts.inter(fontSize: 14, color: Colors.white54),
            ),
          ],
        ),
      );
    }

    return ListView.separated(
      itemCount: records.length,
      separatorBuilder: (_, _) => const SizedBox(height: 10),
      itemBuilder: (context, index) =>
          _buildSkillCard(records[index], index, l10n),
    );
  }

  Widget _buildSkillCard(
    Map<String, dynamic> record,
    int index,
    AppLocalizations l10n,
  ) {
    final skillName = record['skill_name'] as String? ?? '';
    final safe = record['safe'] as bool? ?? false;
    final trusted = record['trusted'] as bool? ?? false;
    final deleted = record['deleted'] as bool? ?? false;
    final riskLevel = record['risk_level'] as String? ?? '';
    final skillPath = record['skill_path'] as String? ?? '';
    final scannedAt = record['scanned_at'] as String? ?? '';
    final issues = record['issues'] as List<String>? ?? [];
    final issueDetails =
        (record['issue_details'] as List?)
            ?.map((item) => Map<String, String>.from(item as Map))
            .toList() ??
        const <Map<String, String>>[];
    final isExpanded = _expandedIndices.contains(index);

    Color statusColor;
    IconData statusIcon;
    String statusText;

    if (deleted) {
      statusColor = const Color(0xFF9CA3AF);
      statusIcon = LucideIcons.trash2;
      statusText = l10n.localeName.startsWith('zh') ? '已删除' : 'Deleted';
    } else if (trusted) {
      statusColor = const Color(0xFF3B82F6);
      statusIcon = LucideIcons.shieldCheck;
      statusText = l10n.skillScanTrusted;
    } else if (safe) {
      statusColor = const Color(0xFF22C55E);
      statusIcon = LucideIcons.shieldCheck;
      statusText = l10n.skillScanSafe;
    } else {
      statusColor = const Color(0xFFEF4444);
      statusIcon = LucideIcons.alertTriangle;
      statusText = _getRiskLevelText(riskLevel, l10n);
    }

    return Container(
      padding: const EdgeInsets.all(14),
      decoration: BoxDecoration(
        color: statusColor.withValues(alpha: 0.08),
        borderRadius: BorderRadius.circular(10),
        border: Border.all(color: statusColor.withValues(alpha: 0.25)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Icon(statusIcon, color: statusColor, size: 18),
              const SizedBox(width: 10),
              Expanded(
                child: Text(
                  skillName,
                  style: AppFonts.inter(
                    fontSize: 13,
                    fontWeight: FontWeight.w600,
                    color: Colors.white,
                  ),
                ),
              ),
              Container(
                padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
                decoration: BoxDecoration(
                  color: statusColor.withValues(alpha: 0.2),
                  borderRadius: BorderRadius.circular(4),
                ),
                child: Text(
                  statusText,
                  style: AppFonts.inter(
                    fontSize: 10,
                    fontWeight: FontWeight.w500,
                    color: statusColor,
                  ),
                ),
              ),
            ],
          ),
          if (scannedAt.isNotEmpty) ...[
            const SizedBox(height: 6),
            Text(
              l10n.skillScanResultScannedAt(_formatTime(scannedAt)),
              style: AppFonts.inter(fontSize: 11, color: Colors.white38),
            ),
          ],
          if (skillPath.isNotEmpty) ...[
            const SizedBox(height: 6),
            Row(
              children: [
                const Icon(LucideIcons.folder, size: 12, color: Colors.white38),
                const SizedBox(width: 4),
                Expanded(
                  child: Text(
                    skillPath,
                    style: AppFonts.firaCode(
                      fontSize: 10,
                      color: Colors.white54,
                    ),
                    overflow: TextOverflow.ellipsis,
                  ),
                ),
              ],
            ),
          ],
          if (!safe && !trusted && issues.isNotEmpty) ...[
            const SizedBox(height: 8),
            InkWell(
              onTap: () {
                setState(() {
                  if (isExpanded) {
                    _expandedIndices.remove(index);
                  } else {
                    _expandedIndices.add(index);
                  }
                });
              },
              borderRadius: BorderRadius.circular(6),
              child: Padding(
                padding: const EdgeInsets.symmetric(vertical: 2),
                child: Row(
                  children: [
                    Icon(
                      isExpanded
                          ? LucideIcons.chevronDown
                          : LucideIcons.chevronRight,
                      size: 14,
                      color: Colors.white54,
                    ),
                    const SizedBox(width: 4),
                    Text(
                      l10n.skillScanResultIssueCount(issues.length),
                      style: AppFonts.inter(
                        fontSize: 11,
                        color: Colors.white54,
                      ),
                    ),
                  ],
                ),
              ),
            ),
            if (isExpanded)
              ...issues.asMap().entries.map(
                (entry) => Padding(
                  padding: const EdgeInsets.only(left: 18, top: 4),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Row(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          const Text(
                            '- ',
                            style: TextStyle(
                              color: Colors.white54,
                              fontSize: 11,
                            ),
                          ),
                          Expanded(
                            child: Text(
                              entry.value,
                              style: AppFonts.inter(
                                fontSize: 11,
                                color: Colors.white54,
                              ),
                            ),
                          ),
                        ],
                      ),
                      if (entry.key < issueDetails.length &&
                          (issueDetails[entry.key]['file'] ?? '').isNotEmpty)
                        Padding(
                          padding: const EdgeInsets.only(left: 12, top: 4),
                          child: Row(
                            children: [
                              const Icon(
                                LucideIcons.file,
                                size: 11,
                                color: Colors.white38,
                              ),
                              const SizedBox(width: 4),
                              Expanded(
                                child: Text(
                                  _resolveIssueDisplayPath(
                                    skillPath,
                                    issueDetails[entry.key]['file']!,
                                  ),
                                  style: AppFonts.firaCode(
                                    fontSize: 10,
                                    color: Colors.white38,
                                  ),
                                  overflow: TextOverflow.ellipsis,
                                ),
                              ),
                            ],
                          ),
                        ),
                      if (entry.key < issueDetails.length &&
                          (issueDetails[entry.key]['evidence'] ?? '')
                              .isNotEmpty)
                        Padding(
                          padding: const EdgeInsets.only(left: 12, top: 4),
                          child: Text(
                            issueDetails[entry.key]['evidence']!,
                            style: AppFonts.inter(
                              fontSize: 10,
                              color: Colors.white38,
                            ),
                          ),
                        ),
                    ],
                  ),
                ),
              ),
          ],
        ],
      ),
    );
  }

  String _getRiskLevelText(String riskLevel, AppLocalizations l10n) {
    return switch (riskLevel.toLowerCase()) {
      'critical' => l10n.riskLevelCritical,
      'high' => l10n.riskLevelHigh,
      'medium' => l10n.riskLevelMedium,
      'low' => l10n.riskLevelLow,
      _ => l10n.skillScanRiskDetected,
    };
  }

  String _formatTime(String isoTime) {
    try {
      final dt = DateTime.parse(isoTime).toLocal();
      return '${dt.year}-${_pad(dt.month)}-${_pad(dt.day)} ${_pad(dt.hour)}:${_pad(dt.minute)}';
    } catch (_) {
      return isoTime;
    }
  }

  String _pad(int n) => n.toString().padLeft(2, '0');

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

  Widget _buildActions(AppLocalizations l10n) {
    return Row(
      mainAxisAlignment: MainAxisAlignment.end,
      children: [
        ElevatedButton(
          onPressed: () => Navigator.of(context).pop(),
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
}
