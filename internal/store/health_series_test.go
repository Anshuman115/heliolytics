package store

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestListDaysKeepsExplicitNullsAndDisplayBands(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	day := Day(2026, time.October, 1)

	rows, err := st.ListDays(ctx, "2026-10-01", "2026-10-01")
	if err != nil {
		t.Fatalf("list days: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("rows before any daily write = %d, want none", len(rows))
	}

	if _, err := st.pool.Exec(ctx, `INSERT INTO daily_metrics (day_key) VALUES ($1)`, day); err != nil {
		t.Fatalf("seed daily row: %v", err)
	}
	rows, err = st.ListDays(ctx, "2026-10-01", "2026-10-01")
	if err != nil {
		t.Fatalf("list days: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	got := rows[0]
	if got.Steps != nil || got.AvgHr != nil || got.RestingHr != nil || got.SkinTempNightC != nil {
		t.Fatalf("nullable metrics = %+v, want null", got)
	}
	if got.Spo2Samples != 0 || got.NapCount != 0 || got.WorkoutCount != 0 || got.AutoActivityCount != 0 {
		t.Fatalf("counts = %+v, want zero without measurements", got)
	}
	if got.Readiness != nil || got.ReadinessStatus != nil || got.ReadinessBand != nil {
		t.Fatalf("readiness = %+v, want null", got)
	}
	if got.ReadinessComponents != nil {
		t.Fatalf("components = %s, want null", got.ReadinessComponents)
	}

	if _, err := st.pool.Exec(ctx, `
		UPDATE daily_metrics SET readiness = 70, pai_score = 0,
		  readiness_components = '{"version":1,"status":"scored"}'::jsonb
		WHERE day_key = $1`, day); err != nil {
		t.Fatalf("update readiness: %v", err)
	}
	rows, err = st.ListDays(ctx, "2026-10-01", "2026-10-01")
	if err != nil {
		t.Fatalf("list days: %v", err)
	}
	got = rows[0]
	if got.Readiness == nil || *got.Readiness != 70 {
		t.Fatalf("readiness = %v", got.Readiness)
	}
	if got.ReadinessStatus == nil || *got.ReadinessStatus != "scored" {
		t.Fatalf("readiness status = %v", got.ReadinessStatus)
	}
	if got.ReadinessBand == nil || *got.ReadinessBand != "good" {
		t.Fatalf("readiness band = %v", got.ReadinessBand)
	}
	if got.PaiScore == nil || *got.PaiScore != 0 {
		t.Fatalf("pai score = %v, want a measured zero", got.PaiScore)
	}
}

func TestListHealthSamplesCompactShape(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	day := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	observedAt := day.Add(24 * time.Hour)
	seedSession(t, st, "series-compact", day, observedAt)

	mustTx(t, st, func(tx pgx.Tx) error {
		if _, err := st.WriteHRV(ctx, tx, []HRVSample{
			{At: day.Add(time.Hour), RMSSDMS: 50},
			{At: day.Add(2 * time.Hour), RMSSDMS: 55},
		}, "series-compact", observedAt); err != nil {
			return err
		}
		if _, err := st.WriteStress(ctx, tx,
			[]StressSample{{At: day.Add(time.Hour), Stress: 30}}, "series-compact", observedAt); err != nil {
			return err
		}
		if _, err := st.WriteResp(ctx, tx,
			[]RespSample{{At: day.Add(time.Hour), BRMin: 15.5}}, "series-compact", observedAt); err != nil {
			return err
		}
		_, err := st.WriteSpO2(ctx, tx,
			[]SpO2Sample{{At: day.Add(time.Hour), Source: "0x25", Mode: "auto", Pct: 97}},
			"series-compact", observedAt)
		return err
	})

	days, err := st.ListHealthSamplesCompact(ctx, "2026-10-01", "2026-10-01")
	if err != nil {
		t.Fatalf("series: %v", err)
	}
	if len(days) != 4 {
		t.Fatalf("series rows = %d, want four metrics", len(days))
	}
	metrics := map[string]SeriesDay{}
	for _, d := range days {
		metrics[d.Metric] = d
		if d.DayKey != "2026-10-01" {
			t.Fatalf("%s dayKey = %s", d.Metric, d.DayKey)
		}
		if len(d.Offsets) != len(d.Values) {
			t.Fatalf("%s offsets and values differ in length: %+v", d.Metric, d)
		}
	}
	hrv, ok := metrics["hrv"]
	if !ok {
		t.Fatalf("hrv series missing: %+v", days)
	}
	if len(hrv.Values) != 2 || hrv.Values[0] != 50 || hrv.Values[1] != 55 {
		t.Fatalf("hrv values = %v", hrv.Values)
	}
	if hrv.Offsets[1] != 3600 {
		t.Fatalf("hrv offsets = %v", hrv.Offsets)
	}
	for _, name := range []string{"stress", "resp", "spo2"} {
		if _, ok := metrics[name]; !ok {
			t.Fatalf("%s series missing", name)
		}
	}
}

func TestListSleepAndActivitiesCarryAttributionDays(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	day := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	observedAt := day.Add(24 * time.Hour)
	seedSession(t, st, "attribution", day, observedAt)

	mustTx(t, st, func(tx pgx.Tx) error {
		if _, err := st.WriteSleep(ctx, tx, []SleepRow{
			sleepCandidate(day.Add(time.Hour), day.Add(2*time.Hour)),
		}, "attribution", observedAt, false); err != nil {
			return err
		}
		_, err := st.WriteWorkouts(ctx, tx, []ActivityRow{
			workoutCandidate(day.Add(3*time.Hour), day.Add(4*time.Hour)),
		}, "attribution", observedAt, false)
		return err
	})

	sleep, err := st.ListSleep(ctx, "2026-10-01", "2026-10-01")
	if err != nil {
		t.Fatalf("list sleep: %v", err)
	}
	if len(sleep) != 1 || sleep[0].DayKey != "2026-10-01" {
		t.Fatalf("sleep = %+v", sleep)
	}
	if sleep[0].Row.StagesTruncated != true {
		t.Fatal("stages_truncated not preserved")
	}

	activities, err := st.ListActivities(ctx, "2026-10-01", "2026-10-01")
	if err != nil {
		t.Fatalf("list activities: %v", err)
	}
	if len(activities) != 1 {
		t.Fatalf("activities = %+v", activities)
	}
	if activities[0].DayKey != "2026-10-01" || activities[0].DurationSec != 3600 {
		t.Fatalf("activity = %+v, want dayKey 2026-10-01 and 3600 seconds", activities[0])
	}
	if activities[0].Row.Kind != "workout" {
		t.Fatalf("activity kind = %s", activities[0].Row.Kind)
	}
}
