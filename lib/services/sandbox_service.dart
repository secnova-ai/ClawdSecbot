import 'dart:convert';
import 'dart:ffi' as ffi;
import 'dart:io';
import 'package:ffi/ffi.dart';
import 'package:path/path.dart' as path;
import '../models/protection_config_model.dart';
import '../utils/app_logger.dart';
import 'native_library_service.dart' hide FreeStringDart;

// FFI type definitions
typedef StartSandboxedGatewayC =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> configJSON);
typedef StartSandboxedGatewayDart =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> configJSON);

typedef StopSandboxedGatewayC =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> assetID);
typedef StopSandboxedGatewayDart =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> assetID);

typedef GetSandboxStatusC =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> assetID);
typedef GetSandboxStatusDart =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> assetID);

typedef EnableProcessMonitorC =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> configJSON);
typedef EnableProcessMonitorDart =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> configJSON);

typedef DisableProcessMonitorC =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> assetID);
typedef DisableProcessMonitorDart =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> assetID);

typedef KillUnmanagedGatewayC =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> configJSON);
typedef KillUnmanagedGatewayDart =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> configJSON);

typedef GenerateSandboxPolicyC =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> configJSON);
typedef GenerateSandboxPolicyDart =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> configJSON);

typedef CheckSandboxSupportedC = ffi.Pointer<Utf8> Function();
typedef CheckSandboxSupportedDart = ffi.Pointer<Utf8> Function();

typedef FreeStringC = ffi.Void Function(ffi.Pointer<Utf8>);
typedef FreeStringDart = void Function(ffi.Pointer<Utf8>);

/// Sandbox status from Go side
class SandboxStatus {
  final bool running;
  final int managedPID;
  final String policyPath;
  final String assetName;
  final String? gatewayBinary;
  final String? errorMessage;

  SandboxStatus({
    required this.running,
    required this.managedPID,
    required this.policyPath,
    required this.assetName,
    this.gatewayBinary,
    this.errorMessage,
  });

  factory SandboxStatus.fromJson(Map<String, dynamic> json) {
    return SandboxStatus(
      running: json['running'] == true,
      managedPID: json['managed_pid'] ?? 0,
      policyPath: json['policy_path'] ?? '',
      assetName: json['asset_name'] ?? '',
      gatewayBinary: json['gateway_binary'],
      errorMessage: json['error'],
    );
  }
}

/// Service for managing sandbox execution across platforms
class SandboxService {
  ffi.DynamicLibrary _getDylib() {
    final dylib = NativeLibraryService().dylib;
    if (dylib == null) {
      throw Exception('Plugin library not loaded');
    }
    return dylib;
  }

  /// Check if sandbox is supported on this platform (basic check)
  bool get isSandboxSupported =>
      Platform.isMacOS || Platform.isLinux || Platform.isWindows;

  /// Check if sandbox tooling is actually available (calls Go layer)
  Future<bool> checkSandboxAvailable() async {
    if (!Platform.isMacOS && !Platform.isWindows) return false;

    try {
      final dylib = _getDylib();
      final checkFunc = dylib
          .lookupFunction<CheckSandboxSupportedC, CheckSandboxSupportedDart>(
            'CheckSandboxSupported',
          );
      final freeString = dylib.lookupFunction<FreeStringC, FreeStringDart>(
        'FreeString',
      );

      final resultPtr = checkFunc();
      final resultStr = resultPtr.toDartString();
      freeString(resultPtr);

      final result = jsonDecode(resultStr) as Map<String, dynamic>;
      return result['supported'] == true;
    } catch (e) {
      appLogger.error('[Sandbox] Check available error', e);
      return false;
    }
  }

  /// Validate gateway binary path before use
  bool isValidGatewayPath(String? binaryPath) {
    if (binaryPath == null || binaryPath.isEmpty) return false;
    if (!path.isAbsolute(binaryPath)) return false;

    final file = File(binaryPath);
    if (!file.existsSync()) return false;

    return true;
  }

  /// Get the policy directory path
  /// Note: Policy files must be outside sandbox for sandbox-exec to read them
  Future<String> getPolicyDir() async {
    final homeDir =
        Platform.environment['HOME'] ??
        Platform.environment['USERPROFILE'] ??
        '';
    // Use ~/.botsec/policies instead of app documents directory
    // because sandbox-exec needs to read the policy file before applying sandbox
    final policyDir = '$homeDir/.botsec/policies';

    // Ensure directory exists
    final dir = Directory(policyDir);
    if (!await dir.exists()) {
      await dir.create(recursive: true);
    }

    return policyDir;
  }

