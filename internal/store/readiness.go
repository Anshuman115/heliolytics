package store

import (
	"context"

	"github.com/heliolytics/api/internal/readiness"
)

// RecomputeReadiness computes and stores the recovery score for each given IST
// day from that day's trailing daily-metrics history. Idempotent: re-running on
// the same data yields the same score. Days still in cold-start (no stable
// baseline) are left unchanged.
func (s *Store) RecomputeReadiness(ctx context.Context, days []string) error {
	for _, day := range days {
		hist, err := s.readinessHistory(ctx, day)
		if err != nil {
			return err
		}
		score, ok := readiness.Compute(hist)
		if !ok {
			continue
		}
		if _, err := s.pool.Exec(ctx,
			`UPDATE daily_metrics SET readiness = $2, updated_at = NOW()
			 WHERE day_key = $1::date`, day, score); err != nil {
			return err
		}
	}
	return nil
}

// readinessHistory returns up to 60 trailing days of vitals (oldest→newest,
// target day last) for the readiness baseline.
func (s *Store) readinessHistory(ctx context.Context, day string) ([]readiness.DayVitals, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT hrv_rmssd, resting_hr, resp_rate_avg, sleep_score
		FROM daily_metrics
		WHERE day_key <= $1::date
		ORDER BY day_key DESC
		LIMIT 60`, day)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var desc []readiness.DayVitals
	for rows.Next() {
		var hrv, rhr, resp, sleep *int
		if err := rows.Scan(&hrv, &rhr, &resp, &sleep); err != nil {
			return nil, err
		}
		desc = append(desc, readiness.DayVitals{
			RMSSD: fptr(hrv), RHR: fptr(rhr), Resp: fptr(resp), SleepScore: fptr(sleep),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Query is newest-first; reverse so the target day is last.
	for i, j := 0, len(desc)-1; i < j; i, j = i+1, j-1 {
		desc[i], desc[j] = desc[j], desc[i]
	}
	return desc, nil
}

func fptr(v *int) *float64 {
	if v == nil {
		return nil
	}
	f := float64(*v)
	return &f
}
