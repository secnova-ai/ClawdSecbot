part of 'protection_config_dialog.dart';

extension _ProtectionConfigDialogNetworkExtension
    on _ProtectionConfigDialogState {
  Widget _buildNetworkPermissionSection(AppLocalizations l10n) {
    final isMacSandbox = isRuntimeMacOS && _sandboxEnabled;
    final placeholder = isMacSandbox
        ? l10n.networkPermissionPlaceholderSandbox
        : l10n.networkPermissionPlaceholder;

    return Container(
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: Colors.white.withValues(alpha: 0.03),
        borderRadius: BorderRadius.circular(8),
        border: Border.all(color: Colors.white.withValues(alpha: 0.1)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              const Icon(LucideIcons.globe, color: Color(0xFF6366F1), size: 16),
              const SizedBox(width: 8),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      l10n.networkPermissionTitle,
                      style: AppFonts.inter(
                        fontSize: 13,
                        fontWeight: FontWeight.w600,
                        color: Colors.white,
                      ),
                    ),
                    Text(
                      isMacSandbox
                          ? l10n.networkPermissionDescSandbox
                          : l10n.networkPermissionDesc,
                      style: AppFonts.inter(
                        fontSize: 11,
                        color: Colors.white54,
                      ),
                    ),
                  ],
                ),
              ),
            ],
          ),
          const SizedBox(height: 16),
          _buildDirectionalNetworkBlock(
            title: l10n.networkOutboundTitle,
            desc: l10n.networkOutboundDesc,
            icon: LucideIcons.arrowUpRight,
            mode: _networkOutboundMode,
            onModeChanged: _updateNetworkOutboundMode,
            items: _networkOutboundList,
            inputController: _networkOutboundInputController,
            inputHint: placeholder,
            onAdd: () => _addNetworkAddress(
              _networkOutboundInputController,
              _networkOutboundList,
              l10n,
            ),
            onRemove: _removeNetworkOutboundAt,
          ),
        ],
      ),
    );
  }

  void _addNetworkAddress(
    TextEditingController controller,
    List<String> list,
    AppLocalizations l10n,
  ) {
    final addr = controller.text.trim();
    if (addr.isEmpty || list.contains(addr)) {
      return;
    }
    if (isRuntimeMacOS &&
        _sandboxEnabled &&
        !NetworkPermissionConfig.isValidSandboxAddress(addr)) {
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(l10n.networkAddressInvalidForSandbox),
          backgroundColor: const Color(0xFFEF4444),
          duration: const Duration(seconds: 4),
        ),
      );
      return;
    }
    _appendNetworkAddress(list, addr);
    controller.clear();
  }

  Widget _buildDirectionalNetworkBlock({
    required String title,
    required String desc,
    required IconData icon,
    required PermissionMode mode,
    required Function(PermissionMode) onModeChanged,
    required List<String> items,
    required TextEditingController inputController,
    required String inputHint,
    required VoidCallback onAdd,
    required Function(int) onRemove,
  }) {
    final l10n = AppLocalizations.of(context)!;

    return Container(
      padding: const EdgeInsets.all(10),
      decoration: BoxDecoration(
        color: Colors.white.withValues(alpha: 0.03),
        borderRadius: BorderRadius.circular(6),
        border: Border.all(color: Colors.white.withValues(alpha: 0.08)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Icon(icon, color: Colors.white70, size: 14),
              const SizedBox(width: 6),
              Text(
                title,
                style: AppFonts.inter(
                  fontSize: 12,
                  fontWeight: FontWeight.w600,
                  color: Colors.white,
                ),
              ),
              const SizedBox(width: 8),
              Expanded(
                child: Text(
                  desc,
                  style: AppFonts.inter(fontSize: 10, color: Colors.white38),
                ),
              ),
            ],
          ),
          const SizedBox(height: 8),
          Row(
            children: [
              _buildModeButton(
                label: l10n.blacklistMode,
                isSelected: mode == PermissionMode.blacklist,
                onTap: () => onModeChanged(PermissionMode.blacklist),
              ),
              const SizedBox(width: 8),
              _buildModeButton(
                label: l10n.whitelistMode,
                isSelected: mode == PermissionMode.whitelist,
                onTap: () => onModeChanged(PermissionMode.whitelist),
              ),
            ],
          ),
          const SizedBox(height: 8),
          Row(
            children: [
              Expanded(
                child: Container(
                  height: 38,
                  decoration: BoxDecoration(
                    color: Colors.white.withValues(alpha: 0.05),
                    borderRadius: BorderRadius.circular(6),
                    border: Border.all(
                      color: Colors.white.withValues(alpha: 0.1),
                    ),
                  ),
                  child: TextField(
                    controller: inputController,
                    style: AppFonts.firaCode(fontSize: 12, color: Colors.white),
                    decoration: InputDecoration(
                      hintText: inputHint,
                      hintStyle: AppFonts.inter(
                        fontSize: 11,
                        color: Colors.white38,
                      ),
                      border: InputBorder.none,
                      contentPadding: const EdgeInsets.symmetric(
                        horizontal: 10,
                        vertical: 10,
                      ),
                    ),
                    onSubmitted: (_) => onAdd(),
                  ),
                ),
              ),
              const SizedBox(width: 6),
              MouseRegion(
                cursor: SystemMouseCursors.click,
                child: GestureDetector(
                  onTap: onAdd,
                  child: Container(
                    height: 38,
                    padding: const EdgeInsets.symmetric(horizontal: 10),
                    decoration: BoxDecoration(
                      color: const Color(0xFF6366F1),
                      borderRadius: BorderRadius.circular(6),
                    ),
                    child: const Icon(
                      LucideIcons.plus,
                      size: 14,
                      color: Colors.white,
                    ),
                  ),
                ),
              ),
            ],
          ),
          if (items.isNotEmpty) ...[
            const SizedBox(height: 8),
            Wrap(
              spacing: 6,
              runSpacing: 6,
              children: items.asMap().entries.map((entry) {
                return Container(
                  padding: const EdgeInsets.symmetric(
                    horizontal: 8,
                    vertical: 3,
                  ),
                  decoration: BoxDecoration(
                    color: mode == PermissionMode.blacklist
                        ? const Color(0xFFEF4444).withValues(alpha: 0.2)
                        : const Color(0xFF22C55E).withValues(alpha: 0.2),
                    borderRadius: BorderRadius.circular(4),
                    border: Border.all(
                      color: mode == PermissionMode.blacklist
                          ? const Color(0xFFEF4444).withValues(alpha: 0.3)
                          : const Color(0xFF22C55E).withValues(alpha: 0.3),
                    ),
                  ),
                  child: Row(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Flexible(
                        child: Text(
                          entry.value,
                          style: AppFonts.firaCode(
                            fontSize: 10,
                            color: Colors.white,
                          ),
                        ),
                      ),
                      const SizedBox(width: 4),
                      MouseRegion(
                        cursor: SystemMouseCursors.click,
                        child: GestureDetector(
                          onTap: () => onRemove(entry.key),
                          child: const Icon(
                            LucideIcons.x,
                            size: 10,
                            color: Colors.white54,
                          ),
                        ),
                      ),
                    ],
                  ),
                );
              }).toList(),
            ),
          ],
        ],
      ),
    );
  }
}
