import 'package:flutter/material.dart';
import 'package:lucide_icons/lucide_icons.dart';
import '../l10n/app_localizations.dart';
import '../utils/app_fonts.dart';
import 'general_settings_tab.dart';
import 'security_model_config_form.dart';

/// 带 Tab 的统一设置对话框
/// Tab 0: 安全模型配置（复用 SecurityModelConfigForm）
/// Tab 1: 通用设置（开机启动、清空数据、恢复配置、重新授权）
class SettingsDialog extends StatefulWidget {
  final bool launchAtStartupEnabled;
  final Future<void> Function({
    required bool launchAtStartupEnabled,
    required int scheduledScanIntervalSeconds,
  })
  onSaveGeneralSettings;
  final int scheduledScanIntervalSeconds;
  final VoidCallback onClearData;
  final VoidCallback onRestoreConfig;
  final VoidCallback onShowAbout;
  final VoidCallback onReauthorizeDirectory;
  final bool apiServerEnabled;
  final ValueChanged<bool> onToggleApiServer;

  const SettingsDialog({
    super.key,
    required this.launchAtStartupEnabled,
    required this.onSaveGeneralSettings,
    required this.scheduledScanIntervalSeconds,
    required this.onClearData,
    required this.onRestoreConfig,
    required this.onShowAbout,
    required this.onReauthorizeDirectory,
    required this.apiServerEnabled,
    required this.onToggleApiServer,
  });

  @override
  State<SettingsDialog> createState() => _SettingsDialogState();
}

