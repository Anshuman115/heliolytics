package store

import (
	"context"

	"github.com/heliolytics/api/internal/readiness"
	"github.com/heliolytics/api/internal/store/db"
)

// RecomputeReadiness computes and stores the recovery score for each given IST
// day from that day's trailing daily-metrics history. Idempotent: re-running on
// the same data yields the same score. Days still in cold-start (no stable
// baseline) are left unchanged.
func (s *Store) RecomputeReadiness(ctx context.Context, days []string) error {
	return recomputeReadiness(ctx, s.pool, days)
}

func recomputeReadiness(ctx context.Context, target db.DBTX, days []string) error {
	for _, day := range days {
		hist, err := readinessHistory(ctx, target, day)
		if err != nil {
			return err
		}
		score, _, ok := readiness.Compute(hist)
		if !ok {
			continue
		}
		// Write to computed_readiness only; the device 0x39 `readiness` column is
		// authoritative and reads COALESCE(readiness, computed_readiness).
		if _, err := target.Exec(ctx,
			`UPDATE daily_metrics SET computed_readiness = $2, updated_at = NOW()
			 WHERE day_key = $1::date`, day, score); err != nil {
			return err
		}
	}
	return nil
}

// readinessHistory returns up to 60 trailing days of vitals (oldest→newest,
// target day last) for the readiness baseline.
func (s *Store) readinessHistory(ctx context.Context, day string) ([]readiness.DayVitals, error) {
	return readinessHistory(ctx, s.pool, day)
}

func readinessHistory(ctx context.Context, target db.DBTX, day string) ([]readiness.DayVitals, error) {
	rows, err := target.Query(ctx, `
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

// ReadinessHistoryPublic exposes readinessHistory for the API layer's
// on-demand /recovery endpoint (RecomputeReadiness uses the private version
// internally; this is the same query, just reachable from outside the package).
func (s *Store) ReadinessHistoryPublic(ctx context.Context, day string) ([]readiness.DayVitals, error) {
	return s.readinessHistory(ctx, day)
}

func fptr(v *int) *float64 {
	if v == nil {
		return nil
	}
	f := float64(*v)
	return &f
}
