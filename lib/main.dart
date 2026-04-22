import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:provider/provider.dart';
import 'package:window_manager/window_manager.dart';
import 'constants.dart';
import 'l10n/app_localizations.dart';
import 'pages/main_page.dart';
import 'pages/protection_monitor_window.dart';
import 'pages/audit_log_window.dart';
import 'providers/locale_provider.dart';
import 'services/database_service.dart';
import 'services/native_library_service.dart';
import 'services/plugin_service.dart';
import 'services/protection_service.dart';
import 'utils/app_logger.dart';
import 'utils/app_fonts.dart';
import 'utils/locale_utils.dart';
import 'utils/window_animation_helper.dart';
import 'widgets/hide_window_shortcut.dart';

const appBackground = Color(0xFF0F0F23);

void main(List<String> args) async {
  // 使用 runZonedGuarded 捕获未处理的异步错误
  runZonedGuarded(
    () async {
      WidgetsFlutterBinding.ensureInitialized();

      // 注册 SIGINT 信号处理器，确保 Ctrl+C 时正确清理 Go 资源并退出
      // Go c-shared 库可能会干扰默认的信号处理，需要显式处理
      if (!Platform.isWindows) {
        ProcessSignal.sigint.watch().listen((_) async {
          appLogger.info('[Main] Received SIGINT, cleaning up...');
          try {
            // 停止 Go 保护代理，释放资源
            final service = ProtectionService();
            if (service.isProxyRunning) {
              await service.stopProtectionProxy();
            }
            service.dispose();
          } catch (e) {
            // 忽略清理错误，确保退出
          }
          try {
            // 关闭Go插件
            await PluginService().closePlugin();
          } catch (e) {
            // 忽略清理错误，确保退出
          }
          try {
            // 关闭Go DB
            await NativeLibraryService().close();
          } catch (e) {
            // 忽略清理错误，确保退出
          }
          exit(0);
        });
      }

      // Determine window name for log file
      final isMultiWindow = args.firstOrNull == 'multi_window';
      final argument = args.length > 2 ? args[2] : '{}';
      final windowArgs = isMultiWindow
          ? jsonDecode(argument) as Map<String, dynamic>
          : <String, dynamic>{};
      final windowType = isMultiWindow
          ? (windowArgs['windowType'] ?? 'protection_monitor')
          : 'main';
      final windowLocale = LocaleUtils.resolveLanguageCode(
        explicitLanguage: windowArgs['locale']?.toString(),
      );

      // Initialize logger first
      await appLogger.init(windowName: windowType.toString());

      // Check if this is a sub-window
      if (isMultiWindow) {
        final windowId = args[1]; // 0.3.0 使用 String windowId
        final subWindowSize = windowType == 'audit_log'
            ? const Size(1280, 800)
            : const Size(1440, 900);
        final subWindowMinSize = windowType == 'audit_log'
            ? const Size(1024, 600)
            : const Size(1200, 700);
        final subWindowOptions = WindowOptions(
          size: subWindowSize,
          minimumSize: subWindowMinSize,
          center: true,
          title: 'ClawdSecbot',
          // Linux 使用原生窗口装饰，macOS 隐藏标题栏但保留原生红黄绿按钮
          titleBarStyle: Platform.isLinux
              ? TitleBarStyle.normal
              : TitleBarStyle.hidden,
          backgroundColor: appBackground,
        );

        // Initialize window manager with error handling
        try {
          await windowManager.ensureInitialized();
          windowManager.waitUntilReadyToShow(subWindowOptions, () async {
            await windowManager.setResizable(true);
            if (Platform.isWindows) {
              await windowManager.setTitleBarStyle(TitleBarStyle.hidden);
            }
          });
        } catch (e) {
          appLogger.warning('[Main] Window manager initialization failed: $e');
        }

        // Sub-windows don't use shared_preferences to avoid channel errors
        if (windowType == 'audit_log') {
          // Initialize database path and native library for audit log window
          await DatabaseService().init();
          await NativeLibraryService().initialize();
          runApp(
            AuditLogWindowApp(
              windowId: windowId,
              locale: windowLocale,
              initialAssetName: windowArgs['assetName'] ?? '',
              initialAssetID: windowArgs['assetID'] ?? '',
            ),
          );
        } else {
          // Initialize database path and native library for protection monitor window
          await DatabaseService().init();
          await NativeLibraryService().initialize();
          runApp(
            ProtectionMonitorWindowApp(
              windowId: windowId,
              assetName: windowArgs['assetName'] ?? 'Unknown Asset',
              assetID: windowArgs['assetID'] ?? '',
              locale: windowLocale,
            ),
          );
        }
        return;
      }

      // Main window initialization
      try {
        await windowManager.ensureInitialized();

        // 主窗口在创建时固定尺寸，避免首次显示时出现大小闪动。
        // Linux 使用原生窗口装饰以确保窗口尺寸正确生效；macOS 隐藏标题栏使用
        // Flutter 自定义标题栏。
        WindowOptions windowOptions = WindowOptions(
          size: AppConstants.windowSize,
          minimumSize: AppConstants.windowSize,
          title: 'ClawdSecbot',
          center: true,
          titleBarStyle: Platform.isLinux
              ? TitleBarStyle.normal
              : TitleBarStyle.hidden,
          backgroundColor: appBackground,
        );

        windowManager.waitUntilReadyToShow(windowOptions, () async {
          await windowManager.setResizable(true);
          if (Platform.isWindows) {
            await windowManager.setTitleBarStyle(TitleBarStyle.hidden);
          }
          await windowManager.show();
          await windowManager.focus();
        });
      } catch (e) {
        appLogger.warning('[Main] Window manager initialization failed: $e');
        // Continue without window manager on unsupported platforms
      }

      runApp(
        ChangeNotifierProvider(
          create: (_) => LocaleProvider(),
          child: const ClawdbotGuardApp(),
        ),
      );
    },
    (error, stackTrace) {
      // 记录未处理的错误
      appLogger.error('Unhandled error: $error', error, stackTrace);
    },
  );
}

