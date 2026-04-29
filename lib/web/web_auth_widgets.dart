import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:lucide_icons/lucide_icons.dart';

import '../utils/app_fonts.dart';

typedef WebLoginHandler =
    Future<String?> Function(String username, String password);
typedef WebPasswordChangeHandler =
    Future<String?> Function(String currentPassword, String newPassword);

class WebLoginPanel extends StatefulWidget {
  const WebLoginPanel({
    super.key,
    required this.isZh,
    required this.username,
    required this.onLogin,
  });

  final bool isZh;
  final String username;
  final WebLoginHandler onLogin;

  @override
  State<WebLoginPanel> createState() => _WebLoginPanelState();
}

class _WebLoginPanelState extends State<WebLoginPanel> {
  late final TextEditingController _usernameCtrl;
  late final TextEditingController _passwordCtrl;
  bool _submitting = false;
  bool _obscure = true;
  String? _error;

  String _txt(String zh, String en) => widget.isZh ? zh : en;

  @override
  void initState() {
    super.initState();
    _usernameCtrl = TextEditingController(text: widget.username);
    _passwordCtrl = TextEditingController();
  }

  @override
  void dispose() {
    _usernameCtrl.dispose();
    _passwordCtrl.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    if (_submitting) return;
    setState(() {
      _submitting = true;
      _error = null;
    });
    final error = await widget.onLogin(
      _usernameCtrl.text.trim(),
      _passwordCtrl.text,
    );
    if (!mounted) return;
    setState(() {
      _submitting = false;
      _error = error;
    });
  }

  @override
  Widget build(BuildContext context) {
    return Center(
      key: const ValueKey('web-auth-login'),
      child: Container(
        width: 420,
        margin: const EdgeInsets.all(24),
        padding: const EdgeInsets.all(24),
        decoration: BoxDecoration(
          color: Colors.black.withValues(alpha: 0.38),
          borderRadius: BorderRadius.circular(12),
          border: Border.all(
            color: const Color(0xFF6366F1).withValues(alpha: 0.42),
          ),
        ),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Row(
              children: [
                const Icon(
                  LucideIcons.lock,
                  color: Color(0xFFA5B4FC),
                  size: 22,
                ),
                const SizedBox(width: 10),
                Text(
                  _txt('WebUI 登录', 'WebUI Sign In'),
                  style: AppFonts.inter(
                    color: Colors.white,
                    fontSize: 20,
                    fontWeight: FontWeight.w700,
                  ),
                ),
              ],
            ),
            const SizedBox(height: 22),
            _AuthTextField(
              controller: _usernameCtrl,
              label: _txt('账号', 'Username'),
              icon: LucideIcons.user,
              enabled: !_submitting,
            ),
            const SizedBox(height: 12),
            _AuthTextField(
              controller: _passwordCtrl,
              label: _txt('密码', 'Password'),
              icon: LucideIcons.keyRound,
              enabled: !_submitting,
              obscureText: _obscure,
              onSubmitted: (_) => _submit(),
              suffixIcon: IconButton(
                icon: Icon(
                  _obscure ? LucideIcons.eye : LucideIcons.eyeOff,
                  size: 16,
                  color: Colors.white70,
                ),
                onPressed: _submitting
                    ? null
                    : () => setState(() => _obscure = !_obscure),
              ),
            ),
            if (_error != null) ...[
              const SizedBox(height: 12),
              Text(
                _error!,
                style: AppFonts.inter(
                  color: const Color(0xFFFCA5A5),
                  fontSize: 12,
                ),
              ),
            ],
            const SizedBox(height: 18),
            FilledButton.icon(
              onPressed: _submitting ? null : _submit,
              icon: _submitting
                  ? const SizedBox(
                      width: 14,
                      height: 14,
                      child: CircularProgressIndicator(
                        strokeWidth: 2,
                        color: Colors.white,
                      ),
                    )
                  : const Icon(LucideIcons.logIn, size: 16),
              label: Text(_txt('登录', 'Sign In')),
            ),
          ],
        ),
      ),
    );
  }
}

class InitialPasswordDialog extends StatelessWidget {
  const InitialPasswordDialog({
    super.key,
    required this.isZh,
    required this.username,
    required this.password,
  });

  final bool isZh;
  final String username;
  final String password;

  String _txt(String zh, String en) => isZh ? zh : en;

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      backgroundColor: const Color(0xFF111827),
      icon: const Icon(
        LucideIcons.shieldAlert,
        color: Color(0xFFFBBF24),
        size: 32,
      ),
      title: Text(
        _txt('请立即保存初始密码', 'Save the Initial Password'),
        style: AppFonts.inter(color: Colors.white, fontWeight: FontWeight.w700),
      ),
      content: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            _txt(
              '系统已创建默认管理员账号。这个密码只展示一次，请复制后妥善保存。',
              'The default administrator account has been created. This password is shown only once.',
            ),
            style: AppFonts.inter(color: Colors.white70, fontSize: 13),
          ),
          const SizedBox(height: 14),
          _CredentialBox(label: _txt('账号', 'Username'), value: username),
          const SizedBox(height: 10),
          _CredentialBox(label: _txt('密码', 'Password'), value: password),
        ],
      ),
      actions: [
        TextButton.icon(
          onPressed: () {
            Clipboard.setData(
              ClipboardData(text: 'username=$username\npassword=$password'),
            );
            Navigator.of(context).pop();
          },
          icon: const Icon(LucideIcons.copy, size: 16),
          label: Text(_txt('复制并继续', 'Copy and Continue')),
        ),
      ],
    );
  }
}

