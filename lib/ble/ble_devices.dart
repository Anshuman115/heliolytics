import 'dart:typed_data';

class DiscoveredDevice {
  final String remoteId, name;
  final int rssi;
  const DiscoveredDevice({
    required this.remoteId,
    required this.name,
    required this.rssi,
  });
}

class BleCharacteristic {
  final String uuid;
  final bool canNotify, canWriteWithoutResponse;
  const BleCharacteristic({
    required this.uuid,
    required this.canNotify,
    required this.canWriteWithoutResponse,
  });
}

abstract class GattConnection {
  Future<List<BleCharacteristic>> discoverCharacteristics();
  Future<void> writeChunked(Uint8List bytes);
  Stream<Uint8List> get incoming;
  Future<void> dispose();
}

abstract class BleScanner {
  Stream<DiscoveredDevice> scan({Duration? timeout});
  Future<void> stop();
}

abstract class BleConnector {
  Future<GattConnection> connect(String remoteId);
}
