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
  (SELECT MAX(started_at + make_interval(secs => duration_sec)) FROM activity_sessions) AS activity_end`
	var through, ingest, workoutEnd, activityEnd *time.Time
	err := s.pool.QueryRow(ctx, q).Scan(&through, &ingest, &workoutEnd, &activityEnd)
	if err != nil {
		return DataCoverage{}, err
	}
	types := map[string]*time.Time{
		"0x05": workoutEnd,
		"0x06": workoutEnd,
		"0x3B": activityEnd,
	}
	out := DataCoverage{
		DataThrough:  through,
		LastIngestAt: ingest,
		Types:        types,
	}
	out.HasData = through != nil || ingest != nil
	return out, nil
}
