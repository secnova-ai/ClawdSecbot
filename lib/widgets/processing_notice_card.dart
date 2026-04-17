import 'package:flutter/material.dart';
import '../utils/app_fonts.dart';

/// 统一的处理中提示卡片，风格与首页过渡态一致。
class ProcessingNoticeCard extends StatelessWidget {
  /// 创建处理中提示卡片。
  const ProcessingNoticeCard({
    super.key,
    required this.message,
    this.title = '正在处理中',
  });

  /// 主标题。
  final String title;

  /// 处理中提示信息。
  final String message;

  @override
  Widget build(BuildContext context) {
    return Container(
      constraints: const BoxConstraints(maxWidth: 490),
      padding: const EdgeInsets.symmetric(horizontal: 19, vertical: 16),
      decoration: BoxDecoration(
        gradient: const LinearGradient(
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
          colors: [Color(0xFF202338), Color(0xFF171A2B)],
        ),
        borderRadius: BorderRadius.circular(14),
        border: Border.all(color: Colors.white.withValues(alpha: 0.1)),
        boxShadow: [
          BoxShadow(
            color: Colors.black.withValues(alpha: 0.34),
            blurRadius: 20,
            offset: const Offset(0, 10),
          ),
        ],
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              const SizedBox(
                width: 18,
                height: 18,
                child: CircularProgressIndicator(
                  strokeWidth: 2,
                  valueColor: AlwaysStoppedAnimation<Color>(Color(0xFF6366F1)),
                ),
              ),
              const SizedBox(width: 8),
              Expanded(
                child: Text(
                  title,
                  style: AppFonts.inter(
                    fontSize: 14,
                    color: Colors.white,
                    fontWeight: FontWeight.w400,
                    decoration: TextDecoration.none,
                  ),
                ),
              ),
            ],
          ),
          const SizedBox(height: 10),
          Container(
            width: double.infinity,
            padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 8),
            decoration: BoxDecoration(
              color: Colors.white.withValues(alpha: 0.05),
              borderRadius: BorderRadius.circular(8),
              border: Border.all(color: Colors.white.withValues(alpha: 0.08)),
            ),
            child: Text(
              message,
              textAlign: TextAlign.left,
              softWrap: true,
              style: AppFonts.inter(
                fontSize: 13,
                letterSpacing: 0,
                color: Colors.white.withValues(alpha: 0.9),
                height: 1.55,
                fontWeight: FontWeight.w400,
                decoration: TextDecoration.none,
              ),
            ),
          ),
        ],
      ),
    );
  }
}
