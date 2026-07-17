package rollup

import (
	"context"
	"testing"
	"time"
)

func TestRecomputeDailyWorkoutCounts(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	const day = "2026-06-30"

	clean := func() {
		st.Pool().Exec(ctx, `DELETE FROM workouts WHERE day_key=$1::date`, day)
		st.Pool().Exec(ctx, `DELETE FROM activity_sessions WHERE day_key=$1::date`, day)
		st.Pool().Exec(ctx, `DELETE FROM daily_metrics WHERE day_key=$1::date`, day)
	}
	clean()
	t.Cleanup(clean)
	st.Pool().Exec(ctx, `INSERT INTO daily_metrics (day_key) VALUES ($1::date)`, day)

	base := time.Date(2026, 6, 30, 8, 0, 0, 0, time.UTC)
	st.Pool().Exec(ctx, `
		INSERT INTO workouts (started_at, day_key, sport_type, duration_sec, calories, source_session_id) VALUES
		($1, $2::date, 1, 1800, 300, 'sess-w1'), ($3, $2::date, 1, 1200, 200, 'sess-w2')`,
		base, day, base.Add(2*time.Hour))
	st.Pool().Exec(ctx, `
		INSERT INTO activity_sessions (started_at, day_key, sport_type, duration_sec, source_session_id) VALUES
		($1, $2::date, 2, 600, 'sess-a1')`,
		base.Add(4*time.Hour), day)

	if err := RecomputeDailyWorkoutCounts(ctx, st, []string{day}); err != nil {
		t.Fatalf("recompute: %v", err)
	}

	var workoutCount, activityCount, caloriesTotal int
	if err := st.Pool().QueryRow(ctx,
		`SELECT workout_count, activity_session_count, calories_total FROM daily_metrics WHERE day_key=$1::date`, day).
		Scan(&workoutCount, &activityCount, &caloriesTotal); err != nil {
		t.Fatalf("read: %v", err)
	}
	if workoutCount != 2 {
		t.Fatalf("workout_count=%d, want 2", workoutCount)
	}
	if activityCount != 1 {
		t.Fatalf("activity_session_count=%d, want 1", activityCount)
	}
	if caloriesTotal != 500 {
		t.Fatalf("calories_total=%d, want 500 (sum of 300+200)", caloriesTotal)
	}
}
