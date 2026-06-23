package parse

import "time"

type sleepWin struct {
	start time.Time
	end   time.Time
	score int
}

func ApplyCanonicalVitals(
	days map[string]*DayAcc,
	sleep []SleepRecord,
	stress []HealthSample,
	hrv []HealthSample,
	spo2Spot []HealthSample,
	spo2Sleep []HealthSample,
	rhr []HealthSample,
	respRate []HealthSample,
) {
	wins := bestSleepWindows(sleep)
	for day, acc := range days {
		if w, ok := wins[day]; ok {
			if v := meanInWindow(hrv, w.start, w.end); v != nil {
				acc.HrvSum = *v
				acc.HrvCount = 1
			}
			sp := meanInWindow(spo2Sleep, w.start, w.end)
			if sp == nil {
				sp = meanInWindow(spo2Spot, w.start, w.end)
			}
			if sp != nil {
				acc.Spo2Sum = *sp
				acc.Spo2Count = 1
			}
			if v := meanInWindow(respRate, w.start, w.end); v != nil {
				acc.RespRateSum = *v
				acc.RespRateCount = 1
			}
		}
		if v := latestOnDay(stress, day); v != nil {
			acc.StressSum = *v
			acc.StressCount = 1
		}
		if v := latestOnDay(rhr, day); v != nil {
			acc.RestingHr = v
		}
	}
}

func bestSleepWindows(sleep []SleepRecord) map[string]sleepWin {
	out := map[string]sleepWin{}
	for _, s := range sleep {
		if s.IsNap {
			continue
		}
		ws, we := s.sleepWindow()
		prev, ok := out[s.DayKey]
		if !ok || s.Score > prev.score {
			out[s.DayKey] = sleepWin{start: ws, end: we, score: s.Score}
		}
	}
	return out
}

func meanInWindow(samples []HealthSample, start, end time.Time) *int {
	var sum int
	var n int
	for _, s := range samples {
		if s.SampledAt.Before(start) || s.SampledAt.After(end) {
			continue
		}
		sum += int(s.Value)
		n++
	}
	if n == 0 {
		return nil
	}
	v := sum / n
	return &v
}

func latestOnDay(samples []HealthSample, day string) *int {
	var best *HealthSample
	for i := range samples {
		s := &samples[i]
		if s.DayKey != day {
			continue
		}
		if best == nil || s.SampledAt.After(best.SampledAt) {
			best = s
		}
	}
	if best == nil {
		return nil
	}
	v := int(best.Value)
	return &v
}
