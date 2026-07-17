package store

import (
	"context"
	"time"
)

// SampleValue is one per-minute reading for any of the single-value metric
// tables (hrv, spo2, stress, resp, rhr) — shared shape, each table gets its
// own upsert function because each writes to a different table name.
type SampleValue struct {
	DayKey    string
	SampledAt time.Time
	Value     float64
}

const upsertHrvSampleSQL = `
	INSERT INTO hrv_samples (sampled_at, day_key, value, source_session_id)
	VALUES ($1, $2::date, $3, $4)
	ON CONFLICT (sampled_at) DO UPDATE SET
		value = EXCLUDED.value,
		day_key = EXCLUDED.day_key,
		source_session_id = EXCLUDED.source_session_id`

func (s *Store) UpsertHrvSamples(ctx context.Context, sid string, pts []SampleValue) error {
	if sid == "" {
		return errRequired("hrv_samples.source_session_id")
	}
	rows := make([]queuedRow, 0, len(pts))
	for _, p := range pts {
		rows = append(rows, queuedRow{p.SampledAt.UTC(), p.DayKey, p.Value, sid})
	}
	return s.execBatch(ctx, upsertHrvSampleSQL, rows)
}
