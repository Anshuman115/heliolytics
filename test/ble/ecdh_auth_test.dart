import 'dart:typed_data';
import 'package:flutter_test/flutter_test.dart';
import 'package:heliolytics/ble/ecdh_auth.dart';

void main() {
  test('generateKeypair returns 24-byte private and 48-byte public', () {
    final kp = EcdhAuth.generateKeypair();
    expect(kp.privateKey.length, 24);
    expect(kp.publicKey.length, 48);
  });
  test('deriveSessionKey is deterministic', () {
    final kp = EcdhAuth.generateKeypair();
    final remotePub = Uint8List.fromList(List.generate(48, (i) => i));
    final authKey = Uint8List.fromList(List.generate(16, (i) => 0xA0 + i));
    final s1 = EcdhAuth.deriveSessionKey(
      privateKey: kp.privateKey,
      remotePublicKey: remotePub,
      authKey: authKey,
    );
    final s2 = EcdhAuth.deriveSessionKey(
      privateKey: kp.privateKey,
      remotePublicKey: remotePub,
      authKey: authKey,
    );
    expect(s1, s2);
    expect(s1.length, 16);
  });
  test('different auth keys produce different session keys', () {
    final kp = EcdhAuth.generateKeypair();
    final remotePub = Uint8List.fromList(List.generate(48, (i) => i));
    final s1 = EcdhAuth.deriveSessionKey(
      privateKey: kp.privateKey,
      remotePublicKey: remotePub,
      authKey: Uint8List.fromList(List.generate(16, (_) => 0)),
    );
    final s2 = EcdhAuth.deriveSessionKey(
      privateKey: kp.privateKey,
      remotePublicKey: remotePub,
      authKey: Uint8List.fromList(List.generate(16, (_) => 0xFF)),
    );
    expect(s1, isNot(equals(s2)));
  });
  test('buildAuthPayload produces [header] + 48-byte public key', () {
    final kp = EcdhAuth.generateKeypair();
    final payload = EcdhAuth.buildAuthPayload(kp.publicKey);
    expect(payload.length, 4 + 48);
    expect(payload[0], 0x04);
    expect(payload[1], 0x02);
    expect(payload[2], 0x00);
    expect(payload[3], 0x02);
    expect(payload.sublist(4), kp.publicKey);
  });
  test('buildChallengeResponse produces [0x05] + 32 bytes', () {
    final payload = EcdhAuth.buildChallengeResponse(
      authKey: Uint8List.fromList(List.generate(16, (i) => 0xA0 + i)),
      sessionKey: Uint8List.fromList(List.generate(16, (i) => 0x10 + i)),
      challenge: Uint8List.fromList(List.generate(16, (i) => 0x40 + i)),
    );
    expect(payload[0], 0x05);
    expect(payload.length, 1 + 16 + 16);
  });
}
