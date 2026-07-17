package rollup

import (
	"context"
	"testing"
	"time"

	"github.com/heliolytics/api/internal/store"
)

func TestRecomputeDailyTemperatureAverages(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	const day = "2026-06-26"

	clean := func() {
		st.Pool().Exec(ctx, `DELETE FROM temperature_samples WHERE day_key=$1::date`, day)
		st.Pool().Exec(ctx, `DELETE FROM daily_metrics WHERE day_key=$1::date`, day)
	}
	clean()
	t.Cleanup(clean)
	st.Pool().Exec(ctx, `INSERT INTO daily_metrics (day_key) VALUES ($1::date)`, day)

	base := time.Date(2026, 6, 26, 4, 0, 0, 0, time.UTC)
	rows := []store.TempPoint{
		{DayKey: day, SampledAt: base, Celsius: 36.0},
		{DayKey: day, SampledAt: base.Add(time.Minute), Celsius: 37.0},
	}
	if err := st.UpsertTemperature(ctx, "sess-temp-1", rows); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := RecomputeDailyTemperature(ctx, st, []string{day}); err != nil {
		t.Fatalf("recompute: %v", err)
	}

	var avg float64
	if err := st.Pool().QueryRow(ctx,
		`SELECT temp_avg_c FROM daily_metrics WHERE day_key=$1::date`, day).Scan(&avg); err != nil {
		t.Fatalf("read: %v", err)
	}
	if avg != 36.5 {
		t.Fatalf("temp_avg_c=%v, want 36.5", avg)
	}
}
