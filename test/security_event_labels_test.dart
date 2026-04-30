import 'package:bot_sec_manager/l10n/app_localizations_en.dart';
import 'package:bot_sec_manager/l10n/app_localizations_zh.dart';
import 'package:bot_sec_manager/utils/security_event_labels.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('localizes stable security event action descriptions', () {
    expect(
      localizeSecurityActionDesc(
        'Historical blocked tool result rewritten',
        AppLocalizationsZh(),
      ),
      '历史已拦截工具结果已重写',
    );
    expect(
      localizeSecurityActionDesc(
        'Historical blocked tool result rewritten',
        AppLocalizationsEn(),
      ),
      'Historical blocked tool result rewritten',
    );
  });

  test('localizes rewritten and redacted event types', () {
    expect(localizeSecurityEventType('rewritten', AppLocalizationsZh()), '已重写');
    expect(localizeSecurityEventType('redacted', AppLocalizationsZh()), '已脱敏');
    expect(
      localizeSecurityEventType('rewritten', AppLocalizationsEn()),
      'Rewritten',
    );
    expect(
      localizeSecurityEventType('redacted', AppLocalizationsEn()),
      'Redacted',
    );
  });
}
