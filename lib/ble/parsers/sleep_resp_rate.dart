import 'dart:typed_data';
import 'package:heliolytics/ble/parsers/unknown.dart';

/// Sleep respiratory rate (0x38) — format not yet confirmed.
class SleepRespRateParser {
  static List<UnknownSample> parse(Uint8List bytes) =>
      UnknownParser.parse('0x38', bytes);
}
