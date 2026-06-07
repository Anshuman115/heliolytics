import 'dart:typed_data';

class ActivitySample {
  final int kind, intensity, steps, heartRate;
  const ActivitySample({
    required this.kind,
    required this.intensity,
    required this.steps,
    required this.heartRate,
  });
}

class ActivityParser {
  static List<ActivitySample> parse(Uint8List bytes) {
    if (bytes.isEmpty) return [];
    if (bytes.length % 4 != 0) {
      throw ArgumentError(
        'Activity payload length ${bytes.length} is not a multiple of 4',
      );
    }
    final out = <ActivitySample>[];
    for (var i = 0; i < bytes.length; i += 4) {
      out.add(ActivitySample(
        kind: bytes[i],
        intensity: bytes[i + 1],
        steps: bytes[i + 2],
        heartRate: bytes[i + 3],
      ));
    }
    return out;
  }
}
