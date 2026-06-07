import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:heliolytics/auth/auth_key_storage.dart';
import 'package:heliolytics/ble/ble_devices.dart';
import 'package:heliolytics/ble/connector.dart';
import 'package:heliolytics/ble/ecdh_auth.dart';
import 'package:heliolytics/ble/scanner.dart';
import 'package:heliolytics/ble/session_state.dart';
import 'package:heliolytics/config/constants.dart';
import 'package:heliolytics/data/session_store.dart';

class SessionController extends Notifier<SessionSnapshot> {
  late AuthKeyStorage _authStorage;
  late BleScanner _scanner;
  late BleConnector _connector;
  // ignore: unused_field — wired in Task 27
  SessionStore? _store;
  StreamSubscription<DiscoveredDevice>? _scanSub;
  GattConnection? _gatt;

  @override
  SessionSnapshot build() {
    _authStorage = AuthKeyStorage(store: ref.read(authKeyStoreProvider));
    _scanner = ref.read(bleScannerProvider);
    _connector = ref.read(bleConnectorProvider);
    _initAsync();
    return SessionSnapshot.initial;
  }

  Future<void> _initAsync() async {
    _store = await ref.read(sessionStoreProvider.future);
    final hasKey = await _authStorage.hasKey();
    state = state.copyWith(
      state: hasKey ? SessionState.idle : SessionState.noAuthKey,
    );
  }

  Future<void> saveAuthKey(String key) async {
    await _authStorage.save(key); // throws FormatException on invalid
    state = state.copyWith(state: SessionState.idle);
  }

  Future<void> clearAuthKey() async {
    await _authStorage.clear();
    state = state.copyWith(state: SessionState.noAuthKey);
  }

  Future<void> scan() async {
    if (state.state != SessionState.idle) return;
    state = state.copyWith(
        state: SessionState.scanning, error: SessionError.none);
    final completer = Completer<String?>();
    _scanSub = _scanner
        .scan(timeout: const Duration(seconds: scanTimeoutSec))
        .listen((d) => completer.complete(d.remoteId));
    final id = await completer.future.timeout(
      const Duration(seconds: scanTimeoutSec + 1),
      onTimeout: () {
        _scanSub?.cancel();
        state = state.copyWith(
          state: SessionState.error,
          error: SessionError.scanTimeout,
        );
        return null;
      },
    );
    await _scanSub?.cancel();
    if (id == null) return;
    await _connectAndAuth(id);
  }

  Future<void> _connectAndAuth(String remoteId) async {
    state = state.copyWith(state: SessionState.connecting);
    try {
      _gatt = await _connector.connect(remoteId);
    } catch (_) {
      state = state.copyWith(
        state: SessionState.error,
        error: SessionError.gattFailed,
      );
      return;
    }
    state = state.copyWith(state: SessionState.authenticating);
    try {
      await _runEcdhAuth();
      state = state.copyWith(state: SessionState.connected);
    } catch (_) {
      state = state.copyWith(
        state: SessionState.error,
        error: SessionError.authRejected,
      );
    }
  }

  /// ECDH + AES challenge flow. The real ring wiring is done in Task 26.
  Future<void> _runEcdhAuth() async {
    final kp = EcdhAuth.generateKeypair();
    final authKey = await _authStorage.readBytes();
    if (authKey == null) throw StateError('No auth key');
    final payload = EcdhAuth.buildAuthPayload(kp.publicKey);
    await _gatt!.writeChunked(payload);
    // Real response handling wired in Task 26.
    throw UnimplementedError('Real ECDH auth over BLE wired in Task 26');
  }

  /// Stub startFetch. Full implementation wired in Task 27.
  Future<void> startFetch({
    required List<String> typeCodes,
    required int fetchWindowHours,
    required int listenDurationSec,
  }) async {
    if (state.state != SessionState.connected) return;
    state = state.copyWith(state: SessionState.fetching);
    // Full DataRequester loop wired in Task 27
    state = state.copyWith(state: SessionState.idle);
  }
}

final sessionControllerProvider =
    NotifierProvider<SessionController, SessionSnapshot>(SessionController.new);
