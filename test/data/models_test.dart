import 'dart:convert';
import 'package:flutter_test/flutter_test.dart';
import 'package:heliolytics/data/models.dart';

void main() {
  test('DumpEntry.toJson includes schemaVersion and all fields', () {
    final e = DumpEntry(
      code: '0x01',
      status: DumpStatus.ok,
      samples: 142,
      bytes: 568,
      file: '0x01_activity.bin',
    );
    final j = e.toJson();
    expect(j['schemaVersion'], 1);
    expect(j['code'], '0x01');
    expect(j['status'], 'ok');
    expect(j['samples'], 142);
    expect(j['bytes'], 568);
    expect(j['file'], '0x01_activity.bin');
  });
  test('DumpEntry round-trips through JSON', () {
    final orig = DumpEntry(
      code: '0x05',
      status: DumpStatus.rejected,
      samples: 0,
      bytes: 0,
      errorByte: '0x04',
    );
    final round = DumpEntry.fromJson(
        jsonDecode(jsonEncode(orig.toJson())) as Map<String, dynamic>);
    expect(round.code, '0x05');
    expect(round.status, DumpStatus.rejected);
    expect(round.errorByte, '0x04');
  });
  test('Session.toJson includes schemaVersion and metadata', () {
    final s = Session(
      sessionId: 'abc-123',
      startedAt: DateTime.utc(2026, 6, 7, 14, 32),
      endedAt: DateTime.utc(2026, 6, 7, 14, 38, 12),
      deviceMac: 'AA:BB:CC:DD:EE:FF',
      fetchWindowHours: 48,
      listenDurationSec: 300,
      mode: SessionMode.fetchAndListen,
      entries: const [],
      unsolicited: const [],
    );
    final j = s.toJson();
    expect(j['schemaVersion'], 1);
    expect(j['sessionId'], 'abc-123');
    expect(j['mode'], 'fetch+listen');
  });
  test('SessionCatalog round-trips', () {
    final c = SessionCatalog(
      sessionId: 'sid',
      chunked: const [
        DumpEntry(code: '0x01', status: DumpStatus.ok, samples: 5, bytes: 20),
      ],
      unsolicited: const [],
    );
    final round = SessionCatalog.fromJson(
        jsonDecode(jsonEncode(c.toJson())) as Map<String, dynamic>);
    expect(round.sessionId, 'sid');
    expect(round.chunked.first.code, '0x01');
    expect(round.chunked.first.samples, 5);
  });
}
