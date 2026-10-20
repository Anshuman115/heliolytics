package store

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

var chainDay = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

func sleepCandidate(start, end time.Time) SleepRow {
	return SleepRow{
		StartedAt:       start,
		EndedAt:         end,
		DayKey:          Day(2026, time.October, 1),
		IsNap:           false,
		Score:           new(50),
		TotalMins:       60,
		LightMins:       60,
		Stages:          []SleepStagePoint{},
		StagesTruncated: true,
	}
}

func workoutCandidate(start, end time.Time) ActivityRow {
	return ActivityRow{
		StartedAt: start,
		EndedAt:   end,
		Kind:      "workout",
		SportType: 1,
		SportName: "Run",
	}
}

func writeSleep(t *testing.T, st *Store, existing bool, sessionID string, observedAt time.Time, rows ...SleepRow) Mutation {
	t.Helper()
	var out Mutation
	if err := st.WithTx(context.Background(), func(tx pgx.Tx) error {
		var err error
		out, err = st.WriteSleep(context.Background(), tx, rows, sessionID, observedAt, existing)
		return err
	}); err != nil {
		t.Fatalf("write sleep: %v", err)
	}
	return out
}

func writeWorkouts(t *testing.T, st *Store, existing bool, sessionID string, observedAt time.Time, rows ...ActivityRow) Mutation {
	t.Helper()
	var out Mutation
	if err := st.WithTx(context.Background(), func(tx pgx.Tx) error {
		var err error
		out, err = st.WriteWorkouts(context.Background(), tx, rows, sessionID, observedAt, existing)
		return err
	}); err != nil {
		t.Fatalf("write workouts: %v", err)
	}
	return out
}

func canonicalSleep(t *testing.T, st *Store) []SleepRow {
	t.Helper()
	metrics, err := st.ListSleep(context.Background(), "2026-09-20", "2026-10-10")
	if err != nil {
		t.Fatalf("list sleep: %v", err)
	}
	out := make([]SleepRow, 0, len(metrics))
	for _, m := range metrics {
		out = append(out, m.Row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.Before(out[j].StartedAt) })
	return out
}

func canonicalWorkouts(t *testing.T, st *Store) []ActivityRow {
	t.Helper()
	metrics, err := st.ListActivities(context.Background(), "2026-09-20", "2026-10-10")
	if err != nil {
		t.Fatalf("list activities: %v", err)
	}
	out := make([]ActivityRow, 0, len(metrics))
	for _, m := range metrics {
		if m.Row.Kind != "workout" {
			continue
		}
		out = append(out, m.Row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.Before(out[j].StartedAt) })
	return out
}

var chainOrders = [][]string{
	{"a", "b", "c"}, {"a", "c", "b"}, {"b", "a", "c"},
	{"b", "c", "a"}, {"c", "a", "b"}, {"c", "b", "a"},
}

func chainInterval(name string) (time.Time, time.Time) {
	base := chainDay
	switch name {
	case "a":
		return base.Add(time.Hour), base.Add(2 * time.Hour)
	case "b":
		return base.Add(90 * time.Minute), base.Add(150 * time.Minute)
	default:
		return base.Add(2 * time.Hour), base.Add(3 * time.Hour)
	}
}

func chainObservedAt(name string) time.Time {
	base := chainDay
	switch name {
	case "a":
		return base.Add(12 * time.Hour)
	case "b":
		return base.Add(13 * time.Hour)
	default:
		return base.Add(14 * time.Hour)
	}
}

func chainWinners(uploaded []string) map[string]bool {
	ordered := make([]string, len(uploaded))
	copy(ordered, uploaded)
	sort.Slice(ordered, func(i, j int) bool {
		ti, tj := chainObservedAt(ordered[i]), chainObservedAt(ordered[j])
		if !ti.Equal(tj) {
			return ti.After(tj)
		}
		return ordered[i] > ordered[j]
	})
	winners := make([]string, 0, len(ordered))
	for _, name := range ordered {
		start, end := chainInterval(name)
		blocked := false
		for _, w := range winners {
			wStart, wEnd := chainInterval(w)
			if start.Before(wEnd) && end.After(wStart) {
				blocked = true
				break
			}
		}
		if !blocked {
			winners = append(winners, name)
		}
	}
	out := make(map[string]bool, len(winners))
	for _, name := range winners {
		out[name] = true
	}
	return out
}

