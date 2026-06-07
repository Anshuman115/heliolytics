import 'dart:typed_data';

class ChunkGapException implements Exception {
  final int expected, got;
  ChunkGapException(this.expected, this.got);
  @override
  String toString() => 'ChunkGapException: expected $expected, got $got';
}

class ChunkAssembler {
  final List<int> _payload = [];
  /// The counter value expected for the next chunk.  Initialised to -1
  /// meaning "not yet started"; the first appended chunk sets the base.
  int _next = -1;
  int _expected = -1;
  int _received = 0;
  bool _gapDetected = false;

  void expectChunks(int n) {
    _expected = n;
  }

  void append(Uint8List chunk) {
    if (chunk.isEmpty) throw ArgumentError('Empty chunk');

    if (_gapDetected) {
      // A gap was previously recorded — raise on the very next append.
      throw ChunkGapException(_next, chunk[0]);
    }

    if (_next == -1) {
      // First chunk: accept unconditionally and initialise counter.
      _next = chunk[0];
    }

    if (chunk[0] != _next) {
      // Counter mismatch — accept this chunk's payload but mark the gap.
      for (var i = 1; i < chunk.length; i++) {
        _payload.add(chunk[i]);
      }
      _next = (chunk[0] + 1) & 0xFF;
      _received++;
      _gapDetected = true;
      return;
    }

    for (var i = 1; i < chunk.length; i++) {
      _payload.add(chunk[i]);
    }
    _next = (_next + 1) & 0xFF;
    _received++;
  }

  bool get isComplete => _expected >= 0 && _received >= _expected;

  Uint8List get payload => Uint8List.fromList(_payload);

  void reset() {
    _payload.clear();
    _next = -1;
    _expected = -1;
    _received = 0;
    _gapDetected = false;
  }
}

Uint8List buildChunk(int counter, Uint8List payload) {
  final out = Uint8List(payload.length + 1);
  out[0] = counter & 0xFF;
  for (var i = 0; i < payload.length; i++) {
    out[i + 1] = payload[i];
  }
  return out;
}
