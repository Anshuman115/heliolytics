package rollup

import "github.com/Anshuman115/heliolytics/internal/store"

func meanStress(rows []store.StressSample) *float64 {
	if len(rows) == 0 {
		return nil
	}
	var sum float64
	for _, r := range rows {
		sum += float64(r.Stress)
	}
	return new(sum / float64(len(rows)))
}
