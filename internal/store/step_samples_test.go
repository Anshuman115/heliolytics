package store

import (
	"context"
	"testing"
	"time"
)

// Overlapping syncs re-send the same minutes; daily steps must stay a SUM of
// distinct minutes, never doubled.
func TestRecomputeDailyStepsIgnoresOverlap(t *testing.T) {
	st, cleanup := testStore(t)
	defer cleanup()
	ctx := context.Background()
	const day = "2026-06-22"

	clean := func() {
		st.pool.Exec(ctx, `DELETE FROM step_samples WHERE day_key=$1::date`, day)
		st.pool.Exec(ctx, `DELETE FROM daily_metrics WHERE day_key=$1::date`, day)
	}
	clean()
	t.Cleanup(clean)

	// Seed a daily row with a deliberately wrong (inflated) step count.
	if _, err := st.pool.Exec(ctx,
		`INSERT INTO daily_metrics (day_key, steps) VALUES ($1::date, 9999)`, day); err != nil {
		t.Fatalf("seed daily_metrics: %v", err)
	}

	base := time.Date(2026, 6, 22, 4, 0, 0, 0, time.UTC) // 09:30 IST
	sample := func(min, steps int) StepSample {
		return StepSample{DayKey: day, SampledAt: base.Add(time.Duration(min) * time.Minute), Steps: steps}
	}

	// Sync A: minutes 0..9, 10 steps each = 100.
	var a []StepSample
	for i := 0; i < 10; i++ {
		a = append(a, sample(i, 10))
	}
	if err := st.UpsertStepSamples(ctx, "sess-A", a); err != nil {
		t.Fatalf("upsert A: %v", err)
	}
	// Sync B overlaps minutes 5..9 (re-sent) and adds 10..14. Distinct = 0..14.
	var b []StepSample
	for i := 5; i < 15; i++ {
		b = append(b, sample(i, 10))
	}
	if err := st.UpsertStepSamples(ctx, "sess-B", b); err != nil {
		t.Fatalf("upsert B: %v", err)
	}

	if err := st.RecomputeDailySteps(ctx, []string{day}); err != nil {
		t.Fatalf("recompute: %v", err)
	}

	var steps int
	if err := st.pool.QueryRow(ctx,
		`SELECT steps FROM daily_metrics WHERE day_key=$1::date`, day).Scan(&steps); err != nil {
		t.Fatalf("read steps: %v", err)
	}
	// 15 distinct minutes * 10 = 150. NOT 200 (overlap double-count) and NOT 9999 (stale seed).
	if steps != 150 {
		t.Fatalf("steps=%d, want 150 (distinct minutes, overlap not double-counted)", steps)
	}
}
