import 'package:flutter_test/flutter_test.dart';
import 'package:heliolytics/auth/auth_key_validator.dart';

void main() {
  group('AuthKeyValidator.validate', () {
    test('accepts valid 32-char lowercase hex', () {
      expect(AuthKeyValidator.validate('a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6'), isNull);
    });
    test('accepts valid 32-char uppercase hex', () {
      expect(AuthKeyValidator.validate('A1B2C3D4E5F6A7B8C9D0E1F2A3B4C5D6'), isNull);
    });
    test('rejects empty string', () {
      expect(AuthKeyValidator.validate(''), isNotNull);
    });
    test('rejects wrong length', () {
      expect(AuthKeyValidator.validate('a1b2c3d4'), contains('32'));
    });
    test('rejects non-hex chars', () {
      expect(AuthKeyValidator.validate('g1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6'), contains('hex'));
    });
  });
  group('AuthKeyValidator.normalize', () {
    test('lowercases uppercase hex', () {
      expect(AuthKeyValidator.normalize('AABBCC'), 'aabbcc');
    });
    test('trims whitespace', () {
      expect(AuthKeyValidator.normalize('  aabbcc  '), 'aabbcc');
    });
  });
  group('AuthKeyValidator.toBytes', () {
    test('converts 32-char hex to 16 bytes', () {
      final bytes = AuthKeyValidator.toBytes('a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6');
      expect(bytes.length, 16);
      expect(bytes[0], 0xa1);
      expect(bytes[15], 0xd6);
    });
    test('throws on invalid', () {
      expect(() => AuthKeyValidator.toBytes('zzzz'), throwsFormatException);
    });
  });
}
