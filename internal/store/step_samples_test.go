package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func writeStepsTx(t *testing.T, st *Store, rows []StepSample, sessionID string, observedAt time.Time) (Mutation, error) {
	t.Helper()
	var out Mutation
	err := st.WithTx(context.Background(), func(tx pgx.Tx) error {
		var err error
		out, err = st.WriteSteps(context.Background(), tx, rows, sessionID, observedAt)
		return err
	})
	return out, err
}

func TestStepSamplesPreserveObservedZero(t *testing.T) {
	st := newTestStore(t)
	at := time.Date(2026, 10, 1, 4, 0, 0, 0, time.UTC)
	seedSession(t, st, "step-zero", at.Add(-time.Hour), at)

	mut, err := writeStepsTx(t, st, []StepSample{{At: at, Steps: 0}}, "step-zero", at)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if !mut.Changed || len(mut.Days) != 1 || !mut.Days[0].Equal(Day(2026, time.October, 1)) {
		t.Fatalf("mutation = %+v", mut)
	}
	if len(mut.Intervals) != 1 || !mut.Intervals[0].To.Equal(at.Add(time.Microsecond)) {
		t.Fatalf("intervals = %+v", mut.Intervals)
	}

	steps, err := listSteps(context.Background(), st.q, at.Add(-time.Hour), at.Add(time.Hour))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(steps) != 1 || steps[0].Steps != 0 {
		t.Fatalf("stored steps = %+v, want one observed zero", steps)
	}
}

func TestStepSamplesRejectConflictingDuplicate(t *testing.T) {
	st := newTestStore(t)
	at := time.Date(2026, 10, 1, 4, 0, 0, 0, time.UTC)
	seedSession(t, st, "step-conflict", at.Add(-time.Hour), at)

	_, err := writeStepsTx(t, st, []StepSample{
		{At: at, Steps: 10},
		{At: at, Steps: 20},
	}, "step-conflict", at)
	var invalid *InvalidRowError
	if !errors.As(err, &invalid) {
		t.Fatalf("err = %v, want an invalid row error", err)
	}
	if n := countRows(t, st, "step_samples"); n != 0 {
		t.Fatalf("rows after a rejected batch = %d, want 0", n)
	}
}

func TestStepSamplesCollapseIdenticalDuplicates(t *testing.T) {
	st := newTestStore(t)
	at := time.Date(2026, 10, 1, 4, 0, 0, 0, time.UTC)
	seedSession(t, st, "step-dupe", at.Add(-time.Hour), at)

	if _, err := writeStepsTx(t, st, []StepSample{
		{At: at, Steps: 7},
		{At: at, Steps: 7},
	}, "step-dupe", at); err != nil {
		t.Fatalf("write: %v", err)
	}
	if n := countRows(t, st, "step_samples"); n != 1 {
		t.Fatalf("rows = %d, want 1", n)
	}
}

func TestStepSamplesOlderObservationCannotRegressValue(t *testing.T) {
	st := newTestStore(t)
	at := time.Date(2026, 10, 1, 4, 0, 0, 0, time.UTC)
	newer := at.Add(14 * 24 * time.Hour)
	older := at.Add(3 * 24 * time.Hour)
	seedSession(t, st, "step-newer", newer.Add(-time.Hour), newer)
	seedSession(t, st, "step-older", older.Add(-time.Hour), older)

	if _, err := writeStepsTx(t, st, []StepSample{{At: at, Steps: 60}}, "step-newer", newer); err != nil {
		t.Fatalf("newer write: %v", err)
	}
	mut, err := writeStepsTx(t, st, []StepSample{{At: at, Steps: 90}}, "step-older", older)
	if err != nil {
		t.Fatalf("older write: %v", err)
	}
	if mut.Changed || len(mut.Days) != 0 || len(mut.Intervals) != 0 {
		t.Fatalf("older observation reported a mutation: %+v", mut)
	}
	if got := storedStep(t, st, at); got != 60 {
		t.Fatalf("canonical steps = %d, want 60", got)
	}
}

func TestStepSamplesEqualValueFreshnessIsNotDirty(t *testing.T) {
	st := newTestStore(t)
	at := time.Date(2026, 10, 1, 4, 0, 0, 0, time.UTC)
	older := at.Add(3 * 24 * time.Hour)
	newer := at.Add(14 * 24 * time.Hour)
	seedSession(t, st, "step-fresh-a", older.Add(-time.Hour), older)
	seedSession(t, st, "step-fresh-b", newer.Add(-time.Hour), newer)

	if _, err := writeStepsTx(t, st, []StepSample{{At: at, Steps: 60}}, "step-fresh-a", older); err != nil {
		t.Fatalf("first write: %v", err)
	}
	mut, err := writeStepsTx(t, st, []StepSample{{At: at, Steps: 60}}, "step-fresh-b", newer)
	if err != nil {
		t.Fatalf("equal-value write: %v", err)
	}
	if mut.Changed {
		t.Fatalf("equal value advanced a dirty day: %+v", mut)
	}
	if got := scalarString(t, st,
		`SELECT session_id FROM step_samples WHERE sampled_at = $1`, at); got != "step-fresh-b" {
		t.Fatalf("lineage session = %s, want the newest observation", got)
	}
}

func TestStepSamplesExactRetryIsIdempotent(t *testing.T) {
	st := newTestStore(t)
	at := time.Date(2026, 10, 1, 4, 0, 0, 0, time.UTC)
	seedSession(t, st, "step-retry", at.Add(-time.Hour), at)

	if _, err := writeStepsTx(t, st, []StepSample{{At: at, Steps: 12}}, "step-retry", at); err != nil {
		t.Fatalf("write: %v", err)
	}
	mut, err := writeStepsTx(t, st, []StepSample{{At: at, Steps: 12}}, "step-retry", at)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if mut.Changed {
		t.Fatalf("retry reported a change: %+v", mut)
	}
	if n := countRows(t, st, "step_samples"); n != 1 {
		t.Fatalf("rows = %d, want 1", n)
	}
}

func TestStepSamplesRejectOutOfRangeValues(t *testing.T) {
	st := newTestStore(t)
	at := time.Date(2026, 10, 1, 4, 0, 0, 0, time.UTC)
	seedSession(t, st, "step-range", at.Add(-time.Hour), at)

	for _, value := range []int{-1, 401} {
		var invalid *InvalidRowError
		_, err := writeStepsTx(t, st, []StepSample{{At: at, Steps: value}}, "step-range", at)
		if !errors.As(err, &invalid) {
			t.Fatalf("steps=%d err = %v, want an invalid row error", value, err)
		}
	}
}

func storedStep(t *testing.T, st *Store, at time.Time) int {
	t.Helper()
	var steps int
	if err := st.pool.QueryRow(context.Background(),
		`SELECT steps FROM step_samples WHERE sampled_at = $1`, at).Scan(&steps); err != nil {
		t.Fatalf("read steps: %v", err)
	}
	return steps
}
