import 'dart:io';
import 'package:flutter_test/flutter_test.dart';
import 'package:heliolytics/data/models.dart';
import 'package:heliolytics/data/session_store.dart';
import 'package:path/path.dart' as p;

void main() {
  late Directory tmpRoot;

  setUp(() async {
    tmpRoot = await Directory.systemTemp.createTemp('heliolytics_test_');
  });
  tearDown(() async {
    if (tmpRoot.existsSync()) await tmpRoot.delete(recursive: true);
  });

  test('createSession returns id and creates folder', () async {
    final s = SessionStore(rootDir: tmpRoot);
    final id = await s.createSession(
      deviceMac: 'AA:BB:CC:DD:EE:FF',
      fetchWindowHours: 48,
      listenDurationSec: 300,
      mode: SessionMode.fetchAndListen,
    );
    expect(id, isNotEmpty);
    expect(Directory(p.join(tmpRoot.path, 'sessions', id)).existsSync(), isTrue);
  });

  test('appendBytes writes raw bytes to per-type file', () async {
    final s = SessionStore(rootDir: tmpRoot);
    final id = await s.createSession(
      deviceMac: null,
      fetchWindowHours: 48,
      listenDurationSec: 0,
      mode: SessionMode.fetchAndListen,
    );
    await s.appendBytes(id, '0x01', [0x01, 0x02, 0x03]);
    await s.appendBytes(id, '0x01', [0x04, 0x05]);
    final f = File(p.join(tmpRoot.path, 'sessions', id, '0x01_activity.bin'));
    expect(f.existsSync(), isTrue);
    expect(f.readAsBytesSync(), [0x01, 0x02, 0x03, 0x04, 0x05]);
  });

  test('writeSessionJson and readSessionJson round-trip', () async {
    final s = SessionStore(rootDir: tmpRoot);
    final id = await s.createSession(
      deviceMac: 'AA:BB:CC:DD:EE:FF',
      fetchWindowHours: 48,
      listenDurationSec: 300,
      mode: SessionMode.fetchAndListen,
    );
    final session = Session(
      sessionId: id,
      startedAt: DateTime.utc(2026, 6, 7, 14, 32),
      deviceMac: 'AA:BB:CC:DD:EE:FF',
      fetchWindowHours: 48,
      listenDurationSec: 300,
      mode: SessionMode.fetchAndListen,
      entries: const [],
      unsolicited: const [],
    );
    await s.writeSessionJson(session);
    final read = await s.readSessionJson(id);
    expect(read.sessionId, id);
    expect(read.deviceMac, 'AA:BB:CC:DD:EE:FF');
  });

  test('writeCatalogJson and readCatalogJson round-trip', () async {
    final s = SessionStore(rootDir: tmpRoot);
    final id = await s.createSession(
      deviceMac: null,
      fetchWindowHours: 48,
      listenDurationSec: 0,
      mode: SessionMode.fetchAndListen,
    );
    final c = SessionCatalog(
      sessionId: id,
      chunked: const [
        DumpEntry(
          code: '0x01',
          status: DumpStatus.ok,
          samples: 142,
          bytes: 568,
          file: '0x01_activity.bin',
        ),
      ],
      unsolicited: const [],
    );
    await s.writeCatalogJson(c);
    final read = await s.readCatalogJson(id);
    expect(read.chunked.first.code, '0x01');
    expect(read.chunked.first.samples, 142);
  });

  test('listSessions returns reverse chronological', () async {
    final s = SessionStore(rootDir: tmpRoot);
    final id1 = await s.createSession(
      deviceMac: null,
      fetchWindowHours: 48,
      listenDurationSec: 0,
      mode: SessionMode.fetchAndListen,
    );
    await Future<void>.delayed(const Duration(milliseconds: 5));
    final id2 = await s.createSession(
      deviceMac: null,
      fetchWindowHours: 48,
      listenDurationSec: 0,
      mode: SessionMode.fetchAndListen,
    );
    final list = await s.listSessions();
    expect(list, [id2, id1]);
  });
}
