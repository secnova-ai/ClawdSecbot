import 'dart:ui';

class AppConstants {
  /// 主窗口统一尺寸，所有状态（欢迎页、空闲、扫描中、扫描完成）共用同一大小，
  /// 避免窗口大小变化导致位置飘移，提升用户体验。
  static const Size windowSize = Size(610, 780);
}
