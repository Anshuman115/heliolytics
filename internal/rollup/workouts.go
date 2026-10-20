package rollup

import "github.com/Anshuman115/heliolytics/internal/store"

// activityCounts separates recorded workouts from derived auto activities and
// sums only reported workout calories. A reported zero stays zero.
func activityCounts(rows []store.ActivityRow) (workouts, autos int, calories *int) {
	total := 0
	reported := false
	for _, r := range rows {
		if r.Kind == "auto" {
			autos++
			continue
		}
		workouts++
		if r.Calories == nil {
			continue
		}
		total += *r.Calories
		reported = true
	}
	if reported {
		calories = new(total)
	}
	return workouts, autos, calories
}
