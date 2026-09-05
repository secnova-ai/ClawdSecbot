import 'dart:io';
import 'package:flutter/material.dart';
import '../utils/app_fonts.dart';
import 'package:lucide_icons_lite/lucide_icons_lite.dart';
import '../l10n/app_localizations.dart';
import '../services/bookmark_service.dart';
import '../utils/app_logger.dart';

class ScanStartPage extends StatefulWidget {
  final VoidCallback onStartScan;

  const ScanStartPage({super.key, required this.onStartScan});

  @override
  State<ScanStartPage> createState() => _ScanStartPageState();
}

class _ScanStartPageState extends State<ScanStartPage> {
  final BookmarkService _bookmarkService = BookmarkService();
  bool _authorized = false;
  bool _checking = true;

  @override
  void initState() {
    super.initState();
    _checkAuthorization();
  }

  Future<void> _checkAuthorization() async {
    if (!Platform.isMacOS) {
      if (mounted) {
        setState(() {
          _authorized = true;
          _checking = false;
        });
      }
      return;
    }

    try {
      final hasAccess = await _bookmarkService.initialize();
      if (mounted) {
        setState(() {
          _authorized = hasAccess;
          _checking = false;
        });
      }
    } catch (e) {
      appLogger.error('[ScanStartPage] Auth check failed', e);
      if (mounted) {
        setState(() {
          _checking = false;
        });
      }
    }
  }

  Future<void> _requestAuthorization() async {
    try {
      final path = await _bookmarkService.selectAndStoreDirectory();
      if (path != null) {
        final success = await _bookmarkService.startAccessingDirectory();
        if (mounted) {
          setState(() {
            _authorized = success;
          });
        }
      }
    } catch (e) {
      appLogger.error('[ScanStartPage] Request auth failed', e);
    }
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context)!;

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          children: [
            Container(
              padding: const EdgeInsets.all(8),
              decoration: BoxDecoration(
                color: const Color(0xFF6366F1).withValues(alpha: 0.2),
                borderRadius: BorderRadius.circular(8),
              ),
              child: const Icon(
                LucideIcons.radar,
                color: Color(0xFF6366F1),
                size: 20,
              ),
            ),
            const SizedBox(width: 12),
            Text(
              l10n.scanStartTitle,
              style: AppFonts.inter(
                fontSize: 18,
                fontWeight: FontWeight.w600,
                color: Colors.white,
              ),
            ),
          ],
        ),
        const SizedBox(height: 24),
        Text(
          l10n.scanStartDesc,
          style: AppFonts.inter(fontSize: 14, color: Colors.white70),
        ),
        const SizedBox(height: 32),
        if (_checking)
          const Center(child: CircularProgressIndicator())
        else if (!_authorized)
          _buildAuthSection(l10n)
        else
          _buildStartSection(l10n),
      ],
    );
  }

  Widget _buildAuthSection(AppLocalizations l10n) {
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: Colors.amber.withValues(alpha: 0.1),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: Colors.amber.withValues(alpha: 0.3)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              const Icon(
                LucideIcons.shieldAlert,
                color: Colors.amber,
                size: 20,
              ),
              const SizedBox(width: 8),
              Text(
                l10n.scanStartAuthRequired,
                style: AppFonts.inter(
                  fontWeight: FontWeight.w600,
                  color: Colors.white,
                ),
              ),
            ],
          ),
          const SizedBox(height: 8),
          Text(
            l10n.scanStartAuthDesc,
            style: AppFonts.inter(fontSize: 13, color: Colors.white70),
          ),
          const SizedBox(height: 16),
          ElevatedButton(
            onPressed: _requestAuthorization,
            style: ElevatedButton.styleFrom(
              backgroundColor: Colors.amber,
              foregroundColor: Colors.black,
            ),
            child: Text(l10n.scanStartAuthBtn),
          ),
        ],
      ),
    );
  }

  Widget _buildStartSection(AppLocalizations l10n) {
    return Center(
      child: ElevatedButton.icon(
        onPressed: widget.onStartScan,
        icon: const Icon(LucideIcons.play),
        label: Text(l10n.scanStartBtn),
        style: ElevatedButton.styleFrom(
          backgroundColor: const Color(0xFF6366F1),
          foregroundColor: Colors.white,
          padding: const EdgeInsets.symmetric(horizontal: 32, vertical: 16),
          textStyle: AppFonts.inter(
            fontSize: 16,
            fontWeight: FontWeight.w600,
          ),
        ),
      ),
    );
  }
}
