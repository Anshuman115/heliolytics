package store

import (
	"context"
	"time"

	"github.com/Anshuman115/heliolytics/internal/store/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// WriteHeartRate stores one bounded batch of heart rate samples. 0x01 and 0x46
// coexist at the same instant because the canonical key includes the source.
func (s *Store) WriteHeartRate(ctx context.Context, tx pgx.Tx, rows []HeartRateSample,
	sessionID string, observedAt time.Time) (Mutation, error) {
	const table = "heart_rate_samples"
	b := newMutation()
	if err := validateSessionRef(sessionID, table); err != nil {
		return Mutation{}, err
	}
	if err := validateObservedAt(observedAt, table); err != nil {
		return Mutation{}, err
	}
	observedAt = normInstant(observedAt)

	normalized := make([]HeartRateSample, 0, len(rows))
	for _, r := range rows {
		if r.Source != "0x01" && r.Source != "0x46" {
			return Mutation{}, invalidRow(table, "source %q invalid", r.Source)
		}
		if r.BPM < 25 || r.BPM > 250 {
			return Mutation{}, invalidRow(table, "bpm out of range")
		}
		if err := validateInstant(r.At, table); err != nil {
			return Mutation{}, err
		}
		r.At = normInstant(r.At)
		normalized = append(normalized, r)
	}
	normalized, err := dedupeSamples(table, normalized,
		func(r HeartRateSample) sampleKey { return keyOf(r.At, r.Source) },
		func(r HeartRateSample) any { return r.BPM })
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
		stored, err := q.ListHeartRateSamplesByKeys(ctx, db.ListHeartRateSamplesByKeysParams{
			Keys:    keys,
			Sources: sources,
		})
		if err != nil {
			return Mutation{}, err
		}
		type storedRow struct {
			bpm       int
			observed  time.Time
			sessionID string
		}
		prev := make(map[sampleKey]storedRow, len(stored))
		for _, r := range stored {
			prev[keyOf(r.SampledAt.Time, r.Source)] = storedRow{
				bpm:       int(r.Bpm),
				observed:  r.ObservedAt.Time,
				sessionID: r.SessionID,
			}
		}

		params := db.UpsertHeartRateSamplesParams{
			SampledAt:  make([]pgtype.Timestamptz, 0, len(chunk)),
			Source:     make([]string, 0, len(chunk)),
			Bpm:        make([]int16, 0, len(chunk)),
			SessionID:  make([]string, 0, len(chunk)),
			ObservedAt: make([]pgtype.Timestamptz, 0, len(chunk)),
		}
		for _, r := range chunk {
			old, exists := prev[keyOf(r.At, r.Source)]
			if exists && !tupleGE(observedAt, sessionID, old.observed, old.sessionID) {
				continue
			}
			if !exists || old.bpm != r.BPM {
				b.addPoint(r.At)
			}
			params.SampledAt = append(params.SampledAt, tsAt(r.At))
			params.Source = append(params.Source, r.Source)
			params.Bpm = append(params.Bpm, int16(r.BPM))
			params.SessionID = append(params.SessionID, sessionID)
			params.ObservedAt = append(params.ObservedAt, tsAt(observedAt))
		}
		if len(params.SampledAt) == 0 {
			continue
		}
		if err := q.UpsertHeartRateSamples(ctx, params); err != nil {
			return Mutation{}, err
		}
	}
	return b.result(), nil
}

// listHeartRate reads every source so rollup code keeps provenance.
func listHeartRate(ctx context.Context, q *db.Queries, from, to time.Time) ([]HeartRateSample, error) {
	rows, err := q.ListHeartRateSamples(ctx, db.ListHeartRateSamplesParams{
		FromAt: tsAt(from),
		ToAt:   tsAt(to),
	})
	if err != nil {
		return nil, err
	}
	out := make([]HeartRateSample, 0, len(rows))
	for _, r := range rows {
		out = append(out, HeartRateSample{At: r.SampledAt.Time, Source: r.Source, BPM: int(r.Bpm)})
	}
	return out, nil
}

// ListHeartRateSamplesCompact reads source 0x01 only, as the API contract says.
func (s *Store) ListHeartRateSamplesCompact(ctx context.Context, from, to string) ([]CompactDay, error) {
	fromAt, toAt, err := dayRange(from, to)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.HeartRateCompact(ctx, db.HeartRateCompactParams{FromAt: tsAt(fromAt), ToAt: tsAt(toAt)})
	if err != nil {
		return nil, err
	}
	out := make([]CompactDay, 0, len(rows))
	for _, r := range rows {
		out = append(out, CompactDay{
			DayKey:    r.DayKey.Time.Format(dateLayout),
			StartTime: r.StartTime.Time,
			Offsets:   r.Offsets,
			Values:    r.Values,
		})
	}
	return out, nil
}