func sameNames(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for name := range a {
		if !b[name] {
			return false
		}
	}
	return true
}

func TestSleepOverlapChainKeepsEveryCandidateAndEndsAtAC(t *testing.T) {
	for _, order := range chainOrders {
		t.Run(strings.Join(order, ""), func(t *testing.T) {
			st := newTestStore(t)
			for _, name := range []string{"a", "b", "c"} {
				started := chainDay
				seedSession(t, st, "chain-"+name, started, chainObservedAt(name))
			}

			uploaded := make([]string, 0, len(order))
			previous := map[string]bool{}
			for step, name := range order {
				uploaded = append(uploaded, name)
				start, end := chainInterval(name)
				mut := writeSleep(t, st, false, "chain-"+name, chainObservedAt(name), sleepCandidate(start, end))
				want := chainWinners(uploaded)
				wantChanged := !sameNames(want, previous)
				if mut.Changed != wantChanged {
					t.Fatalf("step %d (%s) changed = %v, want %v", step, name, mut.Changed, wantChanged)
				}
				got := canonicalSleep(t, st)
				gotNames := map[string]bool{}
				for _, row := range got {
					gotNames[nameForInterval(row.StartedAt)] = true
				}
				if !sameNames(gotNames, want) {
					t.Fatalf("step %d canonical = %v, want %v", step, gotNames, want)
				}
				previous = want
			}
			if n := countRows(t, st, "sleep_observations"); n != 3 {
				t.Fatalf("candidate rows = %d, want all three retained", n)
			}

			startC, endC := chainInterval("c")
			startA, endA := chainInterval("a")
			retry := writeSleep(t, st, true, "chain-c", chainObservedAt("c"), sleepCandidate(startC, endC))
			if retry.Changed {
				t.Fatalf("exact candidate retry reported a change: %+v", retry)
			}
			rows := canonicalSleep(t, st)
			if len(rows) != 2 {
				t.Fatalf("canonical rows = %d, want A and C", len(rows))
			}
			if !rows[0].StartedAt.Equal(startA) || !rows[0].EndedAt.Equal(endA) {
				t.Fatalf("first canonical sleep = %v", rows[0])
			}
			if !rows[1].StartedAt.Equal(startC) || !rows[1].EndedAt.Equal(endC) {
				t.Fatalf("second canonical sleep = %v", rows[1])
			}
			if rows[0].TotalMins+rows[1].TotalMins != 120 {
				t.Fatalf("canonical sleep minutes = %d, want 120", rows[0].TotalMins+rows[1].TotalMins)
			}
		})
	}
}

func nameForInterval(start time.Time) string {
	switch {
	case start.Equal(chainDay.Add(time.Hour)):
		return "a"
	case start.Equal(chainDay.Add(90 * time.Minute)):
		return "b"
	case start.Equal(chainDay.Add(2 * time.Hour)):
		return "c"
	default:
		return "?"
	}
}

func TestWorkoutOverlapChainKeepsEveryCandidate(t *testing.T) {
	for _, order := range chainOrders {
		t.Run(strings.Join(order, ""), func(t *testing.T) {
			st := newTestStore(t)
			for _, name := range []string{"a", "b", "c"} {
				seedSession(t, st, "chain-"+name, chainDay, chainObservedAt(name))
			}
			uploaded := make([]string, 0, len(order))
			previous := map[string]bool{}
			for step, name := range order {
				uploaded = append(uploaded, name)
				start, end := chainInterval(name)
				mut := writeWorkouts(t, st, false, "chain-"+name, chainObservedAt(name), workoutCandidate(start, end))
				want := chainWinners(uploaded)
				wantChanged := !sameNames(want, previous)
				if mut.Changed != wantChanged {
					t.Fatalf("step %d (%s) changed = %v, want %v", step, name, mut.Changed, wantChanged)
				}
				previous = want
			}
			rows := canonicalWorkouts(t, st)
			if len(rows) != 2 {
				t.Fatalf("canonical workouts = %d, want A and C", len(rows))
			}
			if !rows[0].StartedAt.Equal(chainDay.Add(time.Hour)) || !rows[1].StartedAt.Equal(chainDay.Add(2*time.Hour)) {
				t.Fatalf("canonical workout starts = %v, %v", rows[0].StartedAt, rows[1].StartedAt)
			}
			if n := countRows(t, st, "workout_observations"); n != 3 {
				t.Fatalf("workout candidates = %d, want 3", n)
			}
		})
	}
}

