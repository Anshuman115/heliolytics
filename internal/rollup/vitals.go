package rollup

import "github.com/Anshuman115/heliolytics/internal/store"

// Night sensor sample floors. They are product quality rules.
const (
	hrvSamplesPerNight  = 60
	respSamplesPerNight = 60
	spo2SamplesMinimum  = 3
)

// meanNightHRV averages accepted 0x49 values inside the sleep union. It needs
// 60 samples covering at least half the union duration.
func meanNightHRV(rows []store.HRVSample, w []span) *float64 {
	total := totalMinutes(w)
	if total == 0 || len(rows) < hrvSamplesPerNight {
		return nil
	}
	if float64(len(rows))*64 < 0.5*float64(total)*60 {
		return nil
	}
	var sum float64
	for _, r := range rows {
		sum += r.RMSSDMS
	}
	return new(sum / float64(len(rows)))
}

// meanNightResp averages accepted 0x38 values inside the sleep union.
func meanNightResp(rows []store.RespSample) *float64 {
	if len(rows) < respSamplesPerNight {
		return nil
	}
	var sum float64
	for _, r := range rows {
		sum += r.BRMin
	}
	return new(sum / float64(len(rows)))
}

// spo2Summary returns the count plus average and minimum. Both statistics stay
// NULL below three samples while the count is always reported.
func spo2Summary(rows []store.SpO2Sample) (*float64, *int, int) {
	n := len(rows)
	if n == 0 {
		return nil, nil, 0
	}
	sum := 0
	minPct := rows[0].Pct
	for _, r := range rows {
		sum += r.Pct
		if r.Pct < minPct {
			minPct = r.Pct
		}
	}
	if n < spo2SamplesMinimum {
		return nil, nil, n
	}
	return new(float64(sum) / float64(n)), new(minPct), n
}
