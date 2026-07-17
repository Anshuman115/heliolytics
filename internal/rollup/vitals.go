package rollup

import (
	"context"
	"log"
	"time"

	"github.com/heliolytics/api/internal/store"
)

// recomputeHrvSQL averages hrv_samples.value within the sleep window and
// writes it to daily_metrics.hrv_rmssd.
const recomputeHrvSQL = `
	UPDATE daily_metrics SET hrv_rmssd = (
		SELECT ROUND(AVG(value))::int FROM hrv_samples
		WHERE sampled_at >= $2 AND sampled_at <= $3
	), updated_at = NOW() WHERE day_key = $1::date`

// recomputeSpo2SQL averages spo2_samples.value within the sleep window and
// writes it to daily_metrics.spo2_avg.
const recomputeSpo2SQL = `
	UPDATE daily_metrics SET spo2_avg = (
		SELECT ROUND(AVG(value))::int FROM spo2_samples
		WHERE sampled_at >= $2 AND sampled_at <= $3
	), updated_at = NOW() WHERE day_key = $1::date`

// recomputeRespSQL averages resp_samples.value within the sleep window and
// writes it to daily_metrics.resp_rate_avg.
const recomputeRespSQL = `
	UPDATE daily_metrics SET resp_rate_avg = (
		SELECT ROUND(AVG(value))::int FROM resp_samples
		WHERE sampled_at >= $2 AND sampled_at <= $3
	), updated_at = NOW() WHERE day_key = $1::date`

// RecomputeDailyVitals fills hrv_rmssd, spo2_avg, resp_rate_avg from the DB.
// These three are only meaningful measured during sleep (daytime HRV/SpO2/
// resp rate is dominated by posture, caffeine, movement noise) — so this
// finds the day's best non-nap sleep window first, then averages samples
// falling inside it. Mirrors what internal/parse/vitals_rollup.go used to do
// from in-memory samples; this version reads the DB instead.
func RecomputeDailyVitals(ctx context.Context, st *store.Store, days []string) error {
	log.Printf("rollup vitals days=%v", days)
	for _, day := range days {
		start, end, ok, err := bestSleepWindow(ctx, st, day)
		if err != nil {
			return err
		}
		if !ok {
			log.Printf("rollup vitals day=%s skip=no_sleep_window", day)
			continue
		}
		if _, err := st.Pool().Exec(ctx, recomputeHrvSQL, day, start, end); err != nil {
			return err
		}
		if _, err := st.Pool().Exec(ctx, recomputeSpo2SQL, day, start, end); err != nil {
			return err
		}
		if _, err := st.Pool().Exec(ctx, recomputeRespSQL, day, start, end); err != nil {
			return err
		}
	}
	return nil
}

func bestSleepWindow(ctx context.Context, st *store.Store, day string) (start, end time.Time, ok bool, err error) {
	var totalMins int
	err = st.Pool().QueryRow(ctx, `
		SELECT started_at, total_mins FROM sleep_sessions
		WHERE day_key = $1::date AND is_nap = false
		ORDER BY score DESC
		LIMIT 1`, day).Scan(&start, &totalMins)
	if err != nil {
		return time.Time{}, time.Time{}, false, nil // no rows is not a hard error, just "no window"
	}
	return start, start.Add(time.Duration(totalMins) * time.Minute), true, nil
}
