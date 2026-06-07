import 'dart:typed_data';

class AuthKeyValidator {
  static String? validate(String key) {
    final n = normalize(key);
    if (n.length != 32) return 'Auth key must be exactly 32 hex characters (got ${n.length}).';
    if (!RegExp(r'^[0-9a-f]{32}$').hasMatch(n)) {
      return 'Auth key must contain only hex characters (0-9, a-f).';
    }
    return null;
  }

  static String normalize(String key) => key.trim().toLowerCase();

  static Uint8List toBytes(String key) {
    final reason = validate(key);
    if (reason != null) throw FormatException('Invalid auth key: $reason');
    final n = normalize(key);
    final bytes = Uint8List(16);
    for (var i = 0; i < 16; i++) {
      bytes[i] = int.parse(n.substring(i * 2, i * 2 + 2), radix: 16);
    }
    return bytes;
  }
}
