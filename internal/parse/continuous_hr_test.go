package parse

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseContinuousHrDump(t *testing.T) {
	root := filepath.Join("..", "..", "..", "Heliolytics_App", "helio_dump_v5")
	raw, err := os.ReadFile(filepath.Join(root, "0x46_raw.bin"))
	if err != nil {
		t.Skip("helio_dump_v5 0x46 missing")
	}
	pts := ParseContinuousHr(raw)
	if len(pts) < 100 {
		t.Fatalf("want >=100 HR samples got %d", len(pts))
	}
	for _, p := range pts {
		if p.Bpm < 30 || p.Bpm > 220 {
			t.Fatalf("invalid bpm %d", p.Bpm)
		}
		if p.DayKey == "" || p.Ts.IsZero() {
			t.Fatal("missing ts/day_key")
		}
	}
}