func TestSleepCandidateConflictIsAtomic(t *testing.T) {
	st := newTestStore(t)
	seedSession(t, st, "conflict-a", chainDay, chainObservedAt("a"))
	start, end := chainInterval("a")
	writeSleep(t, st, false, "conflict-a", chainObservedAt("a"), sleepCandidate(start, end))

	changed := sleepCandidate(start, end)
	changed.LightMins = 30
	changed.TotalMins = 30
	err := st.WithTx(context.Background(), func(tx pgx.Tx) error {
		_, err := st.WriteSleep(context.Background(), tx, []SleepRow{changed}, "conflict-a", chainObservedAt("a"), true)
		return err
	})
	if !errors.Is(err, ErrSessionConflict) {
		t.Fatalf("err = %v, want ErrSessionConflict", err)
	}
	if n := countRows(t, st, "sleep_observations"); n != 1 {
		t.Fatalf("candidates = %d, want the original only", n)
	}
	if rows := canonicalSleep(t, st); len(rows) != 1 || rows[0].TotalMins != 60 {
		t.Fatalf("canonical sleep changed after a conflict: %+v", rows)
	}
}

func candidateStarts(t *testing.T, st *Store, table string) []time.Time {
	t.Helper()
	rows, err := st.pool.Query(context.Background(),
		"SELECT started_at FROM "+table+" ORDER BY started_at")
	if err != nil {
		t.Fatalf("query %s starts: %v", table, err)
	}
	defer rows.Close()
	var out []time.Time
	for rows.Next() {
		var at time.Time
		if err := rows.Scan(&at); err != nil {
			t.Fatalf("scan %s start: %v", table, err)
		}
		out = append(out, at)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read %s starts: %v", table, err)
	}
	return out
}

func TestSleepRetryWithShiftedStartConflicts(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	seedSession(t, st, "retry-shift", chainDay, chainObservedAt("c"))
	startA, endA := chainInterval("a")
	startC, endC := chainInterval("c")
	writeSleep(t, st, false, "retry-shift", chainObservedAt("c"),
		sleepCandidate(startA, endA), sleepCandidate(startC, endC))

	shifted := sleepCandidate(startC.Add(30*time.Minute), endC.Add(30*time.Minute))
	err := st.WithTx(ctx, func(tx pgx.Tx) error {
		_, err := st.WriteSleep(ctx, tx,
			[]SleepRow{sleepCandidate(startA, endA), shifted}, "retry-shift", chainObservedAt("c"), true)
		return err
	})
	if !errors.Is(err, ErrSessionConflict) {
		t.Fatalf("err = %v, want ErrSessionConflict", err)
	}
	starts := candidateStarts(t, st, "sleep_observations")
	if len(starts) != 2 || !starts[0].Equal(startA) || !starts[1].Equal(startC) {
		t.Fatalf("candidate starts = %v, want the stored set", starts)
	}
	rows := canonicalSleep(t, st)
	if len(rows) != 2 || !rows[0].StartedAt.Equal(startA) || !rows[1].StartedAt.Equal(startC) {
		t.Fatalf("canonical sleep = %+v, want the stored winners", rows)
	}
}

