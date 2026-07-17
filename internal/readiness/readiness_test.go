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
	if _, _, ok := Compute(baseline(2, 50, 50, 15, 60)); ok {
		t.Fatal("want ok=false with <3 valid HRV nights")
	}
	if _, _, ok := Compute(baseline(3, 50, 50, 15, 60)); !ok {
		t.Fatal("want ok=true (provisional) at 3 valid HRV nights")
	}
}

func TestNeutralAtBaseline(t *testing.T) {
	// 14 identical days, sleep=50 -> every sub-score 50 -> overall 50.
	h := baseline(14, 50, 50, 15, 50)
	score, components, ok := Compute(h)
	if !ok {
		t.Fatal("want ok=true with 14 days")
	}
	if score != 50 {
		t.Fatalf("neutral baseline score=%d, want 50", score)
	}
	if len(components) != 4 {
		t.Fatalf("want 4 components (hrv, rhr, resp, sleep), got %d: %+v", len(components), components)
	}
	wantNames := map[string]struct{ weight, subscore float64 }{
		"hrv":   {wHRV, 50},
		"rhr":   {wRHR, 50},
		"resp":  {wResp, 50},
		"sleep": {wSleep, 50},
	}
	for _, c := range components {
		want, ok := wantNames[c.Name]
		if !ok {
			t.Fatalf("unexpected component name %q", c.Name)
		}
		if c.Weight != want.weight {
			t.Errorf("component %s weight=%v, want %v", c.Name, c.Weight, want.weight)
		}
		if c.Subscore != want.subscore {
			t.Errorf("component %s subscore=%v, want %v (neutral baseline)", c.Name, c.Subscore, want.subscore)
		}
	}
}

func TestHigherHrvRaisesScore(t *testing.T) {
	h := baseline(13, 50, 50, 15, 50)
	h = append(h, DayVitals{RMSSD: f(85), RHR: f(50), Resp: f(15), SleepScore: f(50)})
	score, components, ok := Compute(h)
	if !ok || score <= 50 {
		t.Fatalf("elevated HRV vs baseline should raise score >50, got %d (ok=%v)", score, ok)
	}
	var hrvComp *Component
	for i := range components {
		if components[i].Name == "hrv" {
			hrvComp = &components[i]
		}
	}
	if hrvComp == nil {
		t.Fatal("want hrv component present")
	}
	if hrvComp.Value != 85 {
		t.Errorf("hrv component value=%v, want 85", hrvComp.Value)
	}
	if hrvComp.Subscore <= 50 {
		t.Errorf("hrv subscore=%v, want >50 for elevated HRV", hrvComp.Subscore)
	}
}

func TestLowerHrvAndHigherRhrDropsScore(t *testing.T) {
	h := baseline(13, 50, 50, 15, 50)
	h = append(h, DayVitals{RMSSD: f(30), RHR: f(60), Resp: f(15), SleepScore: f(50)})
	score, components, ok := Compute(h)
	if !ok || score >= 50 {
		t.Fatalf("suppressed HRV + elevated RHR should drop score <50, got %d (ok=%v)", score, ok)
	}
	for _, c := range components {
		switch c.Name {
		case "hrv":
			if c.Value != 30 || c.Subscore >= 50 {
				t.Errorf("hrv component=%+v, want value=30, subscore<50", c)
			}
		case "rhr":
			if c.Value != 60 || c.Subscore >= 50 {
				t.Errorf("rhr component=%+v, want value=60, subscore<50", c)
			}
		}
	}
}

func TestMissingRmssdNoScore(t *testing.T) {
	h := baseline(20, 50, 50, 15, 50)
	h[len(h)-1].RMSSD = nil // target night lacks HRV
	if _, components, ok := Compute(h); ok || components != nil {
		t.Fatal("want ok=false and nil components when target day has no RMSSD")
	}
}

// Target day must NOT be averaged into its own baseline. With a flat 50ms prior
// and a 60ms target (HRV only), the score should clearly reflect the elevation
// (~75); folding the target into the mean would dampen it to the high 60s.
func TestTargetExcludedFromBaseline(t *testing.T) {
	h := []DayVitals{{RMSSD: f(50)}, {RMSSD: f(50)}, {RMSSD: f(60)}}
	score, components, ok := Compute(h)
	if !ok || score < 72 {
		t.Fatalf("score=%d (ok=%v), want >=72 — target leaked into its own baseline?", score, ok)
	}
	if len(components) != 1 || components[0].Name != "hrv" {
		t.Fatalf("want only hrv component (no rhr/resp/sleep data), got %+v", components)
	}
}

func TestMissingRespStillScores(t *testing.T) {
	// No respiratory data at all: weight renormalizes over HRV+RHR+sleep.
	h := baseline(14, 50, 50, 15, 50)
	for i := range h {
		h[i].Resp = nil
	}
	score, components, ok := Compute(h)
	if !ok || score != 50 {
		t.Fatalf("missing resp should renormalize to neutral 50, got %d (ok=%v)", score, ok)
	}
	for _, c := range components {
		if c.Name == "resp" {
			t.Fatalf("want no resp component when resp data missing, got %+v", components)
		}
	}
	if len(components) != 3 {
		t.Fatalf("want 3 components (hrv, rhr, sleep), got %d: %+v", len(components), components)
	}
}
