import 'dart:async';
import 'dart:convert';
import 'package:flutter/material.dart';
import '../utils/app_fonts.dart';
import 'package:lucide_icons/lucide_icons.dart';
import '../l10n/app_localizations.dart';
import '../models/llm_config_model.dart';
import '../services/app_settings_database_service.dart';
import '../services/model_config_service.dart';
import '../services/model_config_database_service.dart';
import '../services/protection_service.dart';
import '../services/provider_service.dart';
import '../utils/app_logger.dart';
import '../utils/locale_utils.dart';
import 'processing_notice_card.dart';

/// Bot 模型配置表单（纯表单组件）
/// 用于代理转发的目标 LLM 配置，写入 openclaw.json
/// 表单本体不包含保存按钮，保存动作由调用方通过 GlobalKey 触发 saveConfig()
class BotModelConfigForm extends StatefulWidget {
  /// Creates a bot model configuration form.
  const BotModelConfigForm({
    super.key,
    required this.assetName,
    this.assetID = '',
  });

  /// 关联的资产名称
  final String assetName;
  final String assetID;

  @override
  State<BotModelConfigForm> createState() => BotModelConfigFormState();
}

/// State for bot model configuration form.
class BotModelConfigFormState extends State<BotModelConfigForm> {
  /// Bot 模型按供应商草稿缓存键前缀。
  static const String _providerDraftSettingKeyPrefix =
      'bot_model_provider_drafts_v1';

  late BotModelConfigService _service;
  final ProviderService _providerService = ProviderService();
  final AppSettingsDatabaseService _appSettingsService =
      AppSettingsDatabaseService();
  bool _loading = true;
  bool _saving = false;
  bool _testing = false;
  String? _error;

  final TextEditingController _endpointController = TextEditingController();
  final TextEditingController _apiKeyController = TextEditingController();
  final TextEditingController _modelController = TextEditingController();
  final TextEditingController _secretKeyController = TextEditingController();
  String _selectedType = 'openai';
  String _savedConfigSignature = '';
  final Map<String, BotModelConfig> _providerDrafts = {};

  /// Dynamically loaded providers from Go layer.
  List<ProviderInfo> _providers = [];

