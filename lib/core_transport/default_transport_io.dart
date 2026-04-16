import 'botsec_transport.dart';
import 'ffi_transport.dart';

BotsecTransport createDefaultTransport() => FfiTransport.instance;
