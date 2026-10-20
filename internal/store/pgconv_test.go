package store

import (
	"math"
	"testing"
	"time"
)

func TestNumericRoundTripKeepsExactDecimals(t *testing.T) {
	for _, want := range []float64{0, 36.27, 24.1, 400} {
		n, err := numericFromFloat(want)
		if err != nil {
			t.Fatalf("numericFromFloat(%v): %v", want, err)
		}
		got := numericVal(n)
		if got == nil || *got != want {
			t.Fatalf("round trip %v = %v", want, got)
		}
	}
}

func TestNumericFromFloatRejectsNonFinite(t *testing.T) {
	for _, v := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if _, err := numericFromFloat(v); err == nil {
			t.Fatalf("numericFromFloat(%v) accepted a non-finite value", v)
		}
	}
}

func TestNumericNullablePointers(t *testing.T) {
	n, err := numericPtr(nil)
	if err != nil || n.Valid {
		t.Fatalf("numericPtr(nil) = %+v, %v", n, err)
	}
	in, err := numericIntPtr(nil)
	if err != nil || in.Valid {
		t.Fatalf("numericIntPtr(nil) = %+v, %v", in, err)
	}
	value := 42
	got, err := numericIntPtr(&value)
	if err != nil || !got.Valid {
		t.Fatalf("numericIntPtr(42) = %+v, %v", got, err)
	}
	if numericVal(got) == nil || *numericVal(got) != 42 {
		t.Fatal("numericIntPtr did not round trip 42")
	}
}

func TestCanonicalJSONIgnoresPropertyOrder(t *testing.T) {
	a, err := canonicalJSON([]byte(`{"b":1,"a":{"y":2,"x":3}}`))
	if err != nil {
		t.Fatalf("canonicalJSON: %v", err)
	}
	b, err := canonicalJSON([]byte("{\"a\":{\"x\":3,\"y\":2},\"b\":1}"))
	if err != nil {
		t.Fatalf("canonicalJSON: %v", err)
	}
	if string(a) != string(b) {
		t.Fatalf("canonical forms differ: %s vs %s", a, b)
	}
	if _, err := canonicalJSON([]byte(`{"a":1} {"b":2}`)); err == nil {
		t.Fatal("canonicalJSON accepted trailing data")
	}
}

func TestValidateTypeCodeShape(t *testing.T) {
	for _, good := range []string{"0x01", "0x0D", "0x2E", "0x49"} {
		if err := validateTypeCode(good); err != nil {
			t.Fatalf("validateTypeCode(%q): %v", good, err)
		}
	}
	for _, bad := range []string{"", "0x1", "0x001", "0X01", "0x0d", "0xGG"} {
		if err := validateTypeCode(bad); err == nil {
			t.Fatalf("validateTypeCode(%q) accepted an invalid code", bad)
		}
	}
}

func TestMergeIntervalsMergesOverlapAndTouch(t *testing.T) {
	base := Day(2026, 10, 1)
	spans := []InstantInterval{
		{From: base.Add(3 * time.Hour), To: base.Add(4 * time.Hour)},
		{From: base.Add(1 * time.Hour), To: base.Add(2 * time.Hour)},
		{From: base.Add(2 * time.Hour), To: base.Add(3 * time.Hour)},
		{From: base.Add(6 * time.Hour), To: base.Add(7 * time.Hour)},
	}
	merged := mergeIntervals(spans)
	if len(merged) != 2 {
		t.Fatalf("merged = %v, want two spans", merged)
	}
	if !merged[0].From.Equal(base.Add(time.Hour)) || !merged[0].To.Equal(base.Add(4*time.Hour)) {
		t.Fatalf("first merged span = %v", merged[0])
	}
}
