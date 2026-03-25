import 'dart:async';
import 'package:flutter/material.dart';
import '../utils/app_fonts.dart';
import 'package:lucide_icons/lucide_icons.dart';
import '../l10n/app_localizations.dart';
import '../models/llm_config_model.dart';
import '../services/model_config_service.dart';
import '../services/protection_service.dart';
import '../services/provider_service.dart';
import '../utils/app_logger.dart';
import '../utils/locale_utils.dart';

/// 安全模型配置表单（纯表单组件）
/// 用于 ShepherdGate 风险检测的 LLM 配置
/// 不包含任何按钮，调用方需通过 GlobalKey 调用 saveConfig() 方法保存
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
  late SecurityModelConfigService _service;
  final ProviderService _providerService = ProviderService();
  bool _loading = true;
  bool _saving = false;
  bool _testing = false;
  String? _error;

  final TextEditingController _endpointController = TextEditingController();
  final TextEditingController _apiKeyController = TextEditingController();
  final TextEditingController _modelController = TextEditingController();
  final TextEditingController _secretKeyController = TextEditingController();
  String _selectedType = 'ollama';

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

  /// 根据当前语言解析 provider 的默认 baseURL。
  /// Moonshot AI 中文环境使用 .cn 域名，其它语言使用默认 .ai 域名。
  String _resolveBaseURL(ProviderInfo provider) {
    if (provider.name == 'moonshot' &&
        LocaleUtils.resolveLanguageCode() == 'zh') {
      return 'https://api.moonshot.cn/v1';
    }
    return provider.defaultBaseURL;
  }

  /// Loads current configuration into the form.
  Future<void> _loadConfig() async {
    // Use initialConfig if provided (reuse scenario)
    if (widget.initialConfig != null) {
      final config = widget.initialConfig!;
      setState(() {
        _selectedType = _normalizeSelectedType(config.provider);
        _endpointController.text = config.endpoint;
        _apiKeyController.text = config.apiKey;
        _modelController.text = config.model;
        _secretKeyController.text = config.secretKey;
        _loading = false;
      });
      return;
    }

    try {
      final config = await _service.loadConfig();
      setState(() {
        _selectedType = _normalizeSelectedType(config.provider);
        _endpointController.text = config.endpoint;
        _apiKeyController.text = config.apiKey;
        _modelController.text = config.model;
        _secretKeyController.text = config.secretKey;
        _loading = false;
      });
    } catch (e) {
      setState(() {
        _error = e.toString();
        _loading = false;
      });
    }
  }

  /// Saves configuration after testing connectivity and returns success status.
  /// This is the public method that should be called by parent widgets.
  Future<bool> saveConfig() async {
    final l10n = AppLocalizations.of(context)!;
    setState(() {
      _saving = true;
      _testing = true;
      _error = null;
    });

    final config = SecurityModelConfig(
      provider: _selectedType,
      endpoint: _endpointController.text.trim(),
      apiKey: _apiKeyController.text.trim(),
      model: _modelController.text.trim(),
      secretKey: _secretKeyController.text.trim(),
    );

    if (!config.isValid) {
      setState(() {
        _error = l10n.modelConfigFillRequired;
        _saving = false;
        _testing = false;
      });
      return false;
    }

    try {
      final testResult = await _service.testConnection(config);
      if (testResult['success'] != true) {
        setState(() {
          _error = l10n.modelConfigTestFailed(
            testResult['error'] ?? 'Unknown error',
          );
          _saving = false;
          _testing = false;
        });
        return false;
      }

      setState(() {
        _testing = false;
      });

      final success = await _service.saveConfig(config);
      if (success) {
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
        _testing = false;
      });
      return false;
    }
  }

  /// Returns whether the form is currently saving.
  bool get isSaving => _saving;

  /// Returns whether the form is currently testing.
  bool get isTesting => _testing;

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
              onTap: widget.readOnly
                  ? null
                  : () {
                      setState(() {
                        _selectedType = provider.name;
                        // 切换 provider 时清空密钥并设置推荐模型
                        _apiKeyController.clear();
                        _secretKeyController.clear();
                        if (provider.defaultModel.isNotEmpty) {
                          _modelController.text = provider.defaultModel;
                        }
                        if (provider.defaultBaseURL.isNotEmpty) {
                          _endpointController.text = _resolveBaseURL(provider);
                        }
                      });
                    },
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
    required VoidCallback? onTap,
  }) {
    final bool isDisabled = onTap == null;
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
                ? const Color(
                    0xFF6366F1,
                  ).withValues(alpha: isDisabled ? 0.15 : 0.3)
                : const Color(0xFF1E1E2E),
            borderRadius: BorderRadius.circular(8),
            border: Border.all(
              color: isSelected
                  ? const Color(
                      0xFF6366F1,
                    ).withValues(alpha: isDisabled ? 0.5 : 1.0)
                  : Colors.white.withValues(alpha: 0.1),
            ),
          ),
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(
                icon,
                size: 16,
                color: isSelected
                    ? (isDisabled ? Colors.white54 : Colors.white)
                    : (isDisabled ? Colors.white38 : Colors.white70),
              ),
              const SizedBox(width: 8),
              Text(
                label,
                style: AppFonts.inter(
                  fontSize: 13,
                  fontWeight: isSelected ? FontWeight.w600 : FontWeight.w500,
                  color: isSelected
                      ? (isDisabled ? Colors.white54 : Colors.white)
                      : (isDisabled ? Colors.white38 : Colors.white70),
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
