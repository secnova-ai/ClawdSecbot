import 'package:flutter/material.dart';
import 'asset_model.dart';

enum RiskLevel { low, medium, high, critical }

class RiskInfo {
  final String id;
  final Map<String, Object>? args;
  final String? assetID;
  final String title;
  final String? titleEn;
  final String description;
  final String? descriptionEn;
  final RiskLevel level;
  final IconData icon;
  final Mitigation? mitigation;
  final String? sourcePlugin;

  RiskInfo({
    required this.id,
    this.args,
    this.assetID,
    required this.title,
    this.titleEn,
    required this.description,
    this.descriptionEn,
    required this.level,
    required this.icon,
    this.mitigation,
    this.sourcePlugin,
  });

  Color get color {
    switch (level) {
      case RiskLevel.low:
        return const Color(0xFF22C55E);
      case RiskLevel.medium:
        return const Color(0xFFF59E0B);
      case RiskLevel.high:
        return const Color(0xFFEF4444);
      case RiskLevel.critical:
        return const Color(0xFFDC2626);
    }
  }

  Map<String, dynamic> toJson() {
    return {
      'id': id,
      'args': args,
      'asset_id': assetID,
      'title': title,
      'title_en': titleEn,
      'description': description,
      'description_en': descriptionEn,
      'level': level.name,
      'icon_code_point': icon.codePoint,
      'icon_font_family': icon.fontFamily,
      'icon_font_package': icon.fontPackage,
      'mitigation': mitigation?.toJson(),
      'source_plugin': sourcePlugin,
    };
  }

  factory RiskInfo.fromJson(Map<String, dynamic> json) {
    RiskLevel parseLevel(dynamic raw) {
      if (raw is int) {
        return RiskLevel.values[raw];
      }
      if (raw is String) {
        switch (raw.toLowerCase()) {
          case 'critical':
            return RiskLevel.critical;
          case 'high':
            return RiskLevel.high;
          case 'medium':
            return RiskLevel.medium;
          case 'low':
            return RiskLevel.low;
        }
      }
      return RiskLevel.low;
    }

    return RiskInfo(
      id: json['id'],
      args: (json['args'] as Map?)?.cast<String, Object>(),
      assetID: _parseAssetID(
        json['asset_id'],
        (json['args'] as Map?)?.cast<String, Object>(),
      ),
      title: json['title'] ?? '',
      titleEn: json['title_en'],
      description: json['description'] ?? '',
      descriptionEn: json['description_en'],
      level: parseLevel(json['level']),
      icon: IconData(
        json['icon_code_point'] as int? ?? Icons.warning.codePoint,
        fontFamily: json['icon_font_family'] as String?,
        fontPackage: json['icon_font_package'] as String?,
      ),
      mitigation: json['mitigation'] != null
          ? Mitigation.fromJson(json['mitigation'])
          : null,
      sourcePlugin: json['source_plugin'],
    );
  }

  static String? _parseAssetID(dynamic rawAssetID, Map<String, Object>? args) {
    final direct = rawAssetID?.toString().trim();
    if (direct != null && direct.isNotEmpty) {
      return direct;
    }
    final fromArgs = args?['asset_id']?.toString().trim();
    if (fromArgs == null || fromArgs.isEmpty) {
      return null;
    }
    return fromArgs;
  }

  String displayTitle(String localeName) {
    final isEnglish = localeName.toLowerCase().startsWith('en');
    if (isEnglish && (titleEn?.trim().isNotEmpty ?? false)) {
      return titleEn!.trim();
    }
    return title;
  }

  String displayDescription(String localeName) {
    final isEnglish = localeName.toLowerCase().startsWith('en');
    if (isEnglish && (descriptionEn?.trim().isNotEmpty ?? false)) {
      return descriptionEn!.trim();
    }
    return description;
  }
}

class Mitigation {
  final String type;
  final List<FormItem> formSchema;
  final String? title;
  final String? titleEn;
  final String? description;
  final String? descriptionEn;
  final List<SuggestionGroup>? suggestions;

  Mitigation({
    required this.type,
    required this.formSchema,
    this.title,
    this.titleEn,
    this.description,
    this.descriptionEn,
    this.suggestions,
  });

  factory Mitigation.fromJson(Map<String, dynamic> json) {
    return Mitigation(
      type: json['type'],
      formSchema:
          (json['form_schema'] as List?)
              ?.map((e) => FormItem.fromJson(e))
              .toList() ??
          [],
      title: json['title'],
      titleEn: json['title_en'],
      description: json['description'],
      descriptionEn: json['description_en'],
      suggestions: (json['suggestions'] as List?)
          ?.map((e) => SuggestionGroup.fromJson(e))
          .toList(),
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'type': type,
      'form_schema': formSchema.map((e) => e.toJson()).toList(),
      if (title != null) 'title': title,
      if (titleEn != null) 'title_en': titleEn,
      if (description != null) 'description': description,
      if (descriptionEn != null) 'description_en': descriptionEn,
      if (suggestions != null)
        'suggestions': suggestions!.map((e) => e.toJson()).toList(),
    };
  }

  String? displayTitle(String localeName) {
    final isEnglish = localeName.toLowerCase().startsWith('en');
    if (isEnglish && (titleEn?.trim().isNotEmpty ?? false)) {
      return titleEn!.trim();
    }
    return title;
  }

