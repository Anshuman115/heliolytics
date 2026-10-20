package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestBulkStepBatchSpansMultipleStatements(t *testing.T) {
	st := newTestStore(t)
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	observedAt := base.Add(48 * time.Hour)
	seedSession(t, st, "bulk-steps", base, observedAt)

	total := sampleBatchSize + 904
	rows := make([]StepSample, total)
	for i := range total {
		rows[i] = StepSample{At: base.Add(time.Duration(i) * time.Minute), Steps: i % 400}
	}
	mut, err := writeStepsTx(t, st, rows, "bulk-steps", observedAt)
	if err != nil {
		t.Fatalf("bulk write: %v", err)
	}
	if !mut.Changed {
		t.Fatal("bulk write reported no change")
	}
	if n := countRows(t, st, "step_samples"); int(n) != total {
		t.Fatalf("rows = %d, want %d", n, total)
	}
	for _, i := range []int{0, sampleBatchSize - 1, sampleBatchSize, total - 1} {
		at := base.Add(time.Duration(i) * time.Minute)
		if got := storedStep(t, st, at); got != i%400 {
			t.Fatalf("steps at row %d = %d, want %d", i, got, i%400)
		}
	}
}

func TestBulkConflictingDuplicateAcrossBatchBoundaryIsRejected(t *testing.T) {
	st := newTestStore(t)
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	observedAt := base.Add(48 * time.Hour)
	seedSession(t, st, "bulk-conflict", base, observedAt)

	rows := make([]StepSample, 0, sampleBatchSize+2)
	for i := range sampleBatchSize + 1 {
		rows = append(rows, StepSample{At: base.Add(time.Duration(i) * time.Minute), Steps: 1})
	}
	rows = append(rows, StepSample{At: base, Steps: 2})

	var invalid *InvalidRowError
	if _, err := writeStepsTx(t, st, rows, "bulk-conflict", observedAt); !errors.As(err, &invalid) {
		t.Fatalf("err = %v, want an invalid row error", err)
	}
	if n := countRows(t, st, "step_samples"); n != 0 {
		t.Fatalf("rows after a rejected bulk batch = %d, want 0", n)
	}
}

func TestBulkInvalidRowAbortsWholeWrite(t *testing.T) {
	st := newTestStore(t)
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	observedAt := base.Add(48 * time.Hour)
	seedSession(t, st, "bulk-invalid", base, observedAt)

	rows := make([]StepSample, 0, sampleBatchSize+2)
	rows = append(rows, StepSample{At: base, Steps: 5})
	for i := 1; i <= sampleBatchSize; i++ {
		rows = append(rows, StepSample{At: base.Add(time.Duration(i) * time.Minute), Steps: 1})
	}
	rows = append(rows, StepSample{At: base.Add(7200 * time.Minute), Steps: 401})

	var invalid *InvalidRowError
	if _, err := writeStepsTx(t, st, rows, "bulk-invalid", observedAt); !errors.As(err, &invalid) {
		t.Fatalf("err = %v, want an invalid row error", err)
	}
	if n := countRows(t, st, "step_samples"); n != 0 {
		t.Fatalf("rows after an invalid row = %d, want 0", n)
	}
}

func TestBulkDailyWritesKeepNullZeroAndNoOpSemantics(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	dayOne := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	dayTwo := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	observedAt := dayTwo.Add(48 * time.Hour)
	seedSession(t, st, "bulk-daily", dayOne, observedAt)

	mustTx(t, st, func(tx pgx.Tx) error {
		if _, err := st.WriteSteps(ctx, tx, []StepSample{
			{At: dayOne.Add(time.Hour), Steps: 0},
			{At: dayTwo.Add(time.Hour), Steps: 0},
		}, "bulk-daily", observedAt); err != nil {
			return err
		}
		return NewTxView(tx).EnsureDailyMetricsRows(ctx, []time.Time{
			Day(2026, time.October, 1),
			Day(2026, time.October, 2),
		})
	})

	aggregates := []DailyAggregate{
		{DayKey: Day(2026, time.October, 1), SpO2N: 0},
		{DayKey: Day(2026, time.October, 2), Steps: new(0), SpO2N: 0, NapCount: 1, WorkoutCount: 0},
	}
	mustTx(t, st, func(tx pgx.Tx) error {
		return NewTxView(tx).WriteDailyAggregates(ctx, aggregates)
	})

	var dayOneStepsNull, dayTwoStepsZero bool
	if err := st.pool.QueryRow(ctx,
		`SELECT steps IS NULL FROM daily_metrics WHERE day_key = $1`, dayOne).Scan(&dayOneStepsNull); err != nil {
		t.Fatalf("day one steps: %v", err)
	}
	if !dayOneStepsNull {
		t.Fatal("day one steps should stay NULL without measurements")
	}
	if err := st.pool.QueryRow(ctx,
		`SELECT steps = 0 FROM daily_metrics WHERE day_key = $1`, dayTwo).Scan(&dayTwoStepsZero); err != nil {
		t.Fatalf("day two steps: %v", err)
	}
	if !dayTwoStepsZero {
		t.Fatal("day two steps should be a measured zero")
	}

	var before time.Time
	if err := st.pool.QueryRow(ctx,
		`SELECT updated_at FROM daily_metrics WHERE day_key = $1`, dayTwo).Scan(&before); err != nil {
		t.Fatalf("updated_at: %v", err)
	}
	mustTx(t, st, func(tx pgx.Tx) error {
		return NewTxView(tx).WriteDailyAggregates(ctx, aggregates)
	})
	var after time.Time
	if err := st.pool.QueryRow(ctx,
		`SELECT updated_at FROM daily_metrics WHERE day_key = $1`, dayTwo).Scan(&after); err != nil {
		t.Fatalf("updated_at: %v", err)
	}
	if !after.Equal(before) {
		t.Fatalf("identical daily write advanced updated_at: %v -> %v", before, after)
	}

	mustTx(t, st, func(tx pgx.Tx) error {
		view := NewTxView(tx)
		if err := view.WriteLoad(ctx, []LoadUpdate{{DayKey: Day(2026, time.October, 2), TRIMP: new(12.5)}}); err != nil {
			return err
		}
		return view.WriteReadiness(ctx, []ReadinessUpdate{{
			DayKey:         Day(2026, time.October, 2),
			Score:          new(70),
			Components:     []byte(`{"version":1,"status":"scored"}`),
			TempDeviationC: nil,
		}})
	})
	rows, err := st.ListDays(ctx, "2026-10-02", "2026-10-02")
	if err != nil {
		t.Fatalf("list days: %v", err)
	}
	if len(rows) != 1 || rows[0].TrainingLoad == nil || *rows[0].TrainingLoad != 12.5 {
		t.Fatalf("training load = %+v", rows)
	}
	if rows[0].Readiness == nil || *rows[0].Readiness != 70 {
		t.Fatalf("readiness = %v", rows[0].Readiness)
	}
}

