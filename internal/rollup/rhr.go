package rollup

import (
	"math"
	"time"

	"github.com/Anshuman115/heliolytics/internal/store"
)

const (
	rhrCoverageRatio  = 0.6
	rhrWindowMinutes  = 5
	rhrWindowRequired = 4
)

func restingHeartRate(rows []store.HeartRateSample, w []span) *float64 {
	total := totalMinutes(w)
	if total == 0 {
		return nil
	}
	byMinute := make(map[int64]int, len(rows))
	for _, r := range rows {
		if r.Source != sourceContinuousHR {
			continue
		}
		byMinute[r.At.Unix()/60] = r.BPM
	}
	if float64(len(byMinute)) < rhrCoverageRatio*float64(total) {
		return nil
	}

	window := rhrWindowMinutes * time.Minute
	best := math.Inf(1)
	found := false
	for _, s := range w {
		start := s.Start.Truncate(time.Minute)
		for !start.Add(window).After(s.End) {
			sum, present := 0, 0
			for i := range rhrWindowMinutes {
				if bpm, ok := byMinute[start.Add(time.Duration(i)*time.Minute).Unix()/60]; ok {
					sum += bpm
					present++
				}
			}
			if present >= rhrWindowRequired {
				avg := float64(sum) / float64(present)
				if avg < best {
					best = avg
				}
				found = true
			}
			start = start.Add(time.Minute)
		}
	}
	if !found {
		return nil
	}
	return new(best)
}
