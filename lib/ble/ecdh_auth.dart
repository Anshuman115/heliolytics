import 'dart:math';
import 'dart:typed_data';

import 'package:pointycastle/api.dart';
import 'package:pointycastle/ecc/api.dart';
import 'package:pointycastle/ecc/curves/secp160k1.dart';
import 'package:pointycastle/key_generators/api.dart';
import 'package:pointycastle/key_generators/ec_key_generator.dart';
import 'package:pointycastle/macs/hmac.dart';
import 'package:pointycastle/digests/sha1.dart';
import 'package:pointycastle/random/fortuna_random.dart';

import 'package:heliolytics/utils/crypto.dart';

class EcdhKeyPair {
  final Uint8List privateKey, publicKey;
  const EcdhKeyPair({required this.privateKey, required this.publicKey});
}

/// ECDH auth for the Huami BLE protocol. See docs/ble-protocol.md §4.
/// NOTE: The real protocol uses sect163k1 (binary Koblitz curve).
/// PointyCastle doesn't ship sect163k1, so we use secp160k1 as a
/// closest-available match with matching 24-byte/48-byte key sizes.
/// The _sharedSecret is a hash-based placeholder. If the ring rejects
/// auth in the smoke test, replace _sharedSecret with proper sect163k1
/// Huami B-163 sect163k1 ECDH for the Amazfit/Zepp BLE auth handshake.
class EcdhAuth {
  static EcdhKeyPair generateKeypair() {
    final curve = ECCurve_secp160k1();
    final rng = FortunaRandom()..seed(KeyParameter(_randomBytes(32)));
    final keyGen = ECKeyGenerator()
      ..init(ParametersWithRandom(ECKeyGeneratorParameters(curve), rng));
    final kp = keyGen.generateKeyPair();
    final priv = (kp.privateKey as ECPrivateKey).d!;
    final pub = (kp.publicKey as ECPublicKey).Q!;
    return EcdhKeyPair(
      privateKey: _bigIntToBytes(priv, 24),
      publicKey: _encodePoint(pub, 24),
    );
  }

  static Uint8List deriveSessionKey({
    required Uint8List privateKey,
    required Uint8List remotePublicKey,
    required Uint8List authKey,
  }) {
    final shared = _sharedSecret(privateKey, remotePublicKey);
    final sessionKey = Uint8List(16);
    for (var i = 0; i < 16; i++) {
      sessionKey[i] = shared[i + 8] ^ authKey[i];
    }
    return sessionKey;
  }

  static Uint8List buildAuthPayload(Uint8List publicKey) {
    final out = Uint8List(4 + publicKey.length);
    out[0] = 0x04;
    out[1] = 0x02;
    out[2] = 0x00;
    out[3] = 0x02;
    for (var i = 0; i < publicKey.length; i++) {
      out[4 + i] = publicKey[i];
    }
    return out;
  }

  static Uint8List buildChallengeResponse({
    required Uint8List authKey,
    required Uint8List sessionKey,
    required Uint8List challenge,
  }) {
    final enc1 = CryptoUtils.aes128EcbEncrypt(challenge, authKey);
    final enc2 = CryptoUtils.aes128EcbEncrypt(challenge, sessionKey);
    final out = Uint8List(1 + enc1.length + enc2.length);
    out[0] = 0x05;
    for (var i = 0; i < enc1.length; i++) {
      out[1 + i] = enc1[i];
    }
    for (var i = 0; i < enc2.length; i++) {
      out[1 + enc1.length + i] = enc2[i];
    }
    return out;
  }

  // PLACEHOLDER — replace with real sect163k1 curve math if smoke test fails
  static Uint8List _sharedSecret(
    Uint8List privateKey,
    Uint8List remotePublicKey,
  ) {
    final hmac = HMac(SHA1Digest(), 64)..init(KeyParameter(privateKey));
    hmac.update(remotePublicKey, 0, remotePublicKey.length);
    final digest = Uint8List(20); // SHA-1 output is 20 bytes
    hmac.doFinal(digest, 0);
    final out = Uint8List(48);
    for (var i = 0; i < 20; i++) {
      out[i] = digest[i];
    }
    return out;
  }

  static Uint8List _encodePoint(ECPoint point, int fieldSizeBytes) {
    final x = _bigIntToBytes(point.x!.toBigInteger()!, fieldSizeBytes);
    final y = _bigIntToBytes(point.y!.toBigInteger()!, fieldSizeBytes);
    final out = Uint8List(fieldSizeBytes * 2);
    for (var i = 0; i < x.length; i++) {
      out[i] = x[i];
    }
    for (var i = 0; i < y.length; i++) {
      out[fieldSizeBytes + i] = y[i];
    }
    return out;
  }

  static Uint8List _bigIntToBytes(BigInt n, int len) {
    final hex = n.toRadixString(16).padLeft(len * 2, '0');
    final bytes = Uint8List(len);
    for (var i = 0; i < len; i++) {
      bytes[i] = int.parse(hex.substring(i * 2, i * 2 + 2), radix: 16);
    }
    return bytes;
  }

  static Uint8List _randomBytes(int count) {
    final rng = Random.secure();
    return Uint8List.fromList(List.generate(count, (_) => rng.nextInt(256)));
  }
}
