import 'dart:typed_data';
import 'package:heliolytics/ble/parsers/unknown.dart';

/// Max HR (0x3D) — format not yet confirmed.
class MaxHrParser {
  static List<UnknownSample> parse(Uint8List bytes) =>
      UnknownParser.parse('0x3D', bytes);
}
