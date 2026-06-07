import 'package:flutter_test/flutter_test.dart';
import 'package:heliolytics/config/constants.dart';

void main() {
  test('all known type codes are present', () {
    expect(knownTypeCodes, containsAll([
      '0x01', '0x05', '0x13', '0x25', '0x2E',
      '0x38', '0x3A', '0x3D', '0x48', '0x49',
    ]));
  });
  test('chunked-write and chunked-read UUIDs are distinct', () {
    expect(chunkedWriteUUID, isNot(equals(chunkedReadUUID)));
  });
}
