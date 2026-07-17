package store

import (
	"context"

	"github.com/jackc/pgx/v5"
)

const upsertRhrSampleSQL = `
	INSERT INTO rhr_samples (sampled_at, day_key, value, source_session_id)
	VALUES ($1, $2::date, $3, $4)
	ON CONFLICT (sampled_at) DO UPDATE SET
		value = EXCLUDED.value,
		day_key = EXCLUDED.day_key,
		source_session_id = EXCLUDED.source_session_id`

func (s *Store) UpsertRhrSamples(ctx context.Context, sid string, pts []SampleValue) error {
	if sid == "" {
		return errRequired("rhr_samples.source_session_id")
	}
	rows := make([]queuedRow, 0, len(pts))
	for _, p := range pts {
		rows = append(rows, queuedRow{p.SampledAt.UTC(), p.DayKey, p.Value, sid})
	}
	return s.execBatch(ctx, upsertRhrSampleSQL, rows)
}

// UpsertRhrSamplesTx is UpsertRhrSamples run against an already-open
// transaction, for use inside Store.WithTx.
func (s *Store) UpsertRhrSamplesTx(ctx context.Context, tx pgx.Tx, sid string, pts []SampleValue) error {
	if sid == "" {
		return errRequired("rhr_samples.source_session_id")
	}
	rows := make([]queuedRow, 0, len(pts))
	for _, p := range pts {
		rows = append(rows, queuedRow{p.SampledAt.UTC(), p.DayKey, p.Value, sid})
	}
	return execBatchTx(ctx, tx, upsertRhrSampleSQL, rows)
}
