import 'package:flutter/material.dart';
import 'package:lucide_icons/lucide_icons.dart';

import '../utils/app_fonts.dart';

class WebBackendConfigPanel extends StatelessWidget {
  const WebBackendConfigPanel({
    super.key,
    required this.isZh,
    required this.bootstrapping,
    required this.apiBaseController,
    required this.workspacePrefixController,
    required this.homeDirController,
    required this.currentVersionController,
    required this.onReconnect,
    required this.onApplyAndReconnect,
  });

  final bool isZh;
  final bool bootstrapping;
  final TextEditingController apiBaseController;
  final TextEditingController workspacePrefixController;
  final TextEditingController homeDirController;
  final TextEditingController currentVersionController;
  final VoidCallback onReconnect;
  final VoidCallback onApplyAndReconnect;

  String _txt(String zh, String en) => isZh ? zh : en;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: Colors.black.withValues(alpha: 0.18),
        border: Border(
          bottom: BorderSide(color: Colors.white.withValues(alpha: 0.08)),
        ),
      ),
      child: Column(
        children: [
          Wrap(
            spacing: 10,
            runSpacing: 10,
            children: [
              _ConfigField(
                label: _txt('API 地址', 'API Endpoint'),
                controller: apiBaseController,
                width: 320,
              ),
              _ConfigField(
                label: _txt('工作目录前缀', 'Workspace Prefix'),
                controller: workspacePrefixController,
                width: 320,
              ),
              _ConfigField(
                label: _txt('Home 目录', 'Home Directory'),
                controller: homeDirController,
                width: 220,
              ),
              _ConfigField(
                label: _txt('当前版本', 'Current Version'),
                controller: currentVersionController,
                width: 140,
              ),
            ],
          ),
          const SizedBox(height: 10),
          Row(
            children: [
              FilledButton.icon(
                onPressed: bootstrapping ? null : onReconnect,
                icon: bootstrapping
                    ? const SizedBox(
                        width: 12,
                        height: 12,
                        child: CircularProgressIndicator(
                          strokeWidth: 2,
                          color: Colors.white,
                        ),
                      )
                    : const Icon(LucideIcons.refreshCw, size: 14),
                label: Text(_txt('重新连接后端', 'Reconnect Backend')),
              ),
              const SizedBox(width: 8),
              OutlinedButton(
                onPressed: bootstrapping ? null : onApplyAndReconnect,
                child: Text(_txt('应用配置并重连', 'Apply Config and Reconnect')),
              ),
            ],
          ),
        ],
      ),
    );
  }
}

class WebBootstrapStatusBanner extends StatelessWidget {
  const WebBootstrapStatusBanner({
    super.key,
    required this.isZh,
    required this.bootstrapping,
    required this.error,
    required this.onRetry,
  });

  final bool isZh;
  final bool bootstrapping;
  final String? error;
  final VoidCallback onRetry;

  String _txt(String zh, String en) => isZh ? zh : en;

  @override
  Widget build(BuildContext context) {
    final message = error?.trim().isNotEmpty == true
        ? error!.trim()
        : _txt('正在自动连接后端服务…', 'Connecting to backend automatically...');
    final isError = error?.trim().isNotEmpty == true;

    return Container(
      width: double.infinity,
      margin: const EdgeInsets.fromLTRB(12, 8, 12, 0),
      padding: const EdgeInsets.all(10),
      decoration: BoxDecoration(
        color: isError
            ? const Color(0xFF7F1D1D).withValues(alpha: 0.35)
            : const Color(0xFF0B3A6E).withValues(alpha: 0.35),
        borderRadius: BorderRadius.circular(8),
        border: Border.all(
          color: isError
              ? const Color(0xFFEF4444).withValues(alpha: 0.5)
              : const Color(0xFF60A5FA).withValues(alpha: 0.5),
        ),
      ),
      child: Row(
        children: [
          if (bootstrapping)
            const SizedBox(
              width: 14,
              height: 14,
              child: CircularProgressIndicator(
                strokeWidth: 2,
                color: Color(0xFF93C5FD),
              ),
            )
          else
            Icon(
              isError ? LucideIcons.alertCircle : LucideIcons.server,
              size: 14,
              color: isError
                  ? const Color(0xFFFCA5A5)
                  : const Color(0xFF93C5FD),
            ),
          const SizedBox(width: 8),
          Expanded(
            child: Text(
              message,
              style: AppFonts.inter(
                color: isError
                    ? const Color(0xFFFEE2E2)
                    : const Color(0xFFDBEAFE),
                fontSize: 12,
              ),
            ),
          ),
          const SizedBox(width: 8),
          TextButton(
            onPressed: bootstrapping ? null : onRetry,
            child: Text(_txt('立即重试', 'Retry Now')),
          ),
        ],
      ),
    );
  }
}

class _ConfigField extends StatelessWidget {
  const _ConfigField({
    required this.label,
    required this.controller,
    required this.width,
  });

  final String label;
  final TextEditingController controller;
  final double width;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      width: width,
      child: TextField(
        controller: controller,
        style: AppFonts.inter(color: Colors.white, fontSize: 13),
        decoration: InputDecoration(
          isDense: true,
          labelText: label,
          labelStyle: AppFonts.inter(color: Colors.white70, fontSize: 12),
          filled: true,
          fillColor: const Color(0xFF142039),
          border: OutlineInputBorder(
            borderRadius: BorderRadius.circular(8),
            borderSide: const BorderSide(color: Color(0xFF33476B)),
          ),
          enabledBorder: OutlineInputBorder(
            borderRadius: BorderRadius.circular(8),
            borderSide: const BorderSide(color: Color(0xFF33476B)),
          ),
          focusedBorder: OutlineInputBorder(
            borderRadius: BorderRadius.circular(8),
            borderSide: const BorderSide(color: Color(0xFF4F7FD9), width: 1.5),
          ),
        ),
      ),
    );
  }
}
