import 'dart:typed_data';
import 'package:heliolytics/ble/parsers/unknown.dart';

/// Stress (0x13) — format not yet confirmed.
class StressParser {
  static List<UnknownSample> parse(Uint8List bytes) =>
      UnknownParser.parse('0x13', bytes);
}
