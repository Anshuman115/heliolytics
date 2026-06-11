package parse

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDumpReadinessReal(t *testing.T) {
	p := filepath.Join("..", "..", "..", "Heliolytics_App", "helio_dump_v5", "0x39_raw.bin")
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Skip("dump missing")
	}
	out := ParseReadiness(raw)
	t.Logf("count %d", len(out))
	for _, r := range out {
		t.Logf("%+v", r)
	}
	if len(out) == 0 {
		t.Fatal("expected readiness scores")
	}
}

func TestDumpSleepNapsReal(t *testing.T) {
	p := filepath.Join("..", "..", "..", "Heliolytics_App", "helio_dump_v5", "0x48_raw.bin")
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Skip("dump missing")
	}
	out := ParseSleep(raw)
	n := 0
	june7 := 0
	for _, s := range out {
		if s.IsNap {
			n++
			t.Logf("nap day=%s mins=%d stages=%d start=%s", s.DayKey, s.TotalMin, len(s.Stages), s.StartedAt.Format(time.RFC3339))
			if s.DayKey == "2026-06-07" {
				june7++
				if s.TotalMin < 110 || s.TotalMin > 140 {
					t.Fatalf("June 7 nap mins %d want ~120-135", s.TotalMin)
				}
			}
		}
	}
	if june7 != 1 {
		t.Fatalf("want 1 nap on 2026-06-07 got %d", june7)
	}
	t.Logf("total %d naps %d", len(out), n)
}

func TestDumpWorkoutSportReal(t *testing.T) {
	p := filepath.Join("..", "..", "..", "Heliolytics_App", "helio_dump_v5", "0x05_raw.bin")
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Skip("dump missing")
	}
	ws := ParseWorkouts(raw)
	if len(ws) != 1 {
		t.Fatalf("want 1 got %d", len(ws))
	}
	if ws[0].SportType != 92 {
		t.Fatalf("sport %d name %q want 92 Badminton", ws[0].SportType, ws[0].SportName)
	}
}
