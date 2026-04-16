import 'dart:convert';
import 'dart:io';

import 'package:path/path.dart' as path;

import '../core_transport/transport_registry.dart';
import '../models/protection_config_model.dart';
import '../utils/app_logger.dart';

/// Sandbox status from Go side.
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

/// Service for managing sandbox execution across platforms.
class SandboxService {
  /// Check if sandbox is supported on this platform (basic check).
  bool get isSandboxSupported =>
      Platform.isMacOS || Platform.isLinux || Platform.isWindows;

  /// Check if sandbox tooling is actually available (calls Go layer).
  Future<bool> checkSandboxAvailable() async {
    if (!Platform.isMacOS && !Platform.isWindows) return false;
    final result = _callNoArg('CheckSandboxSupported');
    return result['supported'] == true;
  }

  /// Validate gateway binary path before use.
  bool isValidGatewayPath(String? binaryPath) {
    if (binaryPath == null || binaryPath.isEmpty) return false;
    if (!path.isAbsolute(binaryPath)) return false;
    final file = File(binaryPath);
    return file.existsSync();
  }

  /// Get the policy directory path.
  Future<String> getPolicyDir() async {
    final homeDir =
        Platform.environment['HOME'] ??
        Platform.environment['USERPROFILE'] ??
        '';
    final policyDir = '$homeDir/.botsec/policies';
    final dir = Directory(policyDir);
    if (!await dir.exists()) {
      await dir.create(recursive: true);
    }
    return policyDir;
  }

  /// Start a gateway process with sandbox protection.
  Future<Map<String, dynamic>> startSandboxedGateway({
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

    final actualPolicyDir = policyDir ?? await getPolicyDir();
    final request = {
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
    final result = _callOneArg('StartSandboxedGateway', jsonEncode(request));
    appLogger.info('[Sandbox] Start result: $result');
    return result;
  }

  /// Stop a sandboxed gateway.
  Future<Map<String, dynamic>> stopSandboxedGateway(String assetName) async {
    if (!isSandboxSupported) {
      return {'success': true, 'message': 'Sandbox not supported'};
    }
    final result = _callOneArg('StopSandboxedGateway', assetName);
    appLogger.info('[Sandbox] Stop result: $result');
    return result;
  }

  /// Get sandbox status.
  Future<SandboxStatus?> getSandboxStatus(String assetName) async {
    if (!isSandboxSupported) return null;
    final result = _callOneArg('GetSandboxStatus', assetName);
    if (result['success'] == false && result['running'] == null) {
      return null;
    }
    return SandboxStatus.fromJson(result);
  }

  /// Enable process monitor to detect and takeover unmanaged gateways.
  Future<Map<String, dynamic>> enableProcessMonitor({
    required String assetName,
    required String gatewayPattern,
    int checkIntervalSeconds = 5,
  }) async {
    if (!isSandboxSupported) {
      return {'success': false, 'error': 'Sandbox not supported'};
    }
    final request = {
      'asset_name': assetName,
      'gateway_pattern': gatewayPattern,
      'check_interval_seconds': checkIntervalSeconds,
    };
    final result = _callOneArg('EnableProcessMonitor', jsonEncode(request));
    appLogger.info('[Sandbox] Enable monitor result: $result');
    return result;
  }

  /// Disable process monitor.
  Future<Map<String, dynamic>> disableProcessMonitor(String assetName) async {
    if (!isSandboxSupported) {
      return {'success': true};
    }
    final result = _callOneArg('DisableProcessMonitor', assetName);
    appLogger.info('[Sandbox] Disable monitor result: $result');
    return result;
  }

  /// Kill unmanaged gateway processes.
  Future<Map<String, dynamic>> killUnmanagedGateway({
    required String gatewayPattern,
    int managedPID = 0,
  }) async {
    if (!isSandboxSupported) {
      return {'success': false, 'error': 'Sandbox not supported'};
    }
    final request = {
      'gateway_pattern': gatewayPattern,
      'managed_pid': managedPID,
    };
    final result = _callOneArg('KillUnmanagedGateway', jsonEncode(request));
    appLogger.info('[Sandbox] Kill unmanaged result: $result');
    return result;
  }

  /// Generate sandbox policy content (for preview).
  Future<String?> generateSandboxPolicy(ProtectionConfig config) async {
    if (!isSandboxSupported) return null;

    final request = {
      'asset_name': config.assetName,
      'gateway_binary_path': config.gatewayBinaryPath ?? '',
      'gateway_config_path': config.gatewayConfigPath ?? '',
      'path_permission': config.pathPermission.toJson(),
      'network_permission': config.networkPermission.toJson(),
      'shell_permission': config.shellPermission.toJson(),
    };
    final result = _callOneArg('GenerateSandboxPolicy', jsonEncode(request));
    if (result['success'] == true) {
      return result['policy'] as String?;
    }
    return null;
  }

  Map<String, dynamic> _callNoArg(String method) {
    final transport = TransportRegistry.transport;
    if (!transport.isReady) {
      return {'success': false, 'error': 'Transport not initialized'};
    }
    try {
      return transport.callNoArg(method);
    } catch (e) {
      appLogger.error('[Sandbox] $method failed', e);
      return {'success': false, 'error': '$method failed: $e'};
    }
  }

  Map<String, dynamic> _callOneArg(String method, String arg) {
    final transport = TransportRegistry.transport;
    if (!transport.isReady) {
      return {'success': false, 'error': 'Transport not initialized'};
    }
    try {
      return transport.callOneArg(method, arg);
    } catch (e) {
      appLogger.error('[Sandbox] $method failed', e);
      return {'success': false, 'error': '$method failed: $e'};
    }
  }
}
