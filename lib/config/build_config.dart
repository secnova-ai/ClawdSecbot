import 'dart:io';

/// Build configuration for different distribution channels
class BuildConfig {
  /// Build variant: 'personal' or 'appstore'
  /// Set via --dart-define=BUILD_VARIANT=personal|appstore
  static const String buildVariant = String.fromEnvironment(
    'BUILD_VARIANT',
    defaultValue: 'personal',
  );

  /// Distribution type: 'community', 'business', or 'appstore'.
  static const String buildType = String.fromEnvironment(
    'BUILD_TYPE',
    defaultValue: 'community',
  );

  /// Check if this is the App Store build
  static bool get isAppStore {
    // Debug override via environment variable
    // Use try-catch to be safe across platforms (though dart:io implies mobile/desktop)
    try {
      if (Platform.environment['IS_APPSTORE_MODEL'] == 'true') {
        return true;
      }
    } catch (_) {}
    return buildVariant == 'appstore';
  }

  /// Check if this is the Personal build
  static bool get isPersonal => !isAppStore;

  /// Check if this is the Business build.
  static bool get isBusiness => !isAppStore && buildType == 'business';

  /// Check if sandbox is required (App Store builds must use sandbox)
  static bool get requiresSandbox => isAppStore;

  /// Check if directory authorization is required (only App Store needs it)
  static bool get requiresDirectoryAuth => isAppStore;

  /// Get build variant display name
  static String get displayName {
    if (isAppStore) {
      return 'App Store Edition';
    }
    if (isBusiness) {
      return 'Business Edition';
    }
    return 'Personal Edition';
  }

  static int get defaultScheduledScanIntervalSeconds => isBusiness ? 300 : 0;

  static bool get defaultApiServerEnabled => isBusiness;
}
