package rollup

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func TestRecomputeDailySleepPicksBestNonNapSession(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	const day = "2026-06-29"

	clean := func() {
		st.Pool().Exec(ctx, `DELETE FROM sleep_sessions WHERE day_key=$1::date`, day)
		st.Pool().Exec(ctx, `DELETE FROM daily_metrics WHERE day_key=$1::date`, day)
	}
	clean()
	t.Cleanup(clean)
	st.Pool().Exec(ctx, `INSERT INTO daily_metrics (day_key) VALUES ($1::date)`, day)

	base := time.Date(2026, 6, 28, 22, 0, 0, 0, time.UTC)
	// A nap (must be excluded from sleep_score) and two non-nap sessions
	// (best score must win), plus nap_count must reflect the nap row.
	_, err := st.Pool().Exec(ctx, `
		INSERT INTO sleep_sessions (started_at, day_key, score, total_mins, deep_mins, rem_mins, light_mins, wake_mins, is_nap, source_session_id) VALUES
		($1, $2::date, 60, 480, 90, 90, 300, 0, false, 'sess-1'),
		($3, $2::date, 85, 470, 110, 110, 250, 0, false, 'sess-2'),
		($4, $2::date, 90, 30, 0, 0, 30, 0, true, 'sess-3')`,
		base, day, base.Add(30*time.Minute), base.Add(10*time.Hour))
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := RecomputeDailySleep(ctx, st, []string{day}); err != nil {
		t.Fatalf("recompute: %v", err)
	}

	var score, mins, naps int
	if err := st.Pool().QueryRow(ctx,
		`SELECT sleep_score, sleep_mins, nap_count FROM daily_metrics WHERE day_key=$1::date`, day).
		Scan(&score, &mins, &naps); err != nil {
		t.Fatalf("read: %v", err)
	}
	if score != 85 {
		t.Fatalf("sleep_score=%d, want 85 (best non-nap session)", score)
	}
	if mins != 470 {
		t.Fatalf("sleep_mins=%d, want 470", mins)
	}
	if naps != 1 {
		t.Fatalf("nap_count=%d, want 1", naps)
	}
}

func TestRecomputeDailySleepNapOnlyDayUpdatesNapCountWithoutClobberingSleepScore(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	const day = "2026-06-29"

	clean := func() {
		st.Pool().Exec(ctx, `DELETE FROM sleep_sessions WHERE day_key=$1::date`, day)
		st.Pool().Exec(ctx, `DELETE FROM daily_metrics WHERE day_key=$1::date`, day)
	}
	clean()
	t.Cleanup(clean)
	// Pre-seed a sleep_score from a prior recompute; this must survive a
	// nap-only-day recompute (no non-nap session present to overwrite it).
	st.Pool().Exec(ctx, `INSERT INTO daily_metrics (day_key, sleep_score) VALUES ($1::date, 77)`, day)

	base := time.Date(2026, 6, 28, 13, 0, 0, 0, time.UTC)
	// Only nap rows for this day — no main (non-nap) sleep session.
	_, err := st.Pool().Exec(ctx, `
		INSERT INTO sleep_sessions (started_at, day_key, score, total_mins, deep_mins, rem_mins, light_mins, wake_mins, is_nap, source_session_id) VALUES
		($1, $2::date, 60, 30, 0, 0, 30, 0, true, 'nap-1'),
		($3, $2::date, 65, 20, 0, 0, 20, 0, true, 'nap-2')`,
		base, day, base.Add(3*time.Hour))
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := RecomputeDailySleep(ctx, st, []string{day}); err != nil {
		t.Fatalf("recompute: %v", err)
	}

	var score sql.NullInt64
	var naps int
	if err := st.Pool().QueryRow(ctx,
		`SELECT sleep_score, nap_count FROM daily_metrics WHERE day_key=$1::date`, day).
		Scan(&score, &naps); err != nil {
		t.Fatalf("read: %v", err)
	}
	if naps != 2 {
		t.Fatalf("nap_count=%d, want 2 (nap-only day must still update nap_count)", naps)
	}
	if !score.Valid || score.Int64 != 77 {
		t.Fatalf("sleep_score=%v, want 77 (prior value must not be clobbered on nap-only day)", score)
	}
}