  /// Start a gateway process with sandbox protection
  Future<Map<String, dynamic>> startSandboxedGateway({
    required String assetID,
    required String assetName,
    required String gatewayBinaryPath,
    required String gatewayConfigPath,
    required List<String> gatewayArgs,
    required List<String> gatewayEnv,
    required ProtectionConfig protectionConfig,
    String? policyDir,
  }) async {
    if (!isSandboxSupported) {
      return {
        'success': false,
        'error': 'Sandbox not supported on this platform',
      };
    }

    try {
      final dylib = _getDylib();
      final startFunc = dylib
          .lookupFunction<StartSandboxedGatewayC, StartSandboxedGatewayDart>(
            'StartSandboxedGateway',
          );
      final freeString = dylib.lookupFunction<FreeStringC, FreeStringDart>(
        'FreeString',
      );

      // Use the correct policy directory (same as database directory)
      final actualPolicyDir = policyDir ?? await getPolicyDir();

      final request = {
        'asset_id': assetID,
        'asset_name': assetName,
        'gateway_binary_path': gatewayBinaryPath,
        'gateway_config_path': gatewayConfigPath,
        'gateway_args': gatewayArgs,
        'gateway_env': gatewayEnv,
        'path_permission': protectionConfig.pathPermission.toJson(),
        'network_permission': protectionConfig.networkPermission.toJson(),
        'shell_permission': protectionConfig.shellPermission.toJson(),
        'policy_dir': actualPolicyDir,
        if (appLogger.logDir != null && appLogger.logDir!.isNotEmpty)
          'log_dir': appLogger.logDir,
      };

      final jsonStr = jsonEncode(request);
      final jsonPtr = jsonStr.toNativeUtf8();

      final resultPtr = startFunc(jsonPtr);
      malloc.free(jsonPtr);

      final resultStr = resultPtr.toDartString();
      freeString(resultPtr);

      final result = jsonDecode(resultStr) as Map<String, dynamic>;
      appLogger.info('[Sandbox] Start result: $result');
      return result;
    } catch (e) {
      appLogger.error('[Sandbox] Start error', e);
      return {'success': false, 'error': e.toString()};
    }
  }

  /// Stop a sandboxed gateway
  Future<Map<String, dynamic>> stopSandboxedGateway(String assetID) async {
    if (!isSandboxSupported) {
      return {'success': true, 'message': 'Sandbox not supported'};
    }

    try {
      final dylib = _getDylib();
      final stopFunc = dylib
          .lookupFunction<StopSandboxedGatewayC, StopSandboxedGatewayDart>(
            'StopSandboxedGateway',
          );
      final freeString = dylib.lookupFunction<FreeStringC, FreeStringDart>(
        'FreeString',
      );

      final assetPtr = assetID.toNativeUtf8();
      final resultPtr = stopFunc(assetPtr);
      malloc.free(assetPtr);

      final resultStr = resultPtr.toDartString();
      freeString(resultPtr);

      final result = jsonDecode(resultStr) as Map<String, dynamic>;
      appLogger.info('[Sandbox] Stop result: $result');
      return result;
    } catch (e) {
      appLogger.error('[Sandbox] Stop error', e);
      return {'success': false, 'error': e.toString()};
    }
  }

  /// Get sandbox status
  Future<SandboxStatus?> getSandboxStatus(String assetID) async {
    if (!isSandboxSupported) return null;

    try {
      final dylib = _getDylib();
      final getStatusFunc = dylib
          .lookupFunction<GetSandboxStatusC, GetSandboxStatusDart>(
            'GetSandboxStatus',
          );
      final freeString = dylib.lookupFunction<FreeStringC, FreeStringDart>(
        'FreeString',
      );

      final assetPtr = assetID.toNativeUtf8();
      final resultPtr = getStatusFunc(assetPtr);
      malloc.free(assetPtr);

      final resultStr = resultPtr.toDartString();
      freeString(resultPtr);

      final result = jsonDecode(resultStr) as Map<String, dynamic>;
      return SandboxStatus.fromJson(result);
    } catch (e) {
      appLogger.error('[Sandbox] Get status error', e);
      return null;
    }
  }

