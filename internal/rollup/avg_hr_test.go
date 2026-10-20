package rollup

import (
	"testing"

	"github.com/Anshuman115/heliolytics/internal/store"
)

func TestMeanDayHeartRateIgnoresSnapshotSource(t *testing.T) {
	rows := []store.HeartRateSample{
		{At: at(9, 0), Source: sourceContinuousHR, BPM: 60},
		{At: at(9, 1), Source: sourceContinuousHR, BPM: 80},
		{At: at(9, 0), Source: sourceSnapshotHR, BPM: 180},
	}
	got := meanDayHeartRate(rows)
	if got == nil || *got != 70 {
		t.Fatalf("mean = %v, want 70", got)
	}
	if meanDayHeartRate(rows[2:]) != nil {
		t.Fatal("a 0x46 only day has no daily heart rate")
	}
}

func TestMeanDayHeartRateIsUnrounded(t *testing.T) {
	rows := []store.HeartRateSample{
		{At: at(9, 0), Source: sourceContinuousHR, BPM: 60},
		{At: at(9, 1), Source: "0x01", BPM: 61},
	}
	got := meanDayHeartRate(rows)
	if got == nil || *got != 60.5 {
		t.Fatalf("mean = %v, want 60.5", got)
	}
}
