package rollup

import (
	"context"
	"log"
)

const recomputeDailyRhrSQL = `
	UPDATE daily_metrics
	SET resting_hr = COALESCE((
		SELECT value FROM rhr_samples
		WHERE day_key = $1::date
		ORDER BY sampled_at DESC LIMIT 1
	), resting_hr), updated_at = NOW()
	WHERE day_key = $1::date`

func RecomputeDailyRhr(ctx context.Context, st Target, days []string) error {
	log.Printf("rollup rhr days=%v", days)
	return st.ExecDayKeys(ctx, recomputeDailyRhrSQL, days)
}
