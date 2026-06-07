import 'dart:typed_data';
import 'package:flutter_test/flutter_test.dart';
import 'package:heliolytics/utils/crypto.dart';

void main() {
  group('hex helpers', () {
    test('bytesToHex lowercases and pads', () {
      expect(CryptoUtils.bytesToHex(Uint8List.fromList([0, 0xab, 0x0f])), '00ab0f');
    });
    test('hexToBytes accepts mixed case', () {
      expect(CryptoUtils.hexToBytes('00AB0F'), [0, 0xab, 0x0f]);
    });
    test('hexToBytes throws on odd length', () {
      expect(() => CryptoUtils.hexToBytes('abc'), throwsFormatException);
    });
    test('hexToBytes throws on non-hex', () {
      expect(() => CryptoUtils.hexToBytes('zzzz'), throwsFormatException);
    });
  });
  group('AES-128 ECB', () {
    test('encrypts deterministically', () {
      final key = Uint8List.fromList(List.generate(16, (i) => i));
      final plain = Uint8List.fromList(List.generate(16, (i) => 0x10 + i));
      final e1 = CryptoUtils.aes128EcbEncrypt(plain, key);
      final e2 = CryptoUtils.aes128EcbEncrypt(plain, key);
      expect(e1, e2);
      expect(e1.length, 16);
      expect(e1, isNot(equals(plain)));
    });
    test('decrypts ciphertext back to plaintext', () {
      final key = Uint8List.fromList(List.generate(16, (i) => i));
      final plain = Uint8List.fromList(List.generate(16, (i) => 0x10 + i));
      final enc = CryptoUtils.aes128EcbEncrypt(plain, key);
      expect(CryptoUtils.aes128EcbDecrypt(enc, key), plain);
    });
  });
}
