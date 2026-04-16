import 'package:flutter/material.dart';
import 'package:lucide_icons/lucide_icons.dart';
import 'package:window_manager/window_manager.dart';
import '../../constants.dart';
import '../../core_transport/transport_registry.dart';
import '../../l10n/app_localizations.dart';
import '../../services/bookmark_service.dart';
import '../../services/protection_database_service.dart';
import '../../services/protection_service.dart';
import '../../utils/app_fonts.dart';
import '../../utils/app_logger.dart';
import '../main_page.dart';

/// 数据管理 Mixin
/// 负责数据清理、配置恢复、目录重新授权等操作
mixin MainPageDataMixin on State<MainPage> {
  // ============ 状态变量 ============
  bool isRestoringConfig = false;

  // ============ 需要 MainPage 提供的状态和方法 ============
  Set<String> get protectedAssets;
  bool get isRestoringProtection;
  set isRestoringProtection(bool value);
  Future<void> showWindow();
  Future<void> closeAllMonitorWindows();
  void resetUIStateAfterClear();
  void clearProtectedAssetMappings();

  // ============ 清空数据方法 ============

  /// 显示清空数据确认对话框
  Future<void> showClearDataConfirmDialog() async {
    // 先显示窗口
    await showWindow();

    if (!mounted) return;

    final l10n = AppLocalizations.of(context)!;

    final result = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        backgroundColor: const Color(0xFF1A1A2E),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
        title: Row(
          children: [
            Container(
              padding: const EdgeInsets.all(8),
              decoration: BoxDecoration(
                color: const Color(0xFFEF4444).withValues(alpha: 0.2),
                borderRadius: BorderRadius.circular(8),
              ),
              child: const Icon(
                LucideIcons.trash2,
                color: Color(0xFFEF4444),
                size: 20,
              ),
            ),
            const SizedBox(width: 12),
            Text(
              l10n.clearDataConfirmTitle,
              style: AppFonts.inter(
                fontSize: 16,
                fontWeight: FontWeight.w600,
                color: Colors.white,
              ),
            ),
          ],
        ),
        content: Text(
          l10n.clearDataConfirmMessage,
          style: AppFonts.inter(fontSize: 13, color: Colors.white70),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(false),
            child: Text(
              l10n.cancel,
              style: AppFonts.inter(color: Colors.white54),
            ),
          ),
          ElevatedButton(
            onPressed: () => Navigator.of(context).pop(true),
            style: ElevatedButton.styleFrom(
              backgroundColor: const Color(0xFFEF4444),
              shape: RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(8),
              ),
            ),
            child: Text(
              l10n.clear,
              style: AppFonts.inter(
                fontWeight: FontWeight.w600,
                color: Colors.white,
              ),
            ),
          ),
        ],
      ),
    );

    if (result == true) {
      await clearAllData();
    }
  }

  /// 清空所有数据
  Future<void> clearAllData() async {
    final l10n = AppLocalizations.of(context)!;
    try {
      final service = ProtectionService();

      // 使用 fullReset 彻底重置服务状态
      await service.fullReset();

      // 清除数据库中的保护状态和业务数据
      try {
        await ProtectionDatabaseService().clearProtectionState();
      } catch (_) {}
      // 通过 Go FFI 清空所有运行数据
      try {
        final transport = TransportRegistry.transport;
        if (transport.isReady) {
          transport.callNoArg('ClearAllDataFFI');
        }
      } catch (e) {
        appLogger.error('[MainPage] ClearAllData FFI failed', e);
      }

      // 关闭所有监控窗口
      await closeAllMonitorWindows();

      // 确保主窗口显示
      await showWindow();

      // 重置 UI 状态到初始页面
      resetUIStateAfterClear();

      await windowManager.setSize(AppConstants.windowSize);

      // 仅重新加载统计基线（此时应为 0）
      try {
        service.loadStatisticsFromDatabase();
      } catch (_) {}

      // 显示成功提示
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text(l10n.clearDataSuccess),
            backgroundColor: Colors.green,
            duration: const Duration(seconds: 2),
          ),
        );
      }

      appLogger.info('[MainPage] All data cleared by user');
    } catch (e) {
      appLogger.error('[MainPage] Failed to clear all data', e);
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text(l10n.clearDataFailed),
            backgroundColor: Colors.red,
            duration: const Duration(seconds: 2),
          ),
        );
      }
    }
  }

  // ============ 恢复配置方法 ============

  /// 显示恢复配置确认对话框
  Future<void> showRestoreConfigConfirmDialog() async {
    // 先显示窗口
    await showWindow();

    if (!mounted) return;

    final l10n = AppLocalizations.of(context)!;

    // 先检查是否存在初始备份
    final service = ProtectionService();
    final hasBackup = await service.hasInitialBackup();
    if (!mounted) return;

    if (!hasBackup) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text(l10n.restoreConfigNoBackup),
            backgroundColor: Colors.orange,
            duration: const Duration(seconds: 3),
          ),
        );
      }
      return;
    }

    final result = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        backgroundColor: const Color(0xFF1A1A2E),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
        title: Row(
          children: [
            Container(
              padding: const EdgeInsets.all(8),
              decoration: BoxDecoration(
                color: const Color(0xFFEAB308).withValues(alpha: 0.2),
                borderRadius: BorderRadius.circular(8),
              ),
              child: const Icon(
                LucideIcons.rotateCcw,
                color: Color(0xFFEAB308),
                size: 20,
              ),
            ),
            const SizedBox(width: 12),
            Text(
              l10n.restoreConfigConfirmTitle,
              style: AppFonts.inter(
                fontSize: 16,
                fontWeight: FontWeight.w600,
                color: Colors.white,
              ),
            ),
          ],
        ),
        content: Text(
          l10n.restoreConfigConfirmMessage,
          style: AppFonts.inter(fontSize: 13, color: Colors.white70),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(false),
            child: Text(
              l10n.cancel,
              style: AppFonts.inter(color: Colors.white54),
            ),
          ),
          ElevatedButton(
            onPressed: () => Navigator.of(ctx).pop(true),
            style: ElevatedButton.styleFrom(
              backgroundColor: const Color(0xFFEAB308),
              shape: RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(8),
              ),
            ),
            child: Text(
              l10n.continueButton,
              style: AppFonts.inter(
                fontWeight: FontWeight.w600,
                color: Colors.white,
              ),
            ),
          ),
        ],
      ),
    );

    if (result == true) {
      await restoreConfig();
    }
  }

  /// 恢复配置到初始状态
  Future<void> restoreConfig() async {
    final l10n = AppLocalizations.of(context)!;

    setState(() {
      isRestoringConfig = true;
    });

    try {
      appLogger.info('[MainPage] Restoring config to initial state...');

      final service = ProtectionService();
      final result = await service.restoreToInitialConfig();

      if (!mounted) return;

      if (result['success'] == true) {
        // 清理已防护资产集合
        protectedAssets.clear();
        clearProtectedAssetMappings();

        // 关闭所有监控窗口
        await closeAllMonitorWindows();
        if (!mounted) return;

        // 重置 UI 状态
        setState(() {
          isRestoringProtection = false;
          isRestoringConfig = false;
        });

        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text(l10n.restoreConfigSuccess),
            backgroundColor: Colors.green,
            duration: const Duration(seconds: 3),
          ),
        );

        appLogger.info('[MainPage] Config restored successfully');
      } else {
        setState(() {
          isRestoringConfig = false;
        });

        final error = result['error'] ?? 'unknown error';
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text(l10n.restoreConfigFailed(error)),
            backgroundColor: Colors.red,
            duration: const Duration(seconds: 3),
          ),
        );
        appLogger.error('[MainPage] Config restore failed: $error');
      }
    } catch (e) {
      appLogger.error('[MainPage] Failed to restore config', e);
      if (mounted) {
        setState(() {
          isRestoringConfig = false;
        });
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text(l10n.restoreConfigFailed(e.toString())),
            backgroundColor: Colors.red,
            duration: const Duration(seconds: 3),
          ),
        );
      }
    }
  }

  // ============ 目录授权方法 ============

  /// 重新授权配置目录
  Future<void> reauthorizeDirectory() async {
    // 清除旧的授权
    await BookmarkService().clearBookmark();
    appLogger.info('[MainPage] 已清除旧的授权,准备重新选择目录');

    // 重置状态
    onConfigAccessChanged(false);

    // 显示授权对话框
    final authorized = await showConfigAccessDialogForReauthorize();

    if (authorized) {
      appLogger.info('[MainPage] 重新授权成功');
      // 显示成功提示
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(
            content: Text('目录授权成功'),
            backgroundColor: Colors.green,
            duration: Duration(seconds: 2),
          ),
        );
      }
    } else {
      appLogger.warning('[MainPage] 用户取消了重新授权');
    }
  }

  /// 配置访问权限变更回调（需要 MainPage 实现）
  void onConfigAccessChanged(bool hasAccess);

  /// 显示配置访问对话框（需要 MainPage 实现）
  Future<bool> showConfigAccessDialogForReauthorize();
}
