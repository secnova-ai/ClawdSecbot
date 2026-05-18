import 'package:flutter/material.dart';

/// 保持 Tab 页子树在当前页面实例内存活，避免切换标签页时丢失未保存草稿。
class KeepAliveTab extends StatefulWidget {
  /// 创建需要在 TabBarView 中缓存状态的标签页容器。
  const KeepAliveTab({super.key, required this.child});

  /// 被缓存的标签页内容。
  final Widget child;

  @override
  State<KeepAliveTab> createState() => _KeepAliveTabState();
}

class _KeepAliveTabState extends State<KeepAliveTab>
    with AutomaticKeepAliveClientMixin<KeepAliveTab> {
  /// 当前标签页始终请求 keep-alive，只在父页面销毁时释放。
  @override
  bool get wantKeepAlive => true;

  /// 构建被缓存的标签页内容。
  @override
  Widget build(BuildContext context) {
    super.build(context);
    return widget.child;
  }
}
