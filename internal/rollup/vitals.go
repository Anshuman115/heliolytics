package rollup

import (
	"context"
	"log"
	"time"

	"github.com/heliolytics/api/internal/store"
)

// recomputeHrvSQL averages hrv_samples.value within the sleep window and
// writes it to daily_metrics.hrv_rmssd. COALESCEd onto the existing value so
// a window with zero in-window samples (metric missing from this sync, but
// present from an earlier one) leaves the prior value intact instead of
// clobbering it to NULL — same guard sleep.go uses for its main-sleep fields.
const recomputeHrvSQL = `
	UPDATE daily_metrics SET hrv_rmssd = COALESCE((
		SELECT ROUND(AVG(value))::int FROM hrv_samples
		WHERE sampled_at >= $2 AND sampled_at <= $3
	), hrv_rmssd), updated_at = NOW() WHERE day_key = $1::date`

// recomputeSpo2SQL averages spo2_samples.value within the sleep window and
// writes it to daily_metrics.spo2_avg. Same COALESCE-onto-prior-value guard
// as recomputeHrvSQL.
const recomputeSpo2SQL = `
	UPDATE daily_metrics SET spo2_avg = COALESCE((
		SELECT ROUND(AVG(value))::int FROM spo2_samples
		WHERE sampled_at >= $2 AND sampled_at <= $3
	), spo2_avg), updated_at = NOW() WHERE day_key = $1::date`

// recomputeRespSQL averages resp_samples.value within the sleep window and
// writes it to daily_metrics.resp_rate_avg. Same COALESCE-onto-prior-value
// guard as recomputeHrvSQL.
const recomputeRespSQL = `
	UPDATE daily_metrics SET resp_rate_avg = COALESCE((
		SELECT ROUND(AVG(value))::int FROM resp_samples
		WHERE sampled_at >= $2 AND sampled_at <= $3
	), resp_rate_avg), updated_at = NOW() WHERE day_key = $1::date`

// RecomputeDailyVitals fills hrv_rmssd, spo2_avg, resp_rate_avg from the DB.
// These three are only meaningful measured during sleep (daytime HRV/SpO2/
// resp rate is dominated by posture, caffeine, movement noise) — so this
// finds the day's best non-nap sleep window first, then averages samples
// falling inside it. Mirrors what internal/parse/vitals_rollup.go used to do
// from in-memory samples; this version reads the DB instead.
func RecomputeDailyVitals(ctx context.Context, st Target, days []string) error {
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
		if err := st.ExecSQL(ctx, recomputeHrvSQL, day, start, end); err != nil {
			return err
		}
		if err := st.ExecSQL(ctx, recomputeSpo2SQL, day, start, end); err != nil {
			return err
		}
		if err := st.ExecSQL(ctx, recomputeRespSQL, day, start, end); err != nil {
			return err
		}
	}
	return nil
}

func bestSleepWindow(ctx context.Context, st Target, day string) (start, end time.Time, ok bool, err error) {
	sessions, err := st.ListSleep(ctx, day, day)
	if err != nil {
		return time.Time{}, time.Time{}, false, err
	}
	var best *store.SleepMetric
	for i := range sessions {
		if sessions[i].IsNap || best != nil && sessions[i].Score <= best.Score {
			continue
		}
		best = &sessions[i]
	}
	if best == nil {
		return time.Time{}, time.Time{}, false, nil
	}
	if len(best.Stages) > 0 {
		start, end = best.Stages[0].Start, best.Stages[0].End
		for _, stage := range best.Stages[1:] {
			if stage.Start.Before(start) {
				start = stage.Start
			}
			if stage.End.After(end) {
				end = stage.End
			}
		}
		return start, end, true, nil
	}
	duration := time.Duration(best.TotalMins+best.WakeMins) * time.Minute
	return best.StartedAt, best.StartedAt.Add(duration), true, nil
}
