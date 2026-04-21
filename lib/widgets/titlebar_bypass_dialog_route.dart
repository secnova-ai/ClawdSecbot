import 'package:flutter/material.dart';

enum ExitRestoreAction { cancel, exitOnly, restoreAndExit }

class TitlebarBypassDialogRoute<T> extends PopupRoute<T> {
  TitlebarBypassDialogRoute({
    required this.builder,
    required this.topInset,
    Color? barrierColor,
    bool barrierDismissible = true,
    String? barrierLabel,
  }) : _barrierColor = barrierColor ?? Colors.black54,
       _barrierDismissible = barrierDismissible,
       _barrierLabel = barrierLabel;

  final WidgetBuilder builder;
  final double topInset;
  final Color _barrierColor;
  final bool _barrierDismissible;
  final String? _barrierLabel;

  @override
  Duration get transitionDuration => const Duration(milliseconds: 180);

  @override
  bool get barrierDismissible => _barrierDismissible;

  @override
  Color? get barrierColor => _barrierColor;

  @override
  String? get barrierLabel => _barrierLabel;

  @override
  Widget buildPage(
    BuildContext context,
    Animation<double> animation,
    Animation<double> secondaryAnimation,
  ) {
    return builder(context);
  }

  @override
  Widget buildTransitions(
    BuildContext context,
    Animation<double> animation,
    Animation<double> secondaryAnimation,
    Widget child,
  ) {
    return FadeTransition(
      opacity: CurvedAnimation(parent: animation, curve: Curves.easeOutCubic),
      child: child,
    );
  }

  @override
  Widget buildModalBarrier() {
    return Stack(
      children: [
        Positioned(
          top: topInset,
          left: 0,
          right: 0,
          bottom: 0,
          child: ModalBarrier(
            dismissible: barrierDismissible,
            color: barrierColor,
            semanticsLabel: barrierLabel,
          ),
        ),
      ],
    );
  }
}
