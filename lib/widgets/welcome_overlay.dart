import 'package:flutter/material.dart';
import 'package:flutter_animate/flutter_animate.dart';
import 'package:lucide_icons_lite/lucide_icons_lite.dart';
import '../l10n/app_localizations.dart';
import '../utils/app_fonts.dart';

/// 欢迎覆盖层组件
/// 应用启动时显示的欢迎动画屏幕
class WelcomeOverlay extends StatelessWidget {
  const WelcomeOverlay({super.key});

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context)!;

    return Container(
      decoration: const BoxDecoration(
        color: Color(0xFF0F0F23),
        gradient: LinearGradient(
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
          colors: [Color(0xFF0F0F23), Color(0xFF1A1A2E)],
        ),
      ),
      child: Stack(
        children: [
          // 左上角装饰圆
          Positioned(
            left: -80,
            top: -60,
            child: Container(
              width: 220,
              height: 220,
              decoration: BoxDecoration(
                shape: BoxShape.circle,
                color: const Color(0xFF6366F1).withValues(alpha: 0.12),
              ),
            ),
          ),
          // 右下角装饰圆
          Positioned(
            right: -110,
            bottom: -90,
            child: Container(
              width: 260,
              height: 260,
              decoration: BoxDecoration(
                shape: BoxShape.circle,
                color: const Color(0xFF8B5CF6).withValues(alpha: 0.12),
              ),
            ),
          ),
          // 中心内容
          Center(
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                // 品牌标签
                Container(
                      padding: const EdgeInsets.symmetric(
                        horizontal: 12,
                        vertical: 4,
                      ),
                      decoration: BoxDecoration(
                        color: Colors.white.withValues(alpha: 0.08),
                        borderRadius: BorderRadius.circular(999),
                        border: Border.all(
                          color: Colors.white.withValues(alpha: 0.12),
                        ),
                      ),
                      child: Text(
                        'ClawdSecbot',
                        style: AppFonts.inter(
                          fontSize: 12,
                          fontWeight: FontWeight.w600,
                          letterSpacing: 1.8,
                          color: Colors.white.withValues(alpha: 0.7),
                        ),
                      ),
                    )
                    .animate()
                    .fadeIn(duration: 400.ms)
                    .slideY(begin: 0.2, end: 0, duration: 400.ms),
                const SizedBox(height: 18),
                // 盾牌图标
                Container(
                      width: 88,
                      height: 88,
                      decoration: BoxDecoration(
                        gradient: LinearGradient(
                          begin: Alignment.topLeft,
                          end: Alignment.bottomRight,
                          colors: [
                            const Color(0xFF6366F1).withValues(alpha: 0.18),
                            const Color(0xFF8B5CF6).withValues(alpha: 0.32),
                          ],
                        ),
                        borderRadius: BorderRadius.circular(24),
                        border: Border.all(
                          color: Colors.white.withValues(alpha: 0.16),
                        ),
                        boxShadow: [
                          BoxShadow(
                            color: const Color(0xFF6366F1)
                                .withValues(alpha: 0.35),
                            blurRadius: 24,
                            offset: const Offset(0, 10),
                          ),
                        ],
                      ),
                      child: const Icon(
                        LucideIcons.shield,
                        size: 36,
                        color: Color(0xFFEEF2FF),
                      ),
                    )
                    .animate(
                      onPlay: (controller) => controller.repeat(reverse: true),
                    )
                    .scale(
                      begin: const Offset(1, 1),
                      end: const Offset(1.06, 1.06),
                      duration: 1200.ms,
                      curve: Curves.easeInOut,
                    ),
                const SizedBox(height: 20),
                // 欢迎标语
                SizedBox(
                      width: 360,
                      child: Text(
                        l10n.welcomeSlogan,
                        textAlign: TextAlign.center,
                        style: AppFonts.inter(
                          fontSize: 18,
                          fontWeight: FontWeight.w600,
                          color: Colors.white,
                          height: 1.5,
                        ),
                      ),
                    )
                    .animate()
                    .fadeIn(duration: 450.ms, delay: 120.ms)
                    .slideY(begin: 0.25, end: 0, duration: 450.ms),
              ],
            ),
          ),
        ],
      ),
    );
  }
}
