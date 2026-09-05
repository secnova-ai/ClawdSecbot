import 'package:flutter/material.dart';
import '../utils/app_fonts.dart';
import 'package:lucide_icons_lite/lucide_icons_lite.dart';
import 'package:flutter_animate/flutter_animate.dart';
import '../l10n/app_localizations.dart';

class ProtectionMonitorDialog extends StatefulWidget {
  final String assetName;

  const ProtectionMonitorDialog({super.key, required this.assetName});

  @override
  State<ProtectionMonitorDialog> createState() =>
      _ProtectionMonitorDialogState();
}

class _ProtectionMonitorDialogState extends State<ProtectionMonitorDialog> {
  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context)!;

    return Dialog(
      backgroundColor: const Color(0xFF1A1A2E),
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
      child: Container(
        width: 520,
        height: 520,
        padding: const EdgeInsets.all(24),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            _buildHeader(l10n),
            const SizedBox(height: 20),
            _buildStatusCard(l10n),
            const SizedBox(height: 16),
            Expanded(child: _buildMonitorContent(l10n)),
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
                color: const Color(0xFF22C55E).withValues(alpha: 0.2),
                borderRadius: BorderRadius.circular(8),
              ),
              child: const Icon(
                LucideIcons.shieldCheck,
                color: Color(0xFF22C55E),
                size: 20,
              ),
            )
            .animate(onPlay: (controller) => controller.repeat())
            .shimmer(duration: 2000.ms, color: Colors.white24),
        const SizedBox(width: 12),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                l10n.protectionMonitorTitle,
                style: AppFonts.inter(
                  fontSize: 18,
                  fontWeight: FontWeight.w600,
                  color: Colors.white,
                ),
              ),
              Text(
                widget.assetName,
                style: AppFonts.inter(fontSize: 12, color: Colors.white54),
              ),
            ],
          ),
        ),
        IconButton(
          icon: const Icon(LucideIcons.x, color: Colors.white54, size: 20),
          onPressed: () => Navigator.of(context).pop(),
        ),
      ],
    );
  }

  Widget _buildStatusCard(AppLocalizations l10n) {
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        gradient: LinearGradient(
          colors: [
            const Color(0xFF22C55E).withValues(alpha: 0.2),
            const Color(0xFF22C55E).withValues(alpha: 0.1),
          ],
        ),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(
          color: const Color(0xFF22C55E).withValues(alpha: 0.3),
        ),
      ),
      child: Row(
        children: [
          Container(
                width: 12,
                height: 12,
                decoration: BoxDecoration(
                  color: const Color(0xFF22C55E),
                  shape: BoxShape.circle,
                  boxShadow: [
                    BoxShadow(
                      color: const Color(0xFF22C55E).withValues(alpha: 0.5),
                      blurRadius: 8,
                      spreadRadius: 2,
                    ),
                  ],
                ),
              )
              .animate(onPlay: (controller) => controller.repeat())
              .fadeIn(duration: 800.ms)
              .then()
              .fadeOut(duration: 800.ms),
          const SizedBox(width: 12),
          Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                l10n.protectionStatus,
                style: AppFonts.inter(fontSize: 12, color: Colors.white54),
              ),
              Text(
                l10n.protectionActive,
                style: AppFonts.inter(
                  fontSize: 16,
                  fontWeight: FontWeight.w600,
                  color: const Color(0xFF22C55E),
                ),
              ),
            ],
          ),
          const Spacer(),
          Container(
            padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
            decoration: BoxDecoration(
              color: const Color(0xFF22C55E).withValues(alpha: 0.2),
              borderRadius: BorderRadius.circular(20),
            ),
            child: Row(
              children: [
                const Icon(
                  LucideIcons.activity,
                  color: Color(0xFF22C55E),
                  size: 14,
                ),
                const SizedBox(width: 6),
                Text(
                  l10n.realTimeMonitor,
                  style: AppFonts.inter(
                    fontSize: 12,
                    color: const Color(0xFF22C55E),
                  ),
                ),
              ],
            ),
          ),
        ],
      ),
    ).animate().fadeIn().slideY(begin: -0.1, end: 0);
  }

  Widget _buildMonitorContent(AppLocalizations l10n) {
    return SingleChildScrollView(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          // 行为分析统计
          _buildMonitorSection(
            l10n.behaviorAnalysis,
            LucideIcons.brain,
            const Color(0xFF6366F1),
            _buildBehaviorStats(),
          ),
          const SizedBox(height: 16),
          // 威胁检测
          _buildMonitorSection(
            l10n.threatDetection,
            LucideIcons.alertTriangle,
            const Color(0xFFF59E0B),
            _buildThreatContent(l10n),
          ),
          const SizedBox(height: 16),
          // 最近活动
          _buildMonitorSection(
            '最近活动',
            LucideIcons.clock,
            const Color(0xFF8B5CF6),
            _buildRecentActivity(),
          ),
        ],
      ),
    );
  }

  Widget _buildMonitorSection(
    String title,
    IconData icon,
    Color color,
    Widget content,
  ) {
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: Colors.white.withValues(alpha: 0.05),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: Colors.white.withValues(alpha: 0.1)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Icon(icon, color: color, size: 16),
              const SizedBox(width: 8),
              Text(
                title,
                style: AppFonts.inter(
                  fontSize: 14,
                  fontWeight: FontWeight.w600,
                  color: Colors.white,
                ),
              ),
            ],
          ),
          const SizedBox(height: 12),
          content,
        ],
      ),
    ).animate().fadeIn().slideX(begin: 0.1, end: 0);
  }

  Widget _buildBehaviorStats() {
    return Column(
      children: [
        Row(
          children: [
            Expanded(
              child: _buildStatCard(
                'Tool 调用',
                '128',
                LucideIcons.wrench,
                const Color(0xFF6366F1),
              ),
            ),
            const SizedBox(width: 12),
            Expanded(
              child: _buildStatCard(
                '文件读取',
                '45',
                LucideIcons.fileText,
                const Color(0xFF8B5CF6),
              ),
            ),
          ],
        ),
        const SizedBox(height: 12),
        Row(
          children: [
            Expanded(
              child: _buildStatCard(
                '命令执行',
                '23',
                LucideIcons.terminal,
                const Color(0xFFA78BFA),
              ),
            ),
            const SizedBox(width: 12),
            Expanded(
              child: _buildStatCard(
                '拦截次数',
                '0',
                LucideIcons.shieldOff,
                const Color(0xFF22C55E),
              ),
            ),
          ],
        ),
      ],
    );
  }

  Widget _buildStatCard(
    String label,
    String value,
    IconData icon,
    Color color,
  ) {
    return Container(
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.1),
        borderRadius: BorderRadius.circular(8),
        border: Border.all(color: color.withValues(alpha: 0.2)),
      ),
      child: Row(
        children: [
          Icon(icon, color: color, size: 16),
          const SizedBox(width: 8),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  value,
                  style: AppFonts.firaCode(
                    fontSize: 18,
                    fontWeight: FontWeight.bold,
                    color: Colors.white,
                  ),
                ),
                Text(
                  label,
                  style: AppFonts.inter(fontSize: 11, color: Colors.white54),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildThreatContent(AppLocalizations l10n) {
    return Container(
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: const Color(0xFF22C55E).withValues(alpha: 0.1),
        borderRadius: BorderRadius.circular(8),
      ),
      child: Row(
        children: [
          const Icon(
            LucideIcons.checkCircle,
            color: Color(0xFF22C55E),
            size: 16,
          ),
          const SizedBox(width: 8),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  l10n.noThreatsDetected,
                  style: AppFonts.inter(
                    fontSize: 13,
                    fontWeight: FontWeight.w500,
                    color: const Color(0xFF22C55E),
                  ),
                ),
                Text(
                  l10n.allSystemsNormal,
                  style: AppFonts.inter(fontSize: 11, color: Colors.white54),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildRecentActivity() {
    final activities = [
      ('Read', '~/.openclaw/openclaw.json', '2s ago'),
      ('Execute', 'ls -la /tmp', '15s ago'),
      ('Read', 'src/main.dart', '32s ago'),
      ('Tool', 'Grep: search pattern', '1m ago'),
    ];

    return Column(
      children: activities
          .map(
            (a) => Padding(
              padding: const EdgeInsets.only(bottom: 8),
              child: _buildActivityRow(a.$1, a.$2, a.$3),
            ),
          )
          .toList(),
    );
  }

  Widget _buildActivityRow(String type, String detail, String time) {
    IconData icon;
    Color color;
    switch (type) {
      case 'Read':
        icon = LucideIcons.eye;
        color = const Color(0xFF6366F1);
        break;
      case 'Execute':
        icon = LucideIcons.terminal;
        color = const Color(0xFFF59E0B);
        break;
      case 'Tool':
        icon = LucideIcons.wrench;
        color = const Color(0xFF8B5CF6);
        break;
      default:
        icon = LucideIcons.activity;
        color = Colors.white54;
    }

    return Row(
      children: [
        Container(
          padding: const EdgeInsets.all(4),
          decoration: BoxDecoration(
            color: color.withValues(alpha: 0.2),
            borderRadius: BorderRadius.circular(4),
          ),
          child: Icon(icon, color: color, size: 12),
        ),
        const SizedBox(width: 8),
        Container(
          padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
          decoration: BoxDecoration(
            color: color.withValues(alpha: 0.1),
            borderRadius: BorderRadius.circular(4),
          ),
          child: Text(
            type,
            style: AppFonts.firaCode(fontSize: 10, color: color),
          ),
        ),
        const SizedBox(width: 8),
        Expanded(
          child: Text(
            detail,
            style: AppFonts.firaCode(fontSize: 11, color: Colors.white70),
            overflow: TextOverflow.ellipsis,
          ),
        ),
        Text(
          time,
          style: AppFonts.inter(fontSize: 10, color: Colors.white38),
        ),
      ],
    );
  }
}