func TestBulkDailyWritesSkipUnsupportedDays(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	supported := Day(2026, time.October, 1)
	unsupported := Day(2026, time.October, 5)

	mustTx(t, st, func(tx pgx.Tx) error {
		return NewTxView(tx).WriteDailyAggregates(ctx, []DailyAggregate{
			{DayKey: supported, SpO2N: 0},
			{DayKey: unsupported, SpO2N: 0},
		})
	})
	if n := countRows(t, st, "daily_metrics"); n != 0 {
		t.Fatalf("daily rows = %d, want none without canonical support", n)
	}
}

func TestNormalizeDailyDomainPrunesUnsupportedAndKeepsSupported(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	day := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	observedAt := day.Add(24 * time.Hour)
	seedSession(t, st, "normalize", day, observedAt)

	mustTx(t, st, func(tx pgx.Tx) error {
		if _, err := st.WriteSteps(ctx, tx,
			[]StepSample{{At: day.Add(time.Hour), Steps: 3}}, "normalize", observedAt); err != nil {
			return err
		}
		return NewTxView(tx).EnsureDailyMetricsRows(ctx, []time.Time{Day(2026, time.October, 1)})
	})
	if _, err := st.pool.Exec(ctx,
		`INSERT INTO daily_metrics (day_key) VALUES ($1)`, Day(2026, time.October, 3)); err != nil {
		t.Fatalf("seed unsupported row: %v", err)
	}

	mustTx(t, st, func(tx pgx.Tx) error {
		deleted, err := NewTxView(tx).NormalizeDailyDomainTx(ctx, []time.Time{
			Day(2026, time.October, 1),
			Day(2026, time.October, 3),
		})
		if err != nil {
			return err
		}
		if len(deleted) != 1 || !deleted[0].Equal(Day(2026, time.October, 3)) {
			t.Fatalf("deleted = %v, want the unsupported day", deleted)
		}
		return nil
	})
	if n := countRows(t, st, "daily_metrics"); n != 1 {
		t.Fatalf("daily rows = %d, want the supported day only", n)
	}
}

func TestCanonicalDayBoundsMatchSupport(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	mustTx(t, st, func(tx pgx.Tx) error {
		first, last, err := NewTxView(tx).CanonicalDayBounds(ctx)
		if err != nil {
			return err
		}
		if first != nil || last != nil {
			t.Fatalf("empty bounds = %v..%v, want nil", first, last)
		}
		return nil
	})

	day := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	observedAt := day.Add(24 * time.Hour)
	seedSession(t, st, "bounds", day, observedAt)
	mustTx(t, st, func(tx pgx.Tx) error {
		_, err := st.WriteHeartRate(ctx, tx,
			[]HeartRateSample{{At: day.Add(2 * time.Hour), Source: "0x01", BPM: 60}}, "bounds", observedAt)
		return err
	})

	mustTx(t, st, func(tx pgx.Tx) error {
		first, last, err := NewTxView(tx).CanonicalDayBounds(ctx)
		if err != nil {
			return err
		}
		if first == nil || last == nil {
			t.Fatal("bounds missing with canonical support")
		}
		want := Day(2026, time.October, 1)
		if !first.Equal(want) || !last.Equal(want) {
			t.Fatalf("bounds = %v..%v, want %v", first, last, want)
		}
		return nil
	})
}
