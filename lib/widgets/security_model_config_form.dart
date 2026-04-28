import 'dart:async';
import 'dart:convert';
import 'package:flutter/material.dart';
import '../utils/app_fonts.dart';
import 'package:lucide_icons/lucide_icons.dart';
import '../l10n/app_localizations.dart';
import '../models/llm_config_model.dart';
import '../services/app_settings_database_service.dart';
import '../services/model_config_service.dart';
import '../services/protection_service.dart';
import '../services/provider_service.dart';
import '../utils/app_logger.dart';
import 'model_id_picker.dart';
import 'model_provider_selector.dart';
import 'processing_notice_card.dart';

/// 安全模型配置表单（纯表单组件）
/// 用于 ShepherdGate 风险检测的 LLM 配置
/// 表单本体不包含保存按钮，保存动作由调用方通过 GlobalKey 触发 saveConfig()
class SecurityModelConfigForm extends StatefulWidget {
  /// Creates a security model configuration form.
  const SecurityModelConfigForm({
    super.key,
    this.readOnly = false,
    this.initialConfig,
  });

  /// When true, all form fields and provider chips are disabled.
  final bool readOnly;

  /// Optional initial config to pre-fill the form (used for reuse scenario).
  final SecurityModelConfig? initialConfig;

  @override
  State<SecurityModelConfigForm> createState() =>
      SecurityModelConfigFormState();
}

/// State for security model configuration form.
class SecurityModelConfigFormState extends State<SecurityModelConfigForm> {
  /// 安全模型按供应商草稿缓存的设置键。
  static const String _providerDraftSettingKey =
      'security_model_provider_drafts_v1';

  late SecurityModelConfigService _service;
  final ProviderService _providerService = ProviderService();
  final AppSettingsDatabaseService _appSettingsService =
      AppSettingsDatabaseService();
  bool _loading = true;
  bool _saving = false;
  bool _testing = false;
  bool _showApiKey = false;
  bool _showSecretKey = false;
  String? _error;

  /// 连通性测试版本号：每次发起测试自增，切换 provider / 关闭表单 /
  /// 发起新测试时自增即可使在途请求的结果被丢弃，实现 UI 侧"取消"。
  int _testSeq = 0;

  /// 当前在途的连通性测试 completer。切换 provider / 关闭表单 / 重新发起测试
  /// 时以 null 完成它，让 [validateConnection] 的 Future 立即 resolve，
  /// 外层按钮的 loading 态可以即时清除（不必等 Go 侧最长 30s 的超时）。
  Completer<Map<String, dynamic>?>? _activeTestCompleter;

  final TextEditingController _endpointController = TextEditingController();
  final TextEditingController _apiKeyController = TextEditingController();
  final TextEditingController _modelController = TextEditingController();
  final TextEditingController _secretKeyController = TextEditingController();
  String _selectedType = 'ollama';
  final Map<String, SecurityModelConfig> _providerDrafts = {};

  /// Dynamically loaded providers from Go layer.
  List<ProviderInfo> _providers = [];

  /// Map provider icon names to IconData.
  static final _iconMap = <String, IconData>{
    'server': LucideIcons.server,
    'sparkles': LucideIcons.sparkles,
    'zap': LucideIcons.zap,
    'message-square': LucideIcons.messageSquare,
    'star': LucideIcons.star,
    'flame': LucideIcons.flame,
    'cloud': LucideIcons.cloud,
    'bot': LucideIcons.bot,
    'plug': LucideIcons.workflow,
  };

  @override
  void initState() {
    super.initState();
    _service = SecurityModelConfigService();
    _loadProviders();
    _loadConfig();
  }

  /// Loads providers from Go layer via FFI.
  void _loadProviders() {
    _providers = _providerService.getProviders(ProviderScope.security);
    _selectedType = _getDefaultType();
  }

  /// Get icon for provider.
  IconData _getIconForProvider(String iconName) {
    return _iconMap[iconName] ?? LucideIcons.sparkles;
  }

  /// Returns available providers.
  List<ProviderInfo> _getVisibleProviders() {
    return _providers;
  }

  /// Returns the default model type.
  String _getDefaultType() {
    return _providers.isNotEmpty ? _providers.first.name : 'ollama';
  }

  /// Normalizes loaded model type to an available option.
  String _normalizeSelectedType(String type) {
    final isVisible = _providers.any((p) => p.name == type);
    return isVisible ? type : _getDefaultType();
  }

