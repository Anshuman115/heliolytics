package rollup

import (
	"context"
	"log"

	"github.com/heliolytics/api/internal/store"
)

// recomputeDailyStepsSQL sets daily_metrics.steps to SUM(step_samples.steps)
// for a given day, reading from the DB rather than the sync payload. Because
// step_samples are idempotent per minute, this is immune to the sync overlap
// that an additive write would double-count.
const recomputeDailyStepsSQL = `
	UPDATE daily_metrics
	SET steps = COALESCE(
		(SELECT SUM(steps) FROM step_samples WHERE day_key = $1::date), 0),
	    updated_at = NOW()
	WHERE day_key = $1::date`

// RecomputeDailySteps recomputes daily_metrics.steps for each day in days by
// summing step_samples straight from the database.
func RecomputeDailySteps(ctx context.Context, st *store.Store, days []string) error {
	log.Printf("rollup steps days=%v", days)
	return st.ExecDayKeys(ctx, recomputeDailyStepsSQL, days)
}
