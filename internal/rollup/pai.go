package rollup

import "github.com/Anshuman115/heliolytics/internal/store"

// paiScore returns the day's canonical PAI score. A legitimate zero is kept
// and a day without a record stays NULL.
func paiScore(rows []store.PAIScore) *int {
	if len(rows) == 0 {
		return nil
	}
	return new(rows[0].Score)
}