  /// Enable process monitor to detect and takeover unmanaged gateways
  Future<Map<String, dynamic>> enableProcessMonitor({
    required String assetID,
    required String assetName,
    required String gatewayPattern,
    int checkIntervalSeconds = 5,
  }) async {
    if (!isSandboxSupported) {
      return {'success': false, 'error': 'Sandbox not supported'};
    }

    try {
      final dylib = _getDylib();
      final enableFunc = dylib
          .lookupFunction<EnableProcessMonitorC, EnableProcessMonitorDart>(
            'EnableProcessMonitor',
          );
      final freeString = dylib.lookupFunction<FreeStringC, FreeStringDart>(
        'FreeString',
      );

      final request = {
        'asset_id': assetID,
        'asset_name': assetName,
        'gateway_pattern': gatewayPattern,
        'check_interval_seconds': checkIntervalSeconds,
      };

      final jsonStr = jsonEncode(request);
      final jsonPtr = jsonStr.toNativeUtf8();

      final resultPtr = enableFunc(jsonPtr);
      malloc.free(jsonPtr);

      final resultStr = resultPtr.toDartString();
      freeString(resultPtr);

      final result = jsonDecode(resultStr) as Map<String, dynamic>;
      appLogger.info('[Sandbox] Enable monitor result: $result');
      return result;
    } catch (e) {
      appLogger.error('[Sandbox] Enable monitor error', e);
      return {'success': false, 'error': e.toString()};
    }
  }

  /// Disable process monitor
  Future<Map<String, dynamic>> disableProcessMonitor(String assetID) async {
    if (!isSandboxSupported) {
      return {'success': true};
    }

    try {
      final dylib = _getDylib();
      final disableFunc = dylib
          .lookupFunction<DisableProcessMonitorC, DisableProcessMonitorDart>(
            'DisableProcessMonitor',
          );
      final freeString = dylib.lookupFunction<FreeStringC, FreeStringDart>(
        'FreeString',
      );

      final assetPtr = assetID.toNativeUtf8();
      final resultPtr = disableFunc(assetPtr);
      malloc.free(assetPtr);

      final resultStr = resultPtr.toDartString();
      freeString(resultPtr);

      final result = jsonDecode(resultStr) as Map<String, dynamic>;
      appLogger.info('[Sandbox] Disable monitor result: $result');
      return result;
    } catch (e) {
      appLogger.error('[Sandbox] Disable monitor error', e);
      return {'success': false, 'error': e.toString()};
    }
  }

  /// Kill unmanaged gateway processes
  Future<Map<String, dynamic>> killUnmanagedGateway({
    required String gatewayPattern,
    int managedPID = 0,
  }) async {
    if (!isSandboxSupported) {
      return {'success': false, 'error': 'Sandbox not supported'};
    }

    try {
      final dylib = _getDylib();
      final killFunc = dylib
          .lookupFunction<KillUnmanagedGatewayC, KillUnmanagedGatewayDart>(
            'KillUnmanagedGateway',
          );
      final freeString = dylib.lookupFunction<FreeStringC, FreeStringDart>(
        'FreeString',
      );

      final request = {
        'gateway_pattern': gatewayPattern,
        'managed_pid': managedPID,
      };

      final jsonStr = jsonEncode(request);
      final jsonPtr = jsonStr.toNativeUtf8();

      final resultPtr = killFunc(jsonPtr);
      malloc.free(jsonPtr);

      final resultStr = resultPtr.toDartString();
      freeString(resultPtr);

      final result = jsonDecode(resultStr) as Map<String, dynamic>;
      appLogger.info('[Sandbox] Kill unmanaged result: $result');
      return result;
    } catch (e) {
      appLogger.error('[Sandbox] Kill unmanaged error', e);
      return {'success': false, 'error': e.toString()};
    }
  }

  /// Generate sandbox policy content (for preview)
  Future<String?> generateSandboxPolicy(ProtectionConfig config) async {
    if (!isSandboxSupported) return null;

    try {
      final dylib = _getDylib();
      final genFunc = dylib
          .lookupFunction<GenerateSandboxPolicyC, GenerateSandboxPolicyDart>(
            'GenerateSandboxPolicy',
          );
      final freeString = dylib.lookupFunction<FreeStringC, FreeStringDart>(
        'FreeString',
      );

      final request = {
        'asset_name': config.assetName,
        'gateway_binary_path': config.gatewayBinaryPath ?? '',
        'gateway_config_path': config.gatewayConfigPath ?? '',
        'path_permission': config.pathPermission.toJson(),
        'network_permission': config.networkPermission.toJson(),
        'shell_permission': config.shellPermission.toJson(),
      };

      final jsonStr = jsonEncode(request);
      final jsonPtr = jsonStr.toNativeUtf8();

      final resultPtr = genFunc(jsonPtr);
      malloc.free(jsonPtr);

      final resultStr = resultPtr.toDartString();
      freeString(resultPtr);

      final result = jsonDecode(resultStr) as Map<String, dynamic>;
      if (result['success'] == true) {
        return result['policy'] as String?;
      }
      return null;
    } catch (e) {
      appLogger.error('[Sandbox] Generate policy error', e);
      return null;
    }
  }
}