  /// Bot-only provider aliases that map to OpenAI backend type.
  static const _botOpenAiAliases = {
    'openrouter',
    'copilot',
    'vercel_gateway',
    'opencode_zen',
    'xiaomi',
  };

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
  };

  @override
  void initState() {
    super.initState();
    _service = BotModelConfigService(
      assetName: widget.assetName,
      assetID: widget.assetID,
    );
    _loadProviders();
    _loadConfig();
  }

  /// Loads providers from Go layer via FFI.
  void _loadProviders() {
    _providers = _providerService.getProviders(ProviderScope.bot);
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
    return _providers.isNotEmpty ? _providers.first.name : 'openai';
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

  /// 根据当前语言解析 provider 的默认 baseURL。
  /// Moonshot AI 中文环境使用 .cn 域名，其它语言使用默认 .ai 域名。
  String _resolveBaseURL(ProviderInfo provider) {
    if (provider.name == 'moonshot' &&
        LocaleUtils.resolveLanguageCode() == 'zh') {
      return 'https://api.moonshot.cn/v1';
    }
    return provider.defaultBaseURL;
  }

  /// Resolves the provider type for form behavior.
  String _resolveProviderType(String type) {
    if (_botOpenAiAliases.contains(type)) {
      return 'openai';
    }
    return type;
  }

  /// 构建当前表单配置对象，用于统一比较和保存逻辑。
  BotModelConfig _buildCurrentConfig() {
    final resolvedType = _resolveProviderType(_selectedType);
    return BotModelConfig(
      assetName: widget.assetName,
      assetID: widget.assetID,
      provider: resolvedType,
      baseUrl: _endpointController.text.trim(),
      apiKey: _apiKeyController.text.trim(),
      model: _modelController.text.trim(),
      secretKey: _secretKeyController.text.trim(),
    );
  }

  /// 生成配置签名，用于判断配置是否发生变化。
  String _buildConfigSignature(BotModelConfig config) {
    return [
      config.provider.trim(),
      config.baseUrl.trim(),
      config.apiKey.trim(),
      config.model.trim(),
      config.secretKey.trim(),
    ].join('|');
  }

  /// Loads current configuration into the form.
  Future<void> _loadConfig() async {
    try {
      await _loadProviderDrafts();
      final config = await _service.loadConfig();
      if (config != null) {
        final selectedType = _normalizeSelectedType(config.provider);
        _providerDrafts[selectedType] = config;
        final selectedConfig =
            _providerDrafts[selectedType] ?? _createEmptyConfig(selectedType);
        setState(() {
          _selectedType = selectedType;
          _applyConfigToControllers(selectedConfig);
          _savedConfigSignature = _buildConfigSignature(config);
          _loading = false;
        });
      } else {
        final selectedConfig =
            _providerDrafts[_selectedType] ?? _createEmptyConfig(_selectedType);
        setState(() {
          _applyConfigToControllers(selectedConfig);
          _savedConfigSignature = _buildConfigSignature(_buildCurrentConfig());
          _loading = false;
        });
      }
    } catch (e) {
      setState(() {
        _error = e.toString();
        _loading = false;
      });
    }
  }

  /// 保存配置并返回保存结果。
  /// 按需通过验证按钮触发连通性测试，保存流程不做自动测试。
  /// This is the public method that should be called by parent widgets.
  /// When [deferProxyRestart] is true, will not trigger proxy restart.
  Future<bool> saveConfig({bool deferProxyRestart = false}) async {
    final l10n = AppLocalizations.of(context)!;
    setState(() {
      _saving = true;
      _error = null;
    });

    final config = _buildCurrentConfig();

    if (!hasConfigChanged && hasRequiredConfig) {
      setState(() {
        _saving = false;
      });
      appLogger.info(
        '[BotModelConfigForm] Config unchanged, skip save.',
      );
      return true;
    }

    if (!hasRequiredConfig) {
      setState(() {
        _error = l10n.modelConfigFillRequired;
        _saving = false;
      });
      return false;
    }

    try {
      final success = await _service.saveConfig(config);
      if (success) {
        _savedConfigSignature = _buildConfigSignature(config);
        _captureCurrentProviderDraft();
        await _persistProviderDrafts();
        // Bot 模型保存后，如果代理正在运行且未延迟重启，需要完整重启
        if (!deferProxyRestart) {
          final protectionService = ProtectionService.forAsset(
            widget.assetName,
            widget.assetID,
          );
          if (protectionService.isProxyRunning) {
            try {
              final securityModelConfig = await ModelConfigDatabaseService()
                  .getSecurityModelConfig();
              if (securityModelConfig != null) {
                // 需要从数据库获取运行时配置
                final result = await _runWithBotRestartNotice(
                  () => protectionService.restartProtectionProxyForBotModelUpdate(
                    securityModelConfig,
                    ProtectionRuntimeConfig(),
                  ),
                );
                if (result['success'] == true) {
                  appLogger.info(
                    '[BotModelConfigForm] Bot model update: proxy restarted',
                  );
                } else {
                  appLogger.warning(
                    '[BotModelConfigForm] Bot model update: proxy restart failed: ${result['error']}',
                  );
                }
              }
            } catch (e) {
              appLogger.warning(
                '[BotModelConfigForm] Bot model update: failed to restart proxy: $e',
              );
            }
          } else {
            appLogger.info(
              '[BotModelConfigForm] Bot model config saved. Will apply on next proxy start.',
            );
          }
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

  /// 手动验证当前配置连通性，不执行保存。
  Future<bool> validateConnection() async {
    final l10n = AppLocalizations.of(context)!;
    final config = _buildCurrentConfig();
    if (!hasRequiredConfig) {
      setState(() {
        _error = l10n.modelConfigFillRequired;
      });
      return false;
    }

    setState(() {
      _testing = true;
      _error = null;
    });
    try {
      final testResult = await _service.testConnection(config);
      if (testResult['success'] == true) {
        if (mounted) {
          ScaffoldMessenger.of(context).showSnackBar(
            SnackBar(
              content: Text(l10n.modelConfigTestSuccess),
              backgroundColor: Colors.green,
            ),
          );
        }
        return true;
      }
      setState(() {
        _error = l10n.modelConfigTestFailed(
          testResult['error'] ?? 'Unknown error',
        );
      });
      return false;
    } catch (e) {
      setState(() {
        _error = e.toString();
      });
      return false;
    } finally {
      if (mounted) {
        setState(() {
          _testing = false;
        });
      }
    }
  }

  /// 执行 Bot 重启动作并显示用户友好的页面提示。
  Future<Map<String, dynamic>> _runWithBotRestartNotice(
    Future<Map<String, dynamic>> Function() action,
  ) async {
    if (!mounted) {
      return action();
    }
    unawaited(
      showDialog<void>(
        context: context,
        barrierDismissible: false,
        builder: (dialogContext) => Dialog(
          backgroundColor: Colors.transparent,
          elevation: 0,
          insetPadding: const EdgeInsets.symmetric(horizontal: 28),
          child: ProcessingNoticeCard(
            title: '正在更新配置',
            message: '在此期间Openclaw Dashboard页面会提示断开连接，稍后将恢复正常',
          ),
        ),
      ),
    );
    await Future.delayed(const Duration(milliseconds: 50));
    try {
      return await action();
    } finally {
      if (mounted) {
        final navigator = Navigator.of(context, rootNavigator: true);
        if (navigator.canPop()) {
          navigator.pop();
        }
      }
    }
  }

  /// 生成 provider 草稿缓存设置键。
  String get _providerDraftSettingKey =>
      '$_providerDraftSettingKeyPrefix:${widget.assetName}:${widget.assetID}';

  /// 创建指定 provider 的空白配置。
  BotModelConfig _createEmptyConfig(String providerName) {
    return BotModelConfig(
      assetName: widget.assetName,
      assetID: widget.assetID,
      provider: _resolveProviderType(providerName),
      baseUrl: '',
      apiKey: '',
      model: '',
      secretKey: '',
    );
  }

  /// 将配置应用到输入控件。
  void _applyConfigToControllers(BotModelConfig config) {
    _endpointController.text = config.baseUrl;
    _apiKeyController.text = config.apiKey;
    _modelController.text = config.model;
    _secretKeyController.text = config.secretKey;
  }

  /// 记录当前 provider 草稿。
  void _captureCurrentProviderDraft() {
    _providerDrafts[_selectedType] = _buildCurrentConfig();
  }

  /// 切换 provider 时恢复对应草稿，避免输入被清空。
  Future<void> _handleProviderSelected(ProviderInfo provider) async {
    _captureCurrentProviderDraft();
    final targetConfig =
        _providerDrafts[provider.name] ?? _createEmptyConfig(provider.name);
    if (!mounted) return;
    setState(() {
      _selectedType = provider.name;
      _applyConfigToControllers(targetConfig);
      _error = null;
    });
    await _persistProviderDrafts();
  }

  /// 从应用设置读取 provider 草稿缓存。
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
        _providerDrafts[provider] = BotModelConfig(
          assetName: widget.assetName,
          assetID: widget.assetID,
          provider: _resolveProviderType(provider),
          baseUrl: (data['base_url'] as String?) ?? '',
          apiKey: (data['api_key'] as String?) ?? '',
          model: (data['model'] as String?) ?? '',
          secretKey: (data['secret_key'] as String?) ?? '',
        );
      });
    } catch (e) {
      appLogger.warning(
        '[BotModelConfigForm] Failed to parse provider drafts: $e',
      );
    }
  }

  /// 持久化 provider 草稿缓存。
  Future<void> _persistProviderDrafts() async {
    final payload = <String, dynamic>{};
    _providerDrafts.forEach((provider, config) {
      payload[provider] = {
        'base_url': config.baseUrl,
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
      appLogger.warning('[BotModelConfigForm] Failed to persist provider drafts');
    }
  }

  /// Returns whether the form is currently saving.
  bool get isSaving => _saving;

  /// Returns whether the form is currently testing.
  bool get isTesting => _testing;

  /// Returns whether required bot model fields are filled.
  bool get hasRequiredConfig {
    final resolvedType = _resolveProviderType(_selectedType);
    return resolvedType.isNotEmpty &&
        _endpointController.text.trim().isNotEmpty &&
        _modelController.text.trim().isNotEmpty;
  }

  /// 返回当前表单配置是否与已保存配置不同。
  bool get hasConfigChanged {
    final current = _buildConfigSignature(_buildCurrentConfig());
    return current != _savedConfigSignature;
  }

  @override
  void dispose() {
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

    return Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        _buildTypeSelector(),
        const SizedBox(height: 16),
        _buildFormFields(),
        if (_error != null) ...[const SizedBox(height: 16), _buildError()],
      ],
    );
  }

  /// Builds the model provider selector.
  Widget _buildTypeSelector() {
    final l10n = AppLocalizations.of(context)!;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          l10n.modelConfigProvider,
          style: AppFonts.inter(
            fontSize: 13,
            fontWeight: FontWeight.w500,
            color: Colors.white70,
          ),
        ),
        const SizedBox(height: 8),
        Wrap(
          spacing: 8,
          runSpacing: 8,
          children: _getVisibleProviders().map((provider) {
            final isSelected = _selectedType == provider.name;
            return _buildProviderChip(
              label: provider.displayName,
              icon: _getIconForProvider(provider.icon),
              isSelected: isSelected,
              onTap: () => unawaited(_handleProviderSelected(provider)),
            );
          }).toList(),
        ),
      ],
    );
  }

  Widget _buildProviderChip({
    required String label,
    required IconData icon,
    required bool isSelected,
    required VoidCallback onTap,
  }) {
    return Material(
      color: Colors.transparent,
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(8),
        child: AnimatedContainer(
          duration: const Duration(milliseconds: 200),
          padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
          decoration: BoxDecoration(
            color: isSelected
                ? const Color(0xFF10B981).withValues(alpha: 0.3)
                : const Color(0xFF1E1E2E),
            borderRadius: BorderRadius.circular(8),
            border: Border.all(
              color: isSelected
                  ? const Color(0xFF10B981)
                  : Colors.white.withValues(alpha: 0.1),
            ),
          ),
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(
                icon,
                size: 16,
                color: isSelected ? Colors.white : Colors.white70,
              ),
              const SizedBox(width: 8),
              Text(
                label,
                style: AppFonts.inter(
                  fontSize: 13,
                  fontWeight: isSelected ? FontWeight.w600 : FontWeight.w500,
                  color: isSelected ? Colors.white : Colors.white70,
                ),
              ),
            ],
          ),
        ),
      ),
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
            hint: providerInfo != null ? _resolveBaseURL(providerInfo) : '',
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
            obscureText: true,
          ),
        ],
        const SizedBox(height: 12),
        _buildTextField(
          controller: _modelController,
          label: l10n.modelConfigModelName,
          hint: providerInfo?.modelHint ?? 'Model name',
          icon: LucideIcons.box,
        ),
        if (needsSecretKey) ...[
          const SizedBox(height: 12),
          _buildTextField(
            controller: _secretKeyController,
            label: l10n.modelConfigSecretKey,
            hint: 'Your Secret Key',
            icon: LucideIcons.keyRound,
            obscureText: true,
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
  }) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          label,
          style: AppFonts.inter(
            fontSize: 12,
            fontWeight: FontWeight.w500,
            color: Colors.white70,
          ),
        ),
        const SizedBox(height: 6),
        TextField(
          controller: controller,
          obscureText: obscureText,
          style: AppFonts.firaCode(fontSize: 13, color: Colors.white),
          decoration: InputDecoration(
            hintText: hint,
            hintStyle: AppFonts.firaCode(fontSize: 13, color: Colors.white30),
            prefixIcon: Icon(icon, color: Colors.white54, size: 18),
            filled: true,
            fillColor: const Color(0xFF1E1E2E),
            border: OutlineInputBorder(
              borderRadius: BorderRadius.circular(8),
              borderSide: BorderSide.none,
            ),
            focusedBorder: OutlineInputBorder(
              borderRadius: BorderRadius.circular(8),
              borderSide: const BorderSide(
                color: Color(0xFF10B981),
                width: 1.5,
              ),
            ),
            contentPadding: const EdgeInsets.symmetric(
              horizontal: 16,
              vertical: 14,
            ),
          ),
        ),
      ],
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
