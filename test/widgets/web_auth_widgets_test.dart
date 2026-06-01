import 'dart:async';

import 'package:bot_sec_manager/web/web_auth_widgets.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

// main 验证 Web 认证组件的关键交互。
void main() {
  testWidgets('InitialPasswordDialog 点击复制后等待复制完成再关闭', (tester) async {
    final copyCompleter = Completer<bool>();
    String? copiedText;

    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: Builder(
            builder: (context) {
              return TextButton(
                onPressed: () {
                  showDialog<void>(
                    context: context,
                    builder: (_) => InitialPasswordDialog(
                      isZh: true,
                      username: 'sysadmin',
                      password: 'secret-pass',
                      copyText: (text) {
                        copiedText = text;
                        return copyCompleter.future;
                      },
                    ),
                  );
                },
                child: const Text('open'),
              );
            },
          ),
        ),
      ),
    );

    await tester.tap(find.text('open'));
    await tester.pumpAndSettle();

    await tester.tap(find.text('复制并继续'));
    await tester.pump();

    expect(copiedText, 'username=sysadmin\npassword=secret-pass');
    expect(find.byType(AlertDialog), findsOneWidget);

    copyCompleter.complete(true);
    await tester.pumpAndSettle();

    expect(find.byType(AlertDialog), findsNothing);
  });

  testWidgets('InitialPasswordDialog 复制失败时保留弹窗并提示手动保存', (tester) async {
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: Builder(
            builder: (context) {
              return TextButton(
                onPressed: () {
                  showDialog<void>(
                    context: context,
                    builder: (_) => InitialPasswordDialog(
                      isZh: true,
                      username: 'sysadmin',
                      password: 'secret-pass',
                      copyText: (_) async => false,
                    ),
                  );
                },
                child: const Text('open'),
              );
            },
          ),
        ),
      ),
    );

    await tester.tap(find.text('open'));
    await tester.pumpAndSettle();

    await tester.tap(find.text('复制并继续'));
    await tester.pumpAndSettle();

    expect(find.byType(AlertDialog), findsOneWidget);
    expect(find.text('复制失败，请手动保存密码'), findsOneWidget);
  });

  testWidgets('InitialPasswordDialog 复制异常时恢复按钮并提示手动保存', (tester) async {
    var copyCount = 0;

    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: Builder(
            builder: (context) {
              return TextButton(
                onPressed: () {
                  showDialog<void>(
                    context: context,
                    builder: (_) => InitialPasswordDialog(
                      isZh: true,
                      username: 'sysadmin',
                      password: 'secret-pass',
                      copyText: (_) async {
                        copyCount++;
                        throw Exception('copy failed');
                      },
                    ),
                  );
                },
                child: const Text('open'),
              );
            },
          ),
        ),
      ),
    );

    await tester.tap(find.text('open'));
    await tester.pumpAndSettle();

    await tester.tap(find.text('复制并继续'));
    await tester.pumpAndSettle();

    expect(copyCount, 1);
    expect(find.byType(AlertDialog), findsOneWidget);
    expect(find.text('复制并继续'), findsOneWidget);
    expect(find.text('复制失败，请手动保存密码'), findsOneWidget);
  });

  testWidgets('InitialPasswordDialog 复制中禁用按钮避免重复提交', (tester) async {
    final copyCompleter = Completer<bool>();
    var copyCount = 0;

    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: Builder(
            builder: (context) {
              return TextButton(
                onPressed: () {
                  showDialog<void>(
                    context: context,
                    builder: (_) => InitialPasswordDialog(
                      isZh: true,
                      username: 'sysadmin',
                      password: 'secret-pass',
                      copyText: (_) {
                        copyCount++;
                        return copyCompleter.future;
                      },
                    ),
                  );
                },
                child: const Text('open'),
              );
            },
          ),
        ),
      ),
    );

    await tester.tap(find.text('open'));
    await tester.pumpAndSettle();

    await tester.tap(find.text('复制并继续'));
    await tester.pump();

    expect(find.text('正在复制'), findsOneWidget);
    await tester.tap(find.text('正在复制'), warnIfMissed: false);
    await tester.pump();

    expect(copyCount, 1);
    expect(find.byType(AlertDialog), findsOneWidget);

    copyCompleter.complete(true);
    await tester.pumpAndSettle();

    expect(find.byType(AlertDialog), findsNothing);
  });
}
