package rollup

import (
	"context"
	"log"

	"github.com/heliolytics/api/internal/store"
)

// RecomputeDailyAvgHr fills daily_metrics.avg_hr from the continuous
// heart_rate_samples table — a plain calendar-day mean, unlike the
// sleep-window-restricted vitals (hrv/spo2/resp), since "average HR today"
// is meant to reflect the whole day, not just sleep.
const recomputeDailyAvgHrSQL = `
	UPDATE daily_metrics
	SET avg_hr = (SELECT ROUND(AVG(bpm))::int FROM heart_rate_samples WHERE day_key = $1::date),
	    updated_at = NOW()
	WHERE day_key = $1::date`

func RecomputeDailyAvgHr(ctx context.Context, st *store.Store, days []string) error {
	log.Printf("rollup avg_hr days=%v", days)
	return st.ExecDayKeys(ctx, recomputeDailyAvgHrSQL, days)
}
