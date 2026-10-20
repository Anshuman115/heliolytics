package rollup

import "github.com/Anshuman115/heliolytics/internal/store"

// Source codes that feed heart rate aggregates.
const (
	sourceContinuousHR = "0x01"
	sourceSnapshotHR   = "0x46"
)

// meanDayHeartRate averages valid continuous 0x01 minutes for the whole IST
// day. The 0x46 snapshot source never contributes.
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
