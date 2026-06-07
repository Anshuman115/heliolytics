import 'dart:typed_data';
import 'package:flutter_test/flutter_test.dart';
import 'package:heliolytics/ble/parsers/hrv.dart';

void main() {
  test('parses a single 6-byte sample', () {
    // timestamp (4 bytes BE) for 2026-06-07 + rmssd=0x42 + unknown=0xFF
    final seconds = DateTime.utc(2026, 6, 7).millisecondsSinceEpoch ~/ 1000;
    final tsBytes = [
      (seconds >> 24) & 0xFF,
      (seconds >> 16) & 0xFF,
      (seconds >> 8) & 0xFF,
      seconds & 0xFF,
    ];
    final bytes = Uint8List.fromList([...tsBytes, 0x42, 0xFF]);
    final s = HrvParser.parse(bytes);
    expect(s.length, 1);
    expect(s[0].rmssd, 0x42);
    expect(s[0].unknown, 0xFF);
    expect(s[0].timestamp.isAtSameMomentAs(DateTime.utc(2026, 6, 7)), isTrue);
  });
  test('parses multiple samples', () {
    final bytes = Uint8List.fromList([
      0x00, 0x00, 0x00, 0x00, 0x40, 0x00,
      0x00, 0x00, 0x00, 0x01, 0x50, 0x00,
    ]);
    expect(HrvParser.parse(bytes).length, 2);
  });
  test('empty input returns empty list', () {
    expect(HrvParser.parse(Uint8List(0)), isEmpty);
  });
  test('throws on non-multiple-of-6 length', () {
    expect(() => HrvParser.parse(Uint8List.fromList([0, 0, 0, 0, 0])),
        throwsArgumentError);
  });
}