func TestWorkoutRetryWithDroppedCandidateConflicts(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	seedSession(t, st, "retry-drop", chainDay, chainObservedAt("c"))
	startA, endA := chainInterval("a")
	startC, endC := chainInterval("c")
	writeWorkouts(t, st, false, "retry-drop", chainObservedAt("c"),
		workoutCandidate(startA, endA), workoutCandidate(startC, endC))

	err := st.WithTx(ctx, func(tx pgx.Tx) error {
		_, err := st.WriteWorkouts(ctx, tx,
			[]ActivityRow{workoutCandidate(startA, endA)}, "retry-drop", chainObservedAt("c"), true)
		return err
	})
	if !errors.Is(err, ErrSessionConflict) {
		t.Fatalf("err = %v, want ErrSessionConflict", err)
	}
	starts := candidateStarts(t, st, "workout_observations")
	if len(starts) != 2 || !starts[0].Equal(startA) || !starts[1].Equal(startC) {
		t.Fatalf("candidate starts = %v, want the stored set", starts)
	}
	rows := canonicalWorkouts(t, st)
	if len(rows) != 2 || !rows[0].StartedAt.Equal(startA) || !rows[1].StartedAt.Equal(startC) {
		t.Fatalf("canonical workouts = %+v, want the stored winners", rows)
	}
}

func TestIdenticalCandidateRetryIsANoOp(t *testing.T) {
	st := newTestStore(t)
	seedSession(t, st, "retry-same", chainDay, chainObservedAt("c"))
	startA, endA := chainInterval("a")
	startC, endC := chainInterval("c")

	sleepRows := []SleepRow{sleepCandidate(startA, endA), sleepCandidate(startC, endC)}
	if mut := writeSleep(t, st, false, "retry-same", chainObservedAt("c"), sleepRows...); !mut.Changed {
		t.Fatal("first sleep write reported no change")
	}
	mut := writeSleep(t, st, true, "retry-same", chainObservedAt("c"), sleepRows...)
	if mut.Changed || len(mut.Days) != 0 || len(mut.Intervals) != 0 {
		t.Fatalf("identical sleep retry = %+v, want no mutation", mut)
	}

	workoutRows := []ActivityRow{workoutCandidate(startA, endA), workoutCandidate(startC, endC)}
	if mut := writeWorkouts(t, st, false, "retry-same", chainObservedAt("c"), workoutRows...); !mut.Changed {
		t.Fatal("first workout write reported no change")
	}
	mut = writeWorkouts(t, st, true, "retry-same", chainObservedAt("c"), workoutRows...)
	if mut.Changed || len(mut.Days) != 0 || len(mut.Intervals) != 0 {
		t.Fatalf("identical workout retry = %+v, want no mutation", mut)
	}

	if n := countRows(t, st, "sleep_observations"); n != 2 {
		t.Fatalf("sleep candidates = %d, want 2", n)
	}
	if n := countRows(t, st, "workout_observations"); n != 2 {
		t.Fatalf("workout candidates = %d, want 2", n)
	}
}

func TestSleepZeroCandidateRetryWithCandidateConflicts(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	seedSession(t, st, "zero-sleep", chainDay, chainObservedAt("a"))
	seedSession(t, st, "zero-sleep-new", chainDay, chainObservedAt("a"))
	start, end := chainInterval("a")

	if mut := writeSleep(t, st, false, "zero-sleep", chainObservedAt("a")); mut.Changed {
		t.Fatalf("empty first write = %+v, want no mutation", mut)
	}
	if mut := writeSleep(t, st, true, "zero-sleep", chainObservedAt("a")); mut.Changed {
		t.Fatalf("empty retry = %+v, want no mutation", mut)
	}

	err := st.WithTx(ctx, func(tx pgx.Tx) error {
		_, err := st.WriteSleep(ctx, tx, []SleepRow{sleepCandidate(start, end)},
			"zero-sleep", chainObservedAt("a"), true)
		return err
	})
	if !errors.Is(err, ErrSessionConflict) {
		t.Fatalf("err = %v, want ErrSessionConflict", err)
	}
	if n := countRows(t, st, "sleep_observations"); n != 0 {
		t.Fatalf("candidates after the conflict = %d, want 0", n)
	}
	if rows := canonicalSleep(t, st); len(rows) != 0 {
		t.Fatalf("canonical sleep after the conflict = %+v, want none", rows)
	}

	if mut := writeSleep(t, st, false, "zero-sleep-new", chainObservedAt("a"),
		sleepCandidate(start, end)); !mut.Changed {
		t.Fatal("new session sleep write reported no change")
	}
	if n := countRows(t, st, "sleep_observations"); n != 1 {
		t.Fatalf("candidates after the new session = %d, want 1", n)
	}
}

