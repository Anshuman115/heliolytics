package parse

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseBlobsSleepFromDump(t *testing.T) {
	p := filepath.Join("..", "..", "..", "Heliolytics_App", "helio_dump_v5", "0x48_raw.bin")
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Skip("dump missing")
	}
	batch := ParseBlobs(nil, map[string][]byte{"0x48": raw}, time.Time{})
	if len(batch.Sleep) == 0 {
		t.Fatal("expected sleep records from 0x48 blob")
	}
	june7Naps := 0
	for _, s := range batch.Sleep {
		if s.IsNap && s.DayKey == "2026-06-07" {
			june7Naps++
			if s.TotalMin < 110 || s.TotalMin > 140 {
				t.Fatalf("June 7 nap mins %d want ~120-135", s.TotalMin)
			}
		}
	}
	if june7Naps != 1 {
		t.Fatalf("want 1 nap on 2026-06-07 got %d", june7Naps)
	}
}
