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
    SELECT sampled_at AS ts FROM hrv_samples
    UNION ALL SELECT sampled_at FROM spo2_samples
    UNION ALL SELECT sampled_at FROM stress_samples
    UNION ALL SELECT sampled_at FROM resp_samples
    UNION ALL SELECT sampled_at FROM rhr_samples
    UNION ALL SELECT sampled_at FROM temperature_samples
    UNION ALL SELECT sampled_at FROM heart_rate_samples
	UNION ALL SELECT sampled_at FROM step_samples
    UNION ALL SELECT started_at + make_interval(secs => duration_sec) FROM workouts
	UNION ALL SELECT COALESCE(
	  (SELECT MAX((stage->>'end')::timestamptz) FROM jsonb_array_elements(stages_json) stage),
	  started_at + make_interval(mins => total_mins + wake_mins)
	) FROM sleep_sessions
    UNION ALL SELECT started_at + make_interval(secs => duration_sec) FROM activity_sessions
  ) t) AS data_through,
	(SELECT MAX(ingested_at) FROM sync_sessions) AS last_ingest,
	(SELECT MAX(started_at + make_interval(secs => duration_sec)) FROM workouts WHERE has_summary) AS workout_summary_end,
	(SELECT MAX(started_at + make_interval(secs => duration_sec)) FROM workouts WHERE has_detail) AS workout_detail_end,
	(SELECT MAX(COALESCE(
	  (SELECT MAX((stage->>'end')::timestamptz) FROM jsonb_array_elements(stages_json) stage),
	  started_at + make_interval(mins => total_mins + wake_mins)
	)) FROM sleep_sessions WHERE NOT is_nap) AS main_sleep_end,
	(SELECT MAX(sampled_at) FROM temperature_samples) AS temp_end,
  (SELECT MAX(sampled_at) FROM heart_rate_samples WHERE source_type = '0x46') AS hr_end,
  (SELECT MAX(sampled_at) FROM stress_samples) AS stress_end,
  (SELECT MAX(sampled_at) FROM hrv_samples) AS hrv_end,
  (SELECT MAX(sampled_at) FROM spo2_samples WHERE source_type = '0x25') AS spot_spo2_end,
  (SELECT MAX(sampled_at) FROM spo2_samples WHERE source_type = '0x26') AS sleep_spo2_end,
  (SELECT MAX(sampled_at) FROM resp_samples) AS resp_end,
  (SELECT MAX(sampled_at) FROM rhr_samples) AS rhr_end,
	(SELECT MAX(ts) FROM (
	  SELECT sampled_at AS ts FROM step_samples
	  UNION ALL SELECT sampled_at FROM heart_rate_samples WHERE source_type = '0x01'
	) activity_samples) AS activity_end,
	(SELECT MAX(day_key)::timestamp AT TIME ZONE 'Asia/Kolkata' FROM raw_pai_scores) AS pai_end,
	(SELECT MAX(day_key)::timestamp AT TIME ZONE 'Asia/Kolkata' FROM raw_readiness) AS readiness_end`
	var (
		through, ingest                              *time.Time
		workoutSummaryEnd, workoutDetailEnd          *time.Time
		mainSleepEnd, tempEnd, hrEnd                 *time.Time
		stressEnd, hrvEnd, spotSpo2End, sleepSpo2End *time.Time
		respEnd, rhrEnd                              *time.Time
		stepsEnd, paiEnd, readinessEnd               *time.Time
	)
	err := s.pool.QueryRow(ctx, q).Scan(
		&through, &ingest,
		&workoutSummaryEnd, &workoutDetailEnd,
		&mainSleepEnd, &tempEnd, &hrEnd,
		&stressEnd, &hrvEnd, &spotSpo2End, &sleepSpo2End,
		&respEnd, &rhrEnd,
		&stepsEnd, &paiEnd, &readinessEnd,
	)
	if err != nil {
		return DataCoverage{}, err
	}
	types := buildTypeCoverage(
		workoutSummaryEnd, workoutDetailEnd, mainSleepEnd, tempEnd, hrEnd,
		stressEnd, hrvEnd, spotSpo2End, sleepSpo2End, respEnd, rhrEnd,
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
