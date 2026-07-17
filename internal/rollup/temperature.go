package rollup

import (
	"context"
	"log"

	"github.com/heliolytics/api/internal/store"
)

// recomputeDailyTemperatureSQL sets daily_metrics.temp_avg_c to the average of
// temperature_samples.celsius for a given day, reading from the DB rather
// than the sync payload.
const recomputeDailyTemperatureSQL = `
	UPDATE daily_metrics
	SET temp_avg_c = (SELECT AVG(celsius) FROM temperature_samples WHERE day_key = $1::date),
	    updated_at = NOW()
	WHERE day_key = $1::date`

// RecomputeDailyTemperature recomputes daily_metrics.temp_avg_c for each day
// in days by averaging temperature_samples straight from the database.
func RecomputeDailyTemperature(ctx context.Context, st *store.Store, days []string) error {
	log.Printf("rollup temperature days=%v", days)
	return st.ExecDayKeys(ctx, recomputeDailyTemperatureSQL, days)
}
