import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:heliolytics/ble/ble_devices.dart';

final bleScannerProvider = Provider<BleScanner>(
  (ref) => scannerProvider(),
);

class StubScanner implements BleScanner {
  @override
  Stream<DiscoveredDevice> scan({Duration? timeout}) async* {
    throw UnimplementedError('Scanner not yet wired to flutter_blue_plus');
  }

  @override
  Future<void> stop() async {}
}

BleScanner scannerProvider() => StubScanner();
