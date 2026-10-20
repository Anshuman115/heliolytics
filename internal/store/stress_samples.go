package store

import (
	"context"
	"time"

	"github.com/Anshuman115/heliolytics/internal/store/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Store) WriteStress(ctx context.Context, tx pgx.Tx, rows []StressSample,
	sessionID string, observedAt time.Time) (Mutation, error) {
	const table = "stress_samples"
	b := newMutation()
	if err := validateSessionRef(sessionID, table); err != nil {
		return Mutation{}, err
	}
	if err := validateObservedAt(observedAt, table); err != nil {
		return Mutation{}, err
	}
	observedAt = normInstant(observedAt)

	normalized := make([]StressSample, 0, len(rows))
	for _, r := range rows {
		if r.Stress < 0 || r.Stress > 100 {
			return Mutation{}, invalidRow(table, "stress out of range")
		}
		if err := validateInstant(r.At, table); err != nil {
			return Mutation{}, err
		}
		r.At = normInstant(r.At)
		normalized = append(normalized, r)
	}
	normalized, err := dedupeSamples(table, normalized,
		func(r StressSample) sampleKey { return keyOf(r.At, "") },
		func(r StressSample) any { return r.Stress })
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
		stored, err := q.ListStressSamplesByKeys(ctx, keys)
		if err != nil {
			return Mutation{}, err
		}
		type storedRow struct {
			stress    int
			observed  time.Time
			sessionID string
		}
		prev := make(map[sampleKey]storedRow, len(stored))
		for _, r := range stored {
			prev[keyOf(r.SampledAt.Time, "")] = storedRow{
				stress:    int(r.Stress),
				observed:  r.ObservedAt.Time,
				sessionID: r.SessionID,
			}
		}

		params := db.UpsertStressSamplesParams{
			SampledAt:  make([]pgtype.Timestamptz, 0, len(chunk)),
			Stress:     make([]int16, 0, len(chunk)),
			SessionID:  make([]string, 0, len(chunk)),
			ObservedAt: make([]pgtype.Timestamptz, 0, len(chunk)),
		}
		for _, r := range chunk {
			old, exists := prev[keyOf(r.At, "")]
			if exists && !tupleGE(observedAt, sessionID, old.observed, old.sessionID) {
				continue
			}
			if !exists || old.stress != r.Stress {
				b.addPoint(r.At)
			}
			params.SampledAt = append(params.SampledAt, tsAt(r.At))
			params.Stress = append(params.Stress, int16(r.Stress))
			params.SessionID = append(params.SessionID, sessionID)
			params.ObservedAt = append(params.ObservedAt, tsAt(observedAt))
		}
		if len(params.SampledAt) == 0 {
			continue
		}
		if err := q.UpsertStressSamples(ctx, params); err != nil {
			return Mutation{}, err
		}
	}
	return b.result(), nil
}

func listStress(ctx context.Context, q *db.Queries, from, to time.Time) ([]StressSample, error) {
	rows, err := q.ListStressSamples(ctx, db.ListStressSamplesParams{FromAt: tsAt(from), ToAt: tsAt(to)})
	if err != nil {
		return nil, err
	}
	out := make([]StressSample, 0, len(rows))
	for _, r := range rows {
		out = append(out, StressSample{At: r.SampledAt.Time, Stress: int(r.Stress)})
	}
	return out, nil
}
