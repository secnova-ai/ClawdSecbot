import 'dart:async';
import 'dart:io';

import 'package:lucide_icons/lucide_icons.dart';

import '../config/build_config.dart';
import '../l10n/app_localizations.dart';
import '../models/asset_model.dart';
import '../models/risk_model.dart';
import 'plugin_service.dart';
import 'scan_database_service.dart';

/// Three security-discovery stages used by the rescan progress UI.
enum SecurityDiscoveryStage { baseline, vulnerability, skillPoison }

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

  Future<ScanResult> scan({
    void Function(String assetName, bool detected)? onPluginScanned,
  }) async {
    _log('Starting security scan using plugins...');
    await Future.delayed(const Duration(milliseconds: 300));

    final pluginResult = await _runPluginPipeline(
      onPluginScanned: onPluginScanned,
    );

    final baseRisks = List<RiskInfo>.from(pluginResult.riskInfo);
    _checkSystemRisks(baseRisks);
    final skillRisks = await _loadRiskySkills(pluginResult.risks);

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

  /// Re-runs the "Security Discovery" flow and reports real-time progress.
  /// [onAssetsUpdated] streams the actually discovered assets during rescanning.
  /// [onStageProgress] reports stage start and completion per asset.
  Future<ScanResult> rediscoverSecurityFindings({
    required bool configFound,
    String? configPath,
    Map<String, dynamic>? config,
    void Function(List<Asset> assets)? onAssetsUpdated,
    void Function(String assetID, SecurityDiscoveryStage stage, bool completed)?
    onStageProgress,
  }) async {
    _log('Refreshing security findings...');
    await Future.delayed(const Duration(milliseconds: 150));

    final pluginOrder = [
      'openclaw',
      'dintalclaw',
      'nullclaw',
      'qclaw',
      'hermes',
    ];
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

    final stageAssets = <Asset>[];
    final stageAssetIndexByID = <String, int>{};
    final assetsByPlugin = <String, List<Asset>>{};
    final allRisks = <RiskInfo>[];
    final scannedHashes = await _pluginService.getScannedSkillHashesList();

    void emitAssets(List<Asset> assets) {
      if (onAssetsUpdated == null) {
        return;
      }
      onAssetsUpdated(List<Asset>.unmodifiable(assets));
    }

    void upsertStageAssets(Iterable<Asset> assets) {
      var changed = false;
      for (final asset in assets) {
        final existingIndex = stageAssetIndexByID[asset.id];
        if (existingIndex == null) {
          stageAssetIndexByID[asset.id] = stageAssets.length;
          stageAssets.add(asset);
          changed = true;
          continue;
        }
        if (stageAssets[existingIndex] != asset) {
          stageAssets[existingIndex] = asset;
          changed = true;
        }
      }
      if (changed) {
        emitAssets(stageAssets);
      }
    }

    void markStage(
      Iterable<Asset> assets,
      SecurityDiscoveryStage stage,
      bool completed,
    ) {
      for (final asset in assets) {
        onStageProgress?.call(asset.id, stage, completed);
      }
    }

    for (final assetName in activePlugins) {
      final assets = await _pluginService.scanAssetsByPlugin(assetName);
      assetsByPlugin[assetName] = assets;
      _log('Plugin $assetName found ${assets.length} assets.');
      if (assets.isEmpty) {
        continue;
      }

      upsertStageAssets(assets);
      markStage(assets, SecurityDiscoveryStage.baseline, false);
      markStage(assets, SecurityDiscoveryStage.baseline, true);
      markStage(assets, SecurityDiscoveryStage.vulnerability, false);

      final risks = await _pluginService.assessRisksByPlugin(
        assetName,
        scannedHashes,
      );
      allRisks.addAll(risks);
      markStage(assets, SecurityDiscoveryStage.vulnerability, true);
      markStage(assets, SecurityDiscoveryStage.skillPoison, false);
    }

    final baseRisks = List<RiskInfo>.from(allRisks);
    _log('Found ${baseRisks.length} potential issues.');
    _checkSystemRisks(baseRisks);

    final skillRisks = await _loadRiskySkills(allRisks);
    markStage(stageAssets, SecurityDiscoveryStage.skillPoison, true);

    final discoveredAssets = <Asset>[
      for (final plugin in activePlugins) ...?assetsByPlugin[plugin],
    ];

    return ScanResult(
      config: config,
      riskInfo: baseRisks,
      skillResult: skillRisks,
      configFound: discoveredAssets.isNotEmpty || configFound,
      configPath: _resolveConfigPath(discoveredAssets) ?? configPath,
      assets: discoveredAssets,
      scannedAt: DateTime.now(),
    );
  }

  String? _resolveConfigPath(List<Asset> assets) {
    final configPath = assets
        .map((asset) => asset.metadata['config_path'] ?? '')
        .firstWhere(
          (path) => path.toString().trim().isNotEmpty,
          orElse: () => '',
        );
    return configPath.toString().isEmpty ? null : configPath.toString();
  }

  /// Scans assets plugin by plugin and only assesses risks for detected assets.
  Future<ScanResult> _runPluginPipeline({
    void Function(String assetName, bool detected)? onPluginScanned,
  }) async {
    final pluginOrder = [
      'openclaw',
      'dintalclaw',
      'nullclaw',
      'qclaw',
      'hermes',
    ];
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
      _log('Plugin $assetName found ${assets.length} assets.');
      onPluginScanned?.call(assetName, assets.isNotEmpty);
      if (assets.isEmpty) {
        _log('Plugin $assetName has no asset detected, skip risk assessment.');
        continue;
      }
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

    return ScanResult(
      config: null,
      risks: allRisks,
      configFound: allAssets.isNotEmpty || configPath.toString().isNotEmpty,
      configPath: configPath.toString().isEmpty ? null : configPath.toString(),
      assets: allAssets,
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

  Future<List<RiskInfo>> _loadRiskySkills(List<RiskInfo> pluginRisks) async {
    final risks = <RiskInfo>[];
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
      final mergedRiskySkills = _mergeRiskySkillsByPath(riskySkills);
      for (final skill in mergedRiskySkills) {
        final skillName = skill['skill_name'] as String;
        if (unscannedSkillNames.contains(skillName.trim().toLowerCase())) {
          _log('Skip risky skill card for unscanned skill: $skillName');
          continue;
        }
        final issues = skill['issues'] as List<String>;
        final persistedIssueCount =
            (skill['issue_count'] as num?)?.toInt() ?? 0;
        final issueCount = persistedIssueCount > issues.length
            ? persistedIssueCount
            : issues.length;
        final normalizedIssueCount = issueCount > 0 ? issueCount : 1;
        final title = _l10n != null
            ? _l10n!.riskSkillSecurityIssue(skillName)
            : 'Risky Skill: $skillName';
        final description = _l10n != null
            ? (normalizedIssueCount > 0
                  ? _l10n!.riskSkillSecurityIssueDesc(
                      skillName,
                      normalizedIssueCount,
                    )
                  : (_l10n!.localeName.startsWith('zh')
                        ? '技能"$skillName"存在安全风险。建议删除此技能。'
                        : 'Skill "$skillName" is risky. Consider deleting it.'))
            : (normalizedIssueCount > 0
                  ? 'Skill "$skillName" has $normalizedIssueCount security issue(s): ${issues.join("; ")}'
                  : 'Skill "$skillName" was flagged as potentially risky.');

        risks.add(
          RiskInfo(
            id: 'riskSkillSecurityIssue',
            args: {
              'skillName': skillName,
              'skillHash': skill['skill_hash'] as String? ?? '',
              'asset_name': skill['source_plugin'] as String? ?? '',
              'asset_id': skill['asset_id'] as String? ?? '',
              'issueCount': normalizedIssueCount,
              'issues': issues.join('; '),
              if ((skill['skill_path'] as String? ?? '').isNotEmpty)
                'skillPath': skill['skill_path'] as String,
            },
            title: title,
            description: description,
            level: _parseSkillRiskLevel(skill['risk_level'] as String?),
            icon: LucideIcons.alertTriangle,
            sourcePlugin: skill['source_plugin'] as String? ?? '',
            assetID: skill['asset_id'] as String? ?? '',
          ),
        );
        _log('Found risky skill: $skillName ($normalizedIssueCount issues)');
      }
    } catch (e) {
      _log('Failed to check risky skills: $e');
    }

    return risks;
  }

  /// Deduplicates risky-skill records by skill path for stable UI rendering.
  List<Map<String, dynamic>> _mergeRiskySkillsByPath(
    List<Map<String, dynamic>> riskySkills,
  ) {
    final merged = <String, Map<String, dynamic>>{};
    for (final skill in riskySkills) {
      final skillName = (skill['skill_name'] as String? ?? '').trim();
      final skillHash = (skill['skill_hash'] as String? ?? '').trim();
      final skillPath = (skill['skill_path'] as String? ?? '').trim();
      final sourcePlugin = (skill['source_plugin'] as String? ?? '').trim();
      final assetID = (skill['asset_id'] as String? ?? '').trim();
      final key = skillPath.isNotEmpty
          ? 'path:${skillPath.toLowerCase()}|${sourcePlugin.toLowerCase()}|${assetID.toLowerCase()}'
          : 'fallback:${skillName.toLowerCase()}|${skillHash.toLowerCase()}|${sourcePlugin.toLowerCase()}|${assetID.toLowerCase()}';
      final issues = (skill['issues'] as List<String>? ?? const <String>[])
          .where((issue) => issue.trim().isNotEmpty)
          .toList();
      final rawIssueCount = (skill['issue_count'] as num?)?.toInt() ?? 0;
      final normalizedIssueCount = rawIssueCount > 0
          ? rawIssueCount
          : issues.length;

      final existed = merged[key];
      if (existed == null) {
        final newSkill = Map<String, dynamic>.from(skill);
        newSkill['issues'] = issues.toSet().toList();
        newSkill['issue_count'] = normalizedIssueCount;
        merged[key] = newSkill;
        continue;
      }

      final existedIssues =
          (existed['issues'] as List<String>? ?? const <String>[]).toSet();
      existedIssues.addAll(issues);
      existed['issues'] = existedIssues.toList();
      final mergedIssueCount = (existed['issue_count'] as num?)?.toInt() ?? 0;
      final dedupIssueCount = existedIssues.length;
      final nextIssueCount = [
        mergedIssueCount,
        normalizedIssueCount,
        dedupIssueCount,
      ].reduce((a, b) => a > b ? a : b);
      existed['issue_count'] = nextIssueCount;
    }
    return merged.values.toList();
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
