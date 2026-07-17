package store

import (
	"context"
	"testing"
	"time"
)

func TestListHealthSamplesCompactGroupsAcrossTables(t *testing.T) {
	st, cleanup := testStore(t)
	t.Cleanup(cleanup)
	ctx := context.Background()
	const day = "2026-06-29"

	clean := func() {
		st.pool.Exec(ctx, `DELETE FROM hrv_samples WHERE day_key=$1::date`, day)
		st.pool.Exec(ctx, `DELETE FROM rhr_samples WHERE day_key=$1::date`, day)
	}
	clean()
	t.Cleanup(clean)

	base := time.Date(2026, 6, 29, 2, 0, 0, 0, time.UTC)
	if err := st.UpsertHrvSamples(ctx, "sess-series-1", []SampleValue{
		{DayKey: day, SampledAt: base, Value: 40},
		{DayKey: day, SampledAt: base.Add(time.Minute), Value: 42},
	}); err != nil {
		t.Fatalf("seed hrv: %v", err)
	}
	if err := st.UpsertRhrSamples(ctx, "sess-series-1", []SampleValue{
		{DayKey: day, SampledAt: base, Value: 58},
	}); err != nil {
		t.Fatalf("seed rhr: %v", err)
	}

	days, err := st.ListHealthSamplesCompact(ctx, day, day)
	if err != nil {
		t.Fatalf("ListHealthSamplesCompact: %v", err)
	}

	var hrv, rhr *HealthCompactDay
	for i := range days {
		switch days[i].Metric {
		case "hrv":
			hrv = &days[i]
		case "rhr":
			rhr = &days[i]
		}
	}
	if hrv == nil {
		t.Fatal("want an hrv group in the result")
	}
	if len(hrv.Offsets) != 2 || len(hrv.Values) != 2 {
		t.Fatalf("hrv offsets/values len = %d/%d, want 2/2", len(hrv.Offsets), len(hrv.Values))
	}
	if hrv.Offsets[0] != 0 || hrv.Offsets[1] != 60 {
		t.Fatalf("hrv offsets = %v, want [0 60] (seconds since day_start)", hrv.Offsets)
	}
	if hrv.Values[0] != 40 || hrv.Values[1] != 42 {
		t.Fatalf("hrv values = %v, want [40 42]", hrv.Values)
	}
	if rhr == nil {
		t.Fatal("want an rhr group in the result")
	}
	if len(rhr.Values) != 1 || rhr.Values[0] != 58 {
		t.Fatalf("rhr values = %v, want [58]", rhr.Values)
	}
}
