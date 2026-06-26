// Package readiness computes a daily recovery/readiness score (0–100) from
// nightly vitals, using the baseline-deviation method standard in HRV-guided
// training and consumer wearables.
//
// Design (evidence-informed; see docs):
//   - HRV uses ln(RMSSD) (RMSSD is log-normal). Baseline = 7-day rolling mean,
//     spread = 60-day SD (Plews 2012/2013; HRV4Training/Altini).
//   - Baseline is the PRIOR days only (the target day is never folded into its
//     own mean/SD). Each metric -> sub-score = clamp(50 ± 25·z, 0, 100): ±2 SD
//     spans the range; the ~0.5 SD "smallest worthwhile change" ≈ a 12-pt move.
//   - Weights (HRV-dominant): HRV .50, RHR .25, sleep .15, resp .10.
//     Missing optional components are dropped and remaining weights renormalized.
//   - Cold start: <3 valid HRV nights -> no score ("building baseline"). Each
//     metric uses a population-prior SD until it has fullSDDays of its own
//     baseline, then its personal trailing SD.
package readiness

import "math"

const (
	MinDays    = 3  // below this many valid HRV nights: building baseline
	fullSDDays = 14 // below this: use population-prior SDs, not personal (provisional)
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

	// Baseline is built from the PRIOR days only — never fold the target day into
	// its own mean/SD, or its z-score is biased toward zero (score muted), worst
	// when data is thin.
	prior := history[:len(history)-1]
	lnPrior := collect(prior, lnRMSSD)
	if len(lnPrior)+1 < MinDays { // prior + today valid HRV nights
		return 0, false
	}

	type part struct{ v, w float64 }
	parts := []part{
		// HRV (required) — higher is better.
		{subscore(math.Log(*target.RMSSD), lnPrior, priorSDH, floorSDH, +1), wHRV},
	}

	// RHR — lower is better. Each metric decides prior-vs-personal SD from its
	// own baseline length, not HRV's.
	if rhr := collect(prior, getRHR); target.RHR != nil && len(rhr) >= MinDays-1 {
		parts = append(parts, part{subscore(*target.RHR, rhr, priorSDR, floorSDR, -1), wRHR})
	}

	// Respiratory rate — higher is worse.
	if resp := collect(prior, getResp); target.Resp != nil && len(resp) >= MinDays-1 {
		parts = append(parts, part{subscore(*target.Resp, resp, priorSDF, floorSDF, -1), wResp})
	}

	// Sleep: device 0–100 score used directly.
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

// subscore maps today's value against its personal baseline (prior days only)
// to 0–100. It uses the population-prior SD until this metric has fullSDDays of
// baseline, then its own personal SD (floored to avoid a hypersensitive
// baseline). sign=+1 for higher-is-better, -1 for higher-is-worse.
func subscore(x float64, prior []float64, priorSD, floor, sign float64) float64 {
	var sd float64
	if len(prior) < fullSDDays {
		sd = priorSD
	} else {
		sd = math.Max(stdLastN(prior, sdWindow), floor)
	}
	z := (x - meanLastN(prior, meanWindow)) / sd
	return clamp(50+sign*25*z, 0, 100)
}

func lnRMSSD(d DayVitals) *float64 {
	if d.RMSSD == nil || *d.RMSSD <= 0 {
		return nil
	}
	v := math.Log(*d.RMSSD)
	return &v
}

func getRHR(d DayVitals) *float64  { return d.RHR }
func getResp(d DayVitals) *float64 { return d.Resp }

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
