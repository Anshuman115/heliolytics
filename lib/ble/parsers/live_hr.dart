import 'dart:typed_data';
import 'package:heliolytics/ble/parsers/unknown.dart';

/// Live heart rate (0x2a37) — standard BLE HR profile.
class LiveHrParser {
  static List<UnknownSample> parse(Uint8List bytes) =>
      UnknownParser.parse('0x2a37', bytes);
}
