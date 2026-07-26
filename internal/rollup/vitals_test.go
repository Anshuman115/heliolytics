package rollup

import (
	"context"
	"testing"
	"time"

	"github.com/heliolytics/api/internal/store"
)

func TestRecomputeDailyVitalsUsesSleepWindow(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	const day = "2026-06-27"

	clean := func() {
		st.Pool().Exec(ctx, `DELETE FROM hrv_samples WHERE day_key=$1::date`, day)
		st.Pool().Exec(ctx, `DELETE FROM spo2_samples WHERE day_key=$1::date`, day)
		st.Pool().Exec(ctx, `DELETE FROM resp_samples WHERE day_key=$1::date`, day)
		st.Pool().Exec(ctx, `DELETE FROM sleep_sessions WHERE day_key=$1::date`, day)
		st.Pool().Exec(ctx, `DELETE FROM daily_metrics WHERE day_key=$1::date`, day)
	}
	clean()
	t.Cleanup(clean)
	st.Pool().Exec(ctx, `INSERT INTO daily_metrics (day_key) VALUES ($1::date)`, day)

	sleepStart := time.Date(2026, 6, 26, 22, 0, 0, 0, time.UTC) // sleep window: 22:00 -> 06:00 next day
	sleepEnd := sleepStart.Add(8 * time.Hour)
	_, err := st.Pool().Exec(ctx, `
		INSERT INTO sleep_sessions (started_at, day_key, score, total_mins, deep_mins, rem_mins, light_mins, wake_mins, is_nap, source_session_id)
		VALUES ($1, $2::date, 80, 480, 100, 100, 280, 0, false, 'sess-sleep-1')`,
		sleepStart, day)
	if err != nil {
		t.Fatalf("seed sleep: %v", err)
	}

	inWindow := sleepStart.Add(2 * time.Hour)  // inside sleep window
	outOfWindow := sleepEnd.Add(4 * time.Hour) // daytime, must be excluded

	if err := st.UpsertHrvSamples(ctx, "sess-hrv", []store.SampleValue{
		{DayKey: day, SampledAt: inWindow, Value: 50},
		{DayKey: day, SampledAt: outOfWindow, Value: 999}, // must be excluded
	}); err != nil {
		t.Fatalf("seed hrv: %v", err)
	}

	if err := RecomputeDailyVitals(ctx, st, []string{day}); err != nil {
		t.Fatalf("recompute: %v", err)
	}

	var hrv *int
	if err := st.Pool().QueryRow(ctx,
		`SELECT hrv_rmssd FROM daily_metrics WHERE day_key=$1::date`, day).Scan(&hrv); err != nil {
		t.Fatalf("read: %v", err)
	}
	if hrv == nil || *hrv != 50 {
		t.Fatalf("hrv_rmssd=%v, want 50 (daytime sample must be excluded)", hrv)
	}
}

func TestRecomputeDailyVitalsUsesStageTimeline(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	const day = "2026-06-28"

	clean := func() {
		st.Pool().Exec(ctx, `DELETE FROM hrv_samples WHERE day_key=$1::date`, day)
		st.Pool().Exec(ctx, `DELETE FROM sleep_sessions WHERE day_key=$1::date`, day)
		st.Pool().Exec(ctx, `DELETE FROM daily_metrics WHERE day_key=$1::date`, day)
	}
	clean()
	t.Cleanup(clean)
	st.Pool().Exec(ctx, `INSERT INTO daily_metrics (day_key) VALUES ($1::date)`, day)

	stageStart := time.Date(2026, 6, 27, 20, 30, 0, 0, time.UTC)
	stageEnd := stageStart.Add(8 * time.Hour)
	err := st.UpsertSleepSessions(ctx, "sess-stage-window", []store.SleepRow{{
		DayKey: day, StartedAt: stageEnd.Add(30 * time.Minute), Score: 80,
		TotalMins: 450, Stages: []store.SleepStagePoint{{
			Start: stageStart, End: stageEnd, Type: 4,
		}},
	}})
	if err != nil {
		t.Fatalf("seed sleep: %v", err)
	}
	if err := st.UpsertHrvSamples(ctx, "sess-stage-hrv", []store.SampleValue{{
		DayKey: day, SampledAt: stageStart.Add(time.Hour), Value: 48,
	}}); err != nil {
		t.Fatalf("seed hrv: %v", err)
	}

	if err := RecomputeDailyVitals(ctx, st, []string{day}); err != nil {
		t.Fatalf("recompute: %v", err)
	}
	var hrv *int
	if err := st.Pool().QueryRow(ctx,
		`SELECT hrv_rmssd FROM daily_metrics WHERE day_key=$1::date`, day).Scan(&hrv); err != nil {
		t.Fatalf("read: %v", err)
	}
	if hrv == nil || *hrv != 48 {
		t.Fatalf("hrv_rmssd=%v, want 48 from stage timeline", hrv)
	}
}
