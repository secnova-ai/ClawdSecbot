import 'package:bot_sec_manager/widgets/keep_alive_tab.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets('KeepAliveTab 在同次打开的 tab 切换中保留未保存输入', (
    WidgetTester tester,
  ) async {
    await tester.pumpWidget(const _KeepAliveTabHarness());
    await tester.tap(find.text('Bot'));
    await tester.pumpAndSettle();

    await tester.enterText(find.byKey(const ValueKey('botInput')), 'draft');
    await tester.tap(find.text('Rules'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Bot'));
    await tester.pumpAndSettle();

    expect(find.widgetWithText(TextField, 'draft'), findsOneWidget);

    await tester.pumpWidget(const SizedBox.shrink());
    await tester.pumpAndSettle();
    await tester.pumpWidget(const _KeepAliveTabHarness());
    await tester.pumpAndSettle();
    await tester.tap(find.text('Bot'));
    await tester.pumpAndSettle();

    expect(find.widgetWithText(TextField, 'draft'), findsNothing);
  });
}

class _KeepAliveTabHarness extends StatelessWidget {
  const _KeepAliveTabHarness();

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      home: DefaultTabController(
        length: 4,
        child: Scaffold(
          appBar: AppBar(
            bottom: const TabBar(
              tabs: [
                Tab(text: 'Rules'),
                Tab(text: 'Token'),
                Tab(text: 'Permission'),
                Tab(text: 'Bot'),
              ],
            ),
          ),
          body: const TabBarView(
            children: [
              KeepAliveTab(child: Text('rules')),
              KeepAliveTab(child: Text('token')),
              KeepAliveTab(child: Text('permission')),
              KeepAliveTab(child: _DraftTextField()),
            ],
          ),
        ),
      ),
    );
  }
}

class _DraftTextField extends StatefulWidget {
  const _DraftTextField();

  @override
  State<_DraftTextField> createState() => _DraftTextFieldState();
}

class _DraftTextFieldState extends State<_DraftTextField> {
  final TextEditingController _controller = TextEditingController();

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return TextField(key: const ValueKey('botInput'), controller: _controller);
  }
}
