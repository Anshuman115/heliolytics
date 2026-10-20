package rollup

import "github.com/Anshuman115/heliolytics/internal/store"

// sumSteps totals one day's canonical step minutes, including observed zeros.
// No rows means the day has no measured steps and the column stays NULL.
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
