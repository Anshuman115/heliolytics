package rollup

import (
	"context"
	"log"

	"github.com/heliolytics/api/internal/store"
)

// calories_total is summed here rather than in its own function because it's
// the same table (workouts) as workout_count — one query, one table scan.
const recomputeDailyWorkoutCountsSQL = `
	UPDATE daily_metrics
	SET workout_count = (SELECT COUNT(*) FROM workouts WHERE day_key = $1::date),
	    activity_session_count = (SELECT COUNT(*) FROM activity_sessions WHERE day_key = $1::date),
	    calories_total = (SELECT SUM(calories) FROM workouts WHERE day_key = $1::date),
	    updated_at = NOW()
	WHERE day_key = $1::date`

func RecomputeDailyWorkoutCounts(ctx context.Context, st *store.Store, days []string) error {
	log.Printf("rollup workout_counts days=%v", days)
	return st.ExecDayKeys(ctx, recomputeDailyWorkoutCountsSQL, days)
}