class ChangeWebPasswordDialog extends StatefulWidget {
  const ChangeWebPasswordDialog({
    super.key,
    required this.isZh,
    required this.onChangePassword,
  });

  final bool isZh;
  final WebPasswordChangeHandler onChangePassword;

  @override
  State<ChangeWebPasswordDialog> createState() =>
      _ChangeWebPasswordDialogState();
}

class _ChangeWebPasswordDialogState extends State<ChangeWebPasswordDialog> {
  final TextEditingController _currentCtrl = TextEditingController();
  final TextEditingController _newCtrl = TextEditingController();
  final TextEditingController _confirmCtrl = TextEditingController();
  bool _submitting = false;
  String? _error;

  String _txt(String zh, String en) => widget.isZh ? zh : en;

  @override
  void dispose() {
    _currentCtrl.dispose();
    _newCtrl.dispose();
    _confirmCtrl.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    if (_submitting) return;
    final next = _newCtrl.text;
    if (next != _confirmCtrl.text) {
      setState(() => _error = _txt('两次输入的新密码不一致', 'Passwords do not match'));
      return;
    }
    if (next.runes.length < 6) {
      setState(
        () => _error = _txt(
          '新密码至少 6 位',
          'New password must be at least 6 characters',
        ),
      );
      return;
    }

    setState(() {
      _submitting = true;
      _error = null;
    });
    final error = await widget.onChangePassword(_currentCtrl.text, next);
    if (!mounted) return;
    if (error == null) {
      Navigator.of(context).pop(true);
      return;
    }
    setState(() {
      _submitting = false;
      _error = error;
    });
  }

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      backgroundColor: const Color(0xFF111827),
      title: Text(
        _txt('修改 WebUI 密码', 'Change WebUI Password'),
        style: AppFonts.inter(color: Colors.white, fontWeight: FontWeight.w700),
      ),
      content: SizedBox(
        width: 360,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            _AuthTextField(
              controller: _currentCtrl,
              label: _txt('当前密码', 'Current Password'),
              icon: LucideIcons.keyRound,
              obscureText: true,
              enabled: !_submitting,
            ),
            const SizedBox(height: 12),
            _AuthTextField(
              controller: _newCtrl,
              label: _txt('新密码', 'New Password'),
              icon: LucideIcons.lock,
              obscureText: true,
              enabled: !_submitting,
            ),
            const SizedBox(height: 12),
            _AuthTextField(
              controller: _confirmCtrl,
              label: _txt('确认新密码', 'Confirm New Password'),
              icon: LucideIcons.lock,
              obscureText: true,
              enabled: !_submitting,
              onSubmitted: (_) => _submit(),
            ),
            if (_error != null) ...[
              const SizedBox(height: 12),
              Align(
                alignment: Alignment.centerLeft,
                child: Text(
                  _error!,
                  style: AppFonts.inter(
                    color: const Color(0xFFFCA5A5),
                    fontSize: 12,
                  ),
                ),
              ),
            ],
          ],
        ),
      ),
      actions: [
        TextButton(
          onPressed: _submitting ? null : () => Navigator.of(context).pop(),
          child: Text(_txt('取消', 'Cancel')),
        ),
        FilledButton.icon(
          onPressed: _submitting ? null : _submit,
          icon: _submitting
              ? const SizedBox(
                  width: 14,
                  height: 14,
                  child: CircularProgressIndicator(
                    strokeWidth: 2,
                    color: Colors.white,
                  ),
                )
              : const Icon(LucideIcons.save, size: 16),
          label: Text(_txt('保存', 'Save')),
        ),
      ],
    );
  }
}

class _CredentialBox extends StatelessWidget {
  const _CredentialBox({required this.label, required this.value});

  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: double.infinity,
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: const Color(0xFF0F172A),
        borderRadius: BorderRadius.circular(8),
        border: Border.all(color: const Color(0xFF334155)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            label,
            style: AppFonts.inter(color: Colors.white54, fontSize: 11),
          ),
          const SizedBox(height: 5),
          SelectableText(
            value,
            style: AppFonts.firaCode(
              color: Colors.white,
              fontSize: 17,
              fontWeight: FontWeight.w700,
            ),
          ),
        ],
      ),
    );
  }
}

class _AuthTextField extends StatelessWidget {
  const _AuthTextField({
    required this.controller,
    required this.label,
    required this.icon,
    this.enabled = true,
    this.obscureText = false,
    this.suffixIcon,
    this.onSubmitted,
  });

  final TextEditingController controller;
  final String label;
  final IconData icon;
  final bool enabled;
  final bool obscureText;
  final Widget? suffixIcon;
  final ValueChanged<String>? onSubmitted;

  @override
  Widget build(BuildContext context) {
    return TextField(
      controller: controller,
      enabled: enabled,
      obscureText: obscureText,
      onSubmitted: onSubmitted,
      style: AppFonts.inter(color: Colors.white, fontSize: 14),
      decoration: InputDecoration(
        isDense: true,
        labelText: label,
        labelStyle: AppFonts.inter(color: Colors.white70, fontSize: 12),
        prefixIcon: Icon(icon, size: 16, color: Colors.white60),
        suffixIcon: suffixIcon,
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
    );
  }
}
