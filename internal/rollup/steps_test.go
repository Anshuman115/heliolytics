package rollup

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/heliolytics/api/internal/store"
)

func testStore(t *testing.T) *store.Store {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = os.Getenv("DATABASE_URL")
	}
	if url == "" {
		t.Skip("TEST_DATABASE_URL or DATABASE_URL not set")
	}
	st, err := store.New(context.Background(), url)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(st.Close)
	return st
}

func TestRecomputeDailyStepsSumsFromSamples(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	const day = "2026-06-24"

	clean := func() {
		st.Pool().Exec(ctx, `DELETE FROM step_samples WHERE day_key=$1::date`, day)
		st.Pool().Exec(ctx, `DELETE FROM daily_metrics WHERE day_key=$1::date`, day)
	}
	clean()
	t.Cleanup(clean)

	st.Pool().Exec(ctx, `INSERT INTO daily_metrics (day_key) VALUES ($1::date)`, day)

	base := time.Date(2026, 6, 24, 4, 0, 0, 0, time.UTC)
	samples := []store.StepSample{
		{DayKey: day, SampledAt: base, Steps: 10},
		{DayKey: day, SampledAt: base.Add(time.Minute), Steps: 20},
	}
	if err := st.UpsertStepSamples(ctx, "sess-steps-1", samples); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := RecomputeDailySteps(ctx, st, []string{day}); err != nil {
		t.Fatalf("recompute: %v", err)
	}

	var steps int
	if err := st.Pool().QueryRow(ctx,
		`SELECT steps FROM daily_metrics WHERE day_key=$1::date`, day).Scan(&steps); err != nil {
		t.Fatalf("read: %v", err)
	}
	if steps != 30 {
		t.Fatalf("steps=%d, want 30", steps)
	}
}

// Overlapping syncs re-send the same minutes; daily steps must stay a SUM of
// distinct minutes, never doubled.
func TestRecomputeDailyStepsIgnoresOverlap(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	const day = "2026-06-25"

	clean := func() {
		st.Pool().Exec(ctx, `DELETE FROM step_samples WHERE day_key=$1::date`, day)
		st.Pool().Exec(ctx, `DELETE FROM daily_metrics WHERE day_key=$1::date`, day)
	}
	clean()
	t.Cleanup(clean)

	st.Pool().Exec(ctx, `INSERT INTO daily_metrics (day_key, steps) VALUES ($1::date, 9999)`, day)

	base := time.Date(2026, 6, 25, 4, 0, 0, 0, time.UTC)
	sample := func(min, steps int) store.StepSample {
		return store.StepSample{DayKey: day, SampledAt: base.Add(time.Duration(min) * time.Minute), Steps: steps}
	}
	var a []store.StepSample
	for i := 0; i < 10; i++ {
		a = append(a, sample(i, 10))
	}
	if err := st.UpsertStepSamples(ctx, "sess-A", a); err != nil {
		t.Fatalf("upsert A: %v", err)
	}
	var b []store.StepSample
	for i := 5; i < 15; i++ {
		b = append(b, sample(i, 10))
	}
	if err := st.UpsertStepSamples(ctx, "sess-B", b); err != nil {
		t.Fatalf("upsert B: %v", err)
	}

	if err := RecomputeDailySteps(ctx, st, []string{day}); err != nil {
		t.Fatalf("recompute: %v", err)
	}

	var steps int
	if err := st.Pool().QueryRow(ctx,
		`SELECT steps FROM daily_metrics WHERE day_key=$1::date`, day).Scan(&steps); err != nil {
		t.Fatalf("read: %v", err)
	}
	if steps != 150 {
		t.Fatalf("steps=%d, want 150 (distinct minutes, overlap not double-counted)", steps)
	}
}
