import 'dart:typed_data';
import 'package:flutter_test/flutter_test.dart';
import 'package:heliolytics/ble/chunked_protocol.dart';

void main() {
  group('ChunkAssembler', () {
    test('appends a single chunk (counter stripped)', () {
      final a = ChunkAssembler();
      a.append(Uint8List.fromList([0x00, 0xAA, 0xBB]));
      expect(a.payload, [0xAA, 0xBB]);
    });
    test('appends multiple chunks in order', () {
      final a = ChunkAssembler();
      a.append(Uint8List.fromList([0x00, 0xAA]));
      a.append(Uint8List.fromList([0x01, 0xBB]));
      a.append(Uint8List.fromList([0x02, 0xCC]));
      expect(a.payload, [0xAA, 0xBB, 0xCC]);
    });
    test('handles counter wrap 0xFF to 0x00', () {
      final a = ChunkAssembler();
      a.append(Uint8List.fromList([0xFE, 0xAA]));
      a.append(Uint8List.fromList([0xFF, 0xBB]));
      a.append(Uint8List.fromList([0x00, 0xCC]));
      expect(a.payload, [0xAA, 0xBB, 0xCC]);
    });
    test('isComplete true after expected chunks', () {
      final a = ChunkAssembler()..expectChunks(3);
      a.append(Uint8List.fromList([0x00, 0xAA]));
      a.append(Uint8List.fromList([0x01, 0xBB]));
      a.append(Uint8List.fromList([0x02, 0xCC]));
      expect(a.isComplete, isTrue);
    });
    test('isComplete false when chunks missing', () {
      final a = ChunkAssembler()..expectChunks(5);
      a.append(Uint8List.fromList([0x00, 0xAA]));
      expect(a.isComplete, isFalse);
    });
    test('detects counter gap', () {
      final a = ChunkAssembler();
      a.append(Uint8List.fromList([0x00, 0xAA]));
      a.append(Uint8List.fromList([0x02, 0xCC])); // gap: 0x01 missing
      expect(() => a.append(Uint8List.fromList([0x03, 0xDD])),
          throwsA(isA<ChunkGapException>()));
    });
    test('reset clears state', () {
      final a = ChunkAssembler();
      a.append(Uint8List.fromList([0x00, 0xAA]));
      a.reset();
      a.append(Uint8List.fromList([0x00, 0xBB]));
      expect(a.payload, [0xBB]);
    });
  });
  group('buildChunk', () {
    test('prepends counter byte', () {
      expect(buildChunk(0x05, Uint8List.fromList([0xAA, 0xBB])), [0x05, 0xAA, 0xBB]);
    });
  });
}
