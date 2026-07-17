package rollup

import (
	"context"
	"testing"
	"time"

	"github.com/heliolytics/api/internal/store"
)

func TestRecomputeDailyStressUsesLatestValue(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	const day = "2026-06-28"

	clean := func() {
		st.Pool().Exec(ctx, `DELETE FROM stress_samples WHERE day_key=$1::date`, day)
		st.Pool().Exec(ctx, `DELETE FROM daily_metrics WHERE day_key=$1::date`, day)
	}
	clean()
	t.Cleanup(clean)
	st.Pool().Exec(ctx, `INSERT INTO daily_metrics (day_key) VALUES ($1::date)`, day)

	base := time.Date(2026, 6, 28, 4, 0, 0, 0, time.UTC)
	if err := st.UpsertStressSamples(ctx, "sess-stress-1", []store.SampleValue{
		{DayKey: day, SampledAt: base, Value: 20},
		{DayKey: day, SampledAt: base.Add(time.Hour), Value: 45}, // latest -> should win
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := RecomputeDailyStress(ctx, st, []string{day}); err != nil {
		t.Fatalf("recompute: %v", err)
	}

	var stress *int
	if err := st.Pool().QueryRow(ctx,
		`SELECT stress_avg FROM daily_metrics WHERE day_key=$1::date`, day).Scan(&stress); err != nil {
		t.Fatalf("read: %v", err)
	}
	if stress == nil || *stress != 45 {
		t.Fatalf("stress_avg=%v, want 45 (latest sample, not average)", stress)
	}
}
