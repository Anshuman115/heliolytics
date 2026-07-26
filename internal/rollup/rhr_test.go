package rollup

import (
	"context"
	"testing"
	"time"

	"github.com/heliolytics/api/internal/store"
)

func TestRecomputeDailyRhrCopiesLatestSample(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	const day = "2026-06-29"

	clean := func() {
		st.Pool().Exec(ctx, `DELETE FROM rhr_samples WHERE day_key=$1::date`, day)
		st.Pool().Exec(ctx, `DELETE FROM daily_metrics WHERE day_key=$1::date`, day)
	}
	clean()
	t.Cleanup(clean)
	st.Pool().Exec(ctx, `INSERT INTO daily_metrics (day_key) VALUES ($1::date)`, day)

	base := time.Date(2026, 6, 29, 1, 0, 0, 0, time.UTC)
	if err := st.UpsertRhrSamples(ctx, "sess-rhr-rollup", []store.SampleValue{
		{DayKey: day, SampledAt: base, Value: 58},
		{DayKey: day, SampledAt: base.Add(time.Hour), Value: 56},
	}); err != nil {
		t.Fatalf("seed rhr: %v", err)
	}
	if err := RecomputeDailyRhr(ctx, st, []string{day}); err != nil {
		t.Fatalf("recompute: %v", err)
	}

	var rhr *int
	if err := st.Pool().QueryRow(ctx,
		`SELECT resting_hr FROM daily_metrics WHERE day_key=$1::date`, day).Scan(&rhr); err != nil {
		t.Fatalf("read: %v", err)
	}
	if rhr == nil || *rhr != 56 {
		t.Fatalf("resting_hr=%v, want latest value 56", rhr)
	}
}
