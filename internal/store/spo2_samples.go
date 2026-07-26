package store

import (
	"context"

	"github.com/jackc/pgx/v5"
)

const upsertSpo2SampleSQL = `
	INSERT INTO spo2_samples (sampled_at, day_key, value, source_session_id, source_type)
	VALUES ($1, $2::date, $3, $4, $5)
	ON CONFLICT (sampled_at) DO UPDATE SET
		value = EXCLUDED.value,
		day_key = EXCLUDED.day_key,
		source_session_id = EXCLUDED.source_session_id,
		source_type = EXCLUDED.source_type`

func spo2SourceType(sourceType string) string {
	if sourceType == "0x25" {
		return sourceType
	}
	return "0x26"
}

func (s *Store) UpsertSpo2Samples(ctx context.Context, sid string, pts []SampleValue) error {
	if sid == "" {
		return errRequired("spo2_samples.source_session_id")
	}
	rows := make([]queuedRow, 0, len(pts))
	for _, p := range pts {
		rows = append(rows, queuedRow{
			p.SampledAt.UTC(), p.DayKey, p.Value, sid,
			spo2SourceType(p.SourceType),
		})
	}
	return s.execBatch(ctx, upsertSpo2SampleSQL, rows)
}

// UpsertSpo2SamplesTx is UpsertSpo2Samples run against an already-open
// transaction, for use inside Store.WithTx.
func (s *Store) UpsertSpo2SamplesTx(ctx context.Context, tx pgx.Tx, sid string, pts []SampleValue) error {
	if sid == "" {
		return errRequired("spo2_samples.source_session_id")
	}
	rows := make([]queuedRow, 0, len(pts))
	for _, p := range pts {
		rows = append(rows, queuedRow{
			p.SampledAt.UTC(), p.DayKey, p.Value, sid,
			spo2SourceType(p.SourceType),
		})
	}
	return execBatchTx(ctx, tx, upsertSpo2SampleSQL, rows)
}