func TestWorkoutZeroCandidateRetryWithCandidateConflicts(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	seedSession(t, st, "zero-workout", chainDay, chainObservedAt("a"))
	seedSession(t, st, "zero-workout-new", chainDay, chainObservedAt("a"))
	start, end := chainInterval("a")

	if mut := writeWorkouts(t, st, false, "zero-workout", chainObservedAt("a")); mut.Changed {
		t.Fatalf("empty first write = %+v, want no mutation", mut)
	}
	if mut := writeWorkouts(t, st, true, "zero-workout", chainObservedAt("a")); mut.Changed {
		t.Fatalf("empty retry = %+v, want no mutation", mut)
	}

	err := st.WithTx(ctx, func(tx pgx.Tx) error {
		_, err := st.WriteWorkouts(ctx, tx, []ActivityRow{workoutCandidate(start, end)},
			"zero-workout", chainObservedAt("a"), true)
		return err
	})
	if !errors.Is(err, ErrSessionConflict) {
		t.Fatalf("err = %v, want ErrSessionConflict", err)
	}
	if n := countRows(t, st, "workout_observations"); n != 0 {
		t.Fatalf("candidates after the conflict = %d, want 0", n)
	}
	if rows := canonicalWorkouts(t, st); len(rows) != 0 {
		t.Fatalf("canonical workouts after the conflict = %+v, want none", rows)
	}

	if mut := writeWorkouts(t, st, false, "zero-workout-new", chainObservedAt("a"),
		workoutCandidate(start, end)); !mut.Changed {
		t.Fatal("new session workout write reported no change")
	}
	if n := countRows(t, st, "workout_observations"); n != 1 {
		t.Fatalf("candidates after the new session = %d, want 1", n)
	}
}

func TestSleepMovedAttributionDayReturnsBothDays(t *testing.T) {
	st := newTestStore(t)
	base := time.Date(2026, 9, 24, 19, 59, 0, 0, time.UTC) // 2026-09-25 01:29 IST
	start := base
	end := start.Add(4*time.Hour + 32*time.Minute)

	seedSession(t, st, "moved-old", start.Add(-time.Hour), start.Add(10*24*time.Hour))
	seedSession(t, st, "moved-new", start.Add(-time.Hour), start.Add(20*24*time.Hour))

	older := sleepCandidate(start, end)
	older.DayKey = Day(2026, time.September, 26)
	writeSleep(t, st, false, "moved-old", start.Add(10*24*time.Hour), older)

	newer := sleepCandidate(start, end)
	newer.DayKey = Day(2026, time.September, 25)
	mut := writeSleep(t, st, false, "moved-new", start.Add(20*24*time.Hour), newer)

	if !mut.Changed {
		t.Fatal("moved attribution reported no change")
	}
	if len(mut.Days) != 2 {
		t.Fatalf("mutation days = %v, want both old and new ownership", mut.Days)
	}
	if !mut.Days[0].Equal(Day(2026, time.September, 25)) || !mut.Days[1].Equal(Day(2026, time.September, 26)) {
		t.Fatalf("mutation days = %v", mut.Days)
	}
	rows := canonicalSleep(t, st)
	if len(rows) != 1 || !rows[0].DayKey.Equal(Day(2026, time.September, 25)) {
		t.Fatalf("canonical sleep = %+v, want day 2026-09-25", rows)
	}
}

func TestSleepChangedStartReplacesWinner(t *testing.T) {
	st := newTestStore(t)
	start := time.Date(2026, 10, 2, 1, 29, 0, 0, time.UTC)
	seedSession(t, st, "start-old", start.Add(-time.Hour), start.Add(24*time.Hour))
	seedSession(t, st, "start-new", start.Add(-time.Hour), start.Add(48*time.Hour))

	writeSleep(t, st, false, "start-old", start.Add(24*time.Hour), sleepCandidate(start, start.Add(4*time.Hour)))
	mut := writeSleep(t, st, false, "start-new", start.Add(48*time.Hour),
		sleepCandidate(start.Add(time.Minute), start.Add(4*time.Hour+time.Minute)))
	if !mut.Changed {
		t.Fatal("changed start reported no change")
	}
	rows := canonicalSleep(t, st)
	if len(rows) != 1 {
		t.Fatalf("canonical rows = %d, want 1", len(rows))
	}
	if !rows[0].StartedAt.Equal(start.Add(time.Minute)) {
		t.Fatalf("canonical start = %v, want the newer interval", rows[0].StartedAt)
	}
	if n := countRows(t, st, "sleep_observations"); n != 2 {
		t.Fatalf("candidates = %d, want both retained", n)
	}
}

