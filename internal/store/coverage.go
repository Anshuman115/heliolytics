package store

import (
	"context"
	"time"
)

type DataCoverage struct {
	DataThrough  *time.Time `json:"dataThrough,omitempty"`
	LastIngestAt *time.Time `json:"lastIngestAt,omitempty"`
	HasData      bool       `json:"hasData"`
	Types        map[string]*time.Time
}

func (s *Store) GetCoverage(ctx context.Context) (DataCoverage, error) {
	const q = `
SELECT
  (SELECT MAX(ts) FROM (
    SELECT sampled_at AS ts FROM health_samples
    UNION ALL SELECT sampled_at FROM temperature_samples
    UNION ALL SELECT started_at + make_interval(secs => duration_sec) FROM workouts
    UNION ALL SELECT started_at + make_interval(mins => total_mins) FROM sleep_sessions
    UNION ALL SELECT started_at + make_interval(secs => duration_sec) FROM activity_sessions
  ) t) AS data_through,
  (SELECT MAX(ingested_at) FROM sync_sessions) AS last_ingest,
  (SELECT MAX(started_at + make_interval(secs => duration_sec)) FROM workouts) AS workout_end,
  (SELECT MAX(started_at + make_interval(secs => duration_sec)) FROM activity_sessions) AS activity_end,
  (SELECT MAX(started_at + make_interval(mins => total_mins)) FROM sleep_sessions WHERE NOT is_nap) AS main_sleep_end,
  (SELECT MAX(started_at + make_interval(mins => total_mins)) FROM sleep_sessions WHERE is_nap) AS nap_end,
  (SELECT MAX(sampled_at) FROM temperature_samples) AS temp_end,
  (SELECT MAX(sampled_at) FROM health_samples WHERE metric = 'stress') AS stress_end,
  (SELECT MAX(sampled_at) FROM health_samples WHERE metric = 'hrv') AS hrv_end,
  (SELECT MAX(sampled_at) FROM health_samples WHERE metric = 'spo2') AS spo2_end,
  (SELECT MAX(sampled_at) FROM health_samples WHERE metric = 'spo2_sleep') AS spo2_sleep_end,
  (SELECT MAX(sampled_at) FROM health_samples WHERE metric = 'resp_rate') AS resp_end,
  (SELECT MAX(sampled_at) FROM health_samples WHERE metric = 'rhr') AS rhr_end,
  (SELECT MAX(sampled_at) FROM health_samples WHERE metric = 'max_hr') AS max_hr_end,
  (SELECT MAX(updated_at) FROM daily_metrics WHERE steps > 0) AS steps_end,
  (SELECT MAX(updated_at) FROM daily_metrics WHERE pai_score IS NOT NULL) AS pai_end,
  (SELECT MAX(updated_at) FROM daily_metrics WHERE readiness IS NOT NULL) AS readiness_end`
	var (
		through, ingest                                                       *time.Time
		workoutEnd, activityEnd, mainSleepEnd, napEnd, tempEnd                *time.Time
		stressEnd, hrvEnd, spo2End, spo2SleepEnd, respEnd, rhrEnd, maxHrEnd *time.Time
		stepsEnd, paiEnd, readinessEnd                                        *time.Time
	)
	err := s.pool.QueryRow(ctx, q).Scan(
		&through, &ingest,
		&workoutEnd, &activityEnd, &mainSleepEnd, &napEnd, &tempEnd,
		&stressEnd, &hrvEnd, &spo2End, &spo2SleepEnd, &respEnd, &rhrEnd, &maxHrEnd,
		&stepsEnd, &paiEnd, &readinessEnd,
	)
	if err != nil {
		return DataCoverage{}, err
	}
	types := buildTypeCoverage(
		workoutEnd, activityEnd, mainSleepEnd, napEnd, tempEnd,
		stressEnd, hrvEnd, spo2End, spo2SleepEnd, respEnd, rhrEnd, maxHrEnd,
		stepsEnd, paiEnd, readinessEnd,
	)
	out := DataCoverage{
		DataThrough:  through,
		LastIngestAt: ingest,
		Types:        types,
	}
	out.HasData = through != nil || ingest != nil
	return out, nil
}
