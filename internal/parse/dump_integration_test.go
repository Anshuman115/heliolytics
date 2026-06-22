package parse

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHelioDumpV5WorkoutsAndPai(t *testing.T) {
	root := filepath.Join("..", "..", "..", "Heliolytics_App", "DUMPP")
	w05, err := os.ReadFile(filepath.Join(root, "0x05_raw.bin"))
	if err != nil {
		t.Skip("DUMPP missing")
	}
	w06, _ := os.ReadFile(filepath.Join(root, "0x06_raw.bin"))
	pai, _ := os.ReadFile(filepath.Join(root, "0x0D_raw.bin"))

	scores := ParsePai(pai)
	t.Logf("PAI records: %d", len(scores))
	if len(scores) < 1 {
		t.Fatalf("want >=1 PAI day got %d", len(scores))
	}
	// PAI total is a float32 ≈ 81 in this dump — must NOT be the old byte-scan
	// bug value of 61. Guard the regression.
	for _, s := range scores {
		t.Logf("  PAI %s = %d", s.DayKey, s.Score)
		if s.Score < 70 || s.Score > 90 {
			t.Fatalf("PAI %s = %d, want ~81 (float32@59); 61 = old byte-scan bug", s.DayKey, s.Score)
		}
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
