import 'dart:async';
import 'dart:io';
import 'package:lucide_icons/lucide_icons.dart';
import '../l10n/app_localizations.dart';
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

  Future<ScanResult> scan() async {
    _log('Starting security scan using plugins...');
    await Future.delayed(const Duration(milliseconds: 300));

    // 1. Load Plugins and Scan (Identify + AssessRisks)
    _log('Loading plugins and scanning...');

    ScanResult pluginResult = await _pluginService.scan();

    _log('Found ${pluginResult.assets.length} assets.');
    for (var asset in pluginResult.assets) {
      _log('Identified asset: ${asset.name} (${asset.type})');
    }

    if (pluginResult.configFound) {
      _log('Configuration found.');
    } else {
      _log('No configuration file found.');
    }

    _log('Risk assessment completed via plugins.');
    _log('Found ${pluginResult.risks.length} potential issues.');

    // 2. Add System Level Checks (e.g. Scanner running as root)
    List<RiskInfo> finalRisks = List.from(pluginResult.risks);
    _checkSystemRisks(finalRisks);

    // 3. Add Risky Skills from database
    await _addRiskySkills(finalRisks);

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

  Future<void> _addRiskySkills(List<RiskInfo> risks) async {
    try {
      final riskySkills = await ScanDatabaseService().getRiskySkills();
      for (var skill in riskySkills) {
        final skillName = skill['skill_name'] as String;
        final issues = skill['issues'] as List<String>;
        final issueCount = issues.length;

        // 使用本地化文本（若有）
        final title = _l10n != null
            ? _l10n!.riskSkillSecurityIssue(skillName)
            : 'Risky Skill: $skillName';
        final description = _l10n != null
            ? _l10n!.riskSkillSecurityIssueDesc(skillName, issueCount)
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
