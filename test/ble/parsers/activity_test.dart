import 'dart:typed_data';
import 'package:flutter_test/flutter_test.dart';
import 'package:heliolytics/ble/parsers/activity.dart';

void main() {
  test('parses a single 4-byte sample', () {
    final bytes = Uint8List.fromList([0x01, 0x40, 0xAB, 0x48]);
    final s = ActivityParser.parse(bytes);
    expect(s.length, 1);
    expect(s[0].kind, 0x01);
    expect(s[0].intensity, 0x40);
    expect(s[0].steps, 0xAB);
    expect(s[0].heartRate, 72);
  });
  test('parses multiple 4-byte samples', () {
    final bytes = Uint8List.fromList([
      0x01, 0x40, 0x0A, 0x48,
      0x01, 0x80, 0x14, 0x50,
    ]);
    final s = ActivityParser.parse(bytes);
    expect(s.length, 2);
    expect(s[1].heartRate, 80);
  });
  test('empty input returns empty list', () {
    expect(ActivityParser.parse(Uint8List(0)), isEmpty);
  });
  test('throws on non-multiple-of-4 length', () {
    expect(() => ActivityParser.parse(Uint8List.fromList([0x01, 0x40, 0x00])),
        throwsArgumentError);
  });
}
