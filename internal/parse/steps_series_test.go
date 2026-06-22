package parse

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Proves the Fix-A invariant on real 0x01 bytes WITHOUT a DB: per-minute step
// samples are keyed by timestamp, so re-syncing the same minutes (the 60-min
// overlap) overwrites in an idempotent store and the daily SUM does not change —
// whereas the old additive path would have doubled it.
func TestStepSeriesIdempotentUnderOverlap(t *testing.T) {
	root := filepath.Join("..", "..", "..", "Heliolytics_App", "DUMPP")
	raw, err := os.ReadFile(filepath.Join(root, "0x01_raw.bin"))
	if err != nil {
		t.Skip("DUMPP/0x01_raw.bin missing")
	}
	fetchEnd := time.Date(2026, 6, 22, 22, 38, 0, 0, time.UTC)

	series := ParseStepSeries(raw, nil, fetchEnd)
	if len(series) == 0 {
		t.Fatal("expected step samples from 0x01")
	}

	// Idempotent store: map keyed by minute timestamp (mirrors ON CONFLICT upsert).
	store := map[int64]int{}
	apply := func(ss []StepSample) {
		for _, s := range ss {
			store[s.SampledAt.Unix()] = s.Steps
		}
	}
	sum := func() int {
		t := 0
		for _, v := range store {
			t += v
		}
		return t
	}

	apply(series)
	once := sum()
	apply(ParseStepSeries(raw, nil, fetchEnd)) // full re-sync (worst-case overlap)
	twice := sum()

	if once != twice {
		t.Fatalf("overlap re-sync changed daily steps: once=%d twice=%d", once, twice)
	}
	// Confirm the naive additive model WOULD have doubled, so the test is meaningful.
	naive := 0
	for _, s := range series {
		naive += s.Steps
	}
	if once != naive {
		t.Fatalf("distinct-minute sum %d != single-parse sum %d", once, naive)
	}
	t.Logf("steps once=%d twice=%d (additive would be %d)", once, twice, naive*2)
}
