import 'package:flutter_test/flutter_test.dart';
import 'package:heliolytics/auth/auth_key_storage.dart';
import 'package:heliolytics/auth/auth_key_store.dart';

class InMemoryStore implements AuthKeyStore {
  final Map<String, String> _m = {};
  @override Future<String?> read(String k) async => _m[k];
  @override Future<void> write(String k, String v) async => _m[k] = v;
  @override Future<void> delete(String k) async => _m.remove(k);
}

void main() {
  late InMemoryStore backend;
  late AuthKeyStorage storage;

  setUp(() {
    backend = InMemoryStore();
    storage = AuthKeyStorage(store: backend);
  });

  test('save normalizes and persists the key', () async {
    await storage.save('AABBCC11223344556677889900AABBCC');
    expect(await storage.read(), 'aabbcc11223344556677889900aabbcc');
  });

  test('save rejects invalid key', () async {
    expect(() => storage.save('not-hex'), throwsA(isA<FormatException>()));
  });

  test('read returns null when nothing stored', () async {
    expect(await storage.read(), isNull);
  });

  test('clear removes stored key', () async {
    await storage.save('aabbcc11223344556677889900aabbcc');
    await storage.clear();
    expect(await storage.read(), isNull);
  });

  test('hasKey returns true after save', () async {
    expect(await storage.hasKey(), isFalse);
    await storage.save('aabbcc11223344556677889900aabbcc');
    expect(await storage.hasKey(), isTrue);
  });
}
