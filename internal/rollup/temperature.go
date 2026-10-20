package rollup

import "github.com/Anshuman115/heliolytics/internal/store"

const temperatureSamplesPerNight = 60

func meanNightTemp(rows []store.TemperatureReadRow, w []span) *float64 {
	total := totalMinutes(w)
	if total == 0 {
		return nil
	}
	var sumCenti int
	n := 0
	for _, r := range rows {
		if !r.OnWrist {
			continue
		}
		sumCenti += r.CentiC
		n++
	}
	if n < temperatureSamplesPerNight || float64(n) < 0.5*float64(total) {
		return nil
	}
	return new(float64(sumCenti) / 100 / float64(n))
}
