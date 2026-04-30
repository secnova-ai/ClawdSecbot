class ShepherdRuleSet {
  final List<ShepherdSemanticRule> semanticRules;

  const ShepherdRuleSet({this.semanticRules = const []});

  factory ShepherdRuleSet.fromJson(Map<String, dynamic> json) {
    final rawRules = json['semantic_rules'];
    return ShepherdRuleSet(
      semanticRules: rawRules is List
          ? rawRules
                .whereType<Map>()
                .map((item) => ShepherdSemanticRule.fromJson(item))
                .toList(growable: false)
          : const [],
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'semantic_rules': semanticRules.map((rule) => rule.toJson()).toList(),
    };
  }
}

class ShepherdSemanticRule {
  final String id;
  final String scope;
  final bool enabled;
  final String description;
  final List<String> appliesTo;
  final String action;
  final String riskType;

  const ShepherdSemanticRule({
    required this.id,
    this.scope = '',
    this.enabled = true,
    required this.description,
    this.appliesTo = const [
      'user_input',
      'tool_call',
      'tool_call_result',
      'final_result',
    ],
    this.action = 'needs_confirmation',
    this.riskType = 'HIGH_RISK_OPERATION',
  });

  factory ShepherdSemanticRule.fromJson(Map<dynamic, dynamic> json) {
    final rawAppliesTo = json['applies_to'];
    return ShepherdSemanticRule(
      id: (json['id'] ?? '').toString(),
      scope: (json['scope'] ?? '').toString(),
      enabled: json['enabled'] != false,
      description: (json['description'] ?? '').toString(),
      appliesTo: rawAppliesTo is List
          ? rawAppliesTo
                .map((item) => item.toString().trim())
                .where((item) => item.isNotEmpty)
                .toList(growable: false)
          : const [
              'user_input',
              'tool_call',
              'tool_call_result',
              'final_result',
            ],
      action: (json['action'] ?? 'needs_confirmation').toString(),
      riskType: (json['risk_type'] ?? 'HIGH_RISK_OPERATION').toString(),
    );
  }

  bool get isCustom {
    final normalizedScope = scope.trim().toLowerCase();
    final normalizedID = id.trim().toLowerCase();
    return normalizedScope == 'custom' ||
        normalizedID.startsWith('custom_') ||
        normalizedID.startsWith('user_rule_');
  }

  ShepherdSemanticRule copyWith({
    String? id,
    String? scope,
    bool? enabled,
    String? description,
    List<String>? appliesTo,
    String? action,
    String? riskType,
  }) {
    return ShepherdSemanticRule(
      id: id ?? this.id,
      scope: scope ?? this.scope,
      enabled: enabled ?? this.enabled,
      description: description ?? this.description,
      appliesTo: appliesTo ?? this.appliesTo,
      action: action ?? this.action,
      riskType: riskType ?? this.riskType,
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'id': id,
      if (scope.trim().isNotEmpty) 'scope': scope,
      'enabled': enabled,
      'description': description,
      'applies_to': appliesTo,
      'action': action,
      'risk_type': riskType,
    };
  }
}
