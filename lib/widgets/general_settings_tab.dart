import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:lucide_icons/lucide_icons.dart';

import '../config/build_config.dart';
import '../l10n/app_localizations.dart';
import '../utils/app_fonts.dart';
import '../utils/runtime_platform.dart';

enum _ScheduledScanUnit { seconds, minutes, hours }

extension on _ScheduledScanUnit {
  int toSeconds(int value) {
    return switch (this) {
      _ScheduledScanUnit.seconds => value,
      _ScheduledScanUnit.minutes => value * 60,
      _ScheduledScanUnit.hours => value * 3600,
    };
  }

  String label(AppLocalizations l10n) {
    return switch (this) {
      _ScheduledScanUnit.seconds => l10n.scheduledScanUnitSeconds,
      _ScheduledScanUnit.minutes => l10n.scheduledScanUnitMinutes,
      _ScheduledScanUnit.hours => l10n.scheduledScanUnitHours,
    };
  }
}

class GeneralSettingsTab extends StatefulWidget {
  final bool launchAtStartupEnabled;
  final ValueChanged<bool> onLaunchAtStartupChanged;
  final int scheduledScanIntervalSeconds;
  final ValueChanged<int> onScheduledScanIntervalChanged;
  final VoidCallback onClearData;
  final VoidCallback onRestoreConfig;
  final VoidCallback onShowAbout;
  final VoidCallback onReauthorizeDirectory;

  const GeneralSettingsTab({
    super.key,
    required this.launchAtStartupEnabled,
    required this.onLaunchAtStartupChanged,
    required this.scheduledScanIntervalSeconds,
    required this.onScheduledScanIntervalChanged,
    required this.onClearData,
    required this.onRestoreConfig,
    required this.onShowAbout,
    required this.onReauthorizeDirectory,
  });

  @override
  State<GeneralSettingsTab> createState() => _GeneralSettingsTabState();
}

class _GeneralSettingsTabState extends State<GeneralSettingsTab> {
  static const String _value60Seconds = '60';
  static const String _value5Minutes = '300';
  static const String _value1Hour = '3600';
  static const String _valueCustom = 'custom';
  static const String _valueOff = '0';

  late final TextEditingController _customValueController;
  late _ScheduledScanUnit _customUnit;
  bool _showCustomEditor = false;
  String? _customErrorText;
  bool _intervalMenuOpen = false;
  bool _unitMenuOpen = false;

  bool get _isCustomSelected {
    final seconds = widget.scheduledScanIntervalSeconds;
    return seconds > 0 && !{60, 300, 3600}.contains(seconds);
  }

  @override
  void initState() {
    super.initState();
    _customValueController = TextEditingController();
    _syncCustomState(widget.scheduledScanIntervalSeconds);
  }

