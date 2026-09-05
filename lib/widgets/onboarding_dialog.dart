import 'dart:async';
import 'package:flutter/material.dart';
import '../utils/app_fonts.dart';
import 'package:lucide_icons_lite/lucide_icons_lite.dart';
import '../config/build_config.dart';
import '../l10n/app_localizations.dart';
import '../models/llm_config_model.dart';
import '../services/onboarding_service.dart';
import '../services/protection_service.dart';
import '../utils/app_logger.dart';
import 'security_model_config_form.dart';
import 'bot_model_config_form.dart';
import '../services/model_config_service.dart';

/// Onboarding dialog for first-time setup.
class OnboardingDialog extends StatefulWidget {
  /// Creates an onboarding dialog.
  const OnboardingDialog({super.key, this.onFinish});

  /// Callback when onboarding finishes without a Navigator pop.
  final FutureOr<void> Function()? onFinish;

  @override
  State<OnboardingDialog> createState() => _OnboardingDialogState();
}

class _OnboardingDialogState extends State<OnboardingDialog> {
  /// Fixed width for the onboarding dialog.
  static const double _dialogWidth = 560;

  /// Fixed height for the onboarding dialog.
  static const double _dialogHeight = 780;

  /// Step index for quick start intro.
  static const int _stepQuickStart = 0;

  /// Step index for bot model configuration.
  static const int _stepBotModel = 1;

  /// Step index for security model configuration.
  static const int _stepSecurityModel = 2;

  /// Step index for bot configuration update.
  static const int _stepConfigUpdate = 3;

  /// Default proxy base URL for bot config update guidance.
  static const String _defaultProxyBaseUrl = 'http://127.0.0.1:13436';

  /// 默认资产名称，onboarding阶段尚未扫描时使用
  static const String _defaultAssetName = 'Openclaw';

  final OnboardingService _onboardingService = OnboardingService();

  /// GlobalKeys for accessing form states
  final GlobalKey<SecurityModelConfigFormState> _securityModelFormKey =
      GlobalKey<SecurityModelConfigFormState>();
  final GlobalKey<BotModelConfigFormState> _botModelFormKey =
      GlobalKey<BotModelConfigFormState>();

  bool _loading = true;
  bool _saving = false;
  bool _modelConfigured = false;
  bool _botModelConfigured = false;
  bool _configUpdateCompleted = false;
  bool _autoFinished = false;
  bool _reuseBotModel = true;
  SecurityModelConfig? _securityModelFromBot;
  int _currentStep = _stepQuickStart;
  String _botProviderType = '';
  String _botModelName = '';
  String _proxyBaseUrl = _defaultProxyBaseUrl;

  @override
  void initState() {
    super.initState();
    _initialize();
  }

  /// Initializes onboarding state.
  Future<void> _initialize() async {
    appLogger.info('[OnboardingDialog] Initialize start');

    final onboardingCompleted = await _onboardingService
        .isOnboardingCompleted();

    // 如果引导未完成，重置所有步骤进度，从头开始
    if (!onboardingCompleted) {
      appLogger.info(
        '[OnboardingDialog] Onboarding not completed, reset and start from beginning',
      );
      await _onboardingService.resetStepProgress();

      if (mounted) {
        setState(() {
          _modelConfigured = false;
          _botModelConfigured = false;
          _configUpdateCompleted = false;
          _currentStep = _stepQuickStart;
          _loading = false;
        });
      }
      return;
    }

    // 引导已完成（这个dialog不应该再显示，但为了安全起见，直接关闭）
    appLogger.info(
      '[OnboardingDialog] Onboarding already completed, should not show dialog',
    );
    if (mounted) {
      setState(() {
        _loading = false;
      });
      // 直接触发完成回调
      WidgetsBinding.instance.addPostFrameCallback((_) {
        _finishOnboarding();
      });
    }
  }

