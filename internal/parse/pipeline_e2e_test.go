package parse

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunIngestEndToEndFromFixtures(t *testing.T) {
	root := filepath.Join("..", "..", "..", "Heliolytics_App", "helio_dump_v5")
	blobs := map[string][]byte{}
	for _, code := range []string{"0x05", "0x48"} {
		raw, err := os.ReadFile(filepath.Join(root, code+"_raw.bin"))
		if err != nil {
			t.Skip("fixture dumps missing")
		}
		blobs[code] = raw
	}
	parsed := ParseBlobs(nil, blobs, time.Now().UTC())
	agg := Aggregate(parsed)
	if len(agg.Workouts) == 0 {
		t.Fatal("want workouts in aggregated batch")
	}
	if len(agg.Sleep) == 0 {
		t.Fatal("want sleep in aggregated batch")
	}
}
