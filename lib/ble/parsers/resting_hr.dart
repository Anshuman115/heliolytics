import 'dart:typed_data';
import 'package:heliolytics/ble/parsers/unknown.dart';

/// Resting HR (0x3A) — format not yet confirmed.
class RestingHrParser {
  static List<UnknownSample> parse(Uint8List bytes) =>
      UnknownParser.parse('0x3A', bytes);
}
