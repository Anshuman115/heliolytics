package parse

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAggregateCountsNapFromParsedSleep(t *testing.T) {
	p := filepath.Join("..", "..", "..", "Heliolytics_App", "helio_dump_v5", "0x48_raw.bin")
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Skip("sleep dump missing")
	}
	parsed := ParseBlobs(nil, map[string][]byte{"0x48": raw}, time.Now().UTC())
	agg := Aggregate(parsed)
	// Nap counts are no longer aggregated in-memory (internal/rollup computes
	// them from the DB after commit) — but Aggregate must still pass every
	// parsed sleep record, including naps, through to the write batch.
	var napDays int
	seen := map[string]bool{}
	for _, s := range agg.Sleep {
		if s.IsNap && !seen[s.DayKey] {
			seen[s.DayKey] = true
			napDays++
		}
	}
	if napDays == 0 {
		t.Fatal("want at least one nap sleep record on a day from sleep fixture")
	}
}
