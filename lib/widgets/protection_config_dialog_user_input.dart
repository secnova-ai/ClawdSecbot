part of 'protection_config_dialog.dart';

extension _ProtectionConfigDialogUserInputExtension
    on _ProtectionConfigDialogState {
  Widget _buildUserInputDetectionSwitch(AppLocalizations l10n) {
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: _userInputDetectionEnabled
            ? const Color(0xFF22C55E).withValues(alpha: 0.1)
            : Colors.white.withValues(alpha: 0.05),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(
          color: _userInputDetectionEnabled
              ? const Color(0xFF22C55E).withValues(alpha: 0.3)
              : Colors.white.withValues(alpha: 0.1),
        ),
      ),
      child: Row(
        children: [
          Container(
            padding: const EdgeInsets.all(8),
            decoration: BoxDecoration(
              color: _userInputDetectionEnabled
                  ? const Color(0xFF22C55E).withValues(alpha: 0.2)
                  : Colors.white.withValues(alpha: 0.1),
              borderRadius: BorderRadius.circular(8),
            ),
            child: Icon(
              LucideIcons.messageSquare,
              color: _userInputDetectionEnabled
                  ? const Color(0xFF22C55E)
                  : Colors.white54,
              size: 20,
            ),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  l10n.userInputDetection,
                  style: AppFonts.inter(
                    fontSize: 14,
                    fontWeight: FontWeight.w600,
                    color: Colors.white,
                  ),
                ),
                const SizedBox(height: 2),
                Text(
                  l10n.userInputDetectionDesc,
                  style: AppFonts.inter(fontSize: 11, color: Colors.white54),
                ),
              ],
            ),
          ),
          Switch(
            value: _userInputDetectionEnabled,
            onChanged: _setUserInputDetectionEnabled,
            activeThumbColor: const Color(0xFF22C55E),
            activeTrackColor: const Color(0xFF22C55E).withValues(alpha: 0.3),
          ),
        ],
      ),
    );
  }
}
