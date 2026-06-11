package parse

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHelioDumpV5WorkoutsAndPai(t *testing.T) {
	root := filepath.Join("..", "..", "..", "Heliolytics_App", "helio_dump_v5")
	w05, err := os.ReadFile(filepath.Join(root, "0x05_raw.bin"))
	if err != nil {
		t.Skip("helio_dump_v5 missing")
	}
	w06, _ := os.ReadFile(filepath.Join(root, "0x06_raw.bin"))
	pai, _ := os.ReadFile(filepath.Join(root, "0x0D_raw.bin"))

	scores := ParsePai(pai)
	t.Logf("PAI records: %d", len(scores))
	if len(scores) < 10 {
		t.Fatalf("want >=10 PAI days got %d", len(scores))
	}

	w5 := ParseWorkouts(w05)
	t.Logf("0x05 workouts: %d", len(w5))
	for _, w := range w5 {
		t.Logf("  0x05 %+v", w)
	}
	w6 := ParseWorkoutDetails(w06)
	t.Logf("0x06 heuristic workouts: %d", len(w6))
	w6proto := ParseWorkoutsFromDetailBlob(w06)
	t.Logf("0x06 protobuf workouts: %d", len(w6proto))
	merged := MergeWorkouts(w5, w6proto)
	t.Logf("merged: %d", len(merged))
	for _, w := range merged {
		t.Logf("  merged %+v", w)
	}
	if len(merged) == 0 {
		t.Fatal("expected workouts from dump")
	}
}
