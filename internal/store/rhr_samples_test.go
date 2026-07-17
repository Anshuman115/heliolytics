package store

import (
	"context"
	"testing"
	"time"
)

func TestUpsertRhrSamplesThenReadBack(t *testing.T) {
	st, cleanup := testStore(t)
	defer cleanup()
	ctx := context.Background()
	const day = "2026-06-22"

	clean := func() { st.pool.Exec(ctx, `DELETE FROM rhr_samples WHERE day_key=$1::date`, day) }
	clean()
	t.Cleanup(clean)

	ts := time.Date(2026, 6, 22, 2, 0, 0, 0, time.UTC)
	pts := []SampleValue{{DayKey: day, SampledAt: ts, Value: 45}}
	if err := st.UpsertRhrSamples(ctx, "sess-rhr-1", pts); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	var value float64
	if err := st.pool.QueryRow(ctx,
		`SELECT value FROM rhr_samples WHERE day_key=$1::date`, day).Scan(&value); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if value != 45 {
		t.Fatalf("value=%v, want 45", value)
	}
}
