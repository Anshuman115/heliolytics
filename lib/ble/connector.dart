import 'package:heliolytics/ble/ble_devices.dart';

class StubConnector implements BleConnector {
  @override
  Future<GattConnection> connect(String remoteId) async {
    throw UnimplementedError('Connector not yet wired to flutter_blue_plus');
  }
}

BleConnector connectorProvider() => StubConnector();
