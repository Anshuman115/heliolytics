import 'dart:typed_data';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:heliolytics/auth/auth_key_store.dart';
import 'package:heliolytics/auth/auth_key_validator.dart';
import 'package:heliolytics/config/constants.dart';

final authKeyStoreProvider = Provider<AuthKeyStore>(
  (ref) => throw UnimplementedError(
      'Override authKeyStoreProvider in ProviderScope'),
);

class AuthKeyStorage {
  final AuthKeyStore _store;
  AuthKeyStorage({required AuthKeyStore store}) : _store = store;

  Future<void> save(String key) async {
    final reason = AuthKeyValidator.validate(key);
    if (reason != null) throw FormatException('Invalid auth key: $reason');
    await _store.write(authKeyStorageKey, AuthKeyValidator.normalize(key));
  }

  Future<String?> read() => _store.read(authKeyStorageKey);

  Future<Uint8List?> readBytes() async {
    final s = await read();
    return s == null ? null : AuthKeyValidator.toBytes(s);
  }

  Future<bool> hasKey() async => (await read()) != null;

  Future<void> clear() => _store.delete(authKeyStorageKey);
}