  /// Gets ProviderInfo for the selected type.
  ProviderInfo? _getSelectedProviderInfo() {
    try {
      return _providers.firstWhere((p) => p.name == _selectedType);
    } catch (_) {
      return null;
    }
  }

  /// Loads current configuration into the form.
  Future<void> _loadConfig() async {
    // Use initialConfig if provided (reuse scenario)
    if (widget.initialConfig != null) {
      final config = widget.initialConfig!;
      _providerDrafts[config.provider] = config;
      setState(() {
        _selectedType = _normalizeSelectedType(config.provider);
        _applyConfigToControllers(config);
        _loading = false;
      });
      return;
    }

    try {
      await _loadProviderDrafts();
      final config = await _service.loadConfig();
      _providerDrafts[config.provider] = config;
      final selectedType = _normalizeSelectedType(config.provider);
      final selectedConfig =
          _providerDrafts[selectedType] ?? _createEmptyConfig(selectedType);
      setState(() {
        _selectedType = selectedType;
        _applyConfigToControllers(selectedConfig);
        _loading = false;
      });
    } catch (e) {
      setState(() {
        _error = e.toString();
        _loading = false;
      });
    }
  }

  /// 保存配置并返回保存结果。
  /// 按需调用验证按钮进行连通性测试，保存流程不做额外连接校验。
  /// This is the public method that should be called by parent widgets.
  Future<bool> saveConfig() async {
    final l10n = AppLocalizations.of(context)!;
    setState(() {
      _saving = true;
      _error = null;
    });

    final config = _buildCurrentConfig();

    if (!_hasRequiredFields(config)) {
      setState(() {
        _error = l10n.modelConfigFillRequired;
        _saving = false;
      });
      return false;
    }

    try {
      final testResult = await _runWithConnectionVerifyNotice(
        () => _service.testConnection(config),
      );
      if (testResult['success'] != true) {
        setState(() {
          _error = l10n.modelConfigTestFailed(
            testResult['error']?.toString() ?? l10n.modelConfigUnknownError,
          );
          _saving = false;
        });
        return false;
      }

      final success = await _service.saveConfig(config);
      if (success) {
        _captureCurrentProviderDraft();
        await _persistProviderDrafts();

        // 保存后热更新 ShepherdGate
        try {
          final protectionService = ProtectionService();
          await protectionService.updateSecurityModelConfig(config);
          appLogger.info(
            '[SecurityModelConfigForm] Security model hot reloaded',
          );
        } catch (e) {
          appLogger.warning(
            '[SecurityModelConfigForm] Failed to hot reload security model: $e',
          );
        }

        if (mounted) {
          setState(() {
            _saving = false;
          });
        }
        return true;
      } else {
        setState(() {
          _error = l10n.modelConfigSaveFailed;
          _saving = false;
        });
        return false;
      }
    } catch (e) {
      setState(() {
        _error = e.toString();
        _saving = false;
      });
      return false;
    }
  }

  /// 保存前执行连通性验证，并显示处理中提示。
  /// 若 30 秒后仍未返回，则将提示文案切换为慢响应提醒。
  Future<Map<String, dynamic>> _runWithConnectionVerifyNotice(
    Future<Map<String, dynamic>> Function() action,
  ) async {
    if (!mounted) {
      return action();
    }
    final l10n = AppLocalizations.of(context)!;
    final messageNotifier = ValueNotifier<String>(
      l10n.modelConfigVerifyingConnectionMessage,
    );
    bool verifyCompleted = false;
    Timer? slowResponseTimer;
    slowResponseTimer = Timer(const Duration(seconds: 30), () {
      if (verifyCompleted) {
        return;
      }
      messageNotifier.value = l10n.modelConfigSlowResponseHint;
    });
    unawaited(
      showDialog<void>(
        context: context,
        barrierDismissible: false,
        builder: (dialogContext) => Dialog(
          backgroundColor: Colors.transparent,
          elevation: 0,
          insetPadding: const EdgeInsets.symmetric(horizontal: 28),
          child: ValueListenableBuilder<String>(
            valueListenable: messageNotifier,
            builder: (context, message, child) => ProcessingNoticeCard(
              title: AppLocalizations.of(
                dialogContext,
              )!.modelConfigVerifyingConnectionTitle,
              message: message,
            ),
          ),
        ),
      ),
    );
    await Future.delayed(const Duration(milliseconds: 50));
    try {
      return await action();
    } finally {
      verifyCompleted = true;
      slowResponseTimer.cancel();
      if (mounted) {
        final navigator = Navigator.of(context, rootNavigator: true);
        await navigator.maybePop();
      }
      messageNotifier.dispose();
    }
  }

