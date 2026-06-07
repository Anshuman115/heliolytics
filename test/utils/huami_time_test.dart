import 'dart:typed_data';
import 'package:flutter_test/flutter_test.dart';
import 'package:heliolytics/utils/huami_time.dart';

void main() {
  test('encodes unix epoch to 4 zero bytes', () {
    final b = HuamiTime.fromDateTime(DateTime.utc(1970, 1, 1));
    expect(b, Uint8List.fromList([0, 0, 0, 0]));
  });
  test('encodes 2026-06-07 to expected bytes', () {
    // 2026-06-07 00:00:00 UTC = 1780185600 seconds
    final b = HuamiTime.fromDateTime(DateTime.utc(2026, 6, 7));
    final seconds = DateTime.utc(2026, 6, 7).millisecondsSinceEpoch ~/ 1000;
    final expected = [
      (seconds >> 24) & 0xFF,
      (seconds >> 16) & 0xFF,
      (seconds >> 8) & 0xFF,
      seconds & 0xFF,
    ];
    expect(b, Uint8List.fromList(expected));
  });
  test('decodes 4 zero bytes to unix epoch', () {
    expect(HuamiTime.toDateTime(Uint8List.fromList([0, 0, 0, 0]))
        .isAtSameMomentAs(DateTime.utc(1970, 1, 1)), isTrue);
  });
  test('roundtrips a known timestamp', () {
    final orig = DateTime.utc(2026, 6, 7, 14, 32);
    expect(HuamiTime.toDateTime(HuamiTime.fromDateTime(orig)).isAtSameMomentAs(orig), isTrue);
  });
}
