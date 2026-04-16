import 'botsec_transport.dart';
import 'default_transport_io.dart'
    if (dart.library.html) 'default_transport_web.dart' as default_transport;

/// Global transport registry.
///
/// Desktop defaults to [FfiTransport]. Web will override with HTTP transport.
class TransportRegistry {
  TransportRegistry._();

  static BotsecTransport _transport = default_transport.createDefaultTransport();

  static BotsecTransport get transport => _transport;

  static void setTransport(BotsecTransport transport) {
    _transport = transport;
  }
}
