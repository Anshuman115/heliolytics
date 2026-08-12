package rollup

import (
	"context"
	"log"
)

// RecomputeDailyStress uses the latest stress_samples reading for the day,
// not an average — stress is a point-in-time device reading, and the latest
// value in a day is what the app has always shown as "today's stress."
const recomputeDailyStressSQL = `
	UPDATE daily_metrics
	SET stress_avg = (
		SELECT value::int FROM stress_samples
		WHERE day_key = $1::date
		ORDER BY sampled_at DESC LIMIT 1
	), updated_at = NOW()
	WHERE day_key = $1::date`

func RecomputeDailyStress(ctx context.Context, st Target, days []string) error {
	log.Printf("rollup stress days=%v", days)
	return st.ExecDayKeys(ctx, recomputeDailyStressSQL, days)
}
