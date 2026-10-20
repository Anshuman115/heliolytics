package rollup

import (
	"testing"

	"github.com/Anshuman115/heliolytics/internal/store"
)

func hrRow(minute int, bpm int) store.HeartRateSample {
	return store.HeartRateSample{At: at(1, minute), Source: "0x01", BPM: bpm}
}

func TestRestingHeartRateLowestFiveMinuteMean(t *testing.T) {
	window := spanOf(at(1, 0), at(2, 0))
	rows := make([]store.HeartRateSample, 0, 60)
	for i := range 60 {
		rows = append(rows, hrRow(i, 60+i/10))
	}
	got := restingHeartRate(rows, window)
	if got == nil {
		t.Fatal("full coverage must qualify")
	}
	if *got < 60 || *got >= 62 {
		t.Fatalf("lowest mean = %v, want the opening low window", *got)
	}
}

func TestRestingHeartRateAcceptsFourOfFiveSlots(t *testing.T) {
	window := spanOf(at(1, 0), at(1, 5))
	rows := []store.HeartRateSample{
		hrRow(0, 60), hrRow(1, 62), hrRow(2, 64), hrRow(3, 66),
	}
	got := restingHeartRate(rows, window)
	if got == nil {
		t.Fatal("four actual minutes must qualify")
	}
	if *got != 63 {
		t.Fatalf("mean = %v, want 63", *got)
	}
}

func TestRestingHeartRateRejectsThreeOfFiveSlots(t *testing.T) {
	window := spanOf(at(1, 0), at(1, 5))
	rows := []store.HeartRateSample{hrRow(0, 60), hrRow(1, 60), hrRow(2, 60)}
	if got := restingHeartRate(rows, window); got != nil {
		t.Fatalf("three valid minutes = %v, want NULL", *got)
	}
}

func TestRestingHeartRateNeverBridgesASessionGap(t *testing.T) {
	spans := []span{
		{Start: at(1, 0), End: at(1, 2)},
		{Start: at(1, 3), End: at(1, 5)},
	}
	rows := []store.HeartRateSample{hrRow(0, 60), hrRow(1, 60), hrRow(3, 60), hrRow(4, 60)}
	if got := restingHeartRate(rows, spans); got != nil {
		t.Fatalf("a window across the gap = %v, want NULL", *got)
	}
}

func TestRestingHeartRateCoverageFloor(t *testing.T) {
	window := spanOf(at(1, 0), at(1, 10))
	rows := []store.HeartRateSample{hrRow(0, 60), hrRow(1, 60), hrRow(2, 60), hrRow(3, 60), hrRow(4, 60)}
	if got := restingHeartRate(rows, window); got != nil {
		t.Fatalf("5 of 10 minutes = %v, want NULL below 60%% coverage", *got)
	}
}

func TestRestingHeartRateIgnoresSnapshotSource(t *testing.T) {
	window := spanOf(at(1, 0), at(1, 5))
	rows := []store.HeartRateSample{
		{At: at(1, 0), Source: "0x46", BPM: 40},
		{At: at(1, 1), Source: "0x46", BPM: 40},
		{At: at(1, 2), Source: "0x46", BPM: 40},
		{At: at(1, 3), Source: "0x46", BPM: 40},
		{At: at(1, 4), Source: "0x46", BPM: 40},
	}
	if got := restingHeartRate(rows, window); got != nil {
		t.Fatalf("0x46 rows = %v, want NULL", *got)
	}
}
