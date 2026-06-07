import 'dart:typed_data';
import 'package:heliolytics/utils/huami_time.dart';

class HrvSample {
  final Uint8List timestampBytes;
  final DateTime timestamp;
  final int rmssd, unknown;
  const HrvSample({
    required this.timestampBytes,
    required this.timestamp,
    required this.rmssd,
    required this.unknown,
  });
}

class HrvParser {
  static List<HrvSample> parse(Uint8List bytes) {
    if (bytes.isEmpty) return [];
    if (bytes.length % 6 != 0) {
      throw ArgumentError(
        'HRV payload length ${bytes.length} is not a multiple of 6',
      );
    }
    final out = <HrvSample>[];
    for (var i = 0; i < bytes.length; i += 6) {
      final ts = Uint8List.fromList(bytes.sublist(i, i + 4));
      out.add(HrvSample(
        timestampBytes: ts,
        timestamp: HuamiTime.toDateTime(ts),
        rmssd: bytes[i + 4],
        unknown: bytes[i + 5],
      ));
    }
    return out;
  }
}
