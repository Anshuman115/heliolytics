package store

import (
	"context"
	"time"

	"github.com/Anshuman115/heliolytics/internal/store/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Store) WriteSpO2(ctx context.Context, tx pgx.Tx, rows []SpO2Sample,
	sessionID string, observedAt time.Time) (Mutation, error) {
	const table = "spo2_samples"
	b := newMutation()
	if err := validateSessionRef(sessionID, table); err != nil {
		return Mutation{}, err
	}
	if err := validateObservedAt(observedAt, table); err != nil {
		return Mutation{}, err
	}
	observedAt = normInstant(observedAt)

	normalized := make([]SpO2Sample, 0, len(rows))
	for _, r := range rows {
		if r.Source != "0x25" {
			return Mutation{}, invalidRow(table, "source %q invalid", r.Source)
		}
		if r.Mode != "auto" && r.Mode != "manual" {
			return Mutation{}, invalidRow(table, "mode %q invalid", r.Mode)
		}
		if r.Pct < 70 || r.Pct > 100 {
			return Mutation{}, invalidRow(table, "pct out of range")
		}
		if err := validateInstant(r.At, table); err != nil {
			return Mutation{}, err
		}
		r.At = normInstant(r.At)
		normalized = append(normalized, r)
	}
	normalized, err := dedupeSamples(table, normalized,
		func(r SpO2Sample) sampleKey { return keyOf(r.At, r.Source) },
		func(r SpO2Sample) any { return r.Pct })
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
		sources := make([]string, 0, len(chunk))
		for _, r := range chunk {
			keys = append(keys, tsAt(r.At))
			sources = append(sources, r.Source)
		}
		stored, err := q.ListSpo2SamplesByKeys(ctx, db.ListSpo2SamplesByKeysParams{
			Keys:    keys,
			Sources: sources,
		})
		if err != nil {
			return Mutation{}, err
		}
		type storedRow struct {
			pct       int
			mode      string
			observed  time.Time
			sessionID string
		}
		prev := make(map[sampleKey]storedRow, len(stored))
		for _, r := range stored {
			prev[keyOf(r.SampledAt.Time, r.Source)] = storedRow{
				pct:       int(r.Pct),
				mode:      r.Mode,
				observed:  r.ObservedAt.Time,
				sessionID: r.SessionID,
			}
		}

		params := db.UpsertSpo2SamplesParams{
			SampledAt:  make([]pgtype.Timestamptz, 0, len(chunk)),
			Source:     make([]string, 0, len(chunk)),
			Pct:        make([]int16, 0, len(chunk)),
			Mode:       make([]string, 0, len(chunk)),
			SessionID:  make([]string, 0, len(chunk)),
			ObservedAt: make([]pgtype.Timestamptz, 0, len(chunk)),
		}
		for _, r := range chunk {
			old, exists := prev[keyOf(r.At, r.Source)]
			if exists && !tupleGE(observedAt, sessionID, old.observed, old.sessionID) {
				continue
			}
			if !exists || old.pct != r.Pct || old.mode != r.Mode {
				b.addPoint(r.At)
			}
			params.SampledAt = append(params.SampledAt, tsAt(r.At))
			params.Source = append(params.Source, r.Source)
			params.Pct = append(params.Pct, int16(r.Pct))
			params.Mode = append(params.Mode, r.Mode)
			params.SessionID = append(params.SessionID, sessionID)
			params.ObservedAt = append(params.ObservedAt, tsAt(observedAt))
		}
		if len(params.SampledAt) == 0 {
			continue
		}
		if err := q.UpsertSpo2Samples(ctx, params); err != nil {
			return Mutation{}, err
		}
	}
	return b.result(), nil
}

func listSpO2(ctx context.Context, q *db.Queries, from, to time.Time) ([]SpO2Sample, error) {
	rows, err := q.ListSpo2Samples(ctx, db.ListSpo2SamplesParams{FromAt: tsAt(from), ToAt: tsAt(to)})
	if err != nil {
		return nil, err
	}
	out := make([]SpO2Sample, 0, len(rows))
	for _, r := range rows {
		out = append(out, SpO2Sample{
			At:     r.SampledAt.Time,
			Source: r.Source,
			Mode:   r.Mode,
			Pct:    int(r.Pct),
		})
	}
	return out, nil
}
