// ignore_for_file: avoid_web_libraries_in_flutter, deprecated_member_use

import 'dart:html' as html;

// copyTextForWeb 在 Web 中优先使用浏览器剪贴板 API，失败时降级到 textarea 复制。
Future<bool> copyTextForWeb(String text) async {
  try {
    final clipboard = html.window.navigator.clipboard;
    if (clipboard != null) {
      await clipboard.writeText(text);
      return true;
    }
  } catch (_) {
    // 浏览器可能因 HTTP 非安全上下文或权限策略拒绝 Clipboard API。
    html.window.console.debug(
      'Web clipboard API failed, fallback to textarea copy',
    );
  }

  if (_copyTextWithTextArea(text)) {
    return true;
  }

  html.window.console.warn(
    'Web clipboard copy failed after all fallback attempts',
  );
  return false;
}

// _copyTextWithTextArea 使用传统 execCommand 路径兼容 HTTP/IP 访问场景。
bool _copyTextWithTextArea(String text) {
  final body = html.document.body;
  if (body == null) {
    return false;
  }

  final textArea = html.TextAreaElement()
    ..value = text
    ..setAttribute('readonly', 'readonly');

  var appended = false;
  try {
    textArea.style
      ..position = 'fixed'
      ..left = '-9999px'
      ..top = '0'
      ..opacity = '0'
      ..setProperty('user-select', 'text');

    body.append(textArea);
    appended = true;
    textArea.focus();
    textArea.select();
    textArea.setSelectionRange(0, text.length);

    return html.document.execCommand('copy');
  } catch (_) {
    return false;
  } finally {
    if (appended) {
      textArea.remove();
    }
  }
}
