package rollup

import "github.com/Anshuman115/heliolytics/internal/store"

func paiScore(rows []store.PAIScore) *int {
	if len(rows) == 0 {
		return nil
	}
	return new(rows[0].Score)
}