  @override
  void didUpdateWidget(covariant GeneralSettingsTab oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.scheduledScanIntervalSeconds !=
        widget.scheduledScanIntervalSeconds) {
      _syncCustomState(widget.scheduledScanIntervalSeconds);
    }
  }

  @override
  void dispose() {
    _customValueController.dispose();
    super.dispose();
  }

  void _syncCustomState(int seconds) {
    _customUnit = _guessUnit(seconds);
    _showCustomEditor = _isCustomSelected;
    final normalized = _normalizeToUnitValue(seconds, _customUnit);
    _customValueController.text = normalized > 0 ? normalized.toString() : '';
    _customErrorText = null;
  }

  _ScheduledScanUnit _guessUnit(int seconds) {
    if (seconds > 0 && seconds % 3600 == 0) {
      return _ScheduledScanUnit.hours;
    }
    if (seconds > 0 && seconds % 60 == 0) {
      return _ScheduledScanUnit.minutes;
    }
    return _ScheduledScanUnit.seconds;
  }

  int _normalizeToUnitValue(int seconds, _ScheduledScanUnit unit) {
    if (seconds <= 0) {
      return 0;
    }

    return switch (unit) {
      _ScheduledScanUnit.seconds => seconds,
      _ScheduledScanUnit.minutes => seconds ~/ 60,
      _ScheduledScanUnit.hours => seconds ~/ 3600,
    };
  }

  String _formatScheduledScanLabel(BuildContext context, int seconds) {
    final l10n = AppLocalizations.of(context)!;
    if (seconds <= 0) {
      return l10n.scheduledScanOff;
    }
    if (seconds == 60) {
      return l10n.scheduledScanOption60Seconds;
    }
    if (seconds == 300) {
      return l10n.scheduledScanOption5Minutes;
    }
    if (seconds == 3600) {
      return l10n.scheduledScanOption1Hour;
    }
    if (seconds % 3600 == 0) {
      return l10n.scheduledScanEvery(
        seconds ~/ 3600,
        l10n.scheduledScanUnitHours,
      );
    }
    if (seconds % 60 == 0) {
      return l10n.scheduledScanEvery(
        seconds ~/ 60,
        l10n.scheduledScanUnitMinutes,
      );
    }
    return l10n.scheduledScanEvery(seconds, l10n.scheduledScanUnitSeconds);
  }

  String get _dropdownValue {
    final seconds = widget.scheduledScanIntervalSeconds;
    if (seconds <= 0) {
      return _valueOff;
    }
    if (seconds == 60) {
      return _value60Seconds;
    }
    if (seconds == 300) {
      return _value5Minutes;
    }
    if (seconds == 3600) {
      return _value1Hour;
    }
    return _valueCustom;
  }

  void _handleDropdownChanged(String? value) {
    if (value == null) {
      return;
    }

    if (value == _valueCustom) {
      setState(() {
        _showCustomEditor = true;
        _customErrorText = null;
        if (_customValueController.text.trim().isEmpty) {
          _customValueController.text = '10';
        }
      });
      return;
    }

    final seconds = int.tryParse(value) ?? 0;
    setState(() {
      _showCustomEditor = false;
      _customErrorText = null;
    });
    widget.onScheduledScanIntervalChanged(seconds);
  }

  void _applyCustom() {
    final value = int.tryParse(_customValueController.text.trim());
    if (value == null || value <= 0) {
      setState(() {
        _customErrorText =
            AppLocalizations.of(context)!.scheduledScanInvalidCustomValue;
      });
      return;
    }

    setState(() {
      _customErrorText = null;
    });
    widget.onScheduledScanIntervalChanged(_customUnit.toSeconds(value));
  }

  void _handleCustomValueChanged(String value) {
    if (_customErrorText != null) {
      setState(() {
        _customErrorText = null;
      });
    }

    final parsedValue = int.tryParse(value.trim());
    if (parsedValue == null || parsedValue <= 0) {
      return;
    }

    widget.onScheduledScanIntervalChanged(_customUnit.toSeconds(parsedValue));
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context)!;

    return SingleChildScrollView(
      padding: const EdgeInsets.symmetric(vertical: 2),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          if (!BuildConfig.isAppStore) ...[
            _buildToggleTile(
              icon: LucideIcons.power,
              iconColor: const Color(0xFF6366F1),
              title: l10n.launchAtStartup,
              value: widget.launchAtStartupEnabled,
              onToggle: () => widget.onLaunchAtStartupChanged(
                !widget.launchAtStartupEnabled,
              ),
            ),
            const SizedBox(height: 8),
          ],
          _buildScheduledScanTile(context),
          const SizedBox(height: 16),
          _buildSectionHeader(l10n.dataManagement),
          const SizedBox(height: 8),
          _buildActionTile(
            icon: LucideIcons.trash2,
            iconColor: const Color(0xFFEF4444),
            title: l10n.clearData,
            subtitle: l10n.clearDataDescription,
            onTap: widget.onClearData,
          ),
          const SizedBox(height: 8),
          _buildActionTile(
            icon: LucideIcons.rotateCcw,
            iconColor: const Color(0xFFEAB308),
            title: l10n.restoreConfig,
            subtitle: l10n.restoreConfigDescription,
            onTap: widget.onRestoreConfig,
          ),
          const SizedBox(height: 8),
          _buildActionTile(
            icon: LucideIcons.info,
            iconColor: const Color(0xFF6366F1),
            title: l10n.aboutApp(l10n.appTitle),
            subtitle: '${l10n.version} / ${l10n.buildNumber}',
            onTap: widget.onShowAbout,
          ),
          if (isRuntimeMacOS && BuildConfig.requiresDirectoryAuth) ...[
            const SizedBox(height: 16),
            _buildSectionHeader(l10n.permissionsSection),
            const SizedBox(height: 8),
            _buildActionTile(
              icon: LucideIcons.folderOpen,
              iconColor: const Color(0xFF22C55E),
              title: l10n.permissionsSection,
              onTap: widget.onReauthorizeDirectory,
            ),
          ],
        ],
      ),
    );
  }

  Widget _buildScheduledScanTile(BuildContext context) {
    final l10n = AppLocalizations.of(context)!;
    final currentLabel = _formatScheduledScanLabel(
      context,
      widget.scheduledScanIntervalSeconds,
    );

    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
      decoration: BoxDecoration(
        color: Colors.white.withValues(alpha: 0.05),
        borderRadius: BorderRadius.circular(10),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Container(
                padding: const EdgeInsets.all(6),
                decoration: BoxDecoration(
                  color: const Color(0xFF6366F1).withValues(alpha: 0.15),
                  borderRadius: BorderRadius.circular(6),
                ),
                child: const Icon(
                  LucideIcons.timer,
                  color: Color(0xFF6366F1),
                  size: 16,
                ),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      l10n.scheduledScanSetting,
                      style: AppFonts.inter(fontSize: 14, color: Colors.white),
                    ),
                    const SizedBox(height: 2),
                    Text(
                      widget.scheduledScanIntervalSeconds <= 0
                          ? l10n.scheduledScanDescription
                          : currentLabel,
                      style: AppFonts.inter(
                        fontSize: 12,
                        color: Colors.white38,
                      ),
                    ),
                  ],
                ),
              ),
              const SizedBox(width: 12),
              SizedBox(width: 150, child: _buildIntervalDropdown(l10n)),
            ],
          ),
          if (_showCustomEditor) ...[
            const SizedBox(height: 12),
            Container(
              padding: const EdgeInsets.all(12),
              decoration: BoxDecoration(
                color: Colors.black.withValues(alpha: 0.15),
                borderRadius: BorderRadius.circular(8),
                border: Border.all(color: Colors.white.withValues(alpha: 0.08)),
              ),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    l10n.scheduledScanCustomHint,
                    style: AppFonts.inter(
                      fontSize: 12,
                      color: Colors.white54,
                    ),
                  ),
                  const SizedBox(height: 10),
                  Container(
                    decoration: BoxDecoration(
                      color: Colors.white.withValues(alpha: 0.05),
                      borderRadius: BorderRadius.circular(8),
                      border: Border.all(
                        color: Colors.white.withValues(alpha: 0.1),
                      ),
                    ),
                    child: Row(
                      children: [
                        Expanded(
                          child: TextField(
                            controller: _customValueController,
                            keyboardType: TextInputType.number,
                            inputFormatters: [
                              FilteringTextInputFormatter.digitsOnly,
                            ],
                            style: AppFonts.firaCode(
                              fontSize: 14,
                              color: Colors.white,
                            ),
                            decoration: InputDecoration(
                              hintText: l10n.scheduledScanCustomValueHint,
                              hintStyle: AppFonts.inter(
                                fontSize: 12,
                                color: Colors.white38,
                              ),
                              border: InputBorder.none,
                              contentPadding: const EdgeInsets.symmetric(
                                horizontal: 12,
                                vertical: 12,
                              ),
                            ),
                            onChanged: _handleCustomValueChanged,
                            onSubmitted: (_) => _applyCustom(),
                          ),
                        ),
                        Container(
                          width: 1,
                          height: 28,
                          color: Colors.white.withValues(alpha: 0.1),
                        ),
                        Padding(
                          padding: const EdgeInsets.only(right: 4),
                          child: _buildUnitDropdown(l10n),
                        ),
                      ],
                    ),
                  ),
                  if (_customErrorText != null) ...[
                    const SizedBox(height: 8),
                    Text(
                      _customErrorText!,
                      style: AppFonts.inter(
                        fontSize: 12,
                        color: const Color(0xFFEF4444),
                      ),
                    ),
                  ],
                ],
              ),
            ),
          ],
        ],
      ),
    );
  }

  Widget _buildIntervalDropdown(AppLocalizations l10n) {
    return _buildPopupSelect<String>(
      value: _dropdownValue,
      width: 150,
      compact: false,
      highlightedColor: const Color(0xFF6366F1),
      menuOpen: _intervalMenuOpen,
      items: [
        DropdownMenuItem(
          value: _value60Seconds,
          child: _buildDropdownItemLabel(l10n.scheduledScanOption60Seconds),
        ),
        DropdownMenuItem(
          value: _value5Minutes,
          child: _buildDropdownItemLabel(l10n.scheduledScanOption5Minutes),
        ),
        DropdownMenuItem(
          value: _value1Hour,
          child: _buildDropdownItemLabel(l10n.scheduledScanOption1Hour),
        ),
        DropdownMenuItem(
          value: _valueCustom,
          child: _buildDropdownItemLabel(l10n.scheduledScanCustom),
        ),
        DropdownMenuItem(
          value: _valueOff,
          child: _buildDropdownItemLabel(l10n.scheduledScanOff),
        ),
      ],
      onMenuStateChanged: (open) {
        if (_intervalMenuOpen != open) {
          setState(() {
            _intervalMenuOpen = open;
          });
        }
      },
      onChanged: _handleDropdownChanged,
    );
  }

  Widget _buildUnitDropdown(AppLocalizations l10n) {
    return _buildPopupSelect<_ScheduledScanUnit>(
      value: _customUnit,
      width: 108,
      compact: true,
      highlightedColor: const Color(0xFF6366F1),
      menuOpen: _unitMenuOpen,
      items: _ScheduledScanUnit.values.map((unit) {
        return DropdownMenuItem<_ScheduledScanUnit>(
          value: unit,
          child: _buildDropdownItemLabel(unit.label(l10n)),
        );
      }).toList(),
      onMenuStateChanged: (open) {
        if (_unitMenuOpen != open) {
          setState(() {
            _unitMenuOpen = open;
          });
        }
      },
      onChanged: (value) {
        if (value == null) {
          return;
        }
        setState(() {
          _customUnit = value;
        });
        _applyCustom();
      },
    );
  }

  Widget _buildPopupSelect<T>({
    required T value,
    required double width,
    required List<DropdownMenuItem<T>> items,
    required bool menuOpen,
    required Color highlightedColor,
    required ValueChanged<bool> onMenuStateChanged,
    required ValueChanged<T?> onChanged,
    bool compact = false,
  }) {
    final selectedItem = items
        .cast<DropdownMenuItem<T>?>()
        .firstWhere((item) => item?.value == value)
        ?.child;

    return AnimatedContainer(
      duration: const Duration(milliseconds: 160),
      width: width,
      height: compact ? 40 : 42,
      decoration: BoxDecoration(
        color: menuOpen
            ? highlightedColor.withValues(alpha: 0.12)
            : Colors.white.withValues(alpha: 0.05),
        borderRadius: BorderRadius.circular(10),
        border: Border.all(
          color: menuOpen
              ? highlightedColor.withValues(alpha: 0.45)
              : Colors.white.withValues(alpha: 0.1),
        ),
        boxShadow: menuOpen
            ? [
                BoxShadow(
                  color: highlightedColor.withValues(alpha: 0.14),
                  blurRadius: 12,
                  offset: const Offset(0, 4),
                ),
              ]
            : null,
      ),
      child: PopupMenuButton<T>(
        onSelected: (selected) => onChanged(selected),
        onOpened: () => onMenuStateChanged(true),
        onCanceled: () => onMenuStateChanged(false),
        offset: const Offset(0, 36),
        color: const Color(0xFF1E1E2E),
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(8),
          side: BorderSide(color: Colors.white.withValues(alpha: 0.1)),
        ),
        itemBuilder: (context) => items.map((item) {
          final isSelected = item.value == value;
          return PopupMenuItem<T>(
            value: item.value,
            onTap: () => onMenuStateChanged(false),
            child: DefaultTextStyle.merge(
              style: AppFonts.inter(
                fontSize: 13,
                color: isSelected
                    ? highlightedColor
                    : Colors.white70,
                fontWeight: isSelected ? FontWeight.w600 : FontWeight.w400,
              ),
              child: item.child,
            ),
          );
        }).toList(),
        child: Padding(
          padding: EdgeInsets.symmetric(
            horizontal: compact ? 10 : 12,
            vertical: compact ? 7 : 8,
          ),
          child: Row(
            children: [
              Expanded(
                child: DefaultTextStyle.merge(
                  style: AppFonts.inter(
                    fontSize: 13,
                    fontWeight: FontWeight.w500,
                    color: highlightedColor,
                  ),
                  child: selectedItem ?? const SizedBox(),
                ),
              ),
              const SizedBox(width: 6),
              AnimatedRotation(
                duration: const Duration(milliseconds: 160),
                turns: menuOpen ? 0.5 : 0,
                child: Icon(
                  LucideIcons.chevronDown,
                  size: 14,
                  color: highlightedColor,
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }

  Widget _buildDropdownItemLabel(String label) {
    return Text(
      label,
      style: AppFonts.inter(fontSize: 13),
      overflow: TextOverflow.ellipsis,
    );
  }

  Widget _buildSectionHeader(String title) {
    return Padding(
      padding: const EdgeInsets.only(left: 4),
      child: Text(
        title,
        style: AppFonts.inter(
          fontSize: 12,
          fontWeight: FontWeight.w500,
          color: Colors.white38,
        ),
      ),
    );
  }

  Widget _buildToggleTile({
    required IconData icon,
    required Color iconColor,
    required String title,
    required bool value,
    required VoidCallback onToggle,
  }) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
      decoration: BoxDecoration(
        color: Colors.white.withValues(alpha: 0.05),
        borderRadius: BorderRadius.circular(10),
      ),
      child: Row(
        children: [
          Container(
            padding: const EdgeInsets.all(6),
            decoration: BoxDecoration(
              color: iconColor.withValues(alpha: 0.15),
              borderRadius: BorderRadius.circular(6),
            ),
            child: Icon(icon, color: iconColor, size: 16),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Text(
              title,
              style: AppFonts.inter(fontSize: 14, color: Colors.white),
            ),
          ),
          Switch.adaptive(
            value: value,
            onChanged: (_) => onToggle(),
            activeTrackColor: const Color(0xFF6366F1),
            inactiveTrackColor: Colors.white.withValues(alpha: 0.1),
          ),
        ],
      ),
    );
  }

  Widget _buildActionTile({
    required IconData icon,
    required Color iconColor,
    required String title,
    String? subtitle,
    required VoidCallback onTap,
  }) {
    return Material(
      color: Colors.transparent,
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(10),
        hoverColor: Colors.white.withValues(alpha: 0.03),
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
          decoration: BoxDecoration(
            color: Colors.white.withValues(alpha: 0.05),
            borderRadius: BorderRadius.circular(10),
          ),
          child: Row(
            children: [
              Container(
                padding: const EdgeInsets.all(6),
                decoration: BoxDecoration(
                  color: iconColor.withValues(alpha: 0.15),
                  borderRadius: BorderRadius.circular(6),
                ),
                child: Icon(icon, color: iconColor, size: 16),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      title,
                      style: AppFonts.inter(fontSize: 14, color: Colors.white),
                    ),
                    if (subtitle != null) ...[
                      const SizedBox(height: 2),
                      Text(
                        subtitle,
                        style: AppFonts.inter(
                          fontSize: 12,
                          color: Colors.white38,
                        ),
                      ),
                    ],
                  ],
                ),
              ),
              const Icon(
                LucideIcons.chevronRight,
                color: Colors.white24,
                size: 16,
              ),
            ],
          ),
        ),
      ),
    );
  }

}
