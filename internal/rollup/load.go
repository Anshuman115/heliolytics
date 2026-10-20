package rollup

import (
	"math"
	"sort"
	"time"

	"github.com/Anshuman115/heliolytics/internal/store"
)

const (
	loadMinMinutes      = 720
	loadMinIntensity    = 0.30
	restingBaselineDays = 60
	restingRecencyDays  = 28
	restingMinValues    = 3
)

func completedAge(day, birth time.Time) int {
	age := day.Year() - birth.Year()
	anniversary := time.Date(day.Year(), birth.Month(), birth.Day(), 0, 0, 0, 0, day.Location())
	if day.Before(anniversary) {
		age--
	}
	return age
}

func maxHeartRate(age int) float64 { return 208 - 0.7*float64(age) }

func loadCoefficients(sex string) (a, b float64, ok bool) {
	switch sex {
	case "male":
		return 0.64, 1.92, true
	case "female":
		return 0.86, 1.67, true
	}
	return 0, 0, false
}

func trimp(day time.Time, hr []store.HeartRateSample, hrRest *float64,
	birth *time.Time, sex string) *float64 {
	if birth == nil || sex == "" || hrRest == nil {
		return nil
	}
	a, b, ok := loadCoefficients(sex)
	if !ok {
		return nil
	}
	reserve := maxHeartRate(completedAge(day, *birth)) - *hrRest
	if math.IsNaN(reserve) || math.IsInf(reserve, 0) || reserve <= 0 {
		return nil
	}
	slots := make(map[int64]int, len(hr))
	for _, r := range hr {
		if r.Source != sourceContinuousHR {
			continue
		}
		slots[r.At.Unix()/60] = r.BPM
	}
	if len(slots) < loadMinMinutes {
		return nil
	}
	keys := make([]int64, 0, len(slots))
	for k := range slots {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	total := 0.0
	for _, k := range keys {
		r := (float64(slots[k]) - *hrRest) / reserve
		if r < 0 {
			r = 0
		}
		if r > 1 {
			r = 1
		}
		if r < loadMinIntensity {
			continue
		}
		total += a * r * math.Exp(b*r)
	}
	return new(total)
}

func restingHRBaseline(history map[int]float64, day time.Time, current *float64) *float64 {
	all := make([]float64, 0, restingBaselineDays)
	recent := 0
	for i := 1; i <= restingBaselineDays; i++ {
		v, ok := history[dayKeyOf(day.AddDate(0, 0, -i))]
		if !ok {
			continue
		}
		all = append(all, v)
		if i <= restingRecencyDays {
			recent++
		}
	}
	if len(all) >= restingMinValues && recent >= restingMinValues {
		return new(medianOf(all))
	}
	if current == nil {
		return nil
	}
	return new(*current)
}

func medianOf(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}
