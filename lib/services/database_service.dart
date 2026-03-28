import 'dart:io';
import 'package:path_provider/path_provider.dart';
import '../utils/app_logger.dart';

/// Database path contract service.
///
/// Flutter only resolves the shared app data base directory.
/// Core owns all derived runtime paths such as database, version file, logs,
/// backups, and skill directories.
class DatabaseService {
  static final DatabaseService _instance = DatabaseService._internal();
  String? _appDataDir;

  factory DatabaseService() {
    return _instance;
  }

  DatabaseService._internal();

  /// Shared app data base directory passed to Go core.
  String? get appDataDir => _appDataDir;

  /// Initializes the shared app data directory and ensures it exists.
  Future<void> init() async {
    if (_appDataDir != null) return;

    final dir = await getApplicationSupportDirectory();
    _appDataDir = dir.path;
    appLogger.info('[Database] App data dir: ${dir.path}');

    final baseDir = Directory(dir.path);
    if (!await baseDir.exists()) {
      await baseDir.create(recursive: true);
    }
  }
}
