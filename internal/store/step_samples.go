package store

import (
	"context"
	"time"

	"github.com/Anshuman115/heliolytics/internal/store/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Store) WriteSteps(ctx context.Context, tx pgx.Tx, rows []StepSample,
	sessionID string, observedAt time.Time) (Mutation, error) {
	const table = "step_samples"
	b := newMutation()
	if err := validateSessionRef(sessionID, table); err != nil {
		return Mutation{}, err
	}
	if err := validateObservedAt(observedAt, table); err != nil {
		return Mutation{}, err
	}
	observedAt = normInstant(observedAt)

	normalized := make([]StepSample, 0, len(rows))
	for _, r := range rows {
		if r.Steps < 0 || r.Steps > 400 {
			return Mutation{}, invalidRow(table, "steps out of range")
		}
		if err := validateInstant(r.At, table); err != nil {
			return Mutation{}, err
		}
		r.At = normInstant(r.At)
		normalized = append(normalized, r)
	}
	normalized, err := dedupeSamples(table, normalized,
		func(r StepSample) sampleKey { return keyOf(r.At, "") },
		func(r StepSample) any { return r.Steps })
	if err != nil {
		return Mutation{}, err
	}
	if len(normalized) == 0 {
		return b.result(), nil
	}

	q := db.New(tx)
	for start := 0; start < len(normalized); start += sampleBatchSize {
		chunk := normalized[start:min(start+sampleBatchSize, len(normalized))]
		keys := make([]pgtype.Timestamptz, 0, len(chunk))
		for _, r := range chunk {
			keys = append(keys, tsAt(r.At))
		}
		stored, err := q.ListStepSamplesByKeys(ctx, keys)
		if err != nil {
			return Mutation{}, err
		}
		type storedRow struct {
			steps     int
			observed  time.Time
			sessionID string
		}
		prev := make(map[sampleKey]storedRow, len(stored))
		for _, r := range stored {
			prev[keyOf(r.SampledAt.Time, "")] = storedRow{
				steps:     int(r.Steps),
				observed:  r.ObservedAt.Time,
				sessionID: r.SessionID,
			}
		}

		params := db.UpsertStepSamplesParams{
			SampledAt:  make([]pgtype.Timestamptz, 0, len(chunk)),
			Steps:      make([]int16, 0, len(chunk)),
			SessionID:  make([]string, 0, len(chunk)),
			ObservedAt: make([]pgtype.Timestamptz, 0, len(chunk)),
		}
		for _, r := range chunk {
			old, exists := prev[keyOf(r.At, "")]
			if exists && !tupleGE(observedAt, sessionID, old.observed, old.sessionID) {
				continue
			}
			if !exists || old.steps != r.Steps {
				b.addPoint(r.At)
			}
			params.SampledAt = append(params.SampledAt, tsAt(r.At))
			params.Steps = append(params.Steps, int16(r.Steps))
			params.SessionID = append(params.SessionID, sessionID)
			params.ObservedAt = append(params.ObservedAt, tsAt(observedAt))
		}
		if len(params.SampledAt) == 0 {
			continue
		}
		if err := q.UpsertStepSamples(ctx, params); err != nil {
			return Mutation{}, err
		}
	}
	return b.result(), nil
}

func listSteps(ctx context.Context, q *db.Queries, from, to time.Time) ([]StepSample, error) {
	rows, err := q.ListStepSamples(ctx, db.ListStepSamplesParams{FromAt: tsAt(from), ToAt: tsAt(to)})
	if err != nil {
		return nil, err
	}
	out := make([]StepSample, 0, len(rows))
	for _, r := range rows {
		out = append(out, StepSample{At: r.SampledAt.Time, Steps: int(r.Steps)})
	}
	return out, nil
}
