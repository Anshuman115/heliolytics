import 'dart:typed_data';
import 'package:heliolytics/ble/parsers/unknown.dart';

/// Temperature (0x2E) — format not yet confirmed.
class TemperatureParser {
  static List<UnknownSample> parse(Uint8List bytes) =>
      UnknownParser.parse('0x2E', bytes);
}