  /// 手动验证当前配置连通性，不触发持久化。
  /// 使用 [_testSeq] + [_activeTestCompleter] 做 UI 侧取消：
  /// 切换 provider、关闭弹窗、再次点击测试按钮时，前一次请求会被标记为已取消，
  /// 其 Future 立刻以 false 返回，外层按钮 loading 态可以即时清除。
  Future<bool> validateConnection() async {
    final l10n = AppLocalizations.of(context)!;
    final config = _buildCurrentConfig();
    if (!_hasRequiredFields(config)) {
      setState(() {
        _error = l10n.modelConfigFillRequired;
      });
      return false;
    }

    // 若此前仍有在途测试，以 null 完成其 completer，让对应 Future 立即返回。
    final previous = _activeTestCompleter;
    if (previous != null && !previous.isCompleted) {
      previous.complete(null);
    }

    final int seq = ++_testSeq;
    final completer = Completer<Map<String, dynamic>?>();
    _activeTestCompleter = completer;

    setState(() {
      _testing = true;
      _error = null;
    });

    // 后台 isolate 的真实 FFI 调用；完成后回填 completer，若已被取消则忽略。
    unawaited(
      _service
          .testConnection(config)
          .then((result) {
            if (!completer.isCompleted) {
              completer.complete(result);
            }
          })
          .catchError((Object e) {
            if (!completer.isCompleted) {
              completer.complete({'success': false, 'error': e.toString()});
            }
          }),
    );

    final result = await completer.future;
    if (identical(_activeTestCompleter, completer)) {
      _activeTestCompleter = null;
    }

    // Widget 已销毁、被新序列号取代或被主动取消时，直接丢弃结果。
    if (!mounted || seq != _testSeq || result == null) {
      return false;
    }

    if (result['success'] == true) {
      setState(() {
        _testing = false;
      });
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(l10n.modelConfigTestSuccess),
          backgroundColor: Colors.green,
        ),
      );
      return true;
    }

    setState(() {
      _error = l10n.modelConfigTestFailed(
        result['error']?.toString() ?? l10n.modelConfigUnknownError,
      );
      _testing = false;
    });
    return false;
  }

  /// 构建当前表单配置对象。
  SecurityModelConfig _buildCurrentConfig() {
    return SecurityModelConfig(
      provider: _selectedType,
      endpoint: _endpointController.text.trim(),
      apiKey: _apiKeyController.text.trim(),
      model: _modelController.text.trim(),
      secretKey: _secretKeyController.text.trim(),
    );
  }

  /// 根据当前 provider 能力检查必填字段。
  bool _hasRequiredFields(SecurityModelConfig config) {
    final providerInfo = _getSelectedProviderInfo();
    final needsEndpoint = providerInfo?.needsEndpoint ?? true;
    final needsApiKey = providerInfo?.needsAPIKey ?? true;
    final needsSecretKey = providerInfo?.needsSecretKey ?? false;
    if (needsEndpoint && config.endpoint.isEmpty) {
      return false;
    }
    if (needsApiKey && config.apiKey.isEmpty) {
      return false;
    }
    if (needsSecretKey && config.secretKey.isEmpty) {
      return false;
    }
    if (config.model.isEmpty) {
      return false;
    }
    return true;
  }

  /// 将配置应用到当前输入控件。
  void _applyConfigToControllers(SecurityModelConfig config) {
    _endpointController.text = config.endpoint;
    _apiKeyController.text = config.apiKey;
    _modelController.text = config.model;
    _secretKeyController.text = config.secretKey;
  }

  /// 创建空白配置，用于首次切换到新 provider。
  SecurityModelConfig _createEmptyConfig(String providerName) {
    return SecurityModelConfig(
      provider: providerName,
      endpoint: '',
      apiKey: '',
      model: '',
      secretKey: '',
    );
  }

  /// 记录当前 provider 的草稿。
  void _captureCurrentProviderDraft() {
    _providerDrafts[_selectedType] = _buildCurrentConfig();
  }

  /// 首次切换到无草稿的 provider 时填入 Go 默认 endpoint / model；兼容协议保持空白。
  void _applyProviderDefaultsForSelection(ProviderInfo p) {
    if (p.name == 'openai_compatible' || p.name == 'anthropic_compatible') {
      _endpointController.clear();
      _apiKeyController.clear();
      _modelController.clear();
      return;
    }
    if (p.defaultBaseURL.isNotEmpty &&
        _endpointController.text.trim().isEmpty) {
      _endpointController.text = p.defaultBaseURL;
    }
    if (p.defaultModel.isNotEmpty && _modelController.text.trim().isEmpty) {
      _modelController.text = p.defaultModel;
    }
  }

  /// 切换 provider 时加载其对应草稿，避免输入被清空。
  /// 同步作废可能在途的连通性测试结果，并立即停止 loading 指示。
  Future<void> _handleProviderSelected(ProviderInfo provider) async {
    _testSeq++;
    final pending = _activeTestCompleter;
    if (pending != null && !pending.isCompleted) {
      pending.complete(null);
    }
    _activeTestCompleter = null;
    final hadDraft = _providerDrafts.containsKey(provider.name);
    _captureCurrentProviderDraft();
    final targetConfig =
        _providerDrafts[provider.name] ?? _createEmptyConfig(provider.name);
    if (!mounted) return;
    setState(() {
      _selectedType = provider.name;
      _applyConfigToControllers(targetConfig);
      if (!hadDraft) {
        _applyProviderDefaultsForSelection(provider);
      }
      _error = null;
      _testing = false;
    });
    await _persistProviderDrafts();
  }

  /// 从应用设置加载 provider 草稿缓存。
  Future<void> _loadProviderDrafts() async {
    final raw = await _appSettingsService.getSetting(_providerDraftSettingKey);
    if (raw.isEmpty) {
      return;
    }
    try {
      final decoded = jsonDecode(raw);
      if (decoded is! Map<String, dynamic>) {
        return;
      }
      decoded.forEach((provider, value) {
        if (value is! Map) {
          return;
        }
        final data = Map<String, dynamic>.from(value);
        _providerDrafts[provider] = SecurityModelConfig(
          provider: provider,
          endpoint: (data['endpoint'] as String?) ?? '',
          apiKey: (data['api_key'] as String?) ?? '',
          model: (data['model'] as String?) ?? '',
          secretKey: (data['secret_key'] as String?) ?? '',
        );
      });
    } catch (e) {
      appLogger.warning(
        '[SecurityModelConfigForm] Failed to parse provider drafts: $e',
      );
    }
  }

  /// 持久化 provider 草稿缓存。
  Future<void> _persistProviderDrafts() async {
    final payload = <String, dynamic>{};
    _providerDrafts.forEach((provider, config) {
      payload[provider] = {
        'endpoint': config.endpoint,
        'api_key': config.apiKey,
        'model': config.model,
        'secret_key': config.secretKey,
      };
    });
    final saved = await _appSettingsService.saveSetting(
      _providerDraftSettingKey,
      jsonEncode(payload),
    );
    if (!saved) {
      appLogger.warning(
        '[SecurityModelConfigForm] Failed to persist provider drafts',
      );
    }
  }

  /// Returns whether the form is currently saving.
  bool get isSaving => _saving;

  /// Returns whether the form is currently testing.
  bool get isTesting => _testing;

  @override
  void dispose() {
    // Let any in-flight validateConnection future resolve immediately.
    final pending = _activeTestCompleter;
    if (pending != null && !pending.isCompleted) {
      pending.complete(null);
    }
    _activeTestCompleter = null;
    _endpointController.dispose();
    _apiKeyController.dispose();
    _modelController.dispose();
    _secretKeyController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    if (_loading) {
      return const Center(child: CircularProgressIndicator());
    }

    return SingleChildScrollView(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          _buildTypeSelector(),
          const SizedBox(height: 16),
          _buildFormFields(),
          if (_error != null) ...[const SizedBox(height: 16), _buildError()],
        ],
      ),
    );
  }

  /// Builds the model provider selector.
  Widget _buildTypeSelector() {
    return ModelProviderSelector(
      providers: _getVisibleProviders(),
      selectedName: _selectedType,
      onProviderSelected: (p) {
        unawaited(_handleProviderSelected(p));
      },
      iconForName: _getIconForProvider,
      readOnly: widget.readOnly,
      accentColor: const Color(0xFF6366F1),
    );
  }

  /// Builds the form input fields.
  Widget _buildFormFields() {
    final l10n = AppLocalizations.of(context)!;
    final providerInfo = _getSelectedProviderInfo();
    final needsEndpoint = providerInfo?.needsEndpoint ?? true;
    final needsApiKey = providerInfo?.needsAPIKey ?? true;
    final needsSecretKey = providerInfo?.needsSecretKey ?? false;

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        if (needsEndpoint)
          _buildTextField(
            controller: _endpointController,
            label: l10n.modelConfigBaseUrl,
            hint: providerInfo?.defaultBaseURL ?? '',
            icon: LucideIcons.link,
          ),
        if (needsApiKey) ...[
          const SizedBox(height: 12),
          _buildTextField(
            controller: _apiKeyController,
            label: needsSecretKey
                ? l10n.modelConfigAccessKey
                : l10n.modelConfigApiKey,
            hint: providerInfo?.apiKeyHint ?? 'Your API key',
            icon: LucideIcons.key,
            obscureText: !_showApiKey,
            onToggleObscureText: widget.readOnly
                ? null
                : () => setState(() {
                    _showApiKey = !_showApiKey;
                  }),
          ),
        ],
        const SizedBox(height: 12),
        ModelIdPicker(
          controller: _modelController,
          providerId: _selectedType,
          baseUrl: () => _endpointController.text.trim(),
          apiKey: () => _apiKeyController.text.trim(),
          label: l10n.modelConfigModelName,
          hint: providerInfo?.modelHint ?? 'Model name',
          icon: LucideIcons.box,
          useFiraCode: false,
          enabled: !widget.readOnly,
        ),
        if (needsSecretKey) ...[
          const SizedBox(height: 12),
          _buildTextField(
            controller: _secretKeyController,
            label: l10n.modelConfigSecretKey,
            hint: 'Your Secret Key',
            icon: LucideIcons.keyRound,
            obscureText: !_showSecretKey,
            onToggleObscureText: widget.readOnly
                ? null
                : () => setState(() {
                    _showSecretKey = !_showSecretKey;
                  }),
          ),
        ],
      ],
    );
  }

  Widget _buildTextField({
    required TextEditingController controller,
    required String label,
    required String hint,
    required IconData icon,
    bool obscureText = false,
    VoidCallback? onToggleObscureText,
  }) {
    final hasVisibilityToggle = onToggleObscureText != null;
    return TextField(
      controller: controller,
      obscureText: obscureText,
      enabled: !widget.readOnly,
      style: AppFonts.inter(
        fontSize: 14,
        color: widget.readOnly ? Colors.white38 : Colors.white,
      ),
      decoration: InputDecoration(
        labelText: label,
        labelStyle: AppFonts.inter(fontSize: 12, color: Colors.white54),
        hintText: hint,
        hintStyle: AppFonts.inter(fontSize: 13, color: Colors.white30),
        prefixIcon: Icon(icon, color: Colors.white54, size: 18),
        suffixIcon: hasVisibilityToggle
            ? IconButton(
                tooltip: obscureText
                    ? AppLocalizations.of(context)!.modelConfigToggleShowSecret
                    : AppLocalizations.of(context)!.modelConfigToggleHideSecret,
                icon: Icon(
                  obscureText ? LucideIcons.eye : LucideIcons.eyeOff,
                  color: Colors.white54,
                  size: 18,
                ),
                onPressed: onToggleObscureText,
              )
            : null,
        filled: true,
        fillColor: const Color(0xFF1E1E2E),
        border: OutlineInputBorder(
          borderRadius: BorderRadius.circular(8),
          borderSide: BorderSide.none,
        ),
        focusedBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(8),
          borderSide: const BorderSide(color: Color(0xFF6366F1), width: 1.5),
        ),
        contentPadding: const EdgeInsets.symmetric(
          horizontal: 16,
          vertical: 14,
        ),
      ),
    );
  }

  /// Builds the error display.
  Widget _buildError() {
    return Container(
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: Colors.red.withValues(alpha: 0.1),
        borderRadius: BorderRadius.circular(8),
        border: Border.all(color: Colors.red.withValues(alpha: 0.3)),
      ),
      child: Row(
        children: [
          const Icon(Icons.error_outline, color: Colors.red, size: 18),
          const SizedBox(width: 10),
          Expanded(
            child: Text(
              _error!,
              style: AppFonts.inter(fontSize: 13, color: Colors.red.shade300),
            ),
          ),
        ],
      ),
    );
  }
}
