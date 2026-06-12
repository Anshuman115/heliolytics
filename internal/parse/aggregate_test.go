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
	var napDays int
	for _, d := range agg.Days {
		if d.NapCount > 0 {
			napDays++
		}
	}
	if napDays == 0 {
		t.Fatal("want nap_count on at least one day from sleep fixture")
	}
}
