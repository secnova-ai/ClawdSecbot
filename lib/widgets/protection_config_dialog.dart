import 'dart:io';
import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import '../utils/app_fonts.dart';
import 'package:lucide_icons/lucide_icons.dart';
import '../config/build_config.dart';
import '../l10n/app_localizations.dart';
import '../models/protection_config_model.dart';
import '../models/llm_config_model.dart';
import '../services/model_config_database_service.dart';
import '../services/model_config_service.dart';
import '../services/protection_service.dart';
import '../services/protection_database_service.dart';
import '../utils/app_logger.dart';
import 'bot_model_config_form.dart';
import 'security_model_config_form.dart';
import '../services/plugin_service.dart';

/// Token 单位枚举
enum _TokenUnit {
  k(1000),
  m(1000000);

  final int multiplier;
  const _TokenUnit(this.multiplier);

  String label(AppLocalizations l10n) =>
      this == k ? l10n.tokenUnitK : l10n.tokenUnitM;
}

/// Token 预设选项
class _TokenPreset {
  final String Function(AppLocalizations) labelBuilder;
  final int rawValue;
  const _TokenPreset(this.labelBuilder, this.rawValue);
}

/// 防护配置弹窗
/// 支持配置智能规则、Token限制和权限设置
class ProtectionConfigDialog extends StatefulWidget {
  final String assetName;
  final String assetID;
  final bool isEditMode; // true: 仅编辑配置, false: 开启防护时的配置

  const ProtectionConfigDialog({
    super.key,
    required this.assetName,
    this.assetID = '',
    this.isEditMode = false,
  });

  @override
  State<ProtectionConfigDialog> createState() => _ProtectionConfigDialogState();
}

