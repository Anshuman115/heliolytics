import 'dart:async';
import 'dart:io';
import 'dart:typed_data';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:heliolytics/auth/auth_key_storage.dart';
import 'package:heliolytics/auth/auth_key_store.dart';
import 'package:heliolytics/ble/ble_devices.dart';
import 'package:heliolytics/ble/connector.dart';
import 'package:heliolytics/ble/scanner.dart';
import 'package:heliolytics/ble/session_controller.dart';
import 'package:heliolytics/ble/session_state.dart';
import 'package:heliolytics/data/session_store.dart';

class _MemStore implements AuthKeyStore {
  String? k;
  @override
  Future<String?> read(String key) async => k;
  @override
  Future<void> write(String key, String value) async => k = value;
  @override
  Future<void> delete(String key) async => k = null;
}

class _FakeScanner implements BleScanner {
  final _ctrl = StreamController<DiscoveredDevice>.broadcast();
  @override
  Stream<DiscoveredDevice> scan({Duration? timeout}) => _ctrl.stream;
  @override
  Future<void> stop() async => _ctrl.close();
}

class _FakeGatt implements GattConnection {
  final _out = StreamController<Uint8List>.broadcast();
  final writes = <Uint8List>[];
  @override
  Stream<Uint8List> get incoming => _out.stream;
  @override
  Future<List<BleCharacteristic>> discoverCharacteristics() async => [];
  @override
  Future<void> writeChunked(Uint8List bytes) async => writes.add(bytes);
  @override
  Future<void> dispose() async => _out.close();
}

class _FakeConnector implements BleConnector {
  @override
  Future<GattConnection> connect(String remoteId) async => _FakeGatt();
}

void main() {
  late ProviderContainer container;
  late _MemStore mem;
  late Directory tmp;

  setUp(() {
    mem = _MemStore();
    tmp = Directory.systemTemp.createTempSync('sc_test_');
    container = ProviderContainer(
      overrides: [
        authKeyStoreProvider.overrideWithValue(mem),
        bleScannerProvider.overrideWithValue(_FakeScanner()),
        bleConnectorProvider.overrideWithValue(_FakeConnector()),
        sessionStoreProvider.overrideWith(
          (ref) async => SessionStore(rootDir: tmp),
        ),
      ],
    );
  });

  tearDown(() {
    container.dispose();
    if (tmp.existsSync()) tmp.deleteSync(recursive: true);
  });

  test('initial state is noAuthKey when no key saved', () {
    expect(
      container.read(sessionControllerProvider).state,
      SessionState.noAuthKey,
    );
  });

  test('saving a valid key transitions to idle', () async {
    await container
        .read(sessionControllerProvider.notifier)
        .saveAuthKey('a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6');
    expect(
      container.read(sessionControllerProvider).state,
      SessionState.idle,
    );
  });

  test('saving invalid key throws and does not transition', () async {
    final notifier = container.read(sessionControllerProvider.notifier);
    expect(
      () => notifier.saveAuthKey('not-hex'),
      throwsA(isA<FormatException>()),
    );
    expect(
      container.read(sessionControllerProvider).state,
      SessionState.noAuthKey,
    );
  });
}
