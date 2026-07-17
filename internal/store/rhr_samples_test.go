package store

import (
	"context"
	"testing"
	"time"
)

func TestUpsertRhrSamplesThenReadBack(t *testing.T) {
	st, cleanup := testStore(t)
	t.Cleanup(cleanup)
	ctx := context.Background()
	const day = "2026-06-22"

	clean := func() { st.pool.Exec(ctx, `DELETE FROM rhr_samples WHERE day_key=$1::date`, day) }
	clean()
	t.Cleanup(clean)

	ts := time.Date(2026, 6, 22, 2, 0, 0, 0, time.UTC)
	pts := []SampleValue{{DayKey: day, SampledAt: ts, Value: 45}}
	if err := st.UpsertRhrSamples(ctx, "sess-rhr-1", pts); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	var value float64
	if err := st.pool.QueryRow(ctx,
		`SELECT value FROM rhr_samples WHERE day_key=$1::date`, day).Scan(&value); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if value != 45 {
		t.Fatalf("value=%v, want 45", value)
	}
}

func TestListRhrSamplesRange(t *testing.T) {
	st, cleanup := testStore(t)
	defer cleanup()
	ctx := context.Background()
	const dayIn = "2026-07-23"
	const dayOut = "2026-05-01"

	clean := func() {
		st.pool.Exec(ctx, `DELETE FROM rhr_samples WHERE day_key IN ($1::date, $2::date)`, dayIn, dayOut)
	}
	clean()
	t.Cleanup(clean)

	inRange := SampleValue{DayKey: dayIn, SampledAt: time.Date(2026, 7, 23, 3, 0, 0, 0, time.UTC), Value: 58}
	outOfRange := SampleValue{DayKey: dayOut, SampledAt: time.Date(2026, 5, 1, 3, 0, 0, 0, time.UTC), Value: 61}
	if err := st.UpsertRhrSamples(ctx, "sess-rhr-range", []SampleValue{inRange, outOfRange}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, err := st.ListRhrSamplesRange(ctx, "2026-07-01", "2026-07-30")
	if err != nil {
		t.Fatalf("ListRhrSamplesRange: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d samples, want 1: %+v", len(got), got)
	}
	if got[0].Value != 58 {
		t.Fatalf("value=%v, want 58", got[0].Value)
	}
	if got[0].DayKey != dayIn {
		t.Fatalf("day_key=%s, want %s", got[0].DayKey, dayIn)
	}
}
