package store

import (
	"context"
	"time"
)

// HealthCompactDay mirrors the v6 compact series shape: one row per
// (metric, day), with per-sample offsets (seconds since the day's first
// sample) and values, so a client can reconstruct timestamps without
// repeating a full RFC3339 string per sample.
type HealthCompactDay struct {
	Metric    string    `json:"metric"`
	DayKey    string    `json:"dayKey"`
	StartTime time.Time `json:"startTime"`
	Offsets   []int32   `json:"offsets"`
	Values    []float64 `json:"values"`
}

// ListHealthSamplesCompact restores the v6 /metrics/series shape after the
// health_samples table split (Task 1) — sourced from the 5 per-metric
// tables instead of one tall table with a metric column, but the same
// UNION-then-group approach as the original hand-written query.
func (s *Store) ListHealthSamplesCompact(ctx context.Context, from, to string) ([]HealthCompactDay, error) {
	fromD, err := dateKey(from)
	if err != nil {
		return nil, err
	}
	toD, err := dateKey(to)
	if err != nil {
		return nil, err
	}

	const q = `
WITH unioned AS (
	SELECT 'hrv' AS metric, sampled_at, day_key, value::float8 AS value FROM hrv_samples WHERE day_key >= $1 AND day_key <= $2
	UNION ALL
	SELECT 'spo2', sampled_at, day_key, value::float8 FROM spo2_samples WHERE day_key >= $1 AND day_key <= $2
	UNION ALL
	SELECT 'stress', sampled_at, day_key, value::float8 FROM stress_samples WHERE day_key >= $1 AND day_key <= $2
	UNION ALL
	SELECT 'resp_rate', sampled_at, day_key, value::float8 FROM resp_samples WHERE day_key >= $1 AND day_key <= $2
	UNION ALL
	SELECT 'rhr', sampled_at, day_key, value::float8 FROM rhr_samples WHERE day_key >= $1 AND day_key <= $2
),
computed AS (
	SELECT
		metric, day_key, sampled_at, value,
		(MIN(sampled_at) OVER (PARTITION BY metric, day_key))::timestamptz AS day_start
	FROM unioned
)
SELECT
	metric, day_key, day_start AS start_time,
	array_agg(EXTRACT(EPOCH FROM (sampled_at - day_start))::int ORDER BY sampled_at)::int[] AS offsets,
	array_agg(value ORDER BY sampled_at)::float8[] AS values
FROM computed
GROUP BY metric, day_key, day_start
ORDER BY metric, day_key ASC`

	rows, err := s.pool.Query(ctx, q, fromD, toD)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []HealthCompactDay
	for rows.Next() {
		var d HealthCompactDay
		var day time.Time
		if err := rows.Scan(&d.Metric, &day, &d.StartTime, &d.Offsets, &d.Values); err != nil {
			return nil, err
		}
		d.DayKey = day.Format("2006-01-02")
		out = append(out, d)
	}
	return out, rows.Err()
}
