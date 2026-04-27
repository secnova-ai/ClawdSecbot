import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'package:launch_at_startup/launch_at_startup.dart';
import 'package:shared_preferences/shared_preferences.dart';
import '../config/build_config.dart';
import '../services/app_settings_database_service.dart';
import '../services/model_config_database_service.dart';
import '../utils/app_logger.dart';

/// Onboarding persistence and local callback server helper.
class OnboardingService {
  /// SharedPreferences key for model config step.
  static const String modelConfiguredKey = 'onboarding_model_configured';

  /// SharedPreferences key for bot model config step.
  static const String botModelConfiguredKey = 'onboarding_bot_model_configured';

  /// SharedPreferences key for config update step.
  static const String configUpdateCompletedKey =
      'onboarding_config_update_completed';

  /// Returns whether onboarding is completed.
  ///
  /// 使用数据库 is_first_launch 标记判断，不再使用 SharedPreferences。
  /// 如果 is_first_launch 为空或 "true"，则引导未完成。
  Future<bool> isOnboardingCompleted() async {
    // 使用数据库标记判断
    final isFirst = await AppSettingsDatabaseService().isFirstLaunch();
    final completed = !isFirst;

    // 如果标记显示已完成，但配置数据缺失，则重置步骤状态
    if (completed) {
      final valid = await _ensureCompletionValid();
      if (!valid) {
        await _resetStepProgressInternal();
        appLogger.info(
          '[Onboarding] Completed but config invalid, reset steps',
        );
        // 注意：这里不重置 is_first_launch，因为用户可能只是删除了配置
        // 重新完成引导后会正常工作
      }
    }
    appLogger.info('[Onboarding] Completed (db): $completed');
    return completed;
  }

  /// Ensures onboarding completion is valid for current build.
  Future<bool> _ensureCompletionValid() async {
    if (BuildConfig.isAppStore) {
      final hasModel = await _ensureModelConfigAvailable();
      final hasBotModel = await _ensureBotModelConfigAvailable();
      final configUpdated = await isConfigUpdateCompleted();
      return hasModel && hasBotModel && configUpdated;
    }
    final hasModel = await _ensureModelConfigAvailable();
    final hasBotModel = await _ensureBotModelConfigAvailable();
    return hasModel && hasBotModel;
  }

  /// Ensures security model flag is valid when config data is missing.
  Future<bool> _ensureModelConfigAvailable() async {
    final dbService = ModelConfigDatabaseService();
    final available = await dbService.hasValidSecurityModelConfig();
    if (!available) {
      await _setStepFlag(modelConfiguredKey, false);
      appLogger.info('[Onboarding] Model config missing, reset security step');
    }
    return available;
  }

  /// Ensures bot model flag is valid when config data is missing.
  Future<bool> _ensureBotModelConfigAvailable() async {
    final dbService = ModelConfigDatabaseService();
    final available = await dbService.hasValidBotModelConfig('Openclaw');
    if (!available) {
      await _setStepFlag(botModelConfiguredKey, false);
      appLogger.info('[Onboarding] Bot model config missing, reset bot step');
    }
    return available;
  }

  /// Marks onboarding completion state.
  ///
  /// 当 completed 为 true 时：
  /// 1. 非 App Store 版本默认启用开机启动
  /// 2. 设置 is_first_launch = "false"
  Future<void> setOnboardingCompleted(bool completed) async {
    if (!completed) {
      appLogger.info('[Onboarding] Set completed: false (no action)');
      return;
    }

    // 1. 非 App Store 版本默认启用开机启动
    if (!BuildConfig.isAppStore) {
      try {
        await launchAtStartup.enable();
        appLogger.info('[Onboarding] Launch at startup enabled by default');
      } catch (e) {
        appLogger.error('[Onboarding] Failed to enable launch at startup', e);
      }
    }

    // 2. 标记首次启动完成
    final success = await AppSettingsDatabaseService()
        .setFirstLaunchCompleted();
    if (success) {
      appLogger.info('[Onboarding] First launch completed, marked in database');
    } else {
      appLogger.error('[Onboarding] Failed to mark first launch completed');
    }
  }

  /// Returns whether the model configuration step is completed.
  Future<bool> isModelConfigured() async {
    if (!await _ensureModelConfigAvailable()) {
      appLogger.info('[Onboarding][Step1] Model : false');
      return false;
    }
    final prefs = await SharedPreferences.getInstance();
    final configured = prefs.getBool(modelConfiguredKey) ?? false;
    appLogger.info('[Onboarding][Step1] Model : $configured');
    return configured;
  }

  /// Returns whether the bot model configuration step is completed.
  Future<bool> isBotModelConfigured() async {
    if (!await _ensureBotModelConfigAvailable()) {
      appLogger.info('[Onboarding][Step2] Bot Model : false');
      return false;
    }
    final prefs = await SharedPreferences.getInstance();
    final configured = prefs.getBool(botModelConfiguredKey) ?? false;
    appLogger.info('[Onboarding][Step2] Bot Model : $configured');
    return configured;
  }

  /// Marks model configuration step completion.
  Future<void> setModelConfigured(bool completed) async {
    await _setStepFlag(modelConfiguredKey, completed);
    appLogger.info('[Onboarding][Step1] Model : $completed');
  }

  /// Marks bot model configuration step completion.
  Future<void> setBotModelConfigured(bool completed) async {
    await _setStepFlag(botModelConfiguredKey, completed);
    appLogger.info('[Onboarding][Step2] Bot Model : $completed');
  }

