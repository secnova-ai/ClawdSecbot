import 'dart:ffi' as ffi;
import 'package:ffi/ffi.dart';

/// 防护代理相关 FFI 签名与 Isolate 安全调用入口。
///
/// 所有与 Go 层 StartProtectionProxy 等相关的 C 签名集中在此，
/// 供 [ProtectionService] / [ProtectionMonitorService] 使用。
/// 在后台 Isolate 中执行的调用必须使用静态方法并仅传入 [libPath]，因 Isolate 无法共享主 isolate 的 DynamicLibrary。
///
/// 【架构变更说明】网关生命周期已内聚到代理防护服务中：
/// - StartProtectionProxy 内部自动完成网关启动（更新 openclaw.json + 重启 gateway）
/// - 不再暴露 ApplyOpenclawConfigForProtection / SyncOpenclawGatewayFFI 给 Flutter 层

// --- 代理生命周期与配置 ---
typedef StartProtectionProxyC = ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);
typedef StartProtectionProxyDart =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);

typedef StopProtectionProxyC = ffi.Pointer<Utf8> Function();
typedef StopProtectionProxyDart = ffi.Pointer<Utf8> Function();

typedef StopProtectionProxyByAssetC =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>, ffi.Pointer<Utf8>);
typedef StopProtectionProxyByAssetDart =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>, ffi.Pointer<Utf8>);

typedef GetProtectionProxyStatusC = ffi.Pointer<Utf8> Function();
typedef GetProtectionProxyStatusDart = ffi.Pointer<Utf8> Function();

typedef GetProtectionProxyStatusByAssetC =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>, ffi.Pointer<Utf8>);
typedef GetProtectionProxyStatusByAssetDart =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>, ffi.Pointer<Utf8>);

typedef SetProtectionProxyAuditOnlyC = ffi.Pointer<Utf8> Function(ffi.Int32);
typedef SetProtectionProxyAuditOnlyDart = ffi.Pointer<Utf8> Function(int);

typedef SetProtectionProxyAuditOnlyByAssetC =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>, ffi.Pointer<Utf8>, ffi.Int32);
typedef SetProtectionProxyAuditOnlyByAssetDart =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>, ffi.Pointer<Utf8>, int);

typedef UpdateProtectionConfigC = ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);
typedef UpdateProtectionConfigDart =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);

typedef UpdateProtectionConfigByAssetC =
    ffi.Pointer<Utf8> Function(
      ffi.Pointer<Utf8>,
      ffi.Pointer<Utf8>,
      ffi.Pointer<Utf8>,
    );
typedef UpdateProtectionConfigByAssetDart =
    ffi.Pointer<Utf8> Function(
      ffi.Pointer<Utf8>,
      ffi.Pointer<Utf8>,
      ffi.Pointer<Utf8>,
    );

typedef UpdateSecurityModelConfigC =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);
typedef UpdateSecurityModelConfigDart =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);

typedef UpdateSecurityModelConfigByAssetC =
    ffi.Pointer<Utf8> Function(
      ffi.Pointer<Utf8>,
      ffi.Pointer<Utf8>,
      ffi.Pointer<Utf8>,
    );
typedef UpdateSecurityModelConfigByAssetDart =
    ffi.Pointer<Utf8> Function(
      ffi.Pointer<Utf8>,
      ffi.Pointer<Utf8>,
      ffi.Pointer<Utf8>,
    );

// 【已移除】ApplyOpenclawConfigForProtection 不再暴露 FFI，网关启动逻辑已内聚到 StartProtectionProxy
// typedef ApplyOpenclawConfigForProtectionC = ...

typedef GetProtectionProxyLogsC = ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);
typedef GetProtectionProxyLogsDart =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);

typedef ResetProtectionStatisticsC = ffi.Pointer<Utf8> Function();
typedef ResetProtectionStatisticsDart = ffi.Pointer<Utf8> Function();

// --- 审计日志缓冲 ---
typedef GetAuditLogsC =
    ffi.Pointer<Utf8> Function(ffi.Int32, ffi.Int32, ffi.Int32);
typedef GetAuditLogsDart = ffi.Pointer<Utf8> Function(int, int, int);

typedef GetPendingAuditLogsC = ffi.Pointer<Utf8> Function();
typedef GetPendingAuditLogsDart = ffi.Pointer<Utf8> Function();

typedef ClearAuditLogsC = ffi.Pointer<Utf8> Function();
typedef ClearAuditLogsDart = ffi.Pointer<Utf8> Function();

typedef ClearAuditLogsWithFilterC =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);
typedef ClearAuditLogsWithFilterDart =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8>);

// --- 安全事件缓冲 ---
typedef GetPendingSecurityEventsC = ffi.Pointer<Utf8> Function();
typedef GetPendingSecurityEventsDart = ffi.Pointer<Utf8> Function();

typedef ClearSecurityEventsBufferC = ffi.Pointer<Utf8> Function();
typedef ClearSecurityEventsBufferDart = ffi.Pointer<Utf8> Function();

// --- 通用 ---
typedef FreeStringC = ffi.Void Function(ffi.Pointer<Utf8>);
typedef FreeStringDart = void Function(ffi.Pointer<Utf8>);

// --- 配置恢复 ---
typedef HasInitialBackupFFIC = ffi.Pointer<Utf8> Function();
typedef HasInitialBackupFFIDart = ffi.Pointer<Utf8> Function();

typedef HasInitialBackupByAssetFFIC =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> assetName);
typedef HasInitialBackupByAssetFFIDart =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> assetName);

