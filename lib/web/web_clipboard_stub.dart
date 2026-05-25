import 'package:flutter/services.dart';

// copyTextForWeb 使用 Flutter 剪贴板作为非 Web 环境的测试后备实现。
Future<bool> copyTextForWeb(String text) async {
  await Clipboard.setData(ClipboardData(text: text));
  return true;
}
