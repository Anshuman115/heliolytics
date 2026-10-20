package rollup

import (
	"testing"
	"time"

	"github.com/Anshuman115/heliolytics/internal/store"
)

func spanOf(start, end time.Time) []span { return []span{{Start: start, End: end}} }

func TestMeanNightTempUsesProxySamplesOnly(t *testing.T) {
	window := spanOf(at(1, 0), at(3, 0))
	rows := make([]store.TemperatureReadRow, 0, 90)
	for i := range 60 {
		rows = append(rows, store.TemperatureReadRow{
			At: at(1, i), CentiC: 3312, OnWrist: true,
		})
	}
	for i := range 30 {
		rows = append(rows, store.TemperatureReadRow{
			At: at(2, i), CentiC: 2600, OnWrist: false,
		})
	}
	got := meanNightTemp(rows, window)
	if got == nil {
		t.Fatal("60 proxy samples over 120 minutes must qualify")
	}
	if *got < 33.119999 || *got > 33.120001 {
		t.Fatalf("mean = %v, want 33.12 from exact hundredths", *got)
	}
}

func TestMeanNightTempNeedsSixtySamplesAndHalfCoverage(t *testing.T) {
	rows := make([]store.TemperatureReadRow, 0, 60)
	for i := range 59 {
		rows = append(rows, store.TemperatureReadRow{At: at(1, i), CentiC: 3300, OnWrist: true})
	}
	if meanNightTemp(rows, spanOf(at(1, 0), at(2, 0))) != nil {
		t.Fatal("59 samples must stay NULL")
	}

	rows = append(rows, store.TemperatureReadRow{At: at(1, 59), CentiC: 3300, OnWrist: true})
	if got := meanNightTemp(rows, spanOf(at(1, 0), at(2, 0))); got == nil {
		t.Fatal("60 samples over 60 minutes must qualify")
	}
	if meanNightTemp(rows, spanOf(at(1, 0), at(3, 10))) != nil {
		t.Fatal("60 samples over 130 minutes must fail the half-coverage rule")
	}
}
