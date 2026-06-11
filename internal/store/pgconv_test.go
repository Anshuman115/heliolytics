package store

import (
	"math"
	"testing"
	"time"
)

func TestNumericFromFloatValid(t *testing.T) {
	for _, v := range []float64{0, 36.5, 37.2, -1.0} {
		n, err := numericFromFloat(v)
		if err != nil {
			t.Fatalf("numericFromFloat(%v): %v", v, err)
		}
		if !n.Valid {
			t.Fatalf("numericFromFloat(%v) not valid", v)
		}
		got := numericVal(n)
		if got == nil {
			t.Fatalf("numericVal after FromFloat(%v) nil", v)
		}
		if *got != v {
			t.Fatalf("round trip %v got %v", v, *got)
		}
	}
}

func TestNumericFromFloatRejectsNonFinite(t *testing.T) {
	for _, v := range []float64{math.NaN(), math.Inf(1)} {
		if _, err := numericFromFloat(v); err == nil {
			t.Fatalf("expected error for %v", v)
		}
	}
}

func TestNumericPtrNullable(t *testing.T) {
	n, err := numericPtr(nil)
	if err != nil {
		t.Fatal(err)
	}
	if n.Valid {
		t.Fatal("nil pointer should produce invalid numeric")
	}
}

func TestTimestamptzRequired(t *testing.T) {
	if _, err := timestamptzRequired(time.Time{}, "x"); err == nil {
		t.Fatal("zero time should fail")
	}
	ts, err := timestamptzRequired(time.Unix(1, 0).UTC(), "x")
	if err != nil || !ts.Valid {
		t.Fatalf("got ts=%+v err=%v", ts, err)
	}
}

func TestValidateTempPoint(t *testing.T) {
	ok := TempPoint{DayKey: "2026-06-10", SampledAt: time.Unix(1, 0).UTC(), Celsius: 36.5}
	if err := validateTempPoint(ok); err != nil {
		t.Fatal(err)
	}
	bad := TempPoint{DayKey: "", SampledAt: time.Unix(1, 0).UTC(), Celsius: 36.5}
	if err := validateTempPoint(bad); err == nil {
		t.Fatal("expected day_key error")
	}
}

func TestValidateHealthSample(t *testing.T) {
	ok := HealthSample{Metric: "hrv", DayKey: "2026-06-10", SampledAt: time.Unix(1, 0).UTC(), Value: 42.5}
	if err := validateHealthSample("sid", ok); err != nil {
		t.Fatal(err)
	}
	if err := validateHealthSample("", ok); err == nil {
		t.Fatal("expected sync_session_id error")
	}
}

func TestValidateSessionMeta(t *testing.T) {
	if err := validateSessionMeta(SessionMeta{}); err == nil {
		t.Fatal("expected error")
	}
	if err := validateSessionMeta(SessionMeta{ID: "1", StartedAt: time.Unix(1, 0).UTC()}); err != nil {
		t.Fatal(err)
	}
}