  /// Handles security model configuration save (now step 2).
  Future<void> _handleModelConfigSaved() async {
    if (_saving) return;
    setState(() {
      _saving = true;
    });

    bool success = false;

    if (_reuseBotModel && _securityModelFromBot != null) {
      // Reuse: copy bot model config to security model directly (skip test)
      final securityConfig = _securityModelFromBot!;
      final configService = SecurityModelConfigService();
      success = await configService.saveConfig(securityConfig);
      if (success) {
        appLogger.info(
          '[OnboardingDialog] Security model saved (reused from bot model)',
        );
        // Hot reload ShepherdGate
        try {
          final protectionService = ProtectionService();
          await protectionService.updateSecurityModelConfig(securityConfig);
          appLogger.info('[OnboardingDialog] Security model hot reloaded');
        } catch (e) {
          appLogger.warning(
            '[OnboardingDialog] Failed to hot reload security model: $e',
          );
        }
      }
    } else {
      // Independent config: use form's saveConfig (with connection test)
      success = await _securityModelFormKey.currentState?.saveConfig() ?? false;
      if (success) {
        appLogger.info('[OnboardingDialog] Security model saved');
      }
    }

    if (success) {
      await _onboardingService.setModelConfigured(true);
      if (!mounted) return;
      if (BuildConfig.isAppStore) {
        setState(() {
          _modelConfigured = true;
          _currentStep = _stepConfigUpdate;
          _saving = false;
        });
        return;
      }
      setState(() {
        _modelConfigured = true;
        _saving = false;
      });
      await _finishOnboarding();
    } else {
      if (mounted) {
        setState(() {
          _saving = false;
        });
      }
    }
  }

  /// Handles bot model configuration save (now step 1).
  Future<void> _handleBotModelConfigSaved() async {
    if (_saving) return;
    setState(() {
      _saving = true;
    });

    final success = await _botModelFormKey.currentState?.saveConfig();
    if (success == true) {
      appLogger.info('[OnboardingDialog] Bot model saved');
      await _onboardingService.setBotModelConfigured(true);
      appLogger.info('[OnboardingDialog] Bot model config persisted');
      await _loadBotConfigSummary();
      if (!mounted) return;
      setState(() {
        _botModelConfigured = true;
        _currentStep = _stepSecurityModel;
        _saving = false;
      });
    } else {
      if (mounted) {
        setState(() {
          _saving = false;
        });
      }
    }
  }

  /// Load bot config summary and cache converted security model config for reuse.
  Future<void> _loadBotConfigSummary() async {
    try {
      final service = BotModelConfigService(assetName: _defaultAssetName);
      final config = await service.loadConfig();
      if (config == null) return;
      final providerType = config.provider.trim();
      final modelName = config.model.trim();
      if (!mounted) return;
      setState(() {
        _botProviderType = providerType;
        _botModelName = modelName;
        _proxyBaseUrl = _defaultProxyBaseUrl;
        // Cache converted security model config for reuse
        _securityModelFromBot = SecurityModelConfig(
          provider: config.provider,
          endpoint: config.baseUrl,
          apiKey: config.apiKey,
          model: config.model,
          secretKey: config.secretKey,
        );
      });
    } catch (e) {
      appLogger.error('[OnboardingDialog] Load bot config summary failed', e);
    }
  }

  /// Handles configuration update completion.
  Future<void> _handleConfigUpdateCompleted() async {
    appLogger.info('[OnboardingDialog] Config update completed');
    await _onboardingService.setConfigUpdateCompleted(true);
    if (!mounted) return;
    setState(() {
      _configUpdateCompleted = true;
    });
    await _finishOnboarding();
  }

