import 'dart:io';
import 'package:flutter/material.dart';
import 'package:lucide_icons/lucide_icons.dart';
import '../config/build_config.dart';
import '../l10n/app_localizations.dart';
import '../utils/app_fonts.dart';

/// 通用设置 Tab
/// 包含开机启动、清空数据、恢复配置、重新授权目录等功能
class GeneralSettingsTab extends StatelessWidget {
  final bool launchAtStartupEnabled;
  final VoidCallback onToggleLaunchAtStartup;
  final VoidCallback onClearData;
  final VoidCallback onRestoreConfig;
  final VoidCallback onShowAbout;
  final VoidCallback onReauthorizeDirectory;

  const GeneralSettingsTab({
    super.key,
    required this.launchAtStartupEnabled,
    required this.onToggleLaunchAtStartup,
    required this.onClearData,
    required this.onRestoreConfig,
    required this.onShowAbout,
    required this.onReauthorizeDirectory,
  });

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context)!;

    return SingleChildScrollView(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          // 开机启动（仅非 App Store 版本）
          if (!BuildConfig.isAppStore) ...[
            _buildToggleTile(
              icon: LucideIcons.power,
              iconColor: const Color(0xFF6366F1),
              title: l10n.launchAtStartup,
              value: launchAtStartupEnabled,
              onToggle: onToggleLaunchAtStartup,
            ),
            const SizedBox(height: 16),
          ],
          // 数据管理
          _buildSectionHeader(l10n.dataManagement),
          const SizedBox(height: 8),
          _buildActionTile(
            icon: LucideIcons.trash2,
            iconColor: const Color(0xFFEF4444),
            title: l10n.clearData,
            subtitle: l10n.clearDataDescription,
            onTap: onClearData,
          ),
          const SizedBox(height: 8),
          _buildActionTile(
            icon: LucideIcons.rotateCcw,
            iconColor: const Color(0xFFEAB308),
            title: l10n.restoreConfig,
            subtitle: l10n.restoreConfigDescription,
            onTap: onRestoreConfig,
          ),
          const SizedBox(height: 8),
          _buildActionTile(
            icon: LucideIcons.info,
            iconColor: const Color(0xFF6366F1),
            title: l10n.aboutApp(l10n.appTitle),
            subtitle: '${l10n.version} / ${l10n.buildNumber}',
            onTap: onShowAbout,
          ),
          // 重新授权目录（仅 macOS App Store 版本）
          if (Platform.isMacOS && BuildConfig.requiresDirectoryAuth) ...[
            const SizedBox(height: 16),
            _buildSectionHeader(l10n.permissionsSection),
            const SizedBox(height: 8),
            _buildActionTile(
              icon: LucideIcons.folderOpen,
              iconColor: const Color(0xFF22C55E),
              title: l10n.permissionsSection,
              onTap: onReauthorizeDirectory,
            ),
          ],
        ],
      ),
    );
  }

  /// 构建分组标题
  Widget _buildSectionHeader(String title) {
    return Padding(
      padding: const EdgeInsets.only(left: 4),
      child: Text(
        title,
        style: AppFonts.inter(
          fontSize: 12,
          fontWeight: FontWeight.w500,
          color: Colors.white38,
        ),
      ),
    );
  }

  /// 构建开关类设置项
  Widget _buildToggleTile({
    required IconData icon,
    required Color iconColor,
    required String title,
    required bool value,
    required VoidCallback onToggle,
  }) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
      decoration: BoxDecoration(
        color: Colors.white.withValues(alpha: 0.05),
        borderRadius: BorderRadius.circular(10),
      ),
      child: Row(
        children: [
          Container(
            padding: const EdgeInsets.all(6),
            decoration: BoxDecoration(
              color: iconColor.withValues(alpha: 0.15),
              borderRadius: BorderRadius.circular(6),
            ),
            child: Icon(icon, color: iconColor, size: 16),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Text(
              title,
              style: AppFonts.inter(fontSize: 14, color: Colors.white),
            ),
          ),
          Switch.adaptive(
            value: value,
            onChanged: (_) => onToggle(),
            activeTrackColor: const Color(0xFF6366F1),
            inactiveTrackColor: Colors.white.withValues(alpha: 0.1),
          ),
        ],
      ),
    );
  }

  /// 构建操作类设置项
  Widget _buildActionTile({
    required IconData icon,
    required Color iconColor,
    required String title,
    String? subtitle,
    required VoidCallback onTap,
  }) {
    return Material(
      color: Colors.transparent,
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(10),
        hoverColor: Colors.white.withValues(alpha: 0.03),
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
          decoration: BoxDecoration(
            color: Colors.white.withValues(alpha: 0.05),
            borderRadius: BorderRadius.circular(10),
          ),
          child: Row(
            children: [
              Container(
                padding: const EdgeInsets.all(6),
                decoration: BoxDecoration(
                  color: iconColor.withValues(alpha: 0.15),
                  borderRadius: BorderRadius.circular(6),
                ),
                child: Icon(icon, color: iconColor, size: 16),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      title,
                      style: AppFonts.inter(fontSize: 14, color: Colors.white),
                    ),
                    if (subtitle != null) ...[
                      const SizedBox(height: 2),
                      Text(
                        subtitle,
                        style: AppFonts.inter(
                          fontSize: 12,
                          color: Colors.white38,
                        ),
                      ),
                    ],
                  ],
                ),
              ),
              const Icon(
                LucideIcons.chevronRight,
                color: Colors.white24,
                size: 16,
              ),
            ],
          ),
        ),
      ),
    );
  }
}
