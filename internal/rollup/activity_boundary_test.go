package rollup

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/Anshuman115/heliolytics/internal/store"
	"github.com/jackc/pgx/v5"
)

func activitySteps(from time.Time, count, steps int) []store.StepSample {
	rows := make([]store.StepSample, count)
	for i := range rows {
		rows[i] = store.StepSample{At: from.Add(time.Duration(i) * time.Minute), Steps: steps}
	}
	return rows
}

func recomputeActivity(t *testing.T, st *store.Store, from, to time.Time, session string) {
	t.Helper()
	runRollup(t, st, RunInput{
		MutationIntervals: []store.InstantInterval{{From: from, To: to}},
		Auto:              &AutoTrigger{SessionID: session, ObservedAt: testDay.Add(2 * time.Hour)},
	})
}

func assertAutoIntervals(t *testing.T, st *store.Store, want ...store.InstantInterval) {
	t.Helper()
	rows, err := st.ListActivities(context.Background(), "2026-10-01", "2026-10-31")
	if err != nil {
		t.Fatal(err)
	}
	var got []store.InstantInterval
	for _, row := range rows {
		if row.Row.Kind == "auto" {
			got = append(got, store.InstantInterval{From: row.Row.StartedAt, To: row.Row.EndedAt})
		}
	}
	equal := len(got) == len(want)
	for i := range got {
		if i >= len(want) || !got[i].From.Equal(want[i].From) || !got[i].To.Equal(want[i].To) {
			equal = false
		}
	}
	if !equal {
		t.Fatalf("auto intervals = %v, want %v", got, want)
	}
}

