import 'package:flutter/material.dart';
import 'package:lucide_icons_lite/lucide_icons_lite.dart';
import '../config/build_config.dart';
import '../l10n/app_localizations.dart';
import '../utils/app_fonts.dart';

/// 引导完成覆盖层组件
/// 用户完成引导后显示的短暂成功动画
class OnboardingCompletionOverlay extends StatelessWidget {
  final bool visible;

  const OnboardingCompletionOverlay({
    super.key,
    required this.visible,
  });

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context)!;
    final message = BuildConfig.isAppStore
        ? l10n.onboardingProtectionEnabled
        : l10n.onboardingScanReady;

    return IgnorePointer(
      ignoring: true,
      child: AnimatedOpacity(
        opacity: visible ? 1 : 0,
        duration: const Duration(milliseconds: 220),
        curve: Curves.easeOut,
        child: Center(
          child: AnimatedScale(
            scale: visible ? 1 : 0.92,
            duration: const Duration(milliseconds: 280),
            curve: Curves.easeOutBack,
            child: Container(
              padding: const EdgeInsets.symmetric(horizontal: 24, vertical: 18),
              decoration: BoxDecoration(
                color: const Color(0xFF1A1A2E).withValues(alpha: 0.95),
                borderRadius: BorderRadius.circular(16),
                border: Border.all(color: Colors.white.withValues(alpha: 0.12)),
                boxShadow: [
                  BoxShadow(
                    color: Colors.black.withValues(alpha: 0.35),
                    blurRadius: 24,
                    offset: const Offset(0, 12),
                  ),
                ],
              ),
              child: Column(
                mainAxisSize: MainAxisSize.min,
                children: [
                  const Icon(
                    LucideIcons.checkCircle2,
                    size: 34,
                    color: Color(0xFF34D399),
                  ),
                  const SizedBox(height: 10),
                  Text(
                    l10n.onboardingCongratsTitle,
                    style: AppFonts.inter(
                      fontSize: 16,
                      fontWeight: FontWeight.w600,
                      color: Colors.white,
                    ),
                  ),
                  const SizedBox(height: 6),
                  Text(
                    message,
                    textAlign: TextAlign.center,
                    style: AppFonts.inter(fontSize: 13, color: Colors.white70),
                  ),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}
