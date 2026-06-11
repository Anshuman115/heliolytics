package parse

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseWorkoutsProtobuf(t *testing.T) {
	p := filepath.Join("..", "..", "..", "Heliolytics_App", "helio_dump_v5", "0x05_raw.bin")
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Skip("dump missing")
	}
	ws := ParseWorkouts(raw)
	if len(ws) != 1 {
		t.Fatalf("want 1 workout got %d", len(ws))
	}
	w := ws[0]
	if w.SportType != 92 {
		t.Fatalf("sport %d name %q", w.SportType, w.SportName)
	}
	if w.SportName != "Badminton" {
		t.Fatalf("name %q", w.SportName)
	}
	if w.Calories == nil || *w.Calories != 872 {
		t.Fatalf("calories %v", w.Calories)
	}
	if w.DurationSec < 180 {
		t.Fatalf("duration %d", w.DurationSec)
	}
}
