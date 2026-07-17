package store

import (
	"context"
)

const upsertSpo2SampleSQL = `
	INSERT INTO spo2_samples (sampled_at, day_key, value, source_session_id)
	VALUES ($1, $2::date, $3, $4)
	ON CONFLICT (sampled_at) DO UPDATE SET
		value = EXCLUDED.value,
		day_key = EXCLUDED.day_key,
		source_session_id = EXCLUDED.source_session_id`

func (s *Store) UpsertSpo2Samples(ctx context.Context, sid string, pts []SampleValue) error {
	if sid == "" {
		return errRequired("spo2_samples.source_session_id")
	}
	rows := make([]queuedRow, 0, len(pts))
	for _, p := range pts {
		rows = append(rows, queuedRow{p.SampledAt.UTC(), p.DayKey, p.Value, sid})
	}
	return s.execBatch(ctx, upsertSpo2SampleSQL, rows)
}
