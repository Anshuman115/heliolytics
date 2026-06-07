import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:heliolytics/ble/ble_devices.dart';

final bleConnectorProvider = Provider<BleConnector>(
  (ref) => connectorProvider(),
);

class StubConnector implements BleConnector {
  @override
  Future<GattConnection> connect(String remoteId) async {
    throw UnimplementedError('Connector not yet wired to flutter_blue_plus');
  }
}

BleConnector connectorProvider() => StubConnector();