class _SettingsDialogState extends State<SettingsDialog>
    with SingleTickerProviderStateMixin {
  late TabController _tabController;
  final GlobalKey<SecurityModelConfigFormState> _formKey =
      GlobalKey<SecurityModelConfigFormState>();
  bool _saving = false;
  int _currentTabIndex = 0;
  late bool _localLaunchAtStartup;
  late int _localScheduledScanIntervalSeconds;
  late bool _localApiServerEnabled;

  @override
  void initState() {
    super.initState();
    _localLaunchAtStartup = widget.launchAtStartupEnabled;
    _localScheduledScanIntervalSeconds = widget.scheduledScanIntervalSeconds;
    _localApiServerEnabled = widget.apiServerEnabled;
    _tabController = TabController(length: 2, vsync: this);
    _tabController.addListener(() {
      if (_tabController.indexIsChanging) return;
      setState(() {
        _currentTabIndex = _tabController.index;
      });
    });
  }

  @override
  void didUpdateWidget(covariant SettingsDialog oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.apiServerEnabled != widget.apiServerEnabled) {
      _localApiServerEnabled = widget.apiServerEnabled;
    }
  }

  @override
  void dispose() {
    _tabController.dispose();
    super.dispose();
  }

  Future<void> _handleSave() async {
    if (_saving) return;
    setState(() {
      _saving = true;
    });

    bool success = false;
    if (_currentTabIndex == 0) {
      success = await _formKey.currentState?.saveConfig() == true;
    } else {
      await widget.onSaveGeneralSettings(
        launchAtStartupEnabled: _localLaunchAtStartup,
        scheduledScanIntervalSeconds: _localScheduledScanIntervalSeconds,
      );
      success = true;
    }

    if (success) {
      if (mounted && Navigator.of(context).canPop()) {
        Navigator.of(context).pop(true);
      }
    } else {
      if (mounted) {
        setState(() {
          _saving = false;
        });
      }
    }
  }

  /// 处理手动验证连通性动作。
  Future<void> _handleValidateConnection() async {
    if (_saving || _currentTabIndex != 0) return;
    await _formKey.currentState?.validateConnection();
  }

  void _handleCancel() {
    if (_saving) return;
    if (Navigator.of(context).canPop()) {
      Navigator.of(context).pop();
    }
  }

  void _handleLaunchAtStartupChanged(bool value) {
    setState(() {
      _localLaunchAtStartup = value;
    });
  }

  void _handleScheduledScanIntervalChanged(int seconds) {
    setState(() {
      _localScheduledScanIntervalSeconds = seconds;
    });
  }

  void _handleToggleApiServer(bool enable) {
    setState(() {
      _localApiServerEnabled = enable;
    });
    widget.onToggleApiServer(enable);
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context)!;
    return Dialog(
      backgroundColor: const Color(0xFF1A1A2E),
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
      child: Container(
        width: 500,
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            // Header
            _buildHeader(l10n),
            const SizedBox(height: 16),
            // Tab bar
            _buildTabBar(l10n),
            const SizedBox(height: 16),
            // Tab content
            SizedBox(
              height: 400,
              child: TabBarView(
                controller: _tabController,
                children: [
                  // Tab 0: 安全模型
                  SecurityModelConfigForm(key: _formKey),
                  // Tab 1: 通用设置
                  GeneralSettingsTab(
                    launchAtStartupEnabled: _localLaunchAtStartup,
                    onLaunchAtStartupChanged: _handleLaunchAtStartupChanged,
                    scheduledScanIntervalSeconds:
                        _localScheduledScanIntervalSeconds,
                    onScheduledScanIntervalChanged:
                        _handleScheduledScanIntervalChanged,
                    onClearData: widget.onClearData,
                    onRestoreConfig: widget.onRestoreConfig,
                    onShowAbout: widget.onShowAbout,
                    onReauthorizeDirectory: widget.onReauthorizeDirectory,
                    apiServerEnabled: _localApiServerEnabled,
                    onToggleApiServer: _handleToggleApiServer,
                  ),
                ],
              ),
            ),
            // Footer: 仅安全模型 Tab 显示保存/取消按钮
            const SizedBox(height: 20),
            _buildFooter(l10n),
          ],
        ),
      ),
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
          child: Text(
            l10n.settings,
            style: AppFonts.inter(
              fontSize: 18,
              fontWeight: FontWeight.w600,
              color: Colors.white,
            ),
          ),
        ),
        IconButton(
          icon: const Icon(LucideIcons.x, color: Colors.white54, size: 20),
          onPressed: _saving ? null : _handleCancel,
        ),
      ],
    );
  }

  /// Tab 样式参照 ProtectionConfigDialog._buildTabs
  Widget _buildTabBar(AppLocalizations l10n) {
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
        dividerHeight: 0,
        tabs: [
          Tab(
            child: Row(
              mainAxisAlignment: MainAxisAlignment.center,
              children: [
                const Icon(LucideIcons.shield, size: 16),
                const SizedBox(width: 6),
                Flexible(
                  child: Text(
                    l10n.modelConfigTitle,
                    textAlign: TextAlign.center,
                    softWrap: true,
                    maxLines: 2,
                    overflow: TextOverflow.ellipsis,
                  ),
                ),
              ],
            ),
          ),
          Tab(
            child: Row(
              mainAxisAlignment: MainAxisAlignment.center,
              children: [
                const Icon(LucideIcons.settings2, size: 16),
                const SizedBox(width: 6),
                Flexible(
                  child: Text(
                    l10n.generalSettings,
                    textAlign: TextAlign.center,
                    softWrap: true,
                    maxLines: 2,
                    overflow: TextOverflow.ellipsis,
                  ),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildFooter(AppLocalizations l10n) {
    return Row(
      mainAxisAlignment: MainAxisAlignment.end,
      children: [
        TextButton(
          onPressed: _saving ? null : _handleCancel,
          child: Text(
            l10n.cancel,
            style: AppFonts.inter(
              fontSize: 14,
              color: _saving ? Colors.white24 : Colors.white54,
            ),
          ),
        ),
        const SizedBox(width: 12),
        if (_currentTabIndex == 0) ...[
          OutlinedButton(
            onPressed: _saving ? null : _handleValidateConnection,
            style: OutlinedButton.styleFrom(
              side: BorderSide(color: Colors.white.withValues(alpha: 0.2)),
              foregroundColor: _saving ? Colors.white24 : Colors.white70,
              padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
            ),
            child: Text(
              l10n.modelConfigValidateConnection,
              style: AppFonts.inter(
                fontSize: 14,
                fontWeight: FontWeight.w500,
              ),
            ),
          ),
          const SizedBox(width: 12),
        ],
        ElevatedButton(
          onPressed: _saving ? null : _handleSave,
          style: ElevatedButton.styleFrom(
            backgroundColor: const Color(0xFF6366F1),
            foregroundColor: Colors.white,
            padding: const EdgeInsets.symmetric(horizontal: 24, vertical: 12),
            shape: RoundedRectangleBorder(
              borderRadius: BorderRadius.circular(8),
            ),
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
                      style: AppFonts.inter(fontSize: 14),
                    ),
                  ],
                )
              : Text(
                  l10n.modelConfigSave,
                  style: AppFonts.inter(
                    fontSize: 14,
                    fontWeight: FontWeight.w600,
                  ),
                ),
        ),
      ],
    );
  }
}