class ClawdbotGuardApp extends StatefulWidget {
  const ClawdbotGuardApp({super.key});

  @override
  State<ClawdbotGuardApp> createState() => _ClawdbotGuardAppState();
}

class _ClawdbotGuardAppState extends State<ClawdbotGuardApp> {
  @override
  Widget build(BuildContext context) {
    return Consumer<LocaleProvider>(
      builder: (context, localeProvider, child) {
        return MaterialApp(
          title: 'ClawdSecbot',
          debugShowCheckedModeBanner: false,
          locale: localeProvider.locale,
          localizationsDelegates: const [
            AppLocalizations.delegate,
            GlobalMaterialLocalizations.delegate,
            GlobalWidgetsLocalizations.delegate,
            GlobalCupertinoLocalizations.delegate,
          ],
          supportedLocales: const [Locale('zh'), Locale('en')],
          theme: ThemeData(
            useMaterial3: true,
            colorScheme: ColorScheme.fromSeed(
              seedColor: const Color(0xFF6366F1),
              brightness: Brightness.dark,
            ),
            scaffoldBackgroundColor: appBackground,
            // 使用内置的 Noto Sans SC 字体，支持中文显示
            textTheme: AppFonts.getTextTheme(),
          ),
          // 在 MaterialApp 级别定义快捷键，确保全局生效
          shortcuts: {
            LogicalKeySet(LogicalKeyboardKey.meta, LogicalKeyboardKey.keyW):
                const HideWindowIntent(),
          },
          actions: {
            HideWindowIntent: CallbackAction<HideWindowIntent>(
              onInvoke: (_) {
                WindowAnimationHelper.hideWithAnimation();
                return null;
              },
            ),
          },
          home: const MainPage(),
        );
      },
    );
  }
}
