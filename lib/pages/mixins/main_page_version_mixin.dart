import 'dart:async';
import 'dart:convert';
import 'dart:ffi' as ffi;
import 'dart:io' show Platform;
import 'package:flutter/material.dart';
import 'package:lucide_icons/lucide_icons.dart';
import 'package:package_info_plus/package_info_plus.dart';
import 'package:url_launcher/url_launcher.dart';
import '../../config/build_config.dart';
import '../../core_transport/transport_registry.dart';
import '../../l10n/app_localizations.dart';
import '../../models/version_info.dart';
import '../../services/message_bridge_service.dart';
import '../../utils/app_fonts.dart';
import '../../utils/app_logger.dart';
import '../main_page.dart';

/// 版本检查 Mixin
/// 负责版本更新检查、订阅和弹窗显示
mixin MainPageVersionMixin on State<MainPage> {
  // ============ 状态变量 ============
  StreamSubscription<VersionInfo>? versionUpdateSubscription;
  bool isUpdateDialogShown = false;

  // ============ 版本检查方法 ============

  /// 订阅版本更新回调
  void subscribeVersionUpdates() {
    versionUpdateSubscription?.cancel();
    versionUpdateSubscription =
        MessageBridgeService().versionUpdateStream.listen((versionInfo) {
      if (mounted && !isUpdateDialogShown) {
        showUpdateDialog(versionInfo);
      }
    });
    appLogger.info('[MainPage] Subscribed to version update stream');
  }

  /// 启动版本检查服务（Go 层）
  Future<void> startVersionCheckService() async {
    // AppStore 版本禁用自动检查
    if (BuildConfig.isAppStore) {
      appLogger.info(
        '[MainPage] AppStore version: automatic update check disabled',
      );
      return;
    }

    try {
      // 获取当前应用版本
      final packageInfo = await PackageInfo.fromPlatform();
      final currentVersion = packageInfo.version;

      // 获取当前语言
      String locale = 'en';
      if (mounted) {
        locale = Localizations.localeOf(context).languageCode;
      }

      // 获取系统信息
      final os = _getOS();
      final arch = _getArch();

      final config = {
        'current_version': currentVersion,
        'os': os,
        'arch': arch,
        'language': locale,
        'enabled': true,
      };

      final transport = TransportRegistry.transport;
      if (transport.isReady) {
        final result = transport.callOneArg(
          'StartVersionCheckServiceFFI',
          jsonEncode(config),
        );
        if (result['success'] == true) {
          appLogger.info('[MainPage] Version check service started');
        } else {
          appLogger.error(
            '[MainPage] Version check service failed: ${result['error']}',
          );
        }
      }
    } catch (e) {
      appLogger.error('[MainPage] Failed to start version check service', e);
    }
  }

  /// 停止版本检查服务（Go 层）
  Future<void> stopVersionCheckService() async {
    try {
      final transport = TransportRegistry.transport;
      if (transport.isReady) {
        final result = transport.callNoArg('StopVersionCheckServiceFFI');
        appLogger.info('[MainPage] Version check service stopped: $result');
      }
    } catch (e) {
      appLogger.error('[MainPage] Failed to stop version check service', e);
    }
  }

  /// 更新版本检查服务语言设置
  Future<void> updateVersionCheckLanguage(String language) async {
    try {
      final transport = TransportRegistry.transport;
      if (transport.isReady) {
        transport.callOneArg('UpdateVersionCheckLanguageFFI', language);
        appLogger.info('[MainPage] Version check language updated: $language');
      }
    } catch (e) {
      appLogger.error('[MainPage] Failed to update version check language', e);
    }
  }

  Future<void> showAppAboutDialog() async {
    final packageInfo = await PackageInfo.fromPlatform();
    if (!mounted) return;

    final l10n = AppLocalizations.of(context)!;

    await showDialog<void>(
      context: context,
      builder: (context) => AlertDialog(
        backgroundColor: const Color(0xFF1A1A2E),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(20)),
        content: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Container(
              width: 104,
              height: 104,
              decoration: BoxDecoration(
                gradient: const LinearGradient(
                  begin: Alignment.topLeft,
                  end: Alignment.bottomRight,
                  colors: [Color(0xFF23233A), Color(0xFF2B2B45)],
                ),
                borderRadius: BorderRadius.circular(24),
                boxShadow: const [
                  BoxShadow(
                    color: Color(0x33000000),
                    blurRadius: 24,
                    offset: Offset(0, 10),
                  ),
                ],
                border: Border.all(
                  color: const Color(0xFF6366F1).withValues(alpha: 0.18),
                ),
              ),
              child: const Center(
                child: Icon(
                  LucideIcons.shield,
                  color: Color(0xFF8B5CF6),
                  size: 48,
                ),
              ),
            ),
            const SizedBox(height: 24),
            Text(
              l10n.appTitle,
              textAlign: TextAlign.center,
              style: AppFonts.inter(
                color: Colors.white,
                fontSize: 24,
                fontWeight: FontWeight.w700,
              ),
            ),
            const SizedBox(height: 10),
            Text(
              l10n.aboutVersionWithBuild(
                packageInfo.version,
                packageInfo.buildNumber,
              ),
              textAlign: TextAlign.center,
              style: AppFonts.inter(
                color: Colors.white70,
                fontSize: 16,
                fontWeight: FontWeight.w500,
              ),
            ),
            const SizedBox(height: 22),
            Text(
              l10n.aboutCopyright,
              textAlign: TextAlign.center,
              style: AppFonts.inter(
                color: Colors.white54,
                fontSize: 14,
              ),
            ),
          ],
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(),
            child: Text(
              l10n.close,
              style: AppFonts.inter(
                color: const Color(0xFF818CF8),
                fontWeight: FontWeight.w600,
              ),
            ),
          ),
        ],
        actionsAlignment: MainAxisAlignment.center,
      ),
    );
  }

  /// 显示版本更新弹窗
  void showUpdateDialog(VersionInfo info) {
    if (isUpdateDialogShown) {
      appLogger.info('[MainPage] Update dialog already shown, skipping');
      return;
    }
    isUpdateDialogShown = true;
    appLogger.info(
      '[MainPage] Showing update dialog for version ${info.version}',
    );

    final l10n = AppLocalizations.of(context)!;

    showDialog(
      context: context,
      barrierDismissible: false,
      builder: (context) => AlertDialog(
        backgroundColor: const Color(0xFF1A1A2E),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
        title: Row(
          children: [
            Icon(LucideIcons.downloadCloud, color: Color(0xFF6366F1)),
            SizedBox(width: 12),
            Text(
              l10n.newVersionAvailable,
              style: AppFonts.inter(
                color: Colors.white,
                fontWeight: FontWeight.bold,
              ),
            ),
          ],
        ),
        content: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              l10n.versionAvailable(info.version),
              style: AppFonts.inter(
                color: Colors.white,
                fontWeight: FontWeight.w600,
              ),
            ),
            SizedBox(height: 12),
            Container(
              padding: EdgeInsets.all(12),
              decoration: BoxDecoration(
                color: Colors.white.withValues(alpha: 0.05),
                borderRadius: BorderRadius.circular(8),
              ),
              child: Text(
                info.changeLog,
                style: AppFonts.firaCode(color: Colors.white70, fontSize: 12),
              ),
            ),
          ],
        ),
        actions: [
          if (!info.forceUpdate)
            TextButton(
              onPressed: () => Navigator.pop(context),
              child: Text(l10n.later, style: TextStyle(color: Colors.white54)),
            ),
          ElevatedButton(
            onPressed: () {
              launchUrl(Uri.parse(info.downloadUrl));
              if (info.forceUpdate) {
                // 强制更新时保持弹窗
              } else {
                Navigator.pop(context);
              }
            },
            style: ElevatedButton.styleFrom(
              backgroundColor: const Color(0xFF6366F1),
            ),
            child: Text(l10n.download, style: TextStyle(color: Colors.white)),
          ),
        ],
      ),
    ).then((_) {
      isUpdateDialogShown = false;
    });
  }

  // ============ 辅助方法 ============

  String _getOS() {
    if (Platform.isMacOS) return 'macos';
    if (Platform.isWindows) return 'windows';
    if (Platform.isLinux) return 'linux';
    return 'unknown';
  }

  String _getArch() {
    final abi = ffi.Abi.current().toString();
    if (abi.contains('arm64')) return 'arm64';
    if (abi.contains('x64')) return 'amd64';
    return 'amd64';
  }

  /// 释放版本检查资源
  void disposeVersionMixin() {
    versionUpdateSubscription?.cancel();
  }
}
