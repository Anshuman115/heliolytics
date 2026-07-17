package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// StepSample is one per-minute step count keyed by its absolute timestamp.
type StepSample struct {
	DayKey    string
	SampledAt time.Time
	Steps     int
}

// UpsertStepSamples writes per-minute steps idempotently, keyed by the minute
// timestamp. A re-synced overlap window overwrites the same rows instead of
// adding, which is what makes daily totals immune to the sync overlap.
const upsertStepSampleSQL = `
	INSERT INTO step_samples (sampled_at, day_key, steps, source_session_id)
	VALUES ($1, $2::date, $3, $4)
	ON CONFLICT (sampled_at) DO UPDATE SET
		steps = EXCLUDED.steps,
		day_key = EXCLUDED.day_key,
		source_session_id = EXCLUDED.source_session_id`

func (s *Store) UpsertStepSamples(ctx context.Context, sid string, pts []StepSample) error {
	if sid == "" {
		return errRequired("step_samples.source_session_id")
	}
	rows := make([]queuedRow, 0, len(pts))
	for _, p := range pts {
		rows = append(rows, queuedRow{p.SampledAt.UTC(), p.DayKey, p.Steps, sid})
	}
	return s.execBatch(ctx, upsertStepSampleSQL, rows)
}

// UpsertStepSamplesTx is UpsertStepSamples run against an already-open
// transaction, for use inside Store.WithTx.
func (s *Store) UpsertStepSamplesTx(ctx context.Context, tx pgx.Tx, sid string, pts []StepSample) error {
	if sid == "" {
		return errRequired("step_samples.source_session_id")
	}
	rows := make([]queuedRow, 0, len(pts))
	for _, p := range pts {
		rows = append(rows, queuedRow{p.SampledAt.UTC(), p.DayKey, p.Steps, sid})
	}
	return execBatchTx(ctx, tx, upsertStepSampleSQL, rows)
}