  /// Returns whether the configuration update step is completed.
  Future<bool> isConfigUpdateCompleted() async {
    final prefs = await SharedPreferences.getInstance();
    final completed = prefs.getBool(configUpdateCompletedKey) ?? false;
    appLogger.info('[Onboarding][Step3] Config Update : $completed');
    return completed;
  }

  /// Marks configuration update step completion.
  Future<void> setConfigUpdateCompleted(bool completed) async {
    await _setStepFlag(configUpdateCompletedKey, completed);
    appLogger.info('[Onboarding][Step3] Config Update : $completed');
  }

  /// Resets onboarding step progress when onboarding is not completed.
  Future<void> resetStepProgress() async {
    await _resetStepProgressInternal();
    appLogger.info('[Onboarding] Reset step progress');
  }

  /// Resets step progress without emitting a public log line.
  Future<void> _resetStepProgressInternal() async {
    await _setStepFlag(modelConfiguredKey, false);
    await _setStepFlag(botModelConfiguredKey, false);
    await _setStepFlag(configUpdateCompletedKey, false);
  }

  /// Sets a step flag value in shared preferences.
  Future<void> _setStepFlag(String key, bool value) async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.setBool(key, value);
  }

  /// Starts a local callback server for onboarding steps.
  Future<OnboardingCallbackServer> startCallbackServer() async {
    final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    final controller = StreamController<OnboardingCallbackResult>.broadcast();
    appLogger.info(
      '[Onboarding] Callback server started: ${server.address.address}:${server.port}',
    );

    server.listen((request) async {
      final response = request.response;
      // Enable CORS
      response.headers.add('Access-Control-Allow-Origin', '*');
      response.headers.add('Access-Control-Allow-Methods', 'POST, OPTIONS');
      response.headers.add('Access-Control-Allow-Headers', 'Content-Type');

      if (request.method == 'OPTIONS') {
        response.statusCode = HttpStatus.ok;
        await response.close();
        return;
      }

      try {
        appLogger.info(
          '[Onboarding] Callback request: ${request.method} ${request.uri}',
        );

        // Handle POST /onboarding/config
        if (request.method == 'POST' &&
            request.uri.path == '/onboarding/config') {
          final content = await utf8.decoder.bind(request).join();
          appLogger.info('[Onboarding] Config received: $content');

          controller.add(
            OnboardingCallbackResult(step: 'config', status: 'received'),
          );

          response.statusCode = HttpStatus.ok;
          response.headers.contentType = ContentType.json;
          response.write(jsonEncode({'success': true}));
          await response.close();
          return;
        }

        response.statusCode = HttpStatus.notFound;
        response.write('not_found');
        await response.close();
      } catch (e) {
        appLogger.error('[Onboarding] Callback error', e);
        response.statusCode = HttpStatus.internalServerError;
        response.write('error');
        await response.close();
      }
    });

    return OnboardingCallbackServer(
      server: server,
      token: '', // No token needed for this flow as it's open for the bot
      resultStream: controller.stream,
      onClose: controller.close,
    );
  }

  /// Returns the default base URL for a known provider.
  String getDefaultBaseURL(String provider) {
    final defaults = <String, String>{
      'anthropic': 'https://api.anthropic.com',
      'openai': 'https://api.openai.com/v1',
      'google': 'https://generativelanguage.googleapis.com',
      'zai': 'https://open.bigmodel.cn/api/paas/v4',
      'deepseek': 'https://api.deepseek.com',
      'openrouter': 'https://openrouter.ai/api/v1',
      'qwen': 'https://dashscope.aliyuncs.com/compatible-mode/v1',
      'groq': 'https://api.groq.com/openai/v1',
      'minimax': 'https://api.minimax.io/anthropic/v1',
      'minimax_cn': 'https://api.minimaxi.com/anthropic/v1',
      'lmstudio': 'http://127.0.0.1:1234/v1',
    };
    return defaults[provider] ?? '';
  }
}

// HTTP 端点相关的类已移除（OnboardingAssetServer, OnboardingAssetIngestResult）
// 不再使用本地 HTTP server 接收 Bot 配置

/// Callback server for onboarding steps.
class OnboardingCallbackServer {
  /// Underlying HTTP server.
  final HttpServer server;

  /// Token required by callback requests.
  final String token;

  /// Stream of callback results.
  final Stream<OnboardingCallbackResult> resultStream;

  /// Close callback for stream controller.
  final FutureOr<void> Function() onClose;

  /// Creates a callback server wrapper.
  OnboardingCallbackServer({
    required this.server,
    required this.token,
    required this.resultStream,
    required this.onClose,
  });

  /// Returns the base URL for local callbacks.
  Uri get baseUri => Uri(
    scheme: 'http',
    host: server.address.address,
    port: server.port,
    path: '/onboarding/callback',
  );

  /// Builds a callback URL with step and status.
  Uri buildCallbackUri({required String step, required String status}) {
    return baseUri.replace(
      queryParameters: {'token': token, 'step': step, 'status': status},
    );
  }

  /// Closes the callback server and result stream.
  Future<void> close() async {
    await server.close(force: true);
    await onClose();
  }
}

/// Result model for onboarding callback events.
class OnboardingCallbackResult {
  /// Step identifier.
  final String step;

  /// Callback status.
  final String status;

  /// Creates a callback result.
  const OnboardingCallbackResult({required this.step, required this.status});
}
