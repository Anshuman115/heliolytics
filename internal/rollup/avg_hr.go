package rollup

import "github.com/Anshuman115/heliolytics/internal/store"

const (
	sourceContinuousHR = "0x01"
	sourceSnapshotHR   = "0x46"
)

func meanDayHeartRate(rows []store.HeartRateSample) *float64 {
	var sum float64
	n := 0
	for _, r := range rows {
		if r.Source != sourceContinuousHR {
			continue
		}
		sum += float64(r.BPM)
		n++
	}
	if n == 0 {
		return nil
	}
	return new(sum / float64(n))
}
