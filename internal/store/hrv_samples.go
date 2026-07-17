package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
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

// UpsertHrvSamplesTx is UpsertHrvSamples run against an already-open
// transaction, for use inside Store.WithTx.
func (s *Store) UpsertHrvSamplesTx(ctx context.Context, tx pgx.Tx, sid string, pts []SampleValue) error {
	if sid == "" {
		return errRequired("hrv_samples.source_session_id")
	}
	rows := make([]queuedRow, 0, len(pts))
	for _, p := range pts {
		rows = append(rows, queuedRow{p.SampledAt.UTC(), p.DayKey, p.Value, sid})
	}
	return execBatchTx(ctx, tx, upsertHrvSampleSQL, rows)
}

// ListHrvSamplesRange returns hrv_samples rows with day_key in [from, to],
// ordered by sampled_at ascending, for the hrv trend endpoint.
func (s *Store) ListHrvSamplesRange(ctx context.Context, from, to string) ([]SampleValue, error) {
	fromD, err := dateKey(from)
	if err != nil {
		return nil, err
	}
	toD, err := dateKey(to)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx,
		`SELECT sampled_at, day_key, value FROM hrv_samples WHERE day_key >= $1 AND day_key <= $2 ORDER BY sampled_at ASC`,
		fromD, toD)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SampleValue
	for rows.Next() {
		var v SampleValue
		var day pgtype.Date
		if err := rows.Scan(&v.SampledAt, &day, &v.Value); err != nil {
			return nil, err
		}
		v.DayKey = dateKeyString(day)
		out = append(out, v)
	}
	return out, rows.Err()
}