  /// Completes onboarding and closes the dialog.
  Future<void> _finishOnboarding() async {
    appLogger.info('[OnboardingDialog] Finish onboarding');
    await _onboardingService.setOnboardingCompleted(true);
    final onFinish = widget.onFinish;
    if (onFinish != null) {
      await onFinish();
      return;
    }
    if (mounted) {
      Navigator.of(context).pop(true);
    }
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context)!;
    return Dialog(
      backgroundColor: const Color(0xFF1A1A2E),
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
      child: SizedBox(
        width: _dialogWidth,
        height: _dialogHeight,
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: _loading
              ? const Center(child: CircularProgressIndicator())
              : Column(
                  mainAxisSize: MainAxisSize.max,
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    _buildHeader(l10n),
                    const SizedBox(height: 20),
                    if (_currentStep == _stepQuickStart)
                      Expanded(child: _buildContent(l10n))
                    else
                      Expanded(child: _buildContent(l10n)),
                  ],
                ),
        ),
      ),
    );
  }

  /// Builds the dialog header.
  Widget _buildHeader(AppLocalizations l10n) {
    // Get title based on current step
    String title;
    switch (_currentStep) {
      case _stepQuickStart:
        title = l10n.onboardingQuickStartTitle;
        break;
      case _stepSecurityModel:
        title = l10n.onboardingSecurityModelTitle;
        break;
      case _stepBotModel:
        title = l10n.onboardingBotModelTitle;
        break;
      case _stepConfigUpdate:
        title = l10n.onboardingConfigUpdateTitle;
        break;
      default:
        title = l10n.onboardingTitle;
    }

    return Row(
      children: [
        Container(
          padding: const EdgeInsets.all(8),
          decoration: BoxDecoration(
            color: const Color(0xFF6366F1).withValues(alpha: 0.2),
            borderRadius: BorderRadius.circular(8),
          ),
          child: const Icon(
            LucideIcons.sparkles,
            color: Color(0xFF6366F1),
            size: 20,
          ),
        ),
        const SizedBox(width: 12),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                title,
                style: AppFonts.inter(
                  fontSize: 18,
                  fontWeight: FontWeight.w600,
                  color: Colors.white,
                ),
              ),
            ],
          ),
        ),
      ],
    );
  }

  Widget _buildContent(AppLocalizations l10n) {
    if (!BuildConfig.isAppStore) {
      switch (_currentStep) {
        case _stepQuickStart:
          return _buildQuickStartStep(l10n);
        case _stepBotModel:
          return _buildBotModelStep(l10n);
        case _stepSecurityModel:
          return _buildSecurityModelStep(l10n);
        default:
          break;
      }
      if (!_botModelConfigured) {
        return _buildBotModelStep(l10n);
      }
      if (!_modelConfigured) {
        return _buildSecurityModelStep(l10n);
      }
      if (!_autoFinished) {
        _autoFinished = true;
        WidgetsBinding.instance.addPostFrameCallback((_) {
          _finishOnboarding();
        });
      }
      return Center(
        child: Text(
          l10n.onboardingPersonalDone,
          style: AppFonts.inter(fontSize: 14, color: Colors.white70),
        ),
      );
    }

    if (_modelConfigured && _botModelConfigured && _configUpdateCompleted) {
      if (!_autoFinished) {
        _autoFinished = true;
        WidgetsBinding.instance.addPostFrameCallback((_) {
          _finishOnboarding();
        });
      }
      return const SizedBox.shrink();
    }

    switch (_currentStep) {
      case _stepQuickStart:
        return _buildQuickStartStep(l10n);
      case _stepBotModel:
        return _buildBotModelStep(l10n);
      case _stepSecurityModel:
        return _buildSecurityModelStep(l10n);
      case _stepConfigUpdate:
        return _buildConfigUpdateStep(l10n);
      default:
        return _buildQuickStartStep(l10n);
    }
  }

  /// Builds quick start step for App Store onboarding.
  Widget _buildQuickStartStep(AppLocalizations l10n) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          l10n.onboardingQuickStartDesc,
          style: AppFonts.inter(
            fontSize: 14,
            fontWeight: FontWeight.w400,
            color: Colors.white.withValues(alpha: 0.85),
            height: 1.6,
            letterSpacing: 0.2,
          ),
        ),
        const SizedBox(height: 28),
        _buildFeatureItem(
          icon: LucideIcons.shield,
          title: l10n.onboardingFeatureInjectTitle,
          description: l10n.onboardingFeatureInjectDesc,
        ),
        const SizedBox(height: 16),
        _buildFeatureItem(
          icon: LucideIcons.sliders,
          title: l10n.onboardingFeaturePermissionTitle,
          description: l10n.onboardingFeaturePermissionDesc,
        ),
        const SizedBox(height: 16),
        _buildFeatureItem(
          icon: LucideIcons.activity,
          title: l10n.onboardingFeatureBaselineTitle,
          description: l10n.onboardingFeatureBaselineDesc,
        ),
        const Spacer(),
        Align(
          alignment: Alignment.centerRight,
          child: ElevatedButton(
            onPressed: _goToNextStep,
            style: ElevatedButton.styleFrom(
              backgroundColor: const Color(0xFF6366F1),
              foregroundColor: Colors.white,
              padding: const EdgeInsets.symmetric(horizontal: 24, vertical: 14),
              shape: RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(10),
              ),
              elevation: 0,
            ),
            child: Text(
              l10n.onboardingActionNext,
              style: AppFonts.inter(
                fontWeight: FontWeight.w600,
                fontSize: 14,
                letterSpacing: 0.2,
              ),
            ),
          ),
        ),
      ],
    );
  }

  /// Builds security model configuration step (now step 2, after bot model).
  Widget _buildSecurityModelStep(AppLocalizations l10n) {
    // Determine button text based on whether this is the last step
    final buttonText = BuildConfig.isAppStore
        ? l10n.onboardingActionSaveNext
        : l10n.onboardingActionSaveFinish;

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        _buildStepIntro(l10n.onboardingSecurityModelDesc),
        const SizedBox(height: 14),
        // Reuse bot model checkbox
        _buildReuseBotModelCheckbox(l10n),
        const SizedBox(height: 14),
        Expanded(
          child: SingleChildScrollView(
            child: Container(
              padding: const EdgeInsets.all(16),
              decoration: BoxDecoration(
                color: Colors.black.withValues(alpha: 0.2),
                borderRadius: BorderRadius.circular(12),
                border: Border.all(color: Colors.white.withValues(alpha: 0.08)),
              ),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  SecurityModelConfigForm(
                    key: _reuseBotModel
                        ? const ValueKey('security_form_reuse')
                        : _securityModelFormKey,
                    readOnly: _reuseBotModel,
                    initialConfig: _reuseBotModel
                        ? _securityModelFromBot
                        : null,
                  ),
                  const SizedBox(height: 20),
                  Row(
                    mainAxisAlignment: MainAxisAlignment.end,
                    children: [
                      TextButton(
                        onPressed: _saving ? null : _goToPreviousStep,
                        style: TextButton.styleFrom(
                          padding: const EdgeInsets.symmetric(
                            horizontal: 20,
                            vertical: 14,
                          ),
                        ),
                        child: Text(
                          l10n.onboardingActionBack,
                          style: AppFonts.inter(
                            fontSize: 14,
                            color: _saving ? Colors.white24 : Colors.white54,
                          ),
                        ),
                      ),
                      const SizedBox(width: 12),
                      ElevatedButton(
                        onPressed: _saving ? null : _handleModelConfigSaved,
                        style: ElevatedButton.styleFrom(
                          backgroundColor: const Color(0xFF6366F1),
                          foregroundColor: Colors.white,
                          padding: const EdgeInsets.symmetric(
                            horizontal: 24,
                            vertical: 14,
                          ),
                          shape: RoundedRectangleBorder(
                            borderRadius: BorderRadius.circular(10),
                          ),
                          elevation: 0,
                        ),
                        child: _saving
                            ? Row(
                                mainAxisSize: MainAxisSize.min,
                                children: [
                                  const SizedBox(
                                    width: 14,
                                    height: 14,
                                    child: CircularProgressIndicator(
                                      strokeWidth: 2,
                                      color: Colors.white,
                                    ),
                                  ),
                                  const SizedBox(width: 10),
                                  Text(
                                    l10n.modelConfigSaving,
                                    style: AppFonts.inter(
                                      fontWeight: FontWeight.w600,
                                      fontSize: 14,
                                    ),
                                  ),
                                ],
                              )
                            : Text(
                                buttonText,
                                style: AppFonts.inter(
                                  fontWeight: FontWeight.w600,
                                  fontSize: 14,
                                  letterSpacing: 0.2,
                                ),
                              ),
                      ),
                    ],
                  ),
                ],
              ),
            ),
          ),
        ),
      ],
    );
  }

  /// Builds the reuse bot model checkbox widget.
  Widget _buildReuseBotModelCheckbox(AppLocalizations l10n) {
    return GestureDetector(
      onTap: () {
        setState(() {
          _reuseBotModel = !_reuseBotModel;
        });
      },
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
        decoration: BoxDecoration(
          color: _reuseBotModel
              ? const Color(0xFF6366F1).withValues(alpha: 0.08)
              : Colors.transparent,
          borderRadius: BorderRadius.circular(10),
          border: Border.all(
            color: _reuseBotModel
                ? const Color(0xFF6366F1).withValues(alpha: 0.3)
                : Colors.white.withValues(alpha: 0.1),
          ),
        ),
        child: Row(
          children: [
            SizedBox(
              width: 20,
              height: 20,
              child: Checkbox(
                value: _reuseBotModel,
                onChanged: (value) {
                  setState(() {
                    _reuseBotModel = value ?? true;
                  });
                },
                activeColor: const Color(0xFF6366F1),
                checkColor: Colors.white,
                side: BorderSide(
                  color: _reuseBotModel
                      ? const Color(0xFF6366F1)
                      : Colors.white38,
                  width: 1.5,
                ),
                shape: RoundedRectangleBorder(
                  borderRadius: BorderRadius.circular(4),
                ),
              ),
            ),
            const SizedBox(width: 10),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    l10n.onboardingReuseBotModel,
                    style: AppFonts.inter(
                      fontSize: 13,
                      fontWeight: FontWeight.w600,
                      color: Colors.white.withValues(alpha: 0.9),
                    ),
                  ),
                  const SizedBox(height: 2),
                  Text(
                    l10n.onboardingReuseBotModelHint,
                    style: AppFonts.inter(
                      fontSize: 11.5,
                      fontWeight: FontWeight.w400,
                      color: Colors.white.withValues(alpha: 0.5),
                    ),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }

  /// Builds bot model configuration step.
  Widget _buildBotModelStep(AppLocalizations l10n) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        _buildStepIntro(l10n.onboardingBotModelDesc),
        const SizedBox(height: 18),
        Expanded(
          child: SingleChildScrollView(
            child: Container(
              padding: const EdgeInsets.all(16),
              decoration: BoxDecoration(
                color: Colors.black.withValues(alpha: 0.2),
                borderRadius: BorderRadius.circular(12),
                border: Border.all(color: Colors.white.withValues(alpha: 0.08)),
              ),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  BotModelConfigForm(
                    key: _botModelFormKey,
                    assetName: _defaultAssetName,
                  ),
                  const SizedBox(height: 20),
                  Row(
                    mainAxisAlignment: MainAxisAlignment.end,
                    children: [
                      TextButton(
                        onPressed: _saving ? null : _goToPreviousStep,
                        style: TextButton.styleFrom(
                          padding: const EdgeInsets.symmetric(
                            horizontal: 20,
                            vertical: 14,
                          ),
                        ),
                        child: Text(
                          l10n.onboardingActionBack,
                          style: AppFonts.inter(
                            fontSize: 14,
                            color: _saving ? Colors.white24 : Colors.white54,
                          ),
                        ),
                      ),
                      const SizedBox(width: 12),
                      ElevatedButton(
                        onPressed: _saving ? null : _handleBotModelConfigSaved,
                        style: ElevatedButton.styleFrom(
                          backgroundColor: const Color(0xFF10B981),
                          foregroundColor: Colors.white,
                          padding: const EdgeInsets.symmetric(
                            horizontal: 24,
                            vertical: 14,
                          ),
                          shape: RoundedRectangleBorder(
                            borderRadius: BorderRadius.circular(10),
                          ),
                          elevation: 0,
                        ),
                        child: _saving
                            ? Row(
                                mainAxisSize: MainAxisSize.min,
                                children: [
                                  const SizedBox(
                                    width: 14,
                                    height: 14,
                                    child: CircularProgressIndicator(
                                      strokeWidth: 2,
                                      color: Colors.white,
                                    ),
                                  ),
                                  const SizedBox(width: 10),
                                  Text(
                                    l10n.modelConfigSaving,
                                    style: AppFonts.inter(
                                      fontWeight: FontWeight.w600,
                                      fontSize: 14,
                                    ),
                                  ),
                                ],
                              )
                            : Text(
                                l10n.onboardingActionSaveNext,
                                style: AppFonts.inter(
                                  fontWeight: FontWeight.w600,
                                  fontSize: 14,
                                  letterSpacing: 0.2,
                                ),
                              ),
                      ),
                    ],
                  ),
                ],
              ),
            ),
          ),
        ),
      ],
    );
  }

  /// Builds bot configuration update step.
  Widget _buildConfigUpdateStep(AppLocalizations l10n) {
    final provider = _botProviderType.isNotEmpty ? _botProviderType : '未填写';
    final modelName = _botModelName.isNotEmpty ? _botModelName : '未填写';
    final baseUrl = _proxyBaseUrl;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Expanded(
          child: SingleChildScrollView(
            child: Container(
              padding: const EdgeInsets.all(16),
              decoration: BoxDecoration(
                color: Colors.black.withValues(alpha: 0.2),
                borderRadius: BorderRadius.circular(12),
                border: Border.all(color: Colors.white.withValues(alpha: 0.08)),
              ),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  _buildConfigUpdateNotice(l10n.onboardingConfigUpdateDesc),
                  const SizedBox(height: 20),
                  Container(
                    width: double.infinity,
                    padding: const EdgeInsets.all(14),
                    decoration: BoxDecoration(
                      color: Colors.black.withValues(alpha: 0.3),
                      borderRadius: BorderRadius.circular(8),
                      border: Border.all(
                        color: Colors.white.withValues(alpha: 0.08),
                      ),
                    ),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          '操作步骤',
                          style: AppFonts.inter(
                            fontSize: 13,
                            fontWeight: FontWeight.w600,
                            color: Colors.white.withValues(alpha: 0.9),
                          ),
                        ),
                        const SizedBox(height: 12),
                        _buildConfigUpdateStepItem(
                          index: '1',
                          text:
                              '执行 openclaw dashboard --no-open 打开 web 页面。\n'
                              '点击左侧 Settings-Config 进入配置。',
                        ),
                        const SizedBox(height: 10),
                        _buildConfigUpdateStepItem(
                          index: '2',
                          text:
                              '在 Models-Providers 中添加供应商。\n'
                              '供应商名称：clawdsecbot-$provider。\n'
                              '模型名称：$modelName。\n'
                              'BaseUrl：$baseUrl。\n'
                              '其余模型名称、API、API Key 保持和原先使用Bot模型配置一样。',
                        ),
                        const SizedBox(height: 10),
                        _buildConfigUpdateStepItem(
                          index: '3',
                          text:
                              '在 Agents-Defaults 中配置默认使用上一步添加的模型。\n'
                              'PrimaryModel：clawdsecbot-$provider/$modelName。\n'
                              'Models：clawdsecbot-$provider/$modelName。',
                        ),
                        const SizedBox(height: 10),
                        _buildConfigUpdateStepItem(
                          index: '4',
                          text: '页面右上角点击 Save 保存，再点击 Reload 加载新配置。',
                        ),
                      ],
                    ),
                  ),
                ],
              ),
            ),
          ),
        ),
        const SizedBox(height: 20),
        // Action buttons
        Row(
          mainAxisAlignment: MainAxisAlignment.end,
          children: [
            TextButton(
              onPressed: _goToPreviousStep,
              style: TextButton.styleFrom(
                padding: const EdgeInsets.symmetric(
                  horizontal: 20,
                  vertical: 14,
                ),
                shape: RoundedRectangleBorder(
                  borderRadius: BorderRadius.circular(10),
                ),
              ),
              child: Text(
                l10n.onboardingActionBack,
                style: AppFonts.inter(
                  fontWeight: FontWeight.w600,
                  fontSize: 14,
                  color: Colors.white54,
                ),
              ),
            ),
            const SizedBox(width: 12),
            ElevatedButton(
              onPressed: _handleConfigUpdateCompleted,
              style: ElevatedButton.styleFrom(
                backgroundColor: const Color(0xFF6366F1),
                foregroundColor: Colors.white,
                padding: const EdgeInsets.symmetric(
                  horizontal: 24,
                  vertical: 14,
                ),
                shape: RoundedRectangleBorder(
                  borderRadius: BorderRadius.circular(10),
                ),
                elevation: 0,
              ),
              child: Text(
                l10n.onboardingConfigUpdateComplete,
                style: AppFonts.inter(
                  fontWeight: FontWeight.w600,
                  fontSize: 14,
                  letterSpacing: 0.2,
                ),
              ),
            ),
          ],
        ),
      ],
    );
  }

  /// Builds a muted notice card for config update guidance.
  Widget _buildConfigUpdateNotice(String text) {
    return Container(
      padding: const EdgeInsets.all(14),
      decoration: BoxDecoration(
        color: const Color(0xFF242015).withValues(alpha: 0.85),
        borderRadius: BorderRadius.circular(10),
        border: Border.all(
          color: const Color(0xFF8F7A4A).withValues(alpha: 0.5),
        ),
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Container(
            width: 26,
            height: 26,
            alignment: Alignment.center,
            decoration: BoxDecoration(
              color: const Color(0xFF3A2E18).withValues(alpha: 0.8),
              borderRadius: BorderRadius.circular(8),
            ),
            child: const Icon(
              LucideIcons.alertTriangle,
              color: Color(0xFFD6B676),
              size: 16,
            ),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Text(
              text,
              style: AppFonts.inter(
                fontSize: 13.5,
                fontWeight: FontWeight.w400,
                color: Colors.white.withValues(alpha: 0.86),
                height: 1.6,
                letterSpacing: 0.2,
              ),
            ),
          ),
        ],
      ),
    );
  }

  /// Builds a feature item with icon, title, and description.
  Widget _buildFeatureItem({
    required IconData icon,
    required String title,
    required String description,
  }) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 18, vertical: 16),
      decoration: BoxDecoration(
        color: Colors.black.withValues(alpha: 0.3),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(
          color: Colors.white.withValues(alpha: 0.06),
          width: 1,
        ),
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Container(
            padding: const EdgeInsets.all(12),
            decoration: BoxDecoration(
              color: const Color(0xFF6366F1).withValues(alpha: 0.12),
              borderRadius: BorderRadius.circular(10),
            ),
            child: Icon(icon, color: const Color(0xFF8B8DF6), size: 20),
          ),
          const SizedBox(width: 14),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  title,
                  style: AppFonts.inter(
                    fontSize: 14,
                    fontWeight: FontWeight.w600,
                    color: Colors.white,
                    height: 1.4,
                    letterSpacing: 0.1,
                  ),
                ),
                const SizedBox(height: 6),
                Text(
                  description,
                  style: AppFonts.inter(
                    fontSize: 12.5,
                    fontWeight: FontWeight.w400,
                    color: Colors.white.withValues(alpha: 0.72),
                    height: 1.5,
                    letterSpacing: 0.05,
                  ),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }

  /// Builds a numbered instruction row for config update steps.
  Widget _buildConfigUpdateStepItem({
    required String index,
    required String text,
  }) {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Container(
          width: 22,
          height: 22,
          alignment: Alignment.center,
          decoration: BoxDecoration(
            color: const Color(0xFF6366F1).withValues(alpha: 0.2),
            borderRadius: BorderRadius.circular(11),
            border: Border.all(
              color: const Color(0xFF6366F1).withValues(alpha: 0.4),
            ),
          ),
          child: Text(
            index,
            style: AppFonts.inter(
              fontSize: 12,
              fontWeight: FontWeight.w600,
              color: const Color(0xFF8B8DF6),
            ),
          ),
        ),
        const SizedBox(width: 10),
        Expanded(
          child: Text(
            text,
            style: AppFonts.inter(
              fontSize: 12.5,
              fontWeight: FontWeight.w400,
              color: Colors.white.withValues(alpha: 0.8),
              height: 1.5,
            ),
          ),
        ),
      ],
    );
  }

  /// Builds a calm, trustworthy intro banner for a step.
  Widget _buildStepIntro(String text) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
      decoration: BoxDecoration(
        color: const Color(0xFF0F1C16).withValues(alpha: 0.35),
        borderRadius: BorderRadius.circular(14),
        border: Border.all(
          color: const Color(0xFF8ED9AA).withValues(alpha: 0.28),
        ),
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Container(
            width: 28,
            height: 28,
            alignment: Alignment.center,
            decoration: BoxDecoration(
              color: const Color(0xFF1C3A2A).withValues(alpha: 0.6),
              borderRadius: BorderRadius.circular(8),
            ),
            child: const Icon(
              LucideIcons.lightbulb,
              color: Color(0xFF9BE7B6),
              size: 16,
            ),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Text(
              text,
              style: AppFonts.inter(
                fontSize: 13.5,
                fontWeight: FontWeight.w400,
                color: Colors.white.withValues(alpha: 0.82),
                height: 1.6,
                letterSpacing: 0.2,
              ),
            ),
          ),
        ],
      ),
    );
  }

  /// Moves to the next onboarding step.
  void _goToNextStep() {
    if (!mounted) return;
    setState(() {
      _currentStep = (_currentStep + 1).clamp(
        _stepQuickStart,
        _stepConfigUpdate,
      );
    });
  }

  /// Moves to the previous onboarding step.
  void _goToPreviousStep() {
    if (!mounted) return;
    setState(() {
      _currentStep = (_currentStep - 1).clamp(
        _stepQuickStart,
        _stepConfigUpdate,
      );
    });
  }
}
