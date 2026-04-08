import 'dart:async';
import 'dart:io';
import 'package:lucide_icons/lucide_icons.dart';
import '../l10n/app_localizations.dart';
import '../models/asset_model.dart';
import '../models/risk_model.dart';
import '../config/build_config.dart';
import 'plugin_service.dart';
import 'scan_database_service.dart';

class BotScanner {
  final StreamController<String> _logController =
      StreamController<String>.broadcast();
  Stream<String> get logStream => _logController.stream;
  final PluginService _pluginService = PluginService();
  AppLocalizations? _l10n;

  void _log(String message) {
    _logController.add(message);
  }

  /// 设置本地化对象，用于生成中文等本地化文本
  void setLocalization(AppLocalizations l10n) {
    _l10n = l10n;
  }

  Future<ScanResult> scan({
    void Function(String assetName, bool detected)? onPluginScanned,
  }) async {
    final pluginOrder = ['openclaw', 'dintalclaw', 'nullclaw', 'qclaw'];
    final registeredPlugins = await _pluginService.getRegisteredPlugins();
    final registeredNames = registeredPlugins
        .map(
          (item) => (item['asset_name'] as String? ?? '').trim().toLowerCase(),
        )
        .where((name) => name.isNotEmpty)
        .toSet();
    final activePlugins = pluginOrder
        .where(registeredNames.contains)
        .toList(growable: false);

    final scannedHashes = await _pluginService.getScannedSkillHashesList();
    final allAssets = <Asset>[];
    final allRisks = <RiskInfo>[];

    for (final assetName in activePlugins) {
      final assets = await _pluginService.scanAssetsByPlugin(assetName);
      allAssets.addAll(assets);
      onPluginScanned?.call(assetName, assets.isNotEmpty);
      final risks = await _pluginService.assessRisksByPlugin(
        assetName,
        scannedHashes,
      );
      allRisks.addAll(risks);
    }

    final configPath = allAssets
        .map((asset) => asset.metadata['config_path'] ?? '')
        .firstWhere(
          (path) => path.toString().trim().isNotEmpty,
          orElse: () => '',
        );

    final pluginResult = ScanResult(
      config: null,
      risks: allRisks,
      configFound: allAssets.isNotEmpty || configPath.toString().isNotEmpty,
      configPath: configPath.toString().isEmpty ? null : configPath.toString(),
      assets: allAssets,
    );

    // 2. Add System Level Checks (e.g. Scanner running as root)
    List<RiskInfo> finalRisks = List.from(pluginResult.risks);
    _checkSystemRisks(finalRisks);

    // 3. Add Risky Skills from database
    await _addRiskySkills(finalRisks, pluginResult.risks);

    return ScanResult(
      config: pluginResult.config,
      risks: finalRisks,
      configFound: pluginResult.configFound,
      configPath: pluginResult.configPath,
      assets: pluginResult.assets,
      scannedAt: DateTime.now(),
    );
  }

  void _checkSystemRisks(List<RiskInfo> risks) {
    // AppStore版本在沙盒中运行,无法执行系统命令和读取环境变量,跳过系统级检查
    if (BuildConfig.isAppStore) {
      _log('Skipping system risk checks (App Store build)');
      return;
    }

    // 检查运行权限
    if (Platform.isLinux || Platform.isMacOS) {
      // This checks if the *scanner itself* is running as root,
      // which might be relevant if it needs to check other users' files.
      // But usually running as root is a risk for the tool itself.
      // We'll keep it as a general warning.
      try {
        final env = Platform.environment;
        final uidStr = env['UID'] ?? env['EUID'];
        if (uidStr != null) {
          final uid = int.tryParse(uidStr);
          if (uid == 0) {
            risks.add(
              RiskInfo(
                id: 'riskRunningAsRoot',
                title: 'Running as root',
                description:
                    'Application is running with root privileges. This increases the attack surface.',
                level: RiskLevel.high,
                icon: LucideIcons.userCog,
              ),
            );
          }
        }
      } catch (e) {
        // Ignore
      }
    }
  }

  Future<void> _addRiskySkills(
    List<RiskInfo> risks,
    List<RiskInfo> pluginRisks,
  ) async {
    try {
      final unscannedSkillNames = <String>{};
      for (final risk in pluginRisks.where(
        (r) => r.id == 'skills_not_scanned',
      )) {
        final names = risk.args?['skill_names'];
        if (names is List) {
          for (final name in names) {
            final normalized = name?.toString().trim();
            if (normalized != null && normalized.isNotEmpty) {
              unscannedSkillNames.add(normalized.toLowerCase());
            }
          }
        }
      }

      final riskySkills = await ScanDatabaseService().getRiskySkills();
      for (var skill in riskySkills) {
        final skillName = skill['skill_name'] as String;
        if (unscannedSkillNames.contains(skillName.trim().toLowerCase())) {
          _log('Skip risky skill card for unscanned skill: $skillName');
          continue;
        }
        final issues = skill['issues'] as List<String>;
        final issueCount = issues.length;

        // 使用本地化文本（若有）
        final title = _l10n != null
            ? _l10n!.riskSkillSecurityIssue(skillName)
            : 'Risky Skill: $skillName';
        final description = _l10n != null
            ? (issueCount > 0
                  ? _l10n!.riskSkillSecurityIssueDesc(skillName, issueCount)
                  : (_l10n!.localeName.startsWith('zh')
                        ? '技能 "$skillName" 存在安全风险。建议删除此技能。'
                        : 'Skill "$skillName" is risky. Consider deleting it.'))
            : (issueCount > 0
                  ? 'Skill "$skillName" has $issueCount security issue(s): ${issues.join("; ")}'
                  : 'Skill "$skillName" was flagged as potentially risky.');

        risks.add(
          RiskInfo(
            id: 'riskSkillSecurityIssue',
            args: {'skillName': skillName, 'issueCount': issueCount},
            title: title,
            description: description,
            level: RiskLevel.high,
            icon: LucideIcons.alertTriangle,
          ),
        );
        _log('Found risky skill: $skillName ($issueCount issues)');
      }
    } catch (e) {
      _log('Failed to check risky skills: $e');
    }
  }

  void dispose() {
    _logController.close();
  }
}
