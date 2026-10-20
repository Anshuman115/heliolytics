package store

import "testing"

func TestMeasuredTypeCodesAreTheContractSet(t *testing.T) {
	want := []string{
		"0x01", "0x05", "0x06", "0x0D", "0x13",
		"0x25", "0x2E", "0x38", "0x46", "0x48", "0x49",
	}
	if len(MeasuredTypeCodes) != len(want) {
		t.Fatalf("measured codes = %v, want %v", MeasuredTypeCodes, want)
	}
	for i, code := range want {
		if MeasuredTypeCodes[i] != code {
			t.Fatalf("measured code %d = %s, want %s", i, MeasuredTypeCodes[i], code)
		}
	}
	if isMeasuredType("0x26") {
		t.Fatal("0x26 is raw-only and must not be a measured code")
	}
	for _, legacy := range []string{"0x39", "0x3A", "0x3B", "0x07"} {
		if isMeasuredType(legacy) {
			t.Fatalf("%s must not be a measured code", legacy)
		}
	}
}
