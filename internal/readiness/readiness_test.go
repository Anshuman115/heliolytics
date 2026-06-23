package readiness

import "testing"

func f(v float64) *float64 { return &v }

// Build n days of identical "baseline" vitals.
func baseline(n int, rmssd, rhr, resp, sleep float64) []DayVitals {
	h := make([]DayVitals, n)
	for i := range h {
		h[i] = DayVitals{RMSSD: f(rmssd), RHR: f(rhr), Resp: f(resp), SleepScore: f(sleep)}
	}
	return h
}

func TestBuildingBaselineBelowMinDays(t *testing.T) {
	if _, ok := Compute(baseline(2, 50, 50, 15, 60)); ok {
		t.Fatal("want ok=false with <3 valid HRV nights")
	}
	if _, ok := Compute(baseline(3, 50, 50, 15, 60)); !ok {
		t.Fatal("want ok=true (provisional) at 3 valid HRV nights")
	}
}

func TestNeutralAtBaseline(t *testing.T) {
	// 14 identical days, sleep=50 -> every sub-score 50 -> overall 50.
	h := baseline(14, 50, 50, 15, 50)
	score, ok := Compute(h)
	if !ok {
		t.Fatal("want ok=true with 14 days")
	}
	if score != 50 {
		t.Fatalf("neutral baseline score=%d, want 50", score)
	}
}

func TestHigherHrvRaisesScore(t *testing.T) {
	h := baseline(13, 50, 50, 15, 50)
	h = append(h, DayVitals{RMSSD: f(85), RHR: f(50), Resp: f(15), SleepScore: f(50)})
	score, ok := Compute(h)
	if !ok || score <= 50 {
		t.Fatalf("elevated HRV vs baseline should raise score >50, got %d (ok=%v)", score, ok)
	}
}

func TestLowerHrvAndHigherRhrDropsScore(t *testing.T) {
	h := baseline(13, 50, 50, 15, 50)
	h = append(h, DayVitals{RMSSD: f(30), RHR: f(60), Resp: f(15), SleepScore: f(50)})
	score, ok := Compute(h)
	if !ok || score >= 50 {
		t.Fatalf("suppressed HRV + elevated RHR should drop score <50, got %d (ok=%v)", score, ok)
	}
}

func TestMissingRmssdNoScore(t *testing.T) {
	h := baseline(20, 50, 50, 15, 50)
	h[len(h)-1].RMSSD = nil // target night lacks HRV
	if _, ok := Compute(h); ok {
		t.Fatal("want ok=false when target day has no RMSSD")
	}
}

func TestMissingRespStillScores(t *testing.T) {
	// No respiratory data at all: weight renormalizes over HRV+RHR+sleep.
	h := baseline(14, 50, 50, 15, 50)
	for i := range h {
		h[i].Resp = nil
	}
	score, ok := Compute(h)
	if !ok || score != 50 {
		t.Fatalf("missing resp should renormalize to neutral 50, got %d (ok=%v)", score, ok)
	}
}
