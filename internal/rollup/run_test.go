package rollup

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Anshuman115/heliolytics/internal/store"
	"github.com/Anshuman115/heliolytics/internal/testutil"
	"github.com/jackc/pgx/v5"
)

const testMAC = "02:00:00:00:00:01"

var testCatalog = []byte(`{"chunked":[{"code":"0x01","roundStart":"2026-10-20T00:00:00","roundSegments":[]}]}`)

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	dsn := testutil.NewDB(t)
	st, err := store.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(st.Close)
	return st
}

func seed(t *testing.T, st *store.Store, sessionID string, fn func(ctx context.Context, tx pgx.Tx) error) {
	t.Helper()
	ctx := context.Background()
	err := st.WithTx(ctx, func(tx pgx.Tx) error {
		if err := st.LockCanonical(ctx, tx); err != nil {
			return err
		}
		_, err := st.WriteSession(ctx, tx, store.SessionMeta{
			ID:        sessionID,
			DeviceMAC: testMAC,
			StartedAt: testDay,
			EndedAt:   testDay.Add(time.Hour),
		}, testCatalog, []store.RawBlob{{TypeCode: "0x01", Payload: []byte{0x01}}})
		if err != nil {
			return err
		}
		return fn(ctx, tx)
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func runRollup(t *testing.T, st *store.Store, in RunInput) {
	t.Helper()
	ctx := context.Background()
	if err := st.WithTx(ctx, func(tx pgx.Tx) error {
		return runAt(ctx, st, tx, in, testDay)
	}); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestRunPersistsHonestZeroStepsAndExactDomain(t *testing.T) {
	st := openTestStore(t)
	seed(t, st, "s1", func(ctx context.Context, tx pgx.Tx) error {
		_, err := st.WriteSteps(ctx, tx,
			[]store.StepSample{{At: at(9, 0), Steps: 0}}, "s1", testDay.Add(time.Hour))
		return err
	})
	runRollup(t, st, RunInput{
		TouchedDays: []time.Time{testDay.AddDate(0, 0, -1), testDay},
	})

	ctx := context.Background()
	days, err := st.ListDays(ctx, "2026-10-19", "2026-10-20")
	if err != nil {
		t.Fatalf("list days: %v", err)
	}
	if len(days) != 1 {
		t.Fatalf("daily rows = %d, want only the supported day", len(days))
	}
	if days[0].DayKey != "2026-10-20" {
		t.Fatalf("day key = %s, want 2026-10-20", days[0].DayKey)
	}
	if days[0].Steps == nil || *days[0].Steps != 0 {
		t.Fatalf("steps = %v, want the measured zero", days[0].Steps)
	}
	if len(days[0].ReadinessComponents) == 0 {
		t.Fatal("readiness explanation must persist even without inputs")
	}
	if days[0].ReadinessStatus == nil || *days[0].ReadinessStatus != "no_data" {
		t.Fatalf("readiness status = %v, want no_data", days[0].ReadinessStatus)
	}
}

func TestRunExcludesSnapshotHeartRateFromDailyMean(t *testing.T) {
	st := openTestStore(t)
	seed(t, st, "s1", func(ctx context.Context, tx pgx.Tx) error {
		_, err := st.WriteHeartRate(ctx, tx, []store.HeartRateSample{
			{At: at(9, 0), Source: "0x01", BPM: 60},
			{At: at(9, 1), Source: "0x01", BPM: 80},
			{At: at(9, 0), Source: "0x46", BPM: 180},
		}, "s1", testDay.Add(time.Hour))
		return err
	})
	runRollup(t, st, RunInput{TouchedDays: []time.Time{testDay}})

	days, err := st.ListDays(context.Background(), "2026-10-20", "2026-10-20")
	if err != nil {
		t.Fatalf("list days: %v", err)
	}
	if len(days) != 1 {
		t.Fatalf("daily rows = %d, want 1", len(days))
	}
	if days[0].AvgHr == nil || *days[0].AvgHr != 70 {
		t.Fatalf("avg hr = %v, want 70", days[0].AvgHr)
	}
}

func TestRunRollsBackWithCallerFailure(t *testing.T) {
	st := openTestStore(t)
	seed(t, st, "s1", func(ctx context.Context, tx pgx.Tx) error {
		_, err := st.WriteSteps(ctx, tx,
			[]store.StepSample{{At: at(9, 0), Steps: 10}}, "s1", testDay.Add(time.Hour))
		return err
	})

	ctx := context.Background()
	forced := errors.New("forced failure")
	err := st.WithTx(ctx, func(tx pgx.Tx) error {
		if err := runAt(ctx, st, tx, RunInput{TouchedDays: []time.Time{testDay}}, testDay); err != nil {
			return err
		}
		return forced
	})
	if !errors.Is(err, forced) {
		t.Fatalf("transaction error = %v, want the forced failure", err)
	}
	days, err := st.ListDays(ctx, "2026-10-20", "2026-10-20")
	if err != nil {
		t.Fatalf("list days: %v", err)
	}
	if len(days) != 0 {
		t.Fatalf("daily rows = %d, want none after a rollback", len(days))
	}
}

func TestRunDetectsAutoActivityAndCountsItsDay(t *testing.T) {
	st := openTestStore(t)
	seed(t, st, "s1", func(ctx context.Context, tx pgx.Tx) error {
		steps := make([]store.StepSample, 0, 20)
		for i := range 20 {
			steps = append(steps, store.StepSample{At: at(9, i), Steps: 130})
		}
		_, err := st.WriteSteps(ctx, tx, steps, "s1", testDay.Add(time.Hour))
		return err
	})

	ctx := context.Background()
	err := st.WithTx(ctx, func(tx pgx.Tx) error {
		return runAt(ctx, st, tx, RunInput{
			TouchedDays:       []time.Time{testDay},
			MutationIntervals: []store.InstantInterval{{From: at(9, 0), To: at(9, 20)}},
			Auto:              &AutoTrigger{SessionID: "s1", ObservedAt: at(9, 30)},
		}, testDay)
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	activities, err := st.ListActivities(ctx, "2026-10-20", "2026-10-20")
	if err != nil {
		t.Fatalf("list activities: %v", err)
	}
	if len(activities) != 1 {
		t.Fatalf("activities = %d, want the single detected session", len(activities))
	}
	if activities[0].Row.Kind != "auto" {
		t.Fatalf("kind = %s, want auto", activities[0].Row.Kind)
	}
	if activities[0].DurationSec != 1200 {
		t.Fatalf("duration = %d, want 1200 seconds", activities[0].DurationSec)
	}

	days, err := st.ListDays(ctx, "2026-10-20", "2026-10-20")
	if err != nil {
		t.Fatalf("list days: %v", err)
	}
	if len(days) != 1 {
		t.Fatalf("daily rows = %d, want 1", len(days))
	}
	if days[0].AutoActivityCount != 1 || days[0].WorkoutCount != 0 {
		t.Fatalf("counts = %d/%d, want 1 auto and 0 workouts",
			days[0].AutoActivityCount, days[0].WorkoutCount)
	}
}

func TestRunWithoutAutoTriggerSkipsDetection(t *testing.T) {
	st := openTestStore(t)
	seed(t, st, "s1", func(ctx context.Context, tx pgx.Tx) error {
		steps := make([]store.StepSample, 0, 20)
		for i := range 20 {
			steps = append(steps, store.StepSample{At: at(9, i), Steps: 130})
		}
		_, err := st.WriteSteps(ctx, tx, steps, "s1", testDay.Add(time.Hour))
		return err
	})

	ctx := context.Background()
	err := st.WithTx(ctx, func(tx pgx.Tx) error {
		return runAt(ctx, st, tx, RunInput{TouchedDays: []time.Time{testDay}}, testDay)
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	activities, err := st.ListActivities(ctx, "2026-10-20", "2026-10-20")
	if err != nil {
		t.Fatalf("list activities: %v", err)
	}
	if len(activities) != 0 {
		t.Fatalf("activities = %d, want none without an auto trigger", len(activities))
	}
}

func TestRunReadsSleepHaloSamplesAcrossMidnight(t *testing.T) {
	st := openTestStore(t)
	seed(t, st, "s1", func(ctx context.Context, tx pgx.Tx) error {
		sleep := store.SleepRow{
			StartedAt: at(-2, 0),
			EndedAt:   at(1, 0),
			DayKey:    dayStart(testDay),
			Score:     new(80),
			TotalMins: 170,
			DeepMins:  60,
			RemMins:   50,
			LightMins: 60,
			WakeMins:  10,
		}
		if _, err := st.WriteSleep(ctx, tx, []store.SleepRow{sleep}, "s1", testDay.Add(time.Hour), false); err != nil {
			return err
		}
		hr := make([]store.HeartRateSample, 0, 180)
		for i := range 180 {
			hr = append(hr, store.HeartRateSample{
				At:     at(-2, 0).Add(time.Duration(i) * time.Minute),
				Source: "0x01",
				BPM:    52,
			})
		}
		_, err := st.WriteHeartRate(ctx, tx, hr, "s1", testDay.Add(time.Hour))
		return err
	})
	runRollup(t, st, RunInput{TouchedDays: []time.Time{testDay}})

	days, err := st.ListDays(context.Background(), "2026-10-20", "2026-10-20")
	if err != nil {
		t.Fatalf("list days: %v", err)
	}
	if len(days) != 1 {
		t.Fatalf("daily rows = %d, want 1", len(days))
	}
	if days[0].RestingHr == nil || *days[0].RestingHr != 52 {
		t.Fatalf("resting hr = %v, want 52 from the previous evening's halo", days[0].RestingHr)
	}
	if days[0].SleepScore == nil || *days[0].SleepScore != 80 {
		t.Fatalf("sleep score = %v, want 80", days[0].SleepScore)
	}
	if days[0].TimeInBedMins == nil || *days[0].TimeInBedMins != 180 {
		t.Fatalf("time in bed = %v, want 180", days[0].TimeInBedMins)
	}
}
