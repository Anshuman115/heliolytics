import 'dart:typed_data';

class HuamiTime {
  static Uint8List fromDateTime(DateTime dt) {
    final s = dt.toUtc().millisecondsSinceEpoch ~/ 1000;
    final b = ByteData(4)..setUint32(0, s, Endian.big);
    return b.buffer.asUint8List();
  }

  static DateTime toDateTime(Uint8List bytes) {
    final b = ByteData.sublistView(bytes);
    return DateTime.fromMillisecondsSinceEpoch(
      b.getUint32(0, Endian.big) * 1000,
      isUtc: true,
    );
  }

  static Uint8List nowMinusHours(int hours) =>
      fromDateTime(DateTime.now().toUtc().subtract(Duration(hours: hours)));
}
