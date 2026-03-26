import 'dart:async';
import 'dart:io';

import 'package:lucide_icons/lucide_icons.dart';

import '../config/build_config.dart';
import '../l10n/app_localizations.dart';
import '../models/risk_model.dart';
import 'plugin_service.dart';
import 'scan_database_service.dart';

class BotScanner {
  final StreamController<String> _logController =
      StreamController<String>.broadcast();
  final PluginService _pluginService = PluginService();

  AppLocalizations? _l10n;

  Stream<String> get logStream => _logController.stream;

  void _log(String message) {
    _logController.add(message);
  }

  /// Sets localization for scan-time risk copy.
  void setLocalization(AppLocalizations l10n) {
    _l10n = l10n;
  }

  Future<ScanResult> scan() async {
    _log('Starting security scan using plugins...');
    await Future.delayed(const Duration(milliseconds: 300));

    _log('Loading plugins and scanning...');
    final pluginResult = await _pluginService.scan();

    _log('Found ${pluginResult.assets.length} assets.');
    for (final asset in pluginResult.assets) {
      _log('Identified asset: ${asset.name} (${asset.type})');
    }

    if (pluginResult.configFound) {
      _log('Configuration found.');
    } else {
      _log('No configuration file found.');
    }

    _log('Risk assessment completed via plugins.');
    _log('Found ${pluginResult.risks.length} potential issues.');

    final baseRisks = List<RiskInfo>.from(pluginResult.riskInfo);
    _checkSystemRisks(baseRisks);

    final skillRisks = await _loadRiskySkills();

    return ScanResult(
      config: pluginResult.config,
      riskInfo: baseRisks,
      skillResult: skillRisks,
      configFound: pluginResult.configFound,
      configPath: pluginResult.configPath,
      assets: pluginResult.assets,
      scannedAt: DateTime.now(),
    );
  }

  void _checkSystemRisks(List<RiskInfo> risks) {
    if (BuildConfig.isAppStore) {
      _log('Skipping system risk checks (App Store build)');
      return;
    }

    if (Platform.isLinux || Platform.isMacOS) {
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
      } catch (_) {
        // Ignore environment read failures here.
      }
    }
  }

  Future<List<RiskInfo>> _loadRiskySkills() async {
    final risks = <RiskInfo>[];

    try {
      final riskySkills = await ScanDatabaseService().getRiskySkills();
      for (final skill in riskySkills) {
        final skillName = skill['skill_name'] as String;
        final issues = skill['issues'] as List<String>;
        final issueCount = issues.length;
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
            args: {
              'skillName': skillName,
              'issueCount': issueCount,
              'issues': issues.join('; '),
              if ((skill['skill_path'] as String? ?? '').isNotEmpty)
                'skillPath': skill['skill_path'] as String,
            },
            title: title,
            description: description,
            level: _parseSkillRiskLevel(skill['risk_level'] as String?),
            icon: LucideIcons.alertTriangle,
          ),
        );
        _log('Found risky skill: $skillName ($issueCount issues)');
      }
    } catch (e) {
      _log('Failed to check risky skills: $e');
    }

    return risks;
  }

  RiskLevel _parseSkillRiskLevel(String? level) {
    switch (level?.toLowerCase()) {
      case 'critical':
        return RiskLevel.critical;
      case 'high':
        return RiskLevel.high;
      case 'medium':
        return RiskLevel.medium;
      case 'low':
        return RiskLevel.low;
      default:
        return RiskLevel.high;
    }
  }

  void dispose() {
    _logController.close();
  }
}
