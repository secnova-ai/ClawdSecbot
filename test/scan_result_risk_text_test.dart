import 'package:bot_sec_manager/l10n/app_localizations_en.dart';
import 'package:bot_sec_manager/l10n/app_localizations_zh.dart';
import 'package:bot_sec_manager/models/risk_model.dart';
import 'package:bot_sec_manager/widgets/scan_result_risk_text.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('localizes OpenClaw config risks from risk IDs', () {
    final risk = RiskInfo(
      id: 'openclaw_insecure_or_dangerous_flags',
      title: 'Insecure or Dangerous Gateway Flags Enabled',
      description: 'raw backend description',
      level: RiskLevel.high,
      icon: Icons.warning,
      args: {
        'flags': ['gateway.allowRealIpFallback=true'],
      },
    );

    final zh = AppLocalizationsZh();
    expect(localizeScanRiskTitle(risk, zh), 'OpenClaw 网关危险开关已启用');
    expect(
      localizeScanRiskDescription(risk, zh),
      contains('gateway.allowRealIpFallback=true'),
    );
  });

  test('uses locale-aware fallback for generic vulnerability risks', () {
    final risk = RiskInfo(
      id: 'openclaw_cve-2026-32913',
      title: 'OpenClaw 安全漏洞(CVE-2026-32913)',
      titleEn: 'OpenClaw security vulnerability (CVE-2026-32913)',
      description: '中文漏洞描述',
      descriptionEn: 'English vulnerability description',
      level: RiskLevel.critical,
      icon: Icons.warning,
    );

    expect(
      localizeScanRiskTitle(risk, AppLocalizationsEn()),
      'OpenClaw security vulnerability (CVE-2026-32913)',
    );
    expect(
      localizeScanRiskDescription(risk, AppLocalizationsEn()),
      'English vulnerability description',
    );
    expect(
      localizeScanRiskTitle(risk, AppLocalizationsZh()),
      'OpenClaw 安全漏洞(CVE-2026-32913)',
    );
  });

  test('uses locale-aware mitigation suggestion text', () {
    final group = SuggestionGroup.fromJson({
      'priority': 'P0',
      'category': '立即处理',
      'category_en': 'Immediate actions',
      'items': [
        {
          'action': '关闭危险开关',
          'action_en': 'Disable dangerous flags',
          'detail': '关闭会削弱认证的配置项。',
          'detail_en': 'Disable settings that weaken authentication.',
        },
      ],
    });

    expect(group.displayCategory('zh'), '立即处理');
    expect(group.displayCategory('en'), 'Immediate actions');
    expect(group.items.single.displayAction('zh'), '关闭危险开关');
    expect(group.items.single.displayAction('en'), 'Disable dangerous flags');
    expect(group.items.single.displayDetail('zh'), '关闭会削弱认证的配置项。');
    expect(
      group.items.single.displayDetail('en'),
      'Disable settings that weaken authentication.',
    );
  });
}
