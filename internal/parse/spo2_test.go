package parse

import "testing"

func TestParseSpo2Sleep30Byte(t *testing.T) {
	// header 0x02 + one 30-byte record: ts=1700000000, spo2=97
	raw := make([]byte, 31)
	raw[0] = 0x02
	// 1700000000 LE at offset 1
	raw[1] = 0x00
	raw[2] = 0xf9
	raw[3] = 0x93
	raw[4] = 0x65
	raw[5] = 0x61
	out := ParseSpo2Sleep(raw)
	if len(out) != 1 || out[0].Percent != 97 {
		t.Fatalf("got %+v", out)
	}
}

func TestDecodeSpo2PctOffset(t *testing.T) {
	if decodeSpo2Pct(98) != 98 {
		t.Fatal()
	}
	if decodeSpo2Pct(226) != 98 {
		t.Fatal()
	}
}
