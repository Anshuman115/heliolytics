import 'dart:typed_data';
import 'package:heliolytics/utils/crypto.dart';

class UnknownSample {
  final String typeCode;
  final Uint8List rawBytes;
  final String firstBytesHex;
  const UnknownSample({
    required this.typeCode,
    required this.rawBytes,
    required this.firstBytesHex,
  });
}

class UnknownParser {
  static List<UnknownSample> parse(String typeCode, Uint8List bytes) {
    final previewLen = bytes.length < 16 ? bytes.length : 16;
    final preview = bytes.sublist(0, previewLen);
    return [
      UnknownSample(
        typeCode: typeCode,
        rawBytes: bytes,
        firstBytesHex: CryptoUtils.bytesToHex(preview),
      ),
    ];
  }
}
