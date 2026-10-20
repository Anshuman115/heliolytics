package rollup

import (
	"testing"

	"github.com/Anshuman115/heliolytics/internal/store"
)

func TestMeanNightHRVQualityBoundaries(t *testing.T) {
	window := spanOf(at(1, 0), at(2, 0))
	rows := make([]store.HRVSample, 0, 60)
	for i := range 60 {
		rows = append(rows, store.HRVSample{At: at(1, i), RMSSDMS: 40 + float64(i%3)})
	}
	got := meanNightHRV(rows, window)
	if got == nil {
		t.Fatal("60 samples over 60 minutes must qualify")
	}
	sum := 0.0
	for _, r := range rows {
		sum += r.RMSSDMS
	}
	if *got != sum/60 {
		t.Fatalf("mean = %v, want the fractional %v", *got, sum/60)
	}
	if meanNightHRV(rows[:59], window) != nil {
		t.Fatal("59 samples must stay NULL")
	}
	if meanNightHRV(rows, spanOf(at(1, 0), at(4, 20))) != nil {
		t.Fatal("60 samples over 200 minutes must fail the coverage rule")
	}
}

func TestMeanNightRespBoundary(t *testing.T) {
	rows := make([]store.RespSample, 0, 60)
	for i := range 60 {
		rows = append(rows, store.RespSample{At: at(1, i), BRMin: 13.5})
	}
	if got := meanNightResp(rows); got == nil || *got != 13.5 {
		t.Fatalf("mean = %v, want 13.5", got)
	}
	if meanNightResp(rows[:59]) != nil {
		t.Fatal("59 samples must stay NULL")
	}
}

func TestSpO2SummaryBelowThreeSamples(t *testing.T) {
	two := []store.SpO2Sample{
		{At: at(1, 0), Source: "0x25", Pct: 96},
		{At: at(1, 1), Source: "0x25", Pct: 94},
	}
	avg, minPct, n := spo2Summary(two)
	if avg != nil || minPct != nil {
		t.Fatalf("two samples avg/min = %v/%v, want NULL", avg, minPct)
	}
	if n != 2 {
		t.Fatalf("count = %d, want the actual 2", n)
	}

	three := append(two, store.SpO2Sample{At: at(1, 2), Source: "0x25", Pct: 98})
	avg, minPct, n = spo2Summary(three)
	if avg == nil || *avg != 96 || minPct == nil || *minPct != 94 || n != 3 {
		t.Fatalf("three samples = %v/%v/%d, want 96/94/3", avg, minPct, n)
	}

	if avg, minPct, n := spo2Summary(nil); avg != nil || minPct != nil || n != 0 {
		t.Fatal("no samples must report a zero count and NULL statistics")
	}
}
