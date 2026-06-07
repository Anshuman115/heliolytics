import 'dart:typed_data';
import 'package:pointycastle/export.dart';

class CryptoUtils {
  static String bytesToHex(Uint8List bytes) {
    final sb = StringBuffer();
    for (final b in bytes) {
      sb.write(b.toRadixString(16).padLeft(2, '0'));
    }
    return sb.toString();
  }

  static Uint8List hexToBytes(String hex) {
    if (hex.length % 2 != 0) {
      throw FormatException('Hex string must have even length (got ${hex.length}).');
    }
    final out = Uint8List(hex.length ~/ 2);
    for (var i = 0; i < out.length; i++) {
      final v = int.tryParse(hex.substring(i * 2, i * 2 + 2), radix: 16);
      if (v == null) throw FormatException('Invalid hex at ${i * 2}');
      out[i] = v;
    }
    return out;
  }

  static Uint8List aes128EcbEncrypt(Uint8List input, Uint8List key) {
    _checkAes(input, key);
    final c = ECBBlockCipher(AESEngine())..init(true, KeyParameter(key));
    return c.process(input);
  }

  static Uint8List aes128EcbDecrypt(Uint8List input, Uint8List key) {
    _checkAes(input, key);
    final c = ECBBlockCipher(AESEngine())..init(false, KeyParameter(key));
    return c.process(input);
  }

  static void _checkAes(Uint8List input, Uint8List key) {
    if (input.length != 16) throw ArgumentError('AES input must be 16 bytes (got ${input.length}).');
    if (key.length != 16) throw ArgumentError('AES key must be 16 bytes (got ${key.length}).');
  }
}
