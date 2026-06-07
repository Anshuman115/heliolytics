import 'dart:typed_data';
import 'package:heliolytics/ble/parsers/unknown.dart';

/// SpO2 (0x25) — format not yet confirmed.
class Spo2Parser {
  static List<UnknownSample> parse(Uint8List bytes) =>
      UnknownParser.parse('0x25', bytes);
}