typedef RestoreToInitialConfigFFIC = ffi.Pointer<Utf8> Function();
typedef RestoreToInitialConfigFFIDart = ffi.Pointer<Utf8> Function();

typedef RestoreToInitialConfigByAssetFFIC =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> assetName);
typedef RestoreToInitialConfigByAssetFFIDart =
    ffi.Pointer<Utf8> Function(ffi.Pointer<Utf8> assetName);

// --- 网关沙箱同步 ---
typedef SyncGatewaySandboxC = ffi.Pointer<Utf8> Function();
typedef SyncGatewaySandboxDart = ffi.Pointer<Utf8> Function();

typedef SyncGatewaySandboxByAssetNameC =
    ffi.Pointer<Utf8> Function(
      ffi.Pointer<Utf8> assetName,
      ffi.Pointer<Utf8> assetID,
    );
typedef SyncGatewaySandboxByAssetNameDart =
    ffi.Pointer<Utf8> Function(
      ffi.Pointer<Utf8> assetName,
      ffi.Pointer<Utf8> assetID,
    );

/// 防护代理 FFI 的 Isolate 安全调用入口（静态方法可在后台 Isolate 中使用）。
class ProtectionProxyFFI {
  ProtectionProxyFFI._();

  /// 在后台 Isolate 中执行 StartProtectionProxy。
  /// [libPath] 为插件 dylib 路径，[configJSON] 为 JSON 字符串。
  /// 返回 Go 层返回的 JSON 字符串。
  static String startProtectionProxyInIsolate(
    String libPath,
    String configJSON,
  ) {
    final dylib = ffi.DynamicLibrary.open(libPath);
    final startProxy = dylib
        .lookupFunction<StartProtectionProxyC, StartProtectionProxyDart>(
          'StartProtectionProxy',
        );
    final freeString = dylib.lookupFunction<FreeStringC, FreeStringDart>(
      'FreeString',
    );

    final configPtr = configJSON.toNativeUtf8();
    final resultPtr = startProxy(configPtr);
    malloc.free(configPtr);

    final resultStr = resultPtr.toDartString();
    freeString(resultPtr);
    return resultStr;
  }

  /// 在后台 Isolate 中执行 SyncGatewaySandbox（网关完整重启，耗时较长）。
  /// [libPath] 为插件 dylib 路径。
  /// 返回 Go 层返回的 JSON 字符串。
  static String syncGatewaySandboxInIsolate(String libPath) {
    final dylib = ffi.DynamicLibrary.open(libPath);
    final syncFunc = dylib
        .lookupFunction<SyncGatewaySandboxC, SyncGatewaySandboxDart>(
          'SyncGatewaySandbox',
        );
    final freeString = dylib.lookupFunction<FreeStringC, FreeStringDart>(
      'FreeString',
    );

    final resultPtr = syncFunc();
    final resultStr = resultPtr.toDartString();
    freeString(resultPtr);
    return resultStr;
  }

  /// 在后台 Isolate 中执行按资产路由的网关沙箱同步。
  static String syncGatewaySandboxByAssetInIsolate(
    String libPath,
    String assetName,
    String assetID,
  ) {
    final dylib = ffi.DynamicLibrary.open(libPath);
    final syncFunc = dylib
        .lookupFunction<
          SyncGatewaySandboxByAssetNameC,
          SyncGatewaySandboxByAssetNameDart
        >('SyncGatewaySandboxByAssetName');
    final freeString = dylib.lookupFunction<FreeStringC, FreeStringDart>(
      'FreeString',
    );

    final assetNamePtr = assetName.toNativeUtf8();
    final assetIDPtr = assetID.toNativeUtf8();
    final resultPtr = syncFunc(assetNamePtr, assetIDPtr);
    malloc.free(assetNamePtr);
    malloc.free(assetIDPtr);

    final resultStr = resultPtr.toDartString();
    freeString(resultPtr);
    return resultStr;
  }

  /// 在后台 Isolate 中执行 RestoreToInitialConfigFFI。
  /// [libPath] 为插件 dylib 路径。
  /// 返回 Go 层返回的 JSON 字符串。
  static String restoreToInitialConfigInIsolate(String libPath) {
    final dylib = ffi.DynamicLibrary.open(libPath);
    final restoreFunc = dylib
        .lookupFunction<
          RestoreToInitialConfigFFIC,
          RestoreToInitialConfigFFIDart
        >('RestoreToInitialConfigFFI');
    final freeString = dylib.lookupFunction<FreeStringC, FreeStringDart>(
      'FreeString',
    );

    final resultPtr = restoreFunc();
    final resultStr = resultPtr.toDartString();
    freeString(resultPtr);
    return resultStr;
  }

  /// 在后台 Isolate 中执行按资产路由的配置恢复。
  static String restoreToInitialConfigByAssetInIsolate(
    String libPath,
    String assetName,
  ) {
    final dylib = ffi.DynamicLibrary.open(libPath);
    final restoreFunc = dylib
        .lookupFunction<
          RestoreToInitialConfigByAssetFFIC,
          RestoreToInitialConfigByAssetFFIDart
        >('RestoreToInitialConfigByAssetFFI');
    final freeString = dylib.lookupFunction<FreeStringC, FreeStringDart>(
      'FreeString',
    );

    final assetNamePtr = assetName.toNativeUtf8();
    final resultPtr = restoreFunc(assetNamePtr);
    malloc.free(assetNamePtr);

    final resultStr = resultPtr.toDartString();
    freeString(resultPtr);
    return resultStr;
  }
}