class _ProtectionConfigDialogState extends State<ProtectionConfigDialog>
    with SingleTickerProviderStateMixin {
  late TabController _tabController;
  late ProtectionConfig _config;
  bool _isLoading = true;
  final GlobalKey<BotModelConfigFormState> _botModelFormKey =
      GlobalKey<BotModelConfigFormState>();

  // Token限制控制器（显示值，非原始值）
  final TextEditingController _singleSessionDisplayController =
      TextEditingController();
  final TextEditingController _dailyDisplayController = TextEditingController();
  _TokenUnit _singleSessionUnit = _TokenUnit.k;
  _TokenUnit _dailyUnit = _TokenUnit.k;

  // 单轮会话预设列表
  static final List<_TokenPreset> _singleSessionPresets = [
    _TokenPreset((l10n) => l10n.tokenNoLimit, 0),
    _TokenPreset((l10n) => l10n.tokenPreset50K, 50000),
    _TokenPreset((l10n) => l10n.tokenPreset100K, 100000),
    _TokenPreset((l10n) => l10n.tokenPreset300K, 300000),
    _TokenPreset((l10n) => l10n.tokenPreset500K, 500000),
    _TokenPreset((l10n) => l10n.tokenPreset1M, 1000000),
  ];

  // 当日总量预设列表
  static final List<_TokenPreset> _dailyPresets = [
    _TokenPreset((l10n) => l10n.tokenNoLimit, 0),
    _TokenPreset((l10n) => l10n.tokenPreset10M, 10000000),
    _TokenPreset((l10n) => l10n.tokenPreset50M, 50000000),
    _TokenPreset((l10n) => l10n.tokenPreset100M, 100000000),
  ];

  // 路径权限
  PermissionMode _pathMode = PermissionMode.blacklist;
  final List<String> _pathList = [];
  final TextEditingController _pathInputController = TextEditingController();

  // 网络权限 - 出栈 (outbound)
  PermissionMode _networkOutboundMode = PermissionMode.blacklist;
  final List<String> _networkOutboundList = [];
  final TextEditingController _networkOutboundInputController =
      TextEditingController();

  // 网络权限 - 入栈 (inbound)
  PermissionMode _networkInboundMode = PermissionMode.blacklist;
  final List<String> _networkInboundList = [];
  final TextEditingController _networkInboundInputController =
      TextEditingController();

  // Shell权限
  PermissionMode _shellMode = PermissionMode.blacklist;
  final List<String> _shellList = [];
  final TextEditingController _shellInputController = TextEditingController();

  // 沙箱防护启用（macOS Personal / Linux / Windows 支持）
  bool _sandboxEnabled = false;

  // 仅审计模式
  bool _auditOnly = false;

  // 防止重复点击保存
  bool _isSaving = false;

  // Shepherd User Rules
  final List<String> _sensitiveActions = [];
  final TextEditingController _sensitiveActionsInputController =
      TextEditingController();

  // 内置安全技能列表
  List<Map<String, dynamic>> _bundledSkills = [];

  // Whether this plugin requires explicit bot model configuration.
  bool _requiresBotModelConfig = true;

  int _tabCountFor(bool requiresBotModelConfig) {
    if (BuildConfig.isAppStore) {
      return requiresBotModelConfig ? 3 : 2;
    }
    return requiresBotModelConfig ? 4 : 3;
  }

  int get _tabCount => _tabCountFor(_requiresBotModelConfig);

  int? get _botTabIndex {
    if (!_requiresBotModelConfig) return null;
    return BuildConfig.isAppStore ? 2 : 3;
  }

  void _updateTabControllerForRequirement(bool requiresBotModelConfig) {
    final expectedLength = _tabCountFor(requiresBotModelConfig);
    if (_requiresBotModelConfig == requiresBotModelConfig &&
        _tabController.length == expectedLength) {
      return;
    }

    final previousController = _tabController;
    final previousIndex = previousController.index;

    _requiresBotModelConfig = requiresBotModelConfig;
    int nextIndex = previousIndex;
    if (nextIndex >= expectedLength) {
      nextIndex = expectedLength - 1;
    }
    if (nextIndex < 0) {
      nextIndex = 0;
    }

    _tabController = TabController(
      length: expectedLength,
      vsync: this,
      initialIndex: nextIndex,
    );

    // Delay old controller disposal until widgets have switched to the new
    // controller, otherwise TabBar/TabBarView may still hold dependents.
    WidgetsBinding.instance.addPostFrameCallback((_) {
      previousController.dispose();
    });
  }

  @override
  void initState() {
    super.initState();
    // 默认按“需要 bot 模型配置”初始化，加载配置后再按插件能力动态调整。
    _tabController = TabController(length: _tabCount, vsync: this);
    _loadConfig();
  }

  @override
  void dispose() {
    _tabController.dispose();
    _singleSessionDisplayController.dispose();
    _dailyDisplayController.dispose();
    _pathInputController.dispose();
    _networkOutboundInputController.dispose();
    _networkInboundInputController.dispose();
    _shellInputController.dispose();
    _sensitiveActionsInputController.dispose();
    super.dispose();
  }

  Future<void> _loadConfig() async {
    try {
      final savedConfig = await ProtectionDatabaseService().getProtectionConfig(
        widget.assetName,
        widget.assetID,
      );
      if (savedConfig != null) {
        _config = savedConfig;
      } else {
        _config = ProtectionConfig.defaultConfig(
          widget.assetName,
        ).copyWith(assetID: widget.assetID);
      }
      if (widget.assetID.isNotEmpty && _config.assetID != widget.assetID) {
        _config = _config.copyWith(assetID: widget.assetID);
      }

      // 更新Token限制UI
      final (sessionText, sessionUnit) = _rawToDisplay(
        _config.singleSessionTokenLimit,
      );
      _singleSessionDisplayController.text = sessionText;
      _singleSessionUnit = sessionUnit;
      final (dailyText, dailyUnit) = _rawToDisplay(_config.dailyTokenLimit);
      _dailyDisplayController.text = dailyText;
      _dailyUnit = dailyUnit;

      // 路径权限
      _pathMode = _config.pathPermission.mode;
      _pathList.clear();
      _pathList.addAll(_config.pathPermission.paths);

      // 网络权限 - 出栈
      _networkOutboundMode = _config.networkPermission.outbound.mode;
      _networkOutboundList.clear();
      _networkOutboundList.addAll(_config.networkPermission.outbound.addresses);
      // 网络权限 - 入栈
      _networkInboundMode = _config.networkPermission.inbound.mode;
      _networkInboundList.clear();
      _networkInboundList.addAll(_config.networkPermission.inbound.addresses);

      // Shell权限
      _shellMode = _config.shellPermission.mode;
      _shellList.clear();
      _shellList.addAll(_config.shellPermission.commands);

      // 沙箱启用状态
      _sandboxEnabled = _config.sandboxEnabled;

      // 仅审计模式
      _auditOnly = _config.auditOnly;

      // Load Shepherd Rules
      final rules = await PluginService().loadAndSyncShepherdRules(
        widget.assetName,
        widget.assetID,
      );
      _sensitiveActions.clear();
      _sensitiveActions.addAll(rules['sensitiveActions']!);

      // Load bundled ReAct skills
      _bundledSkills = PluginService().listBundledReActSkills();

      // Resolve whether this plugin requires bot model config.
      final requiresBotModelConfig = await PluginService()
          .requiresBotModelConfig(widget.assetName);
      _updateTabControllerForRequirement(requiresBotModelConfig);

      setState(() {
        _isLoading = false;
      });
    } catch (e) {
      appLogger.error('[ProtectionConfig] Failed to load config', e);
      _config = ProtectionConfig.defaultConfig(
        widget.assetName,
      ).copyWith(assetID: widget.assetID);
      try {
        final requiresBotModelConfig = await PluginService()
            .requiresBotModelConfig(widget.assetName);
        _updateTabControllerForRequirement(requiresBotModelConfig);
      } catch (_) {}
      setState(() {
        _isLoading = false;
      });
    }
  }

  SecurityModelConfig _toSecurityModelConfig(BotModelConfig botConfig) {
    return SecurityModelConfig(
      provider: botConfig.provider,
      endpoint: botConfig.baseUrl,
      apiKey: botConfig.apiKey,
      model: botConfig.model,
      secretKey: botConfig.secretKey,
    );
  }

  Future<SecurityModelConfig?> _ensureSecurityModelConfigured(
    BotModelConfig? botConfig,
  ) async {
    final existing = await ModelConfigDatabaseService()
        .getSecurityModelConfig();
    if (existing != null) {
      return existing;
    }
    if (!mounted) return null;

    return showDialog<SecurityModelConfig>(
      context: context,
      barrierDismissible: false,
      builder: (dialogContext) {
        bool reuseBotConfig = botConfig != null;
        final formKey = GlobalKey<SecurityModelConfigFormState>();
        return StatefulBuilder(
          builder: (context, setState) => AlertDialog(
            backgroundColor: const Color(0xFF1A1A2E),
            shape: RoundedRectangleBorder(
              borderRadius: BorderRadius.circular(12),
            ),
            title: Text(
              AppLocalizations.of(dialogContext)!.onboardingSecurityModelTitle,
              style: AppFonts.inter(
                color: Colors.white,
                fontSize: 16,
                fontWeight: FontWeight.w600,
              ),
            ),
            content: SizedBox(
              width: 520,
              child: Column(
                mainAxisSize: MainAxisSize.min,
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  if (botConfig != null)
                    CheckboxListTile(
                      value: reuseBotConfig,
                      onChanged: (value) {
                        setState(() {
                          reuseBotConfig = value ?? true;
                        });
                      },
                      activeColor: const Color(0xFF6366F1),
                      checkColor: Colors.white,
                      contentPadding: EdgeInsets.zero,
                      title: Text(
                        AppLocalizations.of(
                          dialogContext,
                        )!.onboardingReuseBotModel,
                        style: AppFonts.inter(
                          color: Colors.white,
                          fontSize: 13,
                          fontWeight: FontWeight.w500,
                        ),
                      ),
                      subtitle: Text(
                        AppLocalizations.of(
                          dialogContext,
                        )!.onboardingReuseBotModelHint,
                        style: AppFonts.inter(
                          color: Colors.white54,
                          fontSize: 11,
                        ),
                      ),
                      controlAffinity: ListTileControlAffinity.leading,
                    ),
                  const SizedBox(height: 8),
                  SizedBox(
                    height: 320,
                    child: SingleChildScrollView(
                      child: SecurityModelConfigForm(
                        key: formKey,
                        readOnly: reuseBotConfig,
                        initialConfig: reuseBotConfig && botConfig != null
                            ? _toSecurityModelConfig(botConfig)
                            : null,
                      ),
                    ),
                  ),
                ],
              ),
            ),
            actions: [
              TextButton(
                onPressed: () => Navigator.of(dialogContext).pop(null),
                child: Text(
                  AppLocalizations.of(dialogContext)!.cancel,
                  style: AppFonts.inter(color: Colors.white54),
                ),
              ),
              ElevatedButton(
                onPressed: () async {
                  SecurityModelConfig? savedConfig;
                  bool success = false;
                  if (reuseBotConfig && botConfig != null) {
                    savedConfig = _toSecurityModelConfig(botConfig);
                    success = await SecurityModelConfigService().saveConfig(
                      savedConfig,
                    );
                    if (success) {
                      try {
                        final protectionService = ProtectionService.forAsset(
                          widget.assetName,
                          _config.assetID,
                        );
                        await protectionService.updateSecurityModelConfig(
                          savedConfig,
                        );
                      } catch (_) {}
                    }
                  } else {
                    success =
                        await (formKey.currentState?.saveConfig() ?? false);
                    if (success) {
                      savedConfig = await ModelConfigDatabaseService()
                          .getSecurityModelConfig();
                    }
                  }

                  if (!dialogContext.mounted) return;
                  if (success && savedConfig != null) {
                    Navigator.of(dialogContext).pop(savedConfig);
                    return;
                  }
                  ScaffoldMessenger.of(dialogContext).showSnackBar(
                    SnackBar(
                      content: Text(
                        AppLocalizations.of(
                          dialogContext,
                        )!.modelConfigSaveFailed,
                      ),
                    ),
                  );
                },
                style: ElevatedButton.styleFrom(
                  backgroundColor: const Color(0xFF6366F1),
                ),
                child: Text(
                  AppLocalizations.of(dialogContext)!.modelConfigSave,
                  style: AppFonts.inter(color: Colors.white),
                ),
              ),
            ],
          ),
        );
      },
    );
  }

  Future<void> _saveConfig({bool closeOnSave = true}) async {
    // 防止重复点击
    if (_isSaving) return;
    setState(() => _isSaving = true);

    try {
      final l10n = AppLocalizations.of(context)!;

      bool botModelSaved = false;
      final botTabIndex = _botTabIndex;
      if (_requiresBotModelConfig) {
        // 1. 保存 Bot 模型配置到数据库（deferProxyRestart=true，延迟重启）
        //    先保存 Bot 模型，但不触发代理重启，等防护配置也保存后再统一重启
        final botFormState = _botModelFormKey.currentState;
        if (botFormState == null || !botFormState.hasRequiredConfig) {
          if (mounted) {
            ScaffoldMessenger.of(context).showSnackBar(
              SnackBar(content: Text(l10n.modelConfigFillRequired)),
            );
            if (botTabIndex != null) {
              _tabController.animateTo(botTabIndex);
            }
          }
          return;
        }
        if (!botFormState.hasConfigChanged) {
          appLogger.info(
            '[ProtectionConfig] Bot model unchanged, skip bot model save.',
          );
        } else {
          final botSaved = await botFormState.saveConfig(
            deferProxyRestart: true,
          );
          if (!botSaved && mounted) {
            ScaffoldMessenger.of(
              context,
            ).showSnackBar(SnackBar(content: Text(l10n.modelConfigSaveFailed)));
            if (botTabIndex != null) {
              _tabController.animateTo(botTabIndex);
            }
            return;
          }
          botModelSaved = true;
        }
      }

      // When opening protection (not edit mode), set enabled=true
      // When editing, preserve the current enabled state
      final shouldEnable = !widget.isEditMode || _config.enabled;
      // 记录防护启用状态变化（用于保存后决定是否启动代理）
      final wasEnabled = _config.enabled;
      final oldSandboxEnabled = _config.sandboxEnabled;
      appLogger.info(
        '[ProtectionConfig] Save config: asset=${widget.assetName}, editMode=${widget.isEditMode}, enabled=$shouldEnable, wasEnabled=$wasEnabled',
      );

      // Shepherd rules are persisted by Go JSON file directly.
      final ruleAssetID = _config.assetID.isNotEmpty
          ? _config.assetID
          : widget.assetID;
      await PluginService().updateShepherdRules(
        widget.assetName,
        ruleAssetID,
        _sensitiveActions,
      );

      final newConfig = _config.copyWith(
        assetID: _config.assetID.isNotEmpty ? _config.assetID : widget.assetID,
        enabled: shouldEnable,
        auditOnly: _auditOnly,
        sandboxEnabled: _sandboxEnabled,
        singleSessionTokenLimit: _displayToRaw(
          _singleSessionDisplayController.text,
          _singleSessionUnit,
        ),
        dailyTokenLimit: _displayToRaw(
          _dailyDisplayController.text,
          _dailyUnit,
        ),
        pathPermission: PathPermissionConfig(
          mode: _pathMode,
          paths: List.from(_pathList),
        ),
        networkPermission: NetworkPermissionConfig(
          outbound: DirectionalNetworkConfig(
            mode: _networkOutboundMode,
            addresses: List.from(_networkOutboundList),
          ),
          inbound: DirectionalNetworkConfig(
            mode: _networkInboundMode,
            addresses: List.from(_networkInboundList),
          ),
        ),
        shellPermission: ShellPermissionConfig(
          mode: _shellMode,
          commands: List.from(_shellList),
        ),
      );

      BotModelConfig? botModelConfig;
      if (_requiresBotModelConfig) {
        botModelConfig = await ModelConfigDatabaseService().getBotModelConfig(
          widget.assetName,
          newConfig.assetID,
        );
        if (botModelConfig == null) {
          if (mounted) {
            ScaffoldMessenger.of(
              context,
            ).showSnackBar(SnackBar(content: Text(l10n.modelConfigSaveFailed)));
            if (botTabIndex != null) {
              _tabController.animateTo(botTabIndex);
            }
          }
          return;
        }
      } else {
        try {
          botModelConfig = await ModelConfigDatabaseService().getBotModelConfig(
            widget.assetName,
            newConfig.assetID,
          );
        } catch (e) {
          appLogger.debug(
            '[ProtectionConfig] Optional bot model config load skipped: $e',
          );
        }
      }

      final securityModelConfig = await _ensureSecurityModelConfigured(
        botModelConfig,
      );
      if (securityModelConfig == null) {
        return;
      }

      try {
        // 2. 保存防护配置到数据库（确保 gateway 重启时能读到最新的沙箱/权限设置）
        await ProtectionDatabaseService().saveProtectionConfig(newConfig);
        _config = newConfig;

        appLogger.info(
          '[ProtectionConfig] Token limits saved: '
          'singleSession=${newConfig.singleSessionTokenLimit}, '
          'daily=${newConfig.dailyTokenLimit}, '
          'auditOnly=$_auditOnly, '
          'asset=${widget.assetName}',
        );

        // 3. 如果防护从禁用变为启用，启动代理
        if (!wasEnabled && shouldEnable) {
          final protectionService = ProtectionService.forAsset(
            widget.assetName,
            newConfig.assetID,
          );
          try {
            final result = await protectionService.startProtectionProxy(
              securityModelConfig,
              ProtectionRuntimeConfig(auditOnly: _auditOnly),
            );
            if (result['success'] == true) {
              appLogger.info(
                '[ProtectionConfig] Protection enabled: proxy started successfully',
              );
            } else {
              appLogger.warning(
                '[ProtectionConfig] Protection enabled: proxy start failed: ${result['error']}',
              );
            }
          } catch (e) {
            appLogger.warning(
              '[ProtectionConfig] Protection enabled: failed to start proxy: $e',
            );
          }
        } else {
          // 4. 推送审计模式和 Token 限额到运行中的代理
          final protectionService = ProtectionService.forAsset(
            widget.assetName,
            newConfig.assetID,
          );
          await protectionService.setAuditOnly(_auditOnly);
          await protectionService.pushTokenLimitsToProxy(
            assetName: widget.assetName,
            assetID: newConfig.assetID,
            singleSessionTokenLimit: newConfig.singleSessionTokenLimit,
            dailyTokenLimit: newConfig.dailyTokenLimit,
          );

          // 4b. 沙箱配置变更时同步到网关（修改 systemd unit / sandbox-exec 并重启 gateway）
          // 当沙箱开关变化或沙箱开启时权限可能变化，统一同步（函数幂等，无变化不重启）
          if (newConfig.sandboxEnabled || oldSandboxEnabled) {
            appLogger.info(
              '[ProtectionConfig] Sandbox config may have changed '
              '(enabled: $oldSandboxEnabled -> ${newConfig.sandboxEnabled}), '
              'syncing gateway...',
            );
            await protectionService.syncGatewaySandbox();
          }
        }

        // 5. Bot 模型变更后，触发完整重启（此时防护配置已保存到 DB，gateway 重启可读到最新配置）
        if (botModelSaved) {
          final protectionService = ProtectionService.forAsset(
            widget.assetName,
            newConfig.assetID,
          );
          if (protectionService.isProxyRunning) {
            try {
              final result = await protectionService
                  .restartProtectionProxyForBotModelUpdate(
                    securityModelConfig,
                    ProtectionRuntimeConfig(),
                  );
              if (result['success'] == true) {
                appLogger.info(
                  '[ProtectionConfig] Bot model update: proxy restarted successfully',
                );
              } else {
                appLogger.warning(
                  '[ProtectionConfig] Bot model update: proxy restart failed: ${result['error']}',
                );
              }
            } catch (e) {
              appLogger.warning(
                '[ProtectionConfig] Bot model update: failed to restart proxy: $e',
              );
            }
          }
        }

        if (closeOnSave && mounted) {
          Navigator.of(context).pop(newConfig);
        }
      } catch (e) {
        if (mounted) {
          ScaffoldMessenger.of(
            context,
          ).showSnackBar(SnackBar(content: Text('保存配置失败: $e')));
        }
      }
    } finally {
      if (mounted) {
        setState(() => _isSaving = false);
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context)!;

    return Stack(
      children: [
        Dialog(
          backgroundColor: const Color(0xFF1A1A2E),
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(16),
          ),
          child: Container(
            width: 800,
            height: 650,
            padding: const EdgeInsets.all(20),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                // 标题栏
                _buildHeader(l10n),
                const SizedBox(height: 16),

                // 标签页
                _buildTabs(l10n),
                const SizedBox(height: 16),

                // 内容区域
                Expanded(
                  child: _isLoading
                      ? const Center(child: CircularProgressIndicator())
                      : TabBarView(
                          controller: _tabController,
                          children: [
                            // 智能规则、Token限制
                            _buildSecurityPromptTab(l10n),
                            _buildTokenLimitTab(l10n),
                            // Personal版：权限设置
                            if (!BuildConfig.isAppStore)
                              _buildPermissionTab(l10n),
                            // 按插件能力决定是否展示 Bot 模型
                            if (_requiresBotModelConfig)
                              _buildBotModelTab(l10n),
                          ],
                        ),
                ),

                const SizedBox(height: 16),

                // 底部按钮
                _buildFooter(l10n),
              ],
            ),
          ),
        ),
        // 保存时的全屏 loading 遮罩
        if (_isSaving)
          Positioned.fill(
            child: Container(
              color: Colors.black.withValues(alpha: 0.5),
              child: Center(
                child: Container(
                  padding: const EdgeInsets.all(24),
                  decoration: BoxDecoration(
                    color: const Color(0xFF1A1A2E),
                    borderRadius: BorderRadius.circular(16),
                    border: Border.all(
                      color: Colors.white.withValues(alpha: 0.1),
                    ),
                    boxShadow: [
                      BoxShadow(
                        color: Colors.black.withValues(alpha: 0.5),
                        blurRadius: 24,
                        offset: const Offset(0, 8),
                      ),
                    ],
                  ),
                  child: Column(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      const SizedBox(
                        width: 32,
                        height: 32,
                        child: CircularProgressIndicator(
                          color: Color(0xFF6366F1),
                          strokeWidth: 3,
                        ),
                      ),
                      const SizedBox(height: 16),
                      Text(
                        l10n.modelConfigSaving,
                        style: AppFonts.inter(
                          color: Colors.white,
                          fontSize: 14,
                          fontWeight: FontWeight.w500,
                          decoration: TextDecoration.none,
                        ),
                      ),
                    ],
                  ),
                ),
              ),
            ),
          ),
      ],
    );
  }

  Widget _buildHeader(AppLocalizations l10n) {
    return Row(
      children: [
        Container(
          padding: const EdgeInsets.all(8),
          decoration: BoxDecoration(
            color: const Color(0xFF6366F1).withValues(alpha: 0.2),
            borderRadius: BorderRadius.circular(8),
          ),
          child: const Icon(
            LucideIcons.settings,
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
                widget.isEditMode
                    ? l10n.protectionConfigTitle
                    : l10n.protectionConfirmTitle,
                style: AppFonts.inter(
                  fontSize: 18,
                  fontWeight: FontWeight.w600,
                  color: Colors.white,
                ),
              ),
              Text(
                widget.assetName,
                style: AppFonts.inter(fontSize: 12, color: Colors.white54),
              ),
            ],
          ),
        ),
        IconButton(
          icon: const Icon(LucideIcons.x, color: Colors.white54, size: 20),
          onPressed: () => Navigator.of(context).pop(),
        ),
      ],
    );
  }

  Widget _buildTabs(AppLocalizations l10n) {
    Widget buildLabeledTab({required IconData icon, required String label}) {
      return Tab(
        child: Row(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Icon(icon, size: 16),
            const SizedBox(width: 6),
            Flexible(
              child: Text(
                label,
                textAlign: TextAlign.center,
                softWrap: true,
                maxLines: 2,
                overflow: TextOverflow.ellipsis,
              ),
            ),
          ],
        ),
      );
    }

    final tabs = <Widget>[
      buildLabeledTab(icon: LucideIcons.brain, label: l10n.securityPromptTab),
      buildLabeledTab(icon: LucideIcons.coins, label: l10n.tokenLimitTab),
      if (!BuildConfig.isAppStore)
        buildLabeledTab(icon: LucideIcons.shield, label: l10n.permissionTab),
      if (_requiresBotModelConfig)
        buildLabeledTab(icon: LucideIcons.bot, label: l10n.botModelTab),
    ];

    return Container(
      decoration: BoxDecoration(
        color: Colors.white.withValues(alpha: 0.05),
        borderRadius: BorderRadius.circular(8),
      ),
      child: TabBar(
        controller: _tabController,
        isScrollable: false,
        indicator: BoxDecoration(
          color: const Color(0xFF6366F1),
          borderRadius: BorderRadius.circular(6),
        ),
        indicatorSize: TabBarIndicatorSize.tab,
        labelColor: Colors.white,
        unselectedLabelColor: Colors.white54,
        labelStyle: AppFonts.inter(fontSize: 13, fontWeight: FontWeight.w500),
        tabs: tabs,
      ),
    );
  }

  Widget _buildSecurityPromptTab(AppLocalizations l10n) {
    return SingleChildScrollView(
      padding: const EdgeInsets.all(8),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          // 仅审计模式开关
          _buildAuditOnlySwitch(l10n),
          const SizedBox(height: 16),

          // Shepherd User Rules（标题 + 敏感操作，整体框起来）
          Container(
            padding: const EdgeInsets.all(12),
            decoration: BoxDecoration(
              color: Colors.white.withValues(alpha: 0.03),
              borderRadius: BorderRadius.circular(8),
              border: Border.all(color: Colors.white.withValues(alpha: 0.1)),
            ),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  children: [
                    const Icon(
                      LucideIcons.shieldAlert,
                      color: Color(0xFF6366F1),
                      size: 18,
                    ),
                    const SizedBox(width: 8),
                    Expanded(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Text(
                            l10n.shepherdRulesTitle,
                            style: AppFonts.inter(
                              fontSize: 14,
                              fontWeight: FontWeight.w600,
                              color: Colors.white,
                            ),
                          ),
                          const SizedBox(height: 2),
                          Text(
                            l10n.shepherdRulesDesc,
                            style: AppFonts.inter(
                              fontSize: 12,
                              color: Colors.white54,
                            ),
                          ),
                        ],
                      ),
                    ),
                  ],
                ),
                const SizedBox(height: 16),

                // Input
                Row(
                  children: [
                    Expanded(
                      child: Container(
                        height: 36,
                        decoration: BoxDecoration(
                          color: Colors.white.withValues(alpha: 0.05),
                          borderRadius: BorderRadius.circular(6),
                          border: Border.all(
                            color: Colors.white.withValues(alpha: 0.1),
                          ),
                        ),
                        child: TextField(
                          controller: _sensitiveActionsInputController,
                          style: AppFonts.firaCode(
                            fontSize: 12,
                            color: Colors.white,
                          ),
                          decoration: InputDecoration(
                            hintText: l10n.shepherdSensitivePlaceholder,
                            hintStyle: AppFonts.inter(
                              fontSize: 11,
                              color: Colors.white38,
                            ),
                            border: InputBorder.none,
                            contentPadding: const EdgeInsets.symmetric(
                              horizontal: 10,
                              vertical: 10,
                            ),
                          ),
                          onSubmitted: (_) {
                            final val = _sensitiveActionsInputController.text
                                .trim();
                            if (val.isNotEmpty &&
                                !_sensitiveActions.contains(val)) {
                              setState(() => _sensitiveActions.add(val));
                              _sensitiveActionsInputController.clear();
                              _saveConfig(closeOnSave: false);
                            }
                          },
                        ),
                      ),
                    ),
                    const SizedBox(width: 8),
                    MouseRegion(
                      cursor: SystemMouseCursors.click,
                      child: GestureDetector(
                        onTap: () {
                          final val = _sensitiveActionsInputController.text
                              .trim();
                          if (val.isNotEmpty &&
                              !_sensitiveActions.contains(val)) {
                            setState(() => _sensitiveActions.add(val));
                            _sensitiveActionsInputController.clear();
                            _saveConfig(closeOnSave: false);
                          }
                        },
                        child: Container(
                          height: 36,
                          padding: const EdgeInsets.symmetric(horizontal: 12),
                          decoration: BoxDecoration(
                            color: const Color(0xFF6366F1),
                            borderRadius: BorderRadius.circular(6),
                          ),
                          child: const Icon(
                            LucideIcons.plus,
                            size: 16,
                            color: Colors.white,
                          ),
                        ),
                      ),
                    ),
                  ],
                ),

                // Items
                if (_sensitiveActions.isNotEmpty) ...[
                  const SizedBox(height: 12),
                  Wrap(
                    spacing: 8,
                    runSpacing: 8,
                    children: _sensitiveActions.asMap().entries.map((entry) {
                      return Container(
                        padding: const EdgeInsets.symmetric(
                          horizontal: 10,
                          vertical: 4,
                        ),
                        decoration: BoxDecoration(
                          color: const Color(0xFFEF4444).withValues(alpha: 0.2),
                          borderRadius: BorderRadius.circular(4),
                          border: Border.all(
                            color: const Color(
                              0xFFEF4444,
                            ).withValues(alpha: 0.3),
                          ),
                        ),
                        child: Row(
                          mainAxisSize: MainAxisSize.min,
                          children: [
                            Flexible(
                              child: Text(
                                entry.value,
                                style: AppFonts.firaCode(
                                  fontSize: 11,
                                  color: Colors.white,
                                ),
                              ),
                            ),
                            const SizedBox(width: 6),
                            MouseRegion(
                              cursor: SystemMouseCursors.click,
                              child: GestureDetector(
                                onTap: () {
                                  setState(
                                    () => _sensitiveActions.removeAt(entry.key),
                                  );
                                  _saveConfig(closeOnSave: false);
                                },
                                child: const Icon(
                                  LucideIcons.x,
                                  size: 12,
                                  color: Colors.white54,
                                ),
                              ),
                            ),
                          ],
                        ),
                      );
                    }).toList(),
                  ),
                ],
              ],
            ),
          ),

          const SizedBox(height: 16),

          // 安全技能展示区域
          _buildSecuritySkillsSection(l10n),

          const SizedBox(height: 16),
          Container(
            padding: const EdgeInsets.all(12),
            decoration: BoxDecoration(
              color: const Color(0xFF6366F1).withValues(alpha: 0.1),
              borderRadius: BorderRadius.circular(8),
              border: Border.all(
                color: const Color(0xFF6366F1).withValues(alpha: 0.3),
              ),
            ),
            child: Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const Icon(
                  LucideIcons.info,
                  color: Color(0xFF6366F1),
                  size: 16,
                ),
                const SizedBox(width: 8),
                Expanded(
                  child: Text(
                    l10n.shepherdRulesTip,
                    style: AppFonts.inter(fontSize: 12, color: Colors.white70),
                  ),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }

  /// 构建安全技能只读展示区域
  Widget _buildSecuritySkillsSection(AppLocalizations l10n) {
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: Colors.white.withValues(alpha: 0.03),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: Colors.white.withValues(alpha: 0.1)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          _buildSectionHeader(
            l10n.securitySkillsTitle,
            LucideIcons.shieldCheck,
            l10n.securitySkillsDesc,
          ),
          const SizedBox(height: 16),
          if (_bundledSkills.isEmpty)
            Container(
              padding: const EdgeInsets.all(16),
              decoration: BoxDecoration(
                color: Colors.white.withValues(alpha: 0.03),
                borderRadius: BorderRadius.circular(8),
                border: Border.all(color: Colors.white.withValues(alpha: 0.1)),
              ),
              child: Text(
                '-',
                style: AppFonts.inter(fontSize: 12, color: Colors.white38),
              ),
            )
          else
            Column(
              children: _bundledSkills.map((skill) {
                final name = skill['name'] ?? '';
                final desc = skill['description'] ?? '';
                return Container(
                  margin: const EdgeInsets.only(bottom: 8),
                  padding: const EdgeInsets.all(12),
                  decoration: BoxDecoration(
                    color: const Color(0xFF22C55E).withValues(alpha: 0.08),
                    borderRadius: BorderRadius.circular(8),
                    border: Border.all(
                      color: const Color(0xFF22C55E).withValues(alpha: 0.25),
                    ),
                  ),
                  child: Row(
                    children: [
                      Container(
                        padding: const EdgeInsets.all(8),
                        decoration: BoxDecoration(
                          color: const Color(0xFF22C55E).withValues(alpha: 0.2),
                          borderRadius: BorderRadius.circular(6),
                        ),
                        child: const Icon(
                          LucideIcons.zap,
                          size: 16,
                          color: Color(0xFF22C55E),
                        ),
                      ),
                      const SizedBox(width: 12),
                      Expanded(
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Text(
                              name,
                              style: AppFonts.firaCode(
                                fontSize: 13,
                                fontWeight: FontWeight.w600,
                                color: Colors.white,
                              ),
                            ),
                            if (desc.isNotEmpty) ...[
                              const SizedBox(height: 4),
                              Text(
                                desc,
                                style: AppFonts.inter(
                                  fontSize: 11,
                                  color: Colors.white54,
                                ),
                                maxLines: 2,
                                overflow: TextOverflow.ellipsis,
                              ),
                            ],
                          ],
                        ),
                      ),
                    ],
                  ),
                );
              }).toList(),
            ),
        ],
      ),
    );
  }

  Widget _buildAuditOnlySwitch(AppLocalizations l10n) {
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: _auditOnly
            ? const Color(0xFFF59E0B).withValues(alpha: 0.1)
            : Colors.white.withValues(alpha: 0.05),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(
          color: _auditOnly
              ? const Color(0xFFF59E0B).withValues(alpha: 0.3)
              : Colors.white.withValues(alpha: 0.1),
        ),
      ),
      child: Row(
        children: [
          Container(
            padding: const EdgeInsets.all(8),
            decoration: BoxDecoration(
              color: _auditOnly
                  ? const Color(0xFFF59E0B).withValues(alpha: 0.2)
                  : Colors.white.withValues(alpha: 0.1),
              borderRadius: BorderRadius.circular(8),
            ),
            child: Icon(
              _auditOnly ? LucideIcons.eye : LucideIcons.shieldCheck,
              color: _auditOnly ? const Color(0xFFF59E0B) : Colors.white54,
              size: 20,
            ),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  l10n.auditOnlyMode,
                  style: AppFonts.inter(
                    fontSize: 14,
                    fontWeight: FontWeight.w600,
                    color: Colors.white,
                  ),
                ),
                const SizedBox(height: 2),
                Text(
                  l10n.auditOnlyModeDesc,
                  style: AppFonts.inter(fontSize: 11, color: Colors.white54),
                ),
              ],
            ),
          ),
          Switch(
            value: _auditOnly,
            onChanged: (value) => setState(() => _auditOnly = value),
            activeThumbColor: const Color(0xFFF59E0B),
            activeTrackColor: const Color(0xFFF59E0B).withValues(alpha: 0.3),
          ),
        ],
      ),
    );
  }

  Widget _buildTokenLimitTab(AppLocalizations l10n) {
    return SingleChildScrollView(
      padding: const EdgeInsets.all(8),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          _buildSectionHeader(
            l10n.tokenLimitTitle,
            LucideIcons.gauge,
            l10n.tokenLimitDesc,
          ),
          const SizedBox(height: 16),

          // 单轮会话Token限制
          _buildTokenQuotaField(
            label: l10n.singleSessionTokenLimit,
            hint: l10n.singleSessionTokenLimitPlaceholder,
            icon: LucideIcons.messageSquare,
            displayController: _singleSessionDisplayController,
            selectedUnit: _singleSessionUnit,
            onUnitChanged: (unit) => _singleSessionUnit = unit,
            presets: _singleSessionPresets,
            l10n: l10n,
          ),
          const SizedBox(height: 16),

          // 当日Token限制
          _buildTokenQuotaField(
            label: l10n.dailyTokenLimit,
            hint: l10n.dailyTokenLimitPlaceholder,
            icon: LucideIcons.calendar,
            displayController: _dailyDisplayController,
            selectedUnit: _dailyUnit,
            onUnitChanged: (unit) => _dailyUnit = unit,
            presets: _dailyPresets,
            l10n: l10n,
          ),
          const SizedBox(height: 16),

          Container(
            padding: const EdgeInsets.all(12),
            decoration: BoxDecoration(
              color: const Color(0xFFF59E0B).withValues(alpha: 0.1),
              borderRadius: BorderRadius.circular(8),
              border: Border.all(
                color: const Color(0xFFF59E0B).withValues(alpha: 0.3),
              ),
            ),
            child: Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const Icon(
                  LucideIcons.alertTriangle,
                  color: Color(0xFFF59E0B),
                  size: 16,
                ),
                const SizedBox(width: 8),
                Expanded(
                  child: Text(
                    l10n.tokenLimitTip,
                    style: AppFonts.inter(fontSize: 12, color: Colors.white70),
                  ),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildPermissionTab(AppLocalizations l10n) {
    // 检查是否支持沙箱（macOS 个人版 + Linux）
    final isSandboxSupported =
        (Platform.isMacOS && BuildConfig.isPersonal) ||
        Platform.isLinux ||
        Platform.isWindows;

    return SingleChildScrollView(
      padding: const EdgeInsets.all(8),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          // 沙箱启用开关（仅 macOS 个人版显示）
          if (isSandboxSupported) ...[
            _buildSandboxEnableSwitch(l10n),
            const SizedBox(height: 16),
          ],

          // 权限设置区域（沙箱禁用时降低透明度）
          Opacity(
            opacity: isSandboxSupported && !_sandboxEnabled ? 0.5 : 1.0,
            child: IgnorePointer(
              ignoring: isSandboxSupported && !_sandboxEnabled,
              child: Column(
                children: [
                  // 路径权限设置
                  _buildPermissionSection(
                    title: l10n.pathPermissionTitle,
                    desc: l10n.pathPermissionDesc,
                    icon: LucideIcons.folder,
                    mode: _pathMode,
                    onModeChanged: (mode) => setState(() => _pathMode = mode),
                    items: _pathList,
                    inputController: _pathInputController,
                    inputHint: l10n.pathPermissionPlaceholder,
                    onAdd: () {
                      final path = _pathInputController.text.trim();
                      if (path.isNotEmpty && !_pathList.contains(path)) {
                        setState(() => _pathList.add(path));
                        _pathInputController.clear();
                      }
                    },
                    onRemove: (index) =>
                        setState(() => _pathList.removeAt(index)),
                    onBrowse: () => _handlePathBrowse(l10n),
                  ),
                  const SizedBox(height: 20),

                  // 网络权限设置（出栈 + 入栈）
                  _buildNetworkPermissionSection(l10n),
                  const SizedBox(height: 20),

                  // Shell权限设置
                  _buildPermissionSection(
                    title: l10n.shellPermissionTitle,
                    desc: l10n.shellPermissionDesc,
                    icon: LucideIcons.terminal,
                    mode: _shellMode,
                    onModeChanged: (mode) => setState(() => _shellMode = mode),
                    items: _shellList,
                    inputController: _shellInputController,
                    inputHint: l10n.shellPermissionPlaceholder,
                    onAdd: () {
                      final cmd = _shellInputController.text.trim();
                      if (cmd.isNotEmpty && !_shellList.contains(cmd)) {
                        setState(() => _shellList.add(cmd));
                        _shellInputController.clear();
                      }
                    },
                    onRemove: (index) =>
                        setState(() => _shellList.removeAt(index)),
                  ),
                ],
              ),
            ),
          ),

          const SizedBox(height: 16),
          Container(
            padding: const EdgeInsets.all(12),
            decoration: BoxDecoration(
              color: Colors.white.withValues(alpha: 0.05),
              borderRadius: BorderRadius.circular(8),
              border: Border.all(color: Colors.white.withValues(alpha: 0.1)),
            ),
            child: Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const Icon(LucideIcons.info, color: Colors.white54, size: 16),
                const SizedBox(width: 8),
                Expanded(
                  child: Text(
                    l10n.permissionNote,
                    style: AppFonts.inter(fontSize: 11, color: Colors.white54),
                  ),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildSandboxEnableSwitch(AppLocalizations l10n) {
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: _sandboxEnabled
            ? const Color(0xFF22C55E).withValues(alpha: 0.1)
            : Colors.white.withValues(alpha: 0.05),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(
          color: _sandboxEnabled
              ? const Color(0xFF22C55E).withValues(alpha: 0.3)
              : Colors.white.withValues(alpha: 0.1),
        ),
      ),
      child: Row(
        children: [
          Container(
            padding: const EdgeInsets.all(8),
            decoration: BoxDecoration(
              color: _sandboxEnabled
                  ? const Color(0xFF22C55E).withValues(alpha: 0.2)
                  : Colors.white.withValues(alpha: 0.1),
              borderRadius: BorderRadius.circular(8),
            ),
            child: Icon(
              _sandboxEnabled ? LucideIcons.shieldCheck : LucideIcons.shield,
              color: _sandboxEnabled ? const Color(0xFF22C55E) : Colors.white54,
              size: 20,
            ),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  l10n.sandboxProtection,
                  style: AppFonts.inter(
                    fontSize: 14,
                    fontWeight: FontWeight.w600,
                    color: Colors.white,
                  ),
                ),
                const SizedBox(height: 2),
                Text(
                  l10n.sandboxProtectionDesc,
                  style: AppFonts.inter(fontSize: 11, color: Colors.white54),
                ),
              ],
            ),
          ),
          Switch(
            value: _sandboxEnabled,
            onChanged: (value) => setState(() => _sandboxEnabled = value),
            activeThumbColor: const Color(0xFF22C55E),
            activeTrackColor: const Color(0xFF22C55E).withValues(alpha: 0.3),
          ),
        ],
      ),
    );
  }

  Widget _buildPermissionSection({
    required String title,
    required String desc,
    required IconData icon,
    required PermissionMode mode,
    required Function(PermissionMode) onModeChanged,
    required List<String> items,
    required TextEditingController inputController,
    required String inputHint,
    required VoidCallback onAdd,
    required Function(int) onRemove,
    VoidCallback? onBrowse,
  }) {
    final l10n = AppLocalizations.of(context)!;

    return Container(
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: Colors.white.withValues(alpha: 0.03),
        borderRadius: BorderRadius.circular(8),
        border: Border.all(color: Colors.white.withValues(alpha: 0.1)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Icon(icon, color: const Color(0xFF6366F1), size: 16),
              const SizedBox(width: 8),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      title,
                      style: AppFonts.inter(
                        fontSize: 13,
                        fontWeight: FontWeight.w600,
                        color: Colors.white,
                      ),
                    ),
                    Text(
                      desc,
                      style: AppFonts.inter(
                        fontSize: 11,
                        color: Colors.white54,
                      ),
                    ),
                  ],
                ),
              ),
            ],
          ),
          const SizedBox(height: 12),

          // 黑白名单切换
          Row(
            children: [
              _buildModeButton(
                label: l10n.blacklistMode,
                isSelected: mode == PermissionMode.blacklist,
                onTap: () => onModeChanged(PermissionMode.blacklist),
              ),
              const SizedBox(width: 8),
              _buildModeButton(
                label: l10n.whitelistMode,
                isSelected: mode == PermissionMode.whitelist,
                onTap: () => onModeChanged(PermissionMode.whitelist),
              ),
            ],
          ),
          const SizedBox(height: 12),

          // 输入框
          Row(
            children: [
              Expanded(
                child: Container(
                  height: 36,
                  decoration: BoxDecoration(
                    color: Colors.white.withValues(alpha: 0.05),
                    borderRadius: BorderRadius.circular(6),
                    border: Border.all(
                      color: Colors.white.withValues(alpha: 0.1),
                    ),
                  ),
                  child: TextField(
                    controller: inputController,
                    style: AppFonts.firaCode(fontSize: 12, color: Colors.white),
                    decoration: InputDecoration(
                      hintText: inputHint,
                      hintStyle: AppFonts.inter(
                        fontSize: 11,
                        color: Colors.white38,
                      ),
                      border: InputBorder.none,
                      contentPadding: const EdgeInsets.symmetric(
                        horizontal: 10,
                        vertical: 10,
                      ),
                    ),
                    onSubmitted: (_) => onAdd(),
                  ),
                ),
              ),
              // 浏览按钮（仅路径权限显示）
              if (onBrowse != null) ...[
                const SizedBox(width: 8),
                MouseRegion(
                  cursor: SystemMouseCursors.click,
                  child: GestureDetector(
                    onTap: onBrowse,
                    child: Container(
                      height: 36,
                      padding: const EdgeInsets.symmetric(horizontal: 12),
                      decoration: BoxDecoration(
                        color: Colors.white.withValues(alpha: 0.1),
                        borderRadius: BorderRadius.circular(6),
                        border: Border.all(
                          color: Colors.white.withValues(alpha: 0.2),
                        ),
                      ),
                      child: const Icon(
                        LucideIcons.folderOpen,
                        size: 16,
                        color: Colors.white70,
                      ),
                    ),
                  ),
                ),
              ],
              const SizedBox(width: 8),
              MouseRegion(
                cursor: SystemMouseCursors.click,
                child: GestureDetector(
                  onTap: onAdd,
                  child: Container(
                    height: 36,
                    padding: const EdgeInsets.symmetric(horizontal: 12),
                    decoration: BoxDecoration(
                      color: const Color(0xFF6366F1),
                      borderRadius: BorderRadius.circular(6),
                    ),
                    child: const Icon(
                      LucideIcons.plus,
                      size: 16,
                      color: Colors.white,
                    ),
                  ),
                ),
              ),
            ],
          ),

          // 已添加的项
          if (items.isNotEmpty) ...[
            const SizedBox(height: 12),
            Wrap(
              spacing: 8,
              runSpacing: 8,
              children: items.asMap().entries.map((entry) {
                return Container(
                  padding: const EdgeInsets.symmetric(
                    horizontal: 10,
                    vertical: 4,
                  ),
                  decoration: BoxDecoration(
                    color: mode == PermissionMode.blacklist
                        ? const Color(0xFFEF4444).withValues(alpha: 0.2)
                        : const Color(0xFF22C55E).withValues(alpha: 0.2),
                    borderRadius: BorderRadius.circular(4),
                    border: Border.all(
                      color: mode == PermissionMode.blacklist
                          ? const Color(0xFFEF4444).withValues(alpha: 0.3)
                          : const Color(0xFF22C55E).withValues(alpha: 0.3),
                    ),
                  ),
                  child: Row(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Flexible(
                        child: Text(
                          entry.value,
                          style: AppFonts.firaCode(
                            fontSize: 11,
                            color: Colors.white,
                          ),
                        ),
                      ),
                      const SizedBox(width: 6),
                      MouseRegion(
                        cursor: SystemMouseCursors.click,
                        child: GestureDetector(
                          onTap: () => onRemove(entry.key),
                          child: const Icon(
                            LucideIcons.x,
                            size: 12,
                            color: Colors.white54,
                          ),
                        ),
                      ),
                    ],
                  ),
                );
              }).toList(),
            ),
          ],
        ],
      ),
    );
  }

  Widget _buildModeButton({
    required String label,
    required bool isSelected,
    required VoidCallback onTap,
  }) {
    return MouseRegion(
      cursor: SystemMouseCursors.click,
      child: GestureDetector(
        onTap: onTap,
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
          decoration: BoxDecoration(
            color: isSelected
                ? const Color(0xFF6366F1)
                : Colors.white.withValues(alpha: 0.05),
            borderRadius: BorderRadius.circular(4),
            border: Border.all(
              color: isSelected
                  ? const Color(0xFF6366F1)
                  : Colors.white.withValues(alpha: 0.1),
            ),
          ),
          child: Text(
            label,
            style: AppFonts.inter(
              fontSize: 11,
              fontWeight: FontWeight.w500,
              color: isSelected ? Colors.white : Colors.white54,
            ),
          ),
        ),
      ),
    );
  }

  Widget _buildSectionHeader(String title, IconData icon, String desc) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          children: [
            Icon(icon, color: const Color(0xFF6366F1), size: 18),
            const SizedBox(width: 8),
            Text(
              title,
              style: AppFonts.inter(
                fontSize: 14,
                fontWeight: FontWeight.w600,
                color: Colors.white,
              ),
            ),
          ],
        ),
        const SizedBox(height: 4),
        Text(desc, style: AppFonts.inter(fontSize: 12, color: Colors.white54)),
      ],
    );
  }

  /// 原始 token 值转换为显示值（数字文本 + 单位）
  (String, _TokenUnit) _rawToDisplay(int rawValue) {
    if (rawValue <= 0) return ('', _TokenUnit.k);
    if (rawValue % 1000000 == 0) {
      return ('${rawValue ~/ 1000000}', _TokenUnit.m);
    }
    if (rawValue % 1000 == 0) {
      return ('${rawValue ~/ 1000}', _TokenUnit.k);
    }
    // 兜底：以K为单位整除
    return ('${rawValue ~/ 1000}', _TokenUnit.k);
  }

  /// 显示值转换为原始 token 值
  int _displayToRaw(String text, _TokenUnit unit) {
    if (text.isEmpty) return 0;
    final parsed = int.tryParse(text);
    if (parsed == null || parsed <= 0) return 0;
    return parsed * unit.multiplier;
  }

  /// 构建 Token 配额复合输入组件（快捷选择 + 输入框 + 单位下拉）
  Widget _buildTokenQuotaField({
    required String label,
    required String hint,
    required IconData icon,
    required TextEditingController displayController,
    required _TokenUnit selectedUnit,
    required ValueChanged<_TokenUnit> onUnitChanged,
    required List<_TokenPreset> presets,
    required AppLocalizations l10n,
  }) {
    final currentRaw = _displayToRaw(displayController.text, selectedUnit);

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        // 标签行
        Row(
          children: [
            Icon(icon, color: Colors.white54, size: 16),
            const SizedBox(width: 8),
            Text(
              label,
              style: AppFonts.inter(
                fontSize: 13,
                fontWeight: FontWeight.w500,
                color: Colors.white,
              ),
            ),
          ],
        ),
        const SizedBox(height: 8),

        // 快捷选择 Chips
        Wrap(
          spacing: 6,
          runSpacing: 6,
          children: presets.map((preset) {
            final isSelected = currentRaw == preset.rawValue;
            return _buildPresetChip(
              label: preset.labelBuilder(l10n),
              isSelected: isSelected,
              onTap: () {
                setState(() {
                  if (preset.rawValue == 0) {
                    displayController.clear();
                  } else {
                    final (text, unit) = _rawToDisplay(preset.rawValue);
                    displayController.text = text;
                    onUnitChanged(unit);
                  }
                });
              },
            );
          }).toList(),
        ),
        const SizedBox(height: 8),

        // 输入框 + 单位下拉
        Container(
          decoration: BoxDecoration(
            color: Colors.white.withValues(alpha: 0.05),
            borderRadius: BorderRadius.circular(8),
            border: Border.all(color: Colors.white.withValues(alpha: 0.1)),
          ),
          child: Row(
            children: [
              // 数字输入框
              Expanded(
                child: TextField(
                  controller: displayController,
                  keyboardType: TextInputType.number,
                  inputFormatters: [FilteringTextInputFormatter.digitsOnly],
                  style: AppFonts.firaCode(fontSize: 14, color: Colors.white),
                  onChanged: (_) => setState(() {}),
                  decoration: InputDecoration(
                    hintText: hint,
                    hintStyle: AppFonts.inter(
                      fontSize: 12,
                      color: Colors.white38,
                    ),
                    border: InputBorder.none,
                    contentPadding: const EdgeInsets.symmetric(
                      horizontal: 12,
                      vertical: 12,
                    ),
                  ),
                ),
              ),
              // 分隔线
              Container(
                width: 1,
                height: 28,
                color: Colors.white.withValues(alpha: 0.1),
              ),
              // 单位选择下拉
              PopupMenuButton<_TokenUnit>(
                onSelected: (unit) {
                  setState(() {
                    onUnitChanged(unit);
                  });
                },
                offset: const Offset(0, 36),
                color: const Color(0xFF1E1E2E),
                shape: RoundedRectangleBorder(
                  borderRadius: BorderRadius.circular(8),
                  side: BorderSide(color: Colors.white.withValues(alpha: 0.1)),
                ),
                itemBuilder: (context) => _TokenUnit.values.map((unit) {
                  return PopupMenuItem<_TokenUnit>(
                    value: unit,
                    child: Text(
                      unit.label(l10n),
                      style: AppFonts.inter(
                        fontSize: 13,
                        color: unit == selectedUnit
                            ? const Color(0xFFF59E0B)
                            : Colors.white70,
                        fontWeight: unit == selectedUnit
                            ? FontWeight.w600
                            : FontWeight.w400,
                      ),
                    ),
                  );
                }).toList(),
                child: Padding(
                  padding: const EdgeInsets.symmetric(
                    horizontal: 12,
                    vertical: 8,
                  ),
                  child: Row(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Text(
                        selectedUnit.label(l10n),
                        style: AppFonts.inter(
                          fontSize: 13,
                          fontWeight: FontWeight.w500,
                          color: const Color(0xFFF59E0B),
                        ),
                      ),
                      const SizedBox(width: 4),
                      const Icon(
                        LucideIcons.chevronDown,
                        size: 14,
                        color: Color(0xFFF59E0B),
                      ),
                    ],
                  ),
                ),
              ),
            ],
          ),
        ),
      ],
    );
  }

  /// 构建预设值 Chip
  Widget _buildPresetChip({
    required String label,
    required bool isSelected,
    required VoidCallback onTap,
  }) {
    return InkWell(
      onTap: onTap,
      borderRadius: BorderRadius.circular(6),
      child: AnimatedContainer(
        duration: const Duration(milliseconds: 150),
        padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 5),
        decoration: BoxDecoration(
          color: isSelected
              ? const Color(0xFFF59E0B).withValues(alpha: 0.2)
              : Colors.white.withValues(alpha: 0.05),
          borderRadius: BorderRadius.circular(6),
          border: Border.all(
            color: isSelected
                ? const Color(0xFFF59E0B)
                : Colors.white.withValues(alpha: 0.1),
          ),
        ),
        child: Text(
          label,
          style: AppFonts.inter(
            fontSize: 12,
            fontWeight: isSelected ? FontWeight.w600 : FontWeight.w400,
            color: isSelected ? const Color(0xFFF59E0B) : Colors.white70,
          ),
        ),
      ),
    );
  }

  /// 构建 Bot 模型标签页
  Widget _buildBotModelTab(AppLocalizations l10n) {
    return SingleChildScrollView(
      padding: const EdgeInsets.all(8),
      child: BotModelConfigForm(
        key: _botModelFormKey,
        assetName: widget.assetName,
        assetID: _config.assetID.isNotEmpty ? _config.assetID : widget.assetID,
      ),
    );
  }

  /// 构建网络权限设置区块（出栈 + 入栈）
  Widget _buildNetworkPermissionSection(AppLocalizations l10n) {
    final isMacSandbox = Platform.isMacOS && _sandboxEnabled;
    final placeholder = isMacSandbox
        ? l10n.networkPermissionPlaceholderSandbox
        : l10n.networkPermissionPlaceholder;

    return Container(
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: Colors.white.withValues(alpha: 0.03),
        borderRadius: BorderRadius.circular(8),
        border: Border.all(color: Colors.white.withValues(alpha: 0.1)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          // 总标题
          Row(
            children: [
              const Icon(LucideIcons.globe, color: Color(0xFF6366F1), size: 16),
              const SizedBox(width: 8),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      l10n.networkPermissionTitle,
                      style: AppFonts.inter(
                        fontSize: 13,
                        fontWeight: FontWeight.w600,
                        color: Colors.white,
                      ),
                    ),
                    Text(
                      isMacSandbox
                          ? l10n.networkPermissionDescSandbox
                          : l10n.networkPermissionDesc,
                      style: AppFonts.inter(
                        fontSize: 11,
                        color: Colors.white54,
                      ),
                    ),
                  ],
                ),
              ),
            ],
          ),
          const SizedBox(height: 16),

          // 出栈 (Outbound) 子区块
          _buildDirectionalNetworkBlock(
            title: l10n.networkOutboundTitle,
            desc: l10n.networkOutboundDesc,
            icon: LucideIcons.arrowUpRight,
            mode: _networkOutboundMode,
            onModeChanged: (mode) =>
                setState(() => _networkOutboundMode = mode),
            items: _networkOutboundList,
            inputController: _networkOutboundInputController,
            inputHint: placeholder,
            onAdd: () => _addNetworkAddress(
              _networkOutboundInputController,
              _networkOutboundList,
              l10n,
            ),
            onRemove: (index) =>
                setState(() => _networkOutboundList.removeAt(index)),
          ),
          const SizedBox(height: 16),

          // 入栈 (Inbound) 子区块
          _buildDirectionalNetworkBlock(
            title: l10n.networkInboundTitle,
            desc: l10n.networkInboundDesc,
            icon: LucideIcons.arrowDownLeft,
            mode: _networkInboundMode,
            onModeChanged: (mode) => setState(() => _networkInboundMode = mode),
            items: _networkInboundList,
            inputController: _networkInboundInputController,
            inputHint: placeholder,
            onAdd: () => _addNetworkAddress(
              _networkInboundInputController,
              _networkInboundList,
              l10n,
            ),
            onRemove: (index) =>
                setState(() => _networkInboundList.removeAt(index)),
          ),
        ],
      ),
    );
  }

  /// 添加网络地址的通用方法（含校验）
  void _addNetworkAddress(
    TextEditingController controller,
    List<String> list,
    AppLocalizations l10n,
  ) {
    final addr = controller.text.trim();
    if (addr.isEmpty) return;
    if (list.contains(addr)) return;
    // 当 macOS 沙箱启用时，校验地址是否符合 sandbox-exec 限制
    if (Platform.isMacOS &&
        _sandboxEnabled &&
        !NetworkPermissionConfig.isValidSandboxAddress(addr)) {
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(l10n.networkAddressInvalidForSandbox),
          backgroundColor: const Color(0xFFEF4444),
          duration: const Duration(seconds: 4),
        ),
      );
      return;
    }
    setState(() => list.add(addr));
    controller.clear();
  }

  Future<void> _handlePathBrowse(AppLocalizations l10n) async {
    try {
      final result = await FilePicker.platform.getDirectoryPath(
        dialogTitle: l10n.pathPermissionTitle,
      );
      if (!mounted || result == null || _pathList.contains(result)) {
        return;
      }
      setState(() => _pathList.add(result));
    } on Exception catch (e) {
      appLogger.warning('[ProtectionConfig] Path picker unavailable: $e');
      if (!mounted) return;
      await _showPathPickerFallback(l10n, e.toString());
    }
  }

  Future<void> _showPathPickerFallback(
    AppLocalizations l10n,
    String errorMessage,
  ) async {
    final controller = TextEditingController(text: _pathInputController.text);
    final fallbackMessage = Platform.isLinux
        ? 'Linux 缺少可用的目录选择器，请手动输入路径，或安装 zenity、qarma、kdialog 后重试。\n$errorMessage'
        : '无法打开目录选择器，请手动输入路径后重试。\n$errorMessage';

    try {
      final selectedPath = await showDialog<String>(
        context: context,
        builder: (dialogContext) => AlertDialog(
          backgroundColor: const Color(0xFF1F2937),
          title: Text(
            l10n.pathPermissionTitle,
            style: AppFonts.inter(
              fontSize: 16,
              fontWeight: FontWeight.w600,
              color: Colors.white,
            ),
          ),
          content: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                fallbackMessage,
                style: AppFonts.inter(fontSize: 12, color: Colors.white70),
              ),
              const SizedBox(height: 12),
              TextField(
                controller: controller,
                autofocus: true,
                style: AppFonts.firaCode(fontSize: 12, color: Colors.white),
                decoration: InputDecoration(
                  hintText: l10n.pathPermissionDesc,
                  hintStyle: AppFonts.inter(
                    fontSize: 11,
                    color: Colors.white38,
                  ),
                  enabledBorder: OutlineInputBorder(
                    borderRadius: BorderRadius.circular(8),
                    borderSide: BorderSide(
                      color: Colors.white.withValues(alpha: 0.15),
                    ),
                  ),
                  focusedBorder: const OutlineInputBorder(
                    borderRadius: BorderRadius.all(Radius.circular(8)),
                    borderSide: BorderSide(color: Color(0xFF6366F1)),
                  ),
                ),
              ),
            ],
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.of(dialogContext).pop(),
              child: Text('取消', style: AppFonts.inter(color: Colors.white70)),
            ),
            ElevatedButton(
              onPressed: () {
                final value = controller.text.trim();
                Navigator.of(dialogContext).pop(value.isEmpty ? null : value);
              },
              style: ElevatedButton.styleFrom(
                backgroundColor: const Color(0xFF6366F1),
              ),
              child: Text('添加', style: AppFonts.inter(color: Colors.white)),
            ),
          ],
        ),
      );

      if (!mounted ||
          selectedPath == null ||
          _pathList.contains(selectedPath)) {
        return;
      }
      setState(() => _pathList.add(selectedPath));
      _pathInputController.clear();
    } finally {
      controller.dispose();
    }
  }

  /// 构建单方向的网络配置子区块
  Widget _buildDirectionalNetworkBlock({
    required String title,
    required String desc,
    required IconData icon,
    required PermissionMode mode,
    required Function(PermissionMode) onModeChanged,
    required List<String> items,
    required TextEditingController inputController,
    required String inputHint,
    required VoidCallback onAdd,
    required Function(int) onRemove,
  }) {
    final l10n = AppLocalizations.of(context)!;

    return Container(
      padding: const EdgeInsets.all(10),
      decoration: BoxDecoration(
        color: Colors.white.withValues(alpha: 0.03),
        borderRadius: BorderRadius.circular(6),
        border: Border.all(color: Colors.white.withValues(alpha: 0.08)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          // 方向标题
          Row(
            children: [
              Icon(icon, color: Colors.white70, size: 14),
              const SizedBox(width: 6),
              Text(
                title,
                style: AppFonts.inter(
                  fontSize: 12,
                  fontWeight: FontWeight.w600,
                  color: Colors.white,
                ),
              ),
              const SizedBox(width: 8),
              Expanded(
                child: Text(
                  desc,
                  style: AppFonts.inter(fontSize: 10, color: Colors.white38),
                ),
              ),
            ],
          ),
          const SizedBox(height: 8),

          // 黑白名单切换
          Row(
            children: [
              _buildModeButton(
                label: l10n.blacklistMode,
                isSelected: mode == PermissionMode.blacklist,
                onTap: () => onModeChanged(PermissionMode.blacklist),
              ),
              const SizedBox(width: 8),
              _buildModeButton(
                label: l10n.whitelistMode,
                isSelected: mode == PermissionMode.whitelist,
                onTap: () => onModeChanged(PermissionMode.whitelist),
              ),
            ],
          ),
          const SizedBox(height: 8),

          // 输入框
          Row(
            children: [
              Expanded(
                child: Container(
                  height: 38,
                  decoration: BoxDecoration(
                    color: Colors.white.withValues(alpha: 0.05),
                    borderRadius: BorderRadius.circular(6),
                    border: Border.all(
                      color: Colors.white.withValues(alpha: 0.1),
                    ),
                  ),
                  child: TextField(
                    controller: inputController,
                    style: AppFonts.firaCode(fontSize: 12, color: Colors.white),
                    decoration: InputDecoration(
                      hintText: inputHint,
                      hintStyle: AppFonts.inter(
                        fontSize: 11,
                        color: Colors.white38,
                      ),
                      border: InputBorder.none,
                      contentPadding: const EdgeInsets.symmetric(
                        horizontal: 10,
                        vertical: 10,
                      ),
                    ),
                    onSubmitted: (_) => onAdd(),
                  ),
                ),
              ),
              const SizedBox(width: 6),
              MouseRegion(
                cursor: SystemMouseCursors.click,
                child: GestureDetector(
                  onTap: onAdd,
                  child: Container(
                    height: 38,
                    padding: const EdgeInsets.symmetric(horizontal: 10),
                    decoration: BoxDecoration(
                      color: const Color(0xFF6366F1),
                      borderRadius: BorderRadius.circular(6),
                    ),
                    child: const Icon(
                      LucideIcons.plus,
                      size: 14,
                      color: Colors.white,
                    ),
                  ),
                ),
              ),
            ],
          ),

          // 已添加的项
          if (items.isNotEmpty) ...[
            const SizedBox(height: 8),
            Wrap(
              spacing: 6,
              runSpacing: 6,
              children: items.asMap().entries.map((entry) {
                return Container(
                  padding: const EdgeInsets.symmetric(
                    horizontal: 8,
                    vertical: 3,
                  ),
                  decoration: BoxDecoration(
                    color: mode == PermissionMode.blacklist
                        ? const Color(0xFFEF4444).withValues(alpha: 0.2)
                        : const Color(0xFF22C55E).withValues(alpha: 0.2),
                    borderRadius: BorderRadius.circular(4),
                    border: Border.all(
                      color: mode == PermissionMode.blacklist
                          ? const Color(0xFFEF4444).withValues(alpha: 0.3)
                          : const Color(0xFF22C55E).withValues(alpha: 0.3),
                    ),
                  ),
                  child: Row(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Flexible(
                        child: Text(
                          entry.value,
                          style: AppFonts.firaCode(
                            fontSize: 10,
                            color: Colors.white,
                          ),
                        ),
                      ),
                      const SizedBox(width: 4),
                      MouseRegion(
                        cursor: SystemMouseCursors.click,
                        child: GestureDetector(
                          onTap: () => onRemove(entry.key),
                          child: const Icon(
                            LucideIcons.x,
                            size: 10,
                            color: Colors.white54,
                          ),
                        ),
                      ),
                    ],
                  ),
                );
              }).toList(),
            ),
          ],
        ],
      ),
    );
  }

  Widget _buildFooter(AppLocalizations l10n) {
    return Row(
      mainAxisAlignment: MainAxisAlignment.end,
      children: [
        TextButton(
          onPressed: _isSaving ? null : () => Navigator.of(context).pop(),
          child: Text(
            l10n.cancel,
            style: AppFonts.inter(
              color: _isSaving ? Colors.white24 : Colors.white54,
            ),
          ),
        ),
        const SizedBox(width: 12),
        ElevatedButton(
          onPressed: _isSaving ? null : _saveConfig,
          style: ElevatedButton.styleFrom(
            backgroundColor: const Color(0xFF6366F1),
            disabledBackgroundColor: const Color(
              0xFF6366F1,
            ).withValues(alpha: 0.5),
            padding: const EdgeInsets.symmetric(horizontal: 24, vertical: 12),
            shape: RoundedRectangleBorder(
              borderRadius: BorderRadius.circular(8),
            ),
          ),
          child: _isSaving
              ? const SizedBox(
                  width: 16,
                  height: 16,
                  child: CircularProgressIndicator(
                    strokeWidth: 2,
                    valueColor: AlwaysStoppedAnimation<Color>(Colors.white),
                  ),
                )
              : Text(
                  widget.isEditMode
                      ? l10n.saveConfig
                      : l10n.protectionConfirmButton,
                  style: AppFonts.inter(
                    fontWeight: FontWeight.w600,
                    color: Colors.white,
                  ),
                ),
        ),
      ],
    );
  }
}
