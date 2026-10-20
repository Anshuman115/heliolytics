package store

import (
	"context"
	"math"
	"time"

	"github.com/Anshuman115/heliolytics/internal/store/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Store) WriteHRV(ctx context.Context, tx pgx.Tx, rows []HRVSample,
	sessionID string, observedAt time.Time) (Mutation, error) {
	const table = "hrv_samples"
	b := newMutation()
	if err := validateSessionRef(sessionID, table); err != nil {
		return Mutation{}, err
	}
	if err := validateObservedAt(observedAt, table); err != nil {
		return Mutation{}, err
	}
	observedAt = normInstant(observedAt)

	normalized := make([]HRVSample, 0, len(rows))
	for _, r := range rows {
		if math.IsNaN(r.RMSSDMS) || math.IsInf(r.RMSSDMS, 0) || r.RMSSDMS <= 0 || r.RMSSDMS > 300 {
			return Mutation{}, invalidRow(table, "rmssd_ms out of range")
		}
		if err := validateInstant(r.At, table); err != nil {
			return Mutation{}, err
		}
		r.At = normInstant(r.At)
		normalized = append(normalized, r)
	}
	normalized, err := dedupeSamples(table, normalized,
		func(r HRVSample) sampleKey { return keyOf(r.At, "") },
		func(r HRVSample) any { return r.RMSSDMS })
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
		stored, err := q.ListHrvSamplesByKeys(ctx, keys)
		if err != nil {
			return Mutation{}, err
		}
		type storedRow struct {
			rmssd     float64
			observed  time.Time
			sessionID string
		}
		prev := make(map[sampleKey]storedRow, len(stored))
		for _, r := range stored {
			prev[keyOf(r.SampledAt.Time, "")] = storedRow{
				rmssd:     r.Rmssd,
				observed:  r.ObservedAt.Time,
				sessionID: r.SessionID,
			}
		}

		params := db.UpsertHrvSamplesParams{
			SampledAt:  make([]pgtype.Timestamptz, 0, len(chunk)),
			Rmssd:      make([]float32, 0, len(chunk)),
			SessionID:  make([]string, 0, len(chunk)),
			ObservedAt: make([]pgtype.Timestamptz, 0, len(chunk)),
		}
		for _, r := range chunk {
			old, exists := prev[keyOf(r.At, "")]
			if exists && !tupleGE(observedAt, sessionID, old.observed, old.sessionID) {
				continue
			}
			if !exists || old.rmssd != r.RMSSDMS {
				b.addPoint(r.At)
			}
			params.SampledAt = append(params.SampledAt, tsAt(r.At))
			params.Rmssd = append(params.Rmssd, float32(r.RMSSDMS))
			params.SessionID = append(params.SessionID, sessionID)
			params.ObservedAt = append(params.ObservedAt, tsAt(observedAt))
		}
		if len(params.SampledAt) == 0 {
			continue
		}
		if err := q.UpsertHrvSamples(ctx, params); err != nil {
			return Mutation{}, err
		}
	}
	return b.result(), nil
}

func listHRV(ctx context.Context, q *db.Queries, from, to time.Time) ([]HRVSample, error) {
	rows, err := q.ListHrvSamples(ctx, db.ListHrvSamplesParams{FromAt: tsAt(from), ToAt: tsAt(to)})
	if err != nil {
		return nil, err
	}
	out := make([]HRVSample, 0, len(rows))
	for _, r := range rows {
		out = append(out, HRVSample{At: r.SampledAt.Time, RMSSDMS: r.Rmssd})
	}
	return out, nil
}
