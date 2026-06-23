// Package readiness computes a daily recovery/readiness score (0–100) from
// nightly vitals, using the baseline-deviation method standard in HRV-guided
// training and consumer wearables (WHOOP/Oura).
//
// Design (evidence-informed; see docs):
//   - HRV uses ln(RMSSD) (RMSSD is log-normal). Baseline = 7-day rolling mean,
//     spread = 60-day SD (Plews 2012/2013; HRV4Training/Altini).
//   - Each metric -> sub-score = clamp(50 ± 25·z, 0, 100): ±2 SD spans the
//     range; the ~0.5 SD "smallest worthwhile change" ≈ a 12-point move.
//   - Weights (HRV-dominant, per WHOOP): HRV .50, RHR .25, sleep .15, resp .10.
//     Missing optional components are dropped and remaining weights renormalized.
//   - Cold start: <7 valid HRV nights -> no score ("building baseline");
//     7–13 nights -> population-prior SDs; ≥14 -> personal trailing SD.
package readiness

import "math"

const (
	MinDays    = 7  // below this many valid HRV nights: building baseline
	fullSDDays = 14 // below this: use population-prior SDs, not personal
	meanWindow = 7
	sdWindow   = 60

	priorSDH = 0.18 // ln(RMSSD)
	priorSDR = 4.0  // bpm
	priorSDF = 1.0  // br/min
	floorSDH = 0.08
	floorSDR = 1.5
	floorSDF = 0.5

	wHRV   = 0.50
	wRHR   = 0.25
	wSleep = 0.15
	wResp  = 0.10
)

// DayVitals holds one day's nightly aggregates; nil = not available.
type DayVitals struct {
	RMSSD      *float64 // nightly mean RMSSD, ms
	RHR        *float64 // bpm
	Resp       *float64 // breaths/min
	SleepScore *float64 // 0–100
}

// Compute returns the 0–100 recovery score for the LAST day in history
// (ordered oldest→newest). ok=false means there isn't enough baseline yet.
func Compute(history []DayVitals) (score int, ok bool) {
	if len(history) == 0 {
		return 0, false
	}
	target := history[len(history)-1]
	if target.RMSSD == nil || *target.RMSSD <= 0 {
		return 0, false
	}

	lnAll := collect(history, func(d DayVitals) *float64 {
		if d.RMSSD == nil || *d.RMSSD <= 0 {
			return nil
		}
		v := math.Log(*d.RMSSD)
		return &v
	})
	if len(lnAll) < MinDays {
		return 0, false
	}
	usePrior := len(lnAll) < fullSDDays

	type part struct{ v, w float64 }
	var parts []part

	// HRV (required)
	xH := math.Log(*target.RMSSD)
	sdH := sd(usePrior, priorSDH, stdLastN(lnAll, sdWindow), floorSDH)
	zH := (xH - meanLastN(lnAll, meanWindow)) / sdH
	parts = append(parts, part{clamp(50+25*zH, 0, 100), wHRV})

	// RHR (lower = better)
	if rhr := collect(history, func(d DayVitals) *float64 { return d.RHR }); target.RHR != nil && len(rhr) >= MinDays {
		sdR := sd(usePrior, priorSDR, stdLastN(rhr, sdWindow), floorSDR)
		zR := (*target.RHR - meanLastN(rhr, meanWindow)) / sdR
		parts = append(parts, part{clamp(50-25*zR, 0, 100), wRHR})
	}

	// Respiratory rate (higher = worse)
	if resp := collect(history, func(d DayVitals) *float64 { return d.Resp }); target.Resp != nil && len(resp) >= MinDays {
		sdF := sd(usePrior, priorSDF, stdLastN(resp, sdWindow), floorSDF)
		zF := (*target.Resp - meanLastN(resp, meanWindow)) / sdF
		parts = append(parts, part{clamp(50-25*zF, 0, 100), wResp})
	}

	// Sleep: device 0–100 score used directly
	if target.SleepScore != nil {
		parts = append(parts, part{clamp(*target.SleepScore, 0, 100), wSleep})
	}

	var sum, wsum float64
	for _, p := range parts {
		sum += p.v * p.w
		wsum += p.w
	}
	return int(math.Round(clamp(sum/wsum, 0, 100))), true
}

func collect(h []DayVitals, get func(DayVitals) *float64) []float64 {
	out := make([]float64, 0, len(h))
	for _, d := range h {
		if v := get(d); v != nil {
			out = append(out, *v)
		}
	}
	return out
}

func meanLastN(xs []float64, n int) float64 {
	w := lastN(xs, n)
	if len(w) == 0 {
		return 0
	}
	var s float64
	for _, v := range w {
		s += v
	}
	return s / float64(len(w))
}

// stdLastN is the sample standard deviation of the last n values.
func stdLastN(xs []float64, n int) float64 {
	w := lastN(xs, n)
	if len(w) < 2 {
		return 0
	}
	m := meanLastN(w, len(w))
	var ss float64
	for _, v := range w {
		ss += (v - m) * (v - m)
	}
	return math.Sqrt(ss / float64(len(w)-1))
}

func lastN(xs []float64, n int) []float64 {
	if len(xs) <= n {
		return xs
	}
	return xs[len(xs)-n:]
}

// sd returns the population prior when in cold-start, else the personal SD
// floored to avoid a falsely tight (hypersensitive) baseline.
func sd(usePrior bool, prior, personal, floor float64) float64 {
	if usePrior {
		return prior
	}
	return math.Max(personal, floor)
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
