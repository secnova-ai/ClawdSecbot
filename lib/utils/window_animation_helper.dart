import 'dart:io';

import 'package:desktop_multi_window/desktop_multi_window.dart';
import 'package:window_manager/window_manager.dart';

/// Window animation helpers for desktop platforms.
class WindowAnimationHelper {
  static Future<void> _linuxShow() async {
    try {
      await windowManager.show();
      await windowManager.focus();
    } catch (_) {
      final controller = await WindowController.fromCurrentEngine();
      await controller.show();
    }
  }

  static Future<void> _linuxHide() async {
    try {
      await windowManager.hide();
    } catch (_) {
      final controller = await WindowController.fromCurrentEngine();
      await controller.hide();
    }
  }

  static Future<void> _setSkipTaskbarSafely(bool skip) async {
    try {
      await windowManager.setSkipTaskbar(skip);
    } catch (_) {}
  }

  /// Hides the window with a fade animation where supported.
  static Future<void> hideWithAnimation({int duration = 200}) async {
    if (Platform.isLinux) {
      await _linuxHide();
      return;
    }

    // On macOS, fading the window during a close/hide action causes a visible
    // flash. Hide immediately and remove the Dock icon to match tray-only app
    // behavior on the other desktop platforms.
    if (Platform.isMacOS) {
      await _setSkipTaskbarSafely(true);
      await windowManager.hide();
      return;
    }

    final currentOpacity = await windowManager.getOpacity();
    if (currentOpacity <= 0.0) {
      await windowManager.hide();
      return;
    }

    const steps = 20;
    final stepDuration = duration ~/ steps;
    final opacityStep = currentOpacity / steps;

    for (int i = 1; i <= steps; i++) {
      final newOpacity = currentOpacity - (opacityStep * i);
      await windowManager.setOpacity(newOpacity.clamp(0.0, 1.0));
      await Future.delayed(Duration(milliseconds: stepDuration));
    }

    await windowManager.hide();
    await windowManager.setOpacity(1.0);
  }

  /// Shows the window with a fade animation where supported.
  static Future<void> showWithAnimation({int duration = 200}) async {
    if (Platform.isLinux) {
      await _linuxShow();
      return;
    }

    if (Platform.isMacOS) {
      await _setSkipTaskbarSafely(false);
      await windowManager.show();
      await windowManager.focus();
      return;
    }

    await windowManager.setOpacity(0.0);
    await windowManager.show();
    await windowManager.focus();

    const steps = 20;
    final stepDuration = duration ~/ steps;
    final opacityStep = 1.0 / steps;

    for (int i = 1; i <= steps; i++) {
      final newOpacity = opacityStep * i;
      await windowManager.setOpacity(newOpacity.clamp(0.0, 1.0));
      await Future.delayed(Duration(milliseconds: stepDuration));
    }

    await windowManager.setOpacity(1.0);
  }

  /// Uses the platform-native minimize animation.
  static Future<void> minimizeWithAnimation() async {
    await windowManager.minimize();
  }

  /// Hides the window without animation.
  static Future<void> hideInstantly() async {
    if (Platform.isLinux) {
      await _linuxHide();
      return;
    }
    if (Platform.isMacOS) {
      await _setSkipTaskbarSafely(true);
    }
    await windowManager.hide();
  }

  /// Shows the window without animation.
  static Future<void> showInstantly() async {
    if (Platform.isLinux) {
      await _linuxShow();
      return;
    }
    if (Platform.isMacOS) {
      await _setSkipTaskbarSafely(false);
    }
    await windowManager.show();
    await windowManager.focus();
  }
}
