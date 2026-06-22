package store

import (
	"context"
	"time"
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
func (s *Store) UpsertStepSamples(ctx context.Context, sid string, pts []StepSample) error {
	if sid == "" {
		return errRequired("step_samples.source_session_id")
	}
	for _, p := range pts {
		if _, err := s.pool.Exec(ctx, `
			INSERT INTO step_samples (sampled_at, day_key, steps, source_session_id)
			VALUES ($1, $2::date, $3, $4)
			ON CONFLICT (sampled_at) DO UPDATE SET
				steps = EXCLUDED.steps,
				day_key = EXCLUDED.day_key,
				source_session_id = EXCLUDED.source_session_id`,
			p.SampledAt.UTC(), p.DayKey, p.Steps, sid); err != nil {
			return err
		}
	}
	return nil
}

// RecomputeDailySteps sets daily_metrics.steps to SUM(step_samples.steps) for
// each given IST day. Because step_samples are idempotent per minute, this is
// immune to the 60-minute sync overlap that the additive upsert double-counts.
func (s *Store) RecomputeDailySteps(ctx context.Context, days []string) error {
	for _, day := range days {
		if _, err := s.pool.Exec(ctx, `
			UPDATE daily_metrics
			SET steps = COALESCE(
				(SELECT SUM(steps) FROM step_samples WHERE day_key = $1::date), 0),
			    updated_at = NOW()
			WHERE day_key = $1::date`, day); err != nil {
			return err
		}
	}
	return nil
}
