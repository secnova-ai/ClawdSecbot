import 'package:flutter/material.dart';
import 'package:lucide_icons_lite/lucide_icons_lite.dart';
import 'package:flutter_animate/flutter_animate.dart';
import '../l10n/app_localizations.dart';
import '../utils/app_fonts.dart';

const _windowBackground = Color(0xFF0F0F23);

/// 防护监控窗口的初始化加载屏幕
class ProtectionMonitorLoadingScreen extends StatelessWidget {
  final AppLocalizations l10n;
  final String assetName;

  const ProtectionMonitorLoadingScreen({
    super.key,
    required this.l10n,
    required this.assetName,
  });

  @override
  Widget build(BuildContext context) {
    return Container(
      decoration: BoxDecoration(
        gradient: RadialGradient(
          center: Alignment.center,
          radius: 1.0,
          colors: [
            const Color(0xFF6366F1).withValues(alpha: 0.15),
            _windowBackground,
          ],
        ),
      ),
      child: Center(
        child: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
            Container(
                  width: 120,
                  height: 120,
                  decoration: BoxDecoration(
                    shape: BoxShape.circle,
                    gradient: LinearGradient(
                      begin: Alignment.topLeft,
                      end: Alignment.bottomRight,
                      colors: [
                        const Color(0xFF6366F1).withValues(alpha: 0.3),
                        const Color(0xFF8B5CF6).withValues(alpha: 0.3),
                      ],
                    ),
                    boxShadow: [
                      BoxShadow(
                        color: const Color(0xFF6366F1).withValues(alpha: 0.3),
                        blurRadius: 40,
                        spreadRadius: 10,
                      ),
                    ],
                  ),
                  child: const Icon(
                    LucideIcons.shield,
                    size: 60,
                    color: Color(0xFF6366F1),
                  ),
                )
                .animate(onPlay: (controller) => controller.repeat())
                .shimmer(duration: 2000.ms, color: Colors.white24)
                .then()
                .shake(duration: 500.ms, hz: 0.5),
            const SizedBox(height: 32),
            Text(
                  l10n.initializingProtectionMonitor,
                  style: AppFonts.inter(
                    fontSize: 24,
                    fontWeight: FontWeight.w600,
                    color: Colors.white,
                  ),
                )
                .animate(onPlay: (controller) => controller.repeat())
                .fadeIn(duration: 800.ms)
                .then()
                .fadeOut(duration: 800.ms),
            const SizedBox(height: 16),
            Text(
              assetName,
              style: AppFonts.inter(fontSize: 14, color: Colors.white54),
            ),
            const SizedBox(height: 32),
            Container(
              width: 320,
              padding: const EdgeInsets.all(20),
              decoration: BoxDecoration(
                color: Colors.white.withValues(alpha: 0.05),
                borderRadius: BorderRadius.circular(16),
                border: Border.all(color: Colors.white.withValues(alpha: 0.1)),
              ),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  _buildLoadingStep(l10n.initDatabase, true),
                  const SizedBox(height: 12),
                  _buildLoadingStep(l10n.startCallbackBridge, true),
                  const SizedBox(height: 12),
                  _buildLoadingStep(l10n.loadStatistics, true),
                  const SizedBox(height: 12),
                  _buildLoadingStep(l10n.startListener, false),
                ],
              ),
            ),
            const SizedBox(height: 24),
            SizedBox(
              width: 240,
              child: ClipRRect(
                borderRadius: BorderRadius.circular(8),
                child: LinearProgressIndicator(
                  minHeight: 6,
                  backgroundColor: Colors.white.withValues(alpha: 0.1),
                  valueColor: AlwaysStoppedAnimation<Color>(
                    const Color(0xFF6366F1),
                  ),
                ),
              ),
            ),
            ],
          ),
        ),
      ),
    );
  }

  Widget _buildLoadingStep(String label, bool completed) {
    return Row(
      children: [
        Container(
          width: 20,
          height: 20,
          decoration: BoxDecoration(
            shape: BoxShape.circle,
            color: completed
                ? const Color(0xFF22C55E).withValues(alpha: 0.2)
                : const Color(0xFF6366F1).withValues(alpha: 0.2),
            border: Border.all(
              color: completed
                  ? const Color(0xFF22C55E)
                  : const Color(0xFF6366F1),
              width: 2,
            ),
          ),
          child: completed
              ? const Icon(
                  LucideIcons.check,
                  size: 12,
                  color: Color(0xFF22C55E),
                )
              : SizedBox(
                  width: 12,
                  height: 12,
                  child: CircularProgressIndicator(
                    strokeWidth: 2,
                    valueColor: AlwaysStoppedAnimation<Color>(
                      const Color(0xFF6366F1),
                    ),
                  ),
                ),
        ),
        const SizedBox(width: 12),
        Text(
          label,
          style: AppFonts.inter(
            fontSize: 13,
            color: completed ? const Color(0xFF22C55E) : Colors.white70,
            fontWeight: completed ? FontWeight.w500 : FontWeight.w400,
          ),
        ),
      ],
    );
  }
}
