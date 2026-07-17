package rollup

import (
	"context"
	"testing"
	"time"
)

func TestRecomputeDailyAvgHr(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	const day = "2026-07-02"

	clean := func() {
		st.Pool().Exec(ctx, `DELETE FROM heart_rate_samples WHERE day_key=$1::date`, day)
		st.Pool().Exec(ctx, `DELETE FROM daily_metrics WHERE day_key=$1::date`, day)
	}
	clean()
	t.Cleanup(clean)
	st.Pool().Exec(ctx, `INSERT INTO daily_metrics (day_key) VALUES ($1::date)`, day)

	base := time.Date(2026, 7, 2, 4, 0, 0, 0, time.UTC)
	st.Pool().Exec(ctx, `
		INSERT INTO heart_rate_samples (sampled_at, day_key, bpm, source_session_id) VALUES
		($1, $2::date, 60, 'sess-hr1'), ($3, $2::date, 80, 'sess-hr1')`,
		base, day, base.Add(time.Minute))

	if err := RecomputeDailyAvgHr(ctx, st, []string{day}); err != nil {
		t.Fatalf("recompute: %v", err)
	}

	var avgHr int
	if err := st.Pool().QueryRow(ctx,
		`SELECT avg_hr FROM daily_metrics WHERE day_key=$1::date`, day).Scan(&avgHr); err != nil {
		t.Fatalf("read: %v", err)
	}
	if avgHr != 70 {
		t.Fatalf("avg_hr=%d, want 70", avgHr)
	}
}
