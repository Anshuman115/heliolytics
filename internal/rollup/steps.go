package rollup

import "github.com/Anshuman115/heliolytics/internal/store"

func sumSteps(rows []store.StepSample) *int {
	if len(rows) == 0 {
		return nil
	}
	total := 0
	for _, r := range rows {
		total += r.Steps
	}
	return new(total)
}