func TestCrossMidnightWorkoutDisplacement(t *testing.T) {
	st := newTestStore(t)
	day := Day(2026, time.October, 2)
	seedSession(t, st, "midnight-a", day.Add(-2*time.Hour), day.Add(24*time.Hour))
	seedSession(t, st, "midnight-b", day.Add(-2*time.Hour), day.Add(48*time.Hour))

	a := workoutCandidate(day.Add(-10*time.Minute), day.Add(10*time.Minute))
	b := workoutCandidate(day, day.Add(20*time.Minute))
	writeWorkouts(t, st, false, "midnight-a", day.Add(24*time.Hour), a)
	mut := writeWorkouts(t, st, false, "midnight-b", day.Add(48*time.Hour), b)

	rows := canonicalWorkouts(t, st)
	if len(rows) != 1 || !rows[0].StartedAt.Equal(day) {
		t.Fatalf("canonical workouts = %+v, want only the newer B", rows)
	}
	if !mut.Changed || len(mut.Days) != 2 {
		t.Fatalf("mutation = %+v, want the displaced day and the new day", mut)
	}
	if n := countRows(t, st, "workout_observations"); n != 2 {
		t.Fatalf("workout candidates = %d, want 2", n)
	}
}

func TestWorkoutReconciliationPreservesAutoRows(t *testing.T) {
	st := newTestStore(t)
	seedSession(t, st, "auto-vs-workout", chainDay, chainObservedAt("a"))
	start, end := chainInterval("a")

	mustTx(t, st, func(tx pgx.Tx) error {
		view := NewTxView(tx)
		_, err := view.ReplaceAutoActivities(context.Background(), start, end,
			[]ActivityRow{{
				StartedAt: start,
				EndedAt:   end,
				Kind:      "auto",
				SportType: 0,
				SportName: "Walk",
			}}, "auto-vs-workout", chainObservedAt("a"))
		return err
	})
	if n := countRows(t, st, "activities"); n != 1 {
		t.Fatalf("auto rows = %d, want 1", n)
	}

	writeWorkouts(t, st, false, "auto-vs-workout", chainObservedAt("a"), workoutCandidate(start, end))
	var auto, workout int
	if err := st.pool.QueryRow(context.Background(), `
		SELECT count(*) FILTER (WHERE kind = 'auto'), count(*) FILTER (WHERE kind = 'workout')
		FROM activities`).Scan(&auto, &workout); err != nil {
		t.Fatalf("count kinds: %v", err)
	}
	if auto != 1 || workout != 1 {
		t.Fatalf("activity kinds = auto %d workout %d, want 1 and 1", auto, workout)
	}
}

func TestSleepCandidatesRejectWithinSessionOverlap(t *testing.T) {
	st := newTestStore(t)
	seedSession(t, st, "overlap-session", chainDay, chainObservedAt("a"))
	startA, endA := chainInterval("a")
	startB, endB := chainInterval("b")

	err := st.WithTx(context.Background(), func(tx pgx.Tx) error {
		_, err := st.WriteSleep(context.Background(), tx, []SleepRow{
			sleepCandidate(startA, endA),
			sleepCandidate(startB, endB),
		}, "overlap-session", chainObservedAt("a"), false)
		return err
	})
	var invalid *InvalidRowError
	if !errors.As(err, &invalid) {
		t.Fatalf("err = %v, want an invalid row error", err)
	}
	if n := countRows(t, st, "sleep_observations"); n != 0 {
		t.Fatalf("candidates after a rejected batch = %d, want 0", n)
	}
}