func activityRawState(t *testing.T, st *store.Store, from, to time.Time) ([]store.ActivityMinute, []string) {
	t.Helper()
	var minutes []store.ActivityMinute
	var workouts []string
	err := st.WithTx(context.Background(), func(tx pgx.Tx) error {
		var err error
		minutes, err = store.NewTxView(tx).ReadActivityMinutes(context.Background(), from, to)
		if err != nil {
			return err
		}
		rows, err := tx.Query(context.Background(), `SELECT row_to_json(a)::text FROM activities a WHERE kind = 'workout' ORDER BY started_at`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var row string
			if err := rows.Scan(&row); err != nil {
				return err
			}
			workouts = append(workouts, row)
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	return minutes, workouts
}

func TestAutoIdleCorrectionPreservesWalkAcrossDistantQueryEdge(t *testing.T) {
	for _, direction := range []int{-1, 1} {
		for _, duration := range []int{19, 30} {
			t.Run(fmt.Sprintf("direction_%d_duration_%d", direction, duration), func(t *testing.T) {
				st := openTestStore(t)
				from := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
				to := from.Add(10 * 24 * time.Hour)
				correction := from.Add(5 * 24 * time.Hour)
				edge := correction.Add(time.Duration(direction) * 84 * time.Hour)
				start := edge.Add(-15 * time.Minute)
				if direction > 0 {
					edge = edge.Add(time.Minute)
					start = edge.Add(time.Duration(15-duration) * time.Minute)
				}
				end := start.Add(time.Duration(duration) * time.Minute)
				steps := activitySteps(from, 10*24*60, 0)
				for i := range steps {
					if !steps[i].At.Before(start) && steps[i].At.Before(end) {
						steps[i].Steps = 100
					}
				}
				seed(t, st, "initial", func(ctx context.Context, tx pgx.Tx) error {
					if _, err := st.WriteSteps(ctx, tx, steps, "initial", testDay); err != nil {
						return err
					}
					_, err := st.WriteWorkouts(ctx, tx, []store.ActivityRow{{
						StartedAt: from.Add(time.Hour), EndedAt: from.Add(2 * time.Hour),
						SportType: 1, SportName: "Recorded", Steps: new(123), HasDetail: true,
					}}, "initial", testDay, false)
					return err
				})
				recomputeActivity(t, st, from, to, "initial")
				want := store.InstantInterval{From: start, To: end}
				assertAutoIntervals(t, st, want)
				before, workouts := activityRawState(t, st, from, to)
				seed(t, st, "correction", func(ctx context.Context, tx pgx.Tx) error {
					_, err := st.WriteSteps(ctx, tx, []store.StepSample{{At: correction, Steps: 1}}, "correction", testDay.Add(time.Hour))
					return err
				})
				recomputeActivity(t, st, correction, correction.Add(time.Minute), "correction")
				after, afterWorkouts := activityRawState(t, st, from, to)
				for i := range before {
					if before[i].At.Equal(correction) {
						before[i].Steps = 1
					}
				}
				if !reflect.DeepEqual(before, after) {
					t.Fatal("automatic recomputation changed step rows beyond the accepted correction")
				}
				if !reflect.DeepEqual(workouts, afterWorkouts) || len(workouts) != 1 {
					t.Fatal("automatic recomputation changed the recorded workout")
				}
				assertAutoIntervals(t, st, want)
			})
		}
	}
}

func TestAutoNewMovementExtendsBeyondContextHalos(t *testing.T) {
	st := openTestStore(t)
	from := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	to := from.Add(20 * time.Hour)
	seed(t, st, "movement", func(ctx context.Context, tx pgx.Tx) error {
		_, err := st.WriteSteps(ctx, tx, activitySteps(from, 20*60, 60), "movement", testDay)
		return err
	})
	recomputeActivity(t, st, from, from.Add(time.Minute), "movement")
	assertAutoIntervals(t, st, store.InstantInterval{From: from, To: to})
}

func TestAutoCorrectionsMergeAndSplitAtIdleReset(t *testing.T) {
	st := openTestStore(t)
	from := at(9, 0)
	steps := activitySteps(from, 24, 100)
	for i := 10; i < 14; i++ {
		steps[i].Steps = 0
	}
	seed(t, st, "initial", func(ctx context.Context, tx pgx.Tx) error {
		_, err := st.WriteSteps(ctx, tx, steps, "initial", testDay)
		return err
	})
	recomputeActivity(t, st, from, from.Add(24*time.Minute), "initial")
	left := store.InstantInterval{From: from, To: from.Add(10 * time.Minute)}
	right := store.InstantInterval{From: from.Add(14 * time.Minute), To: from.Add(24 * time.Minute)}
	assertAutoIntervals(t, st, left, right)
	for index, change := range []struct {
		session string
		steps   int
		want    []store.InstantInterval
	}{
		{"merge", 100, []store.InstantInterval{{From: from, To: from.Add(24 * time.Minute)}}},
		{"split", 0, []store.InstantInterval{left, right}},
	} {
		seed(t, st, change.session, func(ctx context.Context, tx pgx.Tx) error {
			_, err := st.WriteSteps(ctx, tx, []store.StepSample{{At: from.Add(10 * time.Minute), Steps: change.steps}}, change.session, testDay.Add(time.Duration(index+1)*time.Hour))
			return err
		})
		recomputeActivity(t, st, from.Add(10*time.Minute), from.Add(11*time.Minute), change.session)
		assertAutoIntervals(t, st, change.want...)
	}
}

func TestAutoRepairsWholeSpanningIntervalAfterContextGrowth(t *testing.T) {
	st := openTestStore(t)
	from := at(0, 0)
	to := from.Add(14*time.Hour + 20*time.Minute)
	steps := activitySteps(from, 14*60+20, 100)
	for i := 13 * 60; i < 14*60; i++ {
		steps[i].Steps = 0
	}
	seed(t, st, "existing", func(ctx context.Context, tx pgx.Tx) error {
		if _, err := st.WriteSteps(ctx, tx, steps, "existing", testDay); err != nil {
			return err
		}
		_, err := store.NewTxView(tx).ReplaceAutoActivities(ctx, from, to, []store.ActivityRow{{
			StartedAt: from.Add(11 * time.Hour), EndedAt: to, SportName: "Walk",
		}}, "existing", testDay)
		return err
	})
	recomputeActivity(t, st, from, from.Add(time.Minute), "existing")
	assertAutoIntervals(t, st,
		store.InstantInterval{From: from, To: from.Add(13 * time.Hour)},
		store.InstantInterval{From: from.Add(14 * time.Hour), To: to})
}

func TestAutoWorkoutMinuteOverlapMissingAndEndpointAdjacency(t *testing.T) {
	st := openTestStore(t)
	from := at(9, 0)
	steps := activitySteps(from, 42, 100)
	steps = append(steps[:30], steps[31:]...)
	seed(t, st, "initial", func(ctx context.Context, tx pgx.Tx) error {
		if _, err := st.WriteSteps(ctx, tx, steps, "initial", testDay); err != nil {
			return err
		}
		_, err := st.WriteWorkouts(ctx, tx, []store.ActivityRow{{
			StartedAt: from.Add(10*time.Minute + 30*time.Second), EndedAt: from.Add(20 * time.Minute), SportName: "Recorded",
		}}, "initial", testDay, false)
		return err
	})
	recomputeActivity(t, st, from, from.Add(42*time.Minute), "initial")
	want := []store.InstantInterval{
		{From: from, To: from.Add(10 * time.Minute)},
		{From: from.Add(20 * time.Minute), To: from.Add(30 * time.Minute)},
		{From: from.Add(31 * time.Minute), To: from.Add(42 * time.Minute)},
	}
	assertAutoIntervals(t, st, want...)
	_, before := activityRawState(t, st, from, from.Add(42*time.Minute))
	recomputeActivity(t, st, from.Add(10*time.Minute), from.Add(20*time.Minute), "initial")
	assertAutoIntervals(t, st, want...)
	_, after := activityRawState(t, st, from, from.Add(42*time.Minute))
	if !reflect.DeepEqual(before, after) {
		t.Fatal("workout barrier recomputation changed the recorded workout")
	}
	runRollup(t, st, RunInput{
		TouchedDays:       []time.Time{testDay},
		MutationIntervals: []store.InstantInterval{{From: from, To: from.Add(42 * time.Minute)}},
	})
	assertAutoIntervals(t, st, want...)
}
