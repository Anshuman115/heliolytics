package store

import (
	"context"
	"math"
	"time"

	"github.com/Anshuman115/heliolytics/internal/store/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Store) WriteResp(ctx context.Context, tx pgx.Tx, rows []RespSample,
	sessionID string, observedAt time.Time) (Mutation, error) {
	const table = "resp_samples"
	b := newMutation()
	if err := validateSessionRef(sessionID, table); err != nil {
		return Mutation{}, err
	}
	if err := validateObservedAt(observedAt, table); err != nil {
		return Mutation{}, err
	}
	observedAt = normInstant(observedAt)

	normalized := make([]RespSample, 0, len(rows))
	for _, r := range rows {
		if math.IsNaN(r.BRMin) || math.IsInf(r.BRMin, 0) || r.BRMin < 4 || r.BRMin > 40 {
			return Mutation{}, invalidRow(table, "br_min out of range")
		}
		if err := validateInstant(r.At, table); err != nil {
			return Mutation{}, err
		}
		r.At = normInstant(r.At)
		normalized = append(normalized, r)
	}
	normalized, err := dedupeSamples(table, normalized,
		func(r RespSample) sampleKey { return keyOf(r.At, "") },
		func(r RespSample) any { return r.BRMin })
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
		stored, err := q.ListRespSamplesByKeys(ctx, keys)
		if err != nil {
			return Mutation{}, err
		}
		type storedRow struct {
			brMin     float64
			observed  time.Time
			sessionID string
		}
		prev := make(map[sampleKey]storedRow, len(stored))
		for _, r := range stored {
			prev[keyOf(r.SampledAt.Time, "")] = storedRow{
				brMin:     r.BrMin,
				observed:  r.ObservedAt.Time,
				sessionID: r.SessionID,
			}
		}

		params := db.UpsertRespSamplesParams{
			SampledAt:  make([]pgtype.Timestamptz, 0, len(chunk)),
			BrMin:      make([]float32, 0, len(chunk)),
			SessionID:  make([]string, 0, len(chunk)),
			ObservedAt: make([]pgtype.Timestamptz, 0, len(chunk)),
		}
		for _, r := range chunk {
			old, exists := prev[keyOf(r.At, "")]
			if exists && !tupleGE(observedAt, sessionID, old.observed, old.sessionID) {
				continue
			}
			if !exists || old.brMin != r.BRMin {
				b.addPoint(r.At)
			}
			params.SampledAt = append(params.SampledAt, tsAt(r.At))
			params.BrMin = append(params.BrMin, float32(r.BRMin))
			params.SessionID = append(params.SessionID, sessionID)
			params.ObservedAt = append(params.ObservedAt, tsAt(observedAt))
		}
		if len(params.SampledAt) == 0 {
			continue
		}
		if err := q.UpsertRespSamples(ctx, params); err != nil {
			return Mutation{}, err
		}
	}
	return b.result(), nil
}

func listResp(ctx context.Context, q *db.Queries, from, to time.Time) ([]RespSample, error) {
	rows, err := q.ListRespSamples(ctx, db.ListRespSamplesParams{FromAt: tsAt(from), ToAt: tsAt(to)})
	if err != nil {
		return nil, err
	}
	out := make([]RespSample, 0, len(rows))
	for _, r := range rows {
		out = append(out, RespSample{At: r.SampledAt.Time, BRMin: r.BrMin})
	}
	return out, nil
}
