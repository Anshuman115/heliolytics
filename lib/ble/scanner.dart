import 'package:heliolytics/ble/ble_devices.dart';

class StubScanner implements BleScanner {
  @override
  Stream<DiscoveredDevice> scan({Duration? timeout}) async* {
    throw UnimplementedError('Scanner not yet wired to flutter_blue_plus');
  }

  @override
  Future<void> stop() async {}
}

BleScanner scannerProvider() => StubScanner();
