import 'dart:typed_data';
import 'package:heliolytics/ble/parsers/unknown.dart';

/// Workout (0x05) — TBD. Defers to UnknownParser until real format is known.
class WorkoutParser {
  static List<UnknownSample> parse(Uint8List bytes) =>
      UnknownParser.parse('0x05', bytes);
}