  String? displayDescription(String localeName) {
    final isEnglish = localeName.toLowerCase().startsWith('en');
    if (isEnglish && (descriptionEn?.trim().isNotEmpty ?? false)) {
      return descriptionEn!.trim();
    }
    return description;
  }
}

class SuggestionGroup {
  final String priority;
  final String category;
  final List<SuggestionItem> items;

  SuggestionGroup({
    required this.priority,
    required this.category,
    required this.items,
  });

  factory SuggestionGroup.fromJson(Map<String, dynamic> json) {
    return SuggestionGroup(
      priority: json['priority'],
      category: json['category'],
      items: (json['items'] as List)
          .map((e) => SuggestionItem.fromJson(e))
          .toList(),
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'priority': priority,
      'category': category,
      'items': items.map((e) => e.toJson()).toList(),
    };
  }
}

class SuggestionItem {
  final String action;
  final String? actionEn;
  final String detail;
  final String? detailEn;
  final String? command;

  SuggestionItem({
    required this.action,
    this.actionEn,
    required this.detail,
    this.detailEn,
    this.command,
  });

  factory SuggestionItem.fromJson(Map<String, dynamic> json) {
    return SuggestionItem(
      action: json['action'],
      actionEn: json['action_en'],
      detail: json['detail'],
      detailEn: json['detail_en'],
      command: json['command'],
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'action': action,
      if (actionEn != null) 'action_en': actionEn,
      'detail': detail,
      if (detailEn != null) 'detail_en': detailEn,
      if (command != null) 'command': command,
    };
  }

  String displayAction(String localeName) {
    final isEnglish = localeName.toLowerCase().startsWith('en');
    if (isEnglish && (actionEn?.trim().isNotEmpty ?? false)) {
      return actionEn!.trim();
    }
    return action;
  }

  String displayDetail(String localeName) {
    final isEnglish = localeName.toLowerCase().startsWith('en');
    if (isEnglish && (detailEn?.trim().isNotEmpty ?? false)) {
      return detailEn!.trim();
    }
    return detail;
  }
}

class FormItem {
  final String key;
  final String label;
  final String type;
  final dynamic defaultValue;
  final List<String>? options;
  final bool required;
  final int minLength;
  final String? regex;
  final String? regexMsg;

  FormItem({
    required this.key,
    required this.label,
    required this.type,
    this.defaultValue,
    this.options,
    this.required = false,
    this.minLength = 0,
    this.regex,
    this.regexMsg,
  });

  factory FormItem.fromJson(Map<String, dynamic> json) {
    return FormItem(
      key: json['key'],
      label: json['label'],
      type: json['type'],
      defaultValue: json['default_value'],
      options: (json['options'] as List?)?.cast<String>(),
      required: json['required'] ?? false,
      minLength: json['min_length'] ?? 0,
      regex: json['regex'],
      regexMsg: json['regex_msg'],
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'key': key,
      'label': label,
      'type': type,
      'default_value': defaultValue,
      'options': options,
      'required': required,
      'min_length': minLength,
      'regex': regex,
      'regex_msg': regexMsg,
    };
  }
}

class ScanResult {
  final Map<String, dynamic>? config;
  final List<RiskInfo> riskInfo;
  final List<RiskInfo> skillResult;
  final bool configFound;
  final String? configPath;
  final List<Asset> assets;
  final DateTime? scannedAt;

  List<RiskInfo> get risks => [...riskInfo, ...skillResult];

  ScanResult({
    this.config,
    List<RiskInfo>? risks,
    List<RiskInfo>? riskInfo,
    List<RiskInfo>? skillResult,
    required this.configFound,
    this.configPath,
    this.assets = const [],
    this.scannedAt,
  }) : riskInfo = List<RiskInfo>.unmodifiable(riskInfo ?? risks ?? const []),
       skillResult = List<RiskInfo>.unmodifiable(skillResult ?? const []);

  Map<String, dynamic> toJson() {
    return {
      'config': config,
      'risk_info': riskInfo.map((r) => r.toJson()).toList(),
      'skill_result': skillResult.map((r) => r.toJson()).toList(),
      'risks': risks.map((r) => r.toJson()).toList(),
      'config_found': configFound,
      'config_path': configPath,
      'assets': assets.map((a) => a.toJson()).toList(),
      'scanned_at': scannedAt?.toIso8601String(),
    };
  }

  factory ScanResult.fromJson(Map<String, dynamic> json) {
    final legacyRisks = (json['risks'] as List? ?? const [])
        .map((r) => RiskInfo.fromJson(r))
        .toList();

    return ScanResult(
      config: json['config'],
      riskInfo:
          (json['risk_info'] as List?)
              ?.map((r) => RiskInfo.fromJson(r))
              .toList() ??
          legacyRisks,
      skillResult:
          (json['skill_result'] as List?)
              ?.map((r) => RiskInfo.fromJson(r))
              .toList() ??
          const <RiskInfo>[],
      configFound: json['config_found'],
      configPath: json['config_path'],
      assets: (json['assets'] as List).map((a) => Asset.fromJson(a)).toList(),
      scannedAt: json['scanned_at'] != null
          ? DateTime.parse(json['scanned_at'] as String)
          : null,
    );
  }
}

enum ScanState { idle, scanning, completed }
