import 'dart:typed_data';
import 'package:heliolytics/ble/parsers/unknown.dart';

/// Sleep session (0x48) — 594-byte blob per docs, exact structure TBD.
class SleepSessionParser {
  static List<UnknownSample> parse(Uint8List bytes) =>
      UnknownParser.parse('0x48', bytes);
}
