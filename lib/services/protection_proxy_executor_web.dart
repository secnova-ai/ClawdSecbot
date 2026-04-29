import '../core_transport/transport_registry.dart';

class ProtectionProxyExecutor {
  ProtectionProxyExecutor._();

  static Future<Map<String, dynamic>> start(String configJSON) async {
    final transport = TransportRegistry.transport;
    if (!transport.isReady) {
      return {'success': false, 'error': 'Transport not initialized'};
    }
    return transport.callOneArgAsync('StartProtectionProxy', configJSON);
  }

  static Future<Map<String, dynamic>> stop({required String assetID}) async {
    final transport = TransportRegistry.transport;
    if (!transport.isReady) {
      return {'success': false, 'error': 'Transport not initialized'};
    }

    final scopedAssetID = assetID.trim();
    if (scopedAssetID.isNotEmpty) {
      return transport.callOneArgAsync(
        'StopProtectionProxyByAsset',
        scopedAssetID,
      );
    }
    return transport.callNoArgAsync('StopProtectionProxy');
  }

  static Future<Map<String, dynamic>> status({required String assetID}) async {
    final transport = TransportRegistry.transport;
    if (!transport.isReady) {
      return {'running': false, 'error': 'transport not ready'};
    }

    final scopedAssetID = assetID.trim();
    if (scopedAssetID.isNotEmpty) {
      return transport.callOneArgAsync(
        'GetProtectionProxyStatusByAsset',
        scopedAssetID,
      );
    }
    return transport.callNoArgAsync('GetProtectionProxyStatus');
  }
}
