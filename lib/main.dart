import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';

import 'package:heliolytics/app.dart';
import 'package:heliolytics/auth/auth_key_storage.dart';
import 'package:heliolytics/auth/auth_key_store.dart';

class _SecureStore implements AuthKeyStore {
  final _s = const FlutterSecureStorage(
    aOptions: AndroidOptions(encryptedSharedPreferences: true),
  );

  @override
  Future<String?> read(String key) => _s.read(key: key);

  @override
  Future<void> write(String key, String value) => _s.write(key: key, value: value);

  @override
  Future<void> delete(String key) => _s.delete(key: key);
}

void main() {
  runApp(
    ProviderScope(
      overrides: [
        authKeyStoreProvider.overrideWithValue(_SecureStore()),
      ],
      child: const HeliolyticsApp(),
    ),
  );
}
