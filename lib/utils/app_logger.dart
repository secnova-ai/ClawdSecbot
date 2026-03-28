import 'dart:io';
import 'package:flutter/foundation.dart';
import 'package:logger/logger.dart';
import 'package:path/path.dart' as p;
import 'package:path_provider/path_provider.dart';

/// Application logger with file output and rotation support.
///
/// Features:
/// - Writes logs to file in app sandbox directory
/// - Automatic log rotation when file exceeds 50MB
/// - Keeps up to 3 backup files (total ~200MB max)
/// - Thread-safe singleton pattern
class AppLogger {
  static final AppLogger _instance = AppLogger._internal();
  factory AppLogger() => _instance;
  AppLogger._internal();

  Logger? _logger;
  File? _logFile;
  IOSink? _logSink;
  String? _logDir;
  String? _logFileName;
  bool _initialized = false;

  static const int maxFileSize = 50 * 1024 * 1024; // 50MB per file
  static const int maxBackupFiles = 3;
  static const String flutterLogFilePrefix = 'flutter_';

  /// Get the logs directory path (for sharing with Go)
  String? get logDir => _logDir;

  /// Initialize the logger. Must be called before using any log methods.
  ///
  /// The window name will be used in the log file name.
  Future<void> init({String windowName = 'main'}) async {
    if (_initialized) return;

    try {
      final safeWindowName = _sanitizeWindowName(windowName);
      _logDir = await _resolveLogDir();
      final logsDir = Directory(_logDir!);

      if (!await logsDir.exists()) {
        await logsDir.create(recursive: true);
      }

      _logFileName = '$flutterLogFilePrefix$safeWindowName.log';
      _logFile = File(p.join(_logDir!, _logFileName!));
      await _rotateIfNeeded();

      _logSink = _logFile!.openWrite(mode: FileMode.append);

      _logger = Logger(
        printer: _FileLogPrinter(),
        output: _FileLogOutput(_logSink!),
        level: kReleaseMode ? Level.info : Level.debug,
      );

      _initialized = true;
      info('Flutter logger initialized, log dir: $_logDir');
    } catch (e) {
      // Fallback to console-only logging if file init fails
      _logger = Logger(
        printer: PrettyPrinter(
          methodCount: 0,
          dateTimeFormat: DateTimeFormat.onlyTimeAndSinceStart,
        ),
        level: Level.debug,
      );
      _logger?.e('Failed to initialize file logger: $e');
    }
  }

  /// Resolve the shared log directory under the app data base directory.
  Future<String> _resolveLogDir() async {
    final dir = await getApplicationSupportDirectory();
    return p.join(dir.path, 'logs');
  }

  /// Sanitize window name for file naming.
  String _sanitizeWindowName(String windowName) {
    final normalized = windowName.trim().toLowerCase();
    final safe = normalized.replaceAll(RegExp(r'[^a-z0-9_-]+'), '_');
    return safe.isEmpty ? 'main' : safe;
  }

  /// Check if log file needs rotation and perform if necessary
  Future<void> _rotateIfNeeded() async {
    if (_logFile == null || !await _logFile!.exists()) return;

    final size = await _logFile!.length();
    if (size < maxFileSize) return;

    // Close current sink before rotation
    await _logSink?.flush();
    await _logSink?.close();
    _logSink = null;

    // Rotate backup files: .3 -> delete, .2 -> .3, .1 -> .2
    for (int i = maxBackupFiles; i >= 1; i--) {
      final backupFile = File('${_logFile!.path}.$i');
      if (await backupFile.exists()) {
        if (i == maxBackupFiles) {
          await backupFile.delete();
        } else {
          await backupFile.rename('${_logFile!.path}.${i + 1}');
        }
      }
    }

    // Rename current file to .1
    await _logFile!.rename('${_logFile!.path}.1');

    // Create new log file
    if (_logDir == null || _logFileName == null) {
      return;
    }
    _logFile = File(p.join(_logDir!, _logFileName!));
    _logSink = _logFile!.openWrite(mode: FileMode.append);

    // Update logger output
    if (_logger != null) {
      _logger = Logger(
        printer: _FileLogPrinter(),
        output: _FileLogOutput(_logSink!),
        level: kReleaseMode ? Level.info : Level.debug,
      );
    }
  }

  /// Check and rotate if needed (call periodically for long-running apps)
  Future<void> checkRotation() async {
    await _rotateIfNeeded();
  }

  void debug(String message) {
    _logger?.d(message);
  }

  void info(String message) {
    _logger?.i(message);
  }

  void warning(String message) {
    _logger?.w(message);
  }

  void error(String message, [Object? error, StackTrace? stackTrace]) {
    if (error != null) {
      _logger?.e(message, error: error, stackTrace: stackTrace);
    } else {
      _logger?.e(message);
    }
  }

  /// Close the logger and release resources
  Future<void> close() async {
    await _logSink?.flush();
    await _logSink?.close();
    _logSink = null;
    _initialized = false;
  }
}

/// Custom log printer for file output with timestamp and level
class _FileLogPrinter extends LogPrinter {
  @override
  List<String> log(LogEvent event) {
    final timestamp = DateTime.now().toIso8601String();
    final level = _levelToString(event.level);
    final message = event.message;

    final lines = <String>['[$timestamp] [$level] $message'];

    // Include error details if present
    if (event.error != null) {
      lines.add('[$timestamp] [$level] Error: ${event.error}');
    }
    if (event.stackTrace != null) {
      lines.add('[$timestamp] [$level] StackTrace: ${event.stackTrace}');
    }

    return lines;
  }

  String _levelToString(Level level) {
    switch (level) {
      case Level.debug:
        return 'DEBUG';
      case Level.info:
        return 'INFO';
      case Level.warning:
        return 'WARNING';
      case Level.error:
        return 'ERROR';
      case Level.fatal:
        return 'FATAL';
      default:
        return 'UNKNOWN';
    }
  }
}

/// Custom log output that writes to file
class _FileLogOutput extends LogOutput {
  final IOSink sink;

  _FileLogOutput(this.sink);

  @override
  void output(OutputEvent event) {
    for (var line in event.lines) {
      sink.writeln(line);
      // Also print to console in debug mode
      if (kDebugMode) {
        debugPrint(line);
      }
    }
  }
}

/// Global logger instance for convenience
final appLogger = AppLogger();
