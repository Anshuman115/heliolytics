package rollup

import (
	"context"
	"testing"
)

func TestRecomputeDailyPaiAndReadinessCopyFromRawTables(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	const day = "2026-07-01"

	clean := func() {
		st.Pool().Exec(ctx, `DELETE FROM raw_pai_scores WHERE day_key=$1::date`, day)
		st.Pool().Exec(ctx, `DELETE FROM raw_readiness WHERE day_key=$1::date`, day)
		st.Pool().Exec(ctx, `DELETE FROM daily_metrics WHERE day_key=$1::date`, day)
	}
	clean()
	t.Cleanup(clean)
	st.Pool().Exec(ctx, `INSERT INTO daily_metrics (day_key) VALUES ($1::date)`, day)

	if err := st.UpsertPaiScores(ctx, "sess-pai", map[string]int{day: 81}); err != nil {
		t.Fatalf("seed pai: %v", err)
	}
	if err := st.UpsertReadinessScores(ctx, "sess-rdy", map[string]int{day: 72}); err != nil {
		t.Fatalf("seed readiness: %v", err)
	}

	if err := RecomputeDailyPai(ctx, st, []string{day}); err != nil {
		t.Fatalf("recompute pai: %v", err)
	}
	if err := RecomputeDailyReadiness(ctx, st, []string{day}); err != nil {
		t.Fatalf("recompute readiness: %v", err)
	}

	var pai, readiness int
	if err := st.Pool().QueryRow(ctx,
		`SELECT pai_score, readiness FROM daily_metrics WHERE day_key=$1::date`, day).
		Scan(&pai, &readiness); err != nil {
		t.Fatalf("read: %v", err)
	}
	if pai != 81 {
		t.Fatalf("pai_score=%d, want 81", pai)
	}
	if readiness != 72 {
		t.Fatalf("readiness=%d, want 72", readiness)
	}
}
