import 'package:flutter/material.dart';
import 'asset_model.dart';

// 风险等级
enum RiskLevel { low, medium, high, critical }

// 风险信息
class RiskInfo {
  final String id; // 用于国际化的 Key
  final Map<String, Object>? args; // 动态参数
  final String title; // 默认/日志标题
  final String description; // 默认/日志描述
  final RiskLevel level;
  final IconData icon;
  final Mitigation? mitigation;
  final String? sourcePlugin;

  RiskInfo({
    required this.id,
    this.args,
    required this.title,
    required this.description,
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
      'title': title,
      'description': description,
      'level': level.name, // 使用字符串名称与Go端兼容
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
      title: json['title'],
      description: json['description'],
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
}

class Mitigation {
  final String type;
  final List<FormItem> formSchema;
  // For suggestion type
  final String? title;
  final String? description;
  final List<SuggestionGroup>? suggestions;

  Mitigation({
    required this.type,
    required this.formSchema,
    this.title,
    this.description,
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
      description: json['description'],
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
      if (description != null) 'description': description,
      if (suggestions != null)
        'suggestions': suggestions!.map((e) => e.toJson()).toList(),
    };
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
  final String detail;
  final String? command;

  SuggestionItem({required this.action, required this.detail, this.command});

  factory SuggestionItem.fromJson(Map<String, dynamic> json) {
    return SuggestionItem(
      action: json['action'],
      detail: json['detail'],
      command: json['command'],
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'action': action,
      'detail': detail,
      if (command != null) 'command': command,
    };
  }
}

class FormItem {
  final String key;
  final String label;
  final String type;
  final dynamic defaultValue;
  final List<String>? options;
  // Validation
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

// 扫描结果
class ScanResult {
  final Map<String, dynamic>? config;
  final List<RiskInfo> risks;
  final bool configFound;
  final String? configPath;
  final List<Asset> assets;
  final DateTime? scannedAt;

  ScanResult({
    this.config,
    required this.risks,
    required this.configFound,
    this.configPath,
    this.assets = const [],
    this.scannedAt,
  });

  Map<String, dynamic> toJson() {
    return {
      'config': config,
      'risks': risks.map((r) => r.toJson()).toList(),
      'config_found': configFound,
      'config_path': configPath,
      'assets': assets.map((a) => a.toJson()).toList(),
      'scanned_at': scannedAt?.toIso8601String(),
    };
  }

  factory ScanResult.fromJson(Map<String, dynamic> json) {
    return ScanResult(
      config: json['config'],
      risks: (json['risks'] as List).map((r) => RiskInfo.fromJson(r)).toList(),
      configFound: json['config_found'],
      configPath: json['config_path'],
      assets: (json['assets'] as List).map((a) => Asset.fromJson(a)).toList(),
      scannedAt: json['scanned_at'] != null
          ? DateTime.parse(json['scanned_at'] as String)
          : null,
    );
  }
}

// 扫描状态
enum ScanState { idle, scanning, completed }
