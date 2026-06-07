import 'dart:typed_data';
import 'package:flutter_test/flutter_test.dart';
import 'package:heliolytics/ble/parsers/unknown.dart';

void main() {
  test('returns a single sample with raw bytes preserved', () {
    final b = Uint8List.fromList([0xDE, 0xAD, 0xBE, 0xEF]);
    final s = UnknownParser.parse('0x1A', b);
    expect(s.length, 1);
    expect(s[0].typeCode, '0x1A');
    expect(s[0].rawBytes, b);
  });
  test('captures a short hex preview of first bytes', () {
    final b = Uint8List.fromList(List.generate(20, (i) => i));
    final s = UnknownParser.parse('0x2A', b);
    expect(s[0].firstBytesHex.length, 32); // first 16 bytes = 32 hex chars
  });
  test('handles empty input', () {
    final s = UnknownParser.parse('0x00', Uint8List(0));
    expect(s.length, 1);
    expect(s[0].rawBytes.isEmpty, isTrue);
  });
}
