package store

import (
	"context"
	"time"

	"github.com/Anshuman115/heliolytics/internal/store/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// WriteTemperature stores exact centi-degree values. New keys start with
// on_wrist false and on_wrist never changes here; the wear proxy has its own
// maintenance statement, so it is excluded from change detection.
func (s *Store) WriteTemperature(ctx context.Context, tx pgx.Tx, rows []TemperatureSample,
	sessionID string, observedAt time.Time) (Mutation, error) {
	const table = "temperature_samples"
	b := newMutation()
	if err := validateSessionRef(sessionID, table); err != nil {
		return Mutation{}, err
	}
	if err := validateObservedAt(observedAt, table); err != nil {
		return Mutation{}, err
	}
	observedAt = normInstant(observedAt)

	normalized := make([]TemperatureSample, 0, len(rows))
	for _, r := range rows {
		if r.CentiC < 0 || r.CentiC > 5000 {
			return Mutation{}, invalidRow(table, "celsius out of range")
		}
		if err := validateInstant(r.At, table); err != nil {
			return Mutation{}, err
		}
		r.At = normInstant(r.At)
		normalized = append(normalized, r)
	}
	normalized, err := dedupeSamples(table, normalized,
		func(r TemperatureSample) sampleKey { return keyOf(r.At, "") },
		func(r TemperatureSample) any { return r.CentiC })
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
		stored, err := q.ListTemperatureSamplesByKeys(ctx, keys)
		if err != nil {
			return Mutation{}, err
		}
		type storedRow struct {
			centi     int
			observed  time.Time
			sessionID string
		}
		prev := make(map[sampleKey]storedRow, len(stored))
		for _, r := range stored {
			prev[keyOf(r.SampledAt.Time, "")] = storedRow{
				centi:     int(r.CentiC),
				observed:  r.ObservedAt.Time,
				sessionID: r.SessionID,
			}
		}

		params := db.UpsertTemperatureSamplesParams{
			SampledAt:  make([]pgtype.Timestamptz, 0, len(chunk)),
			Celsius:    make([]pgtype.Numeric, 0, len(chunk)),
			SessionID:  make([]string, 0, len(chunk)),
			ObservedAt: make([]pgtype.Timestamptz, 0, len(chunk)),
		}
		for _, r := range chunk {
			old, exists := prev[keyOf(r.At, "")]
			if exists && !tupleGE(observedAt, sessionID, old.observed, old.sessionID) {
				continue
			}
			if !exists || old.centi != r.CentiC {
				b.addPoint(r.At)
			}
			celsius, err := numericFromFloat(float64(r.CentiC) / 100)
			if err != nil {
				return Mutation{}, invalidRow(table, "celsius not representable")
			}
			params.SampledAt = append(params.SampledAt, tsAt(r.At))
			params.Celsius = append(params.Celsius, celsius)
			params.SessionID = append(params.SessionID, sessionID)
			params.ObservedAt = append(params.ObservedAt, tsAt(observedAt))
		}
		if len(params.SampledAt) == 0 {
			continue
		}
		if err := q.UpsertTemperatureSamples(ctx, params); err != nil {
			return Mutation{}, err
		}
	}
	return b.result(), nil
}

func listTemperature(ctx context.Context, q *db.Queries, from, to time.Time) ([]TemperatureReadRow, error) {
	rows, err := q.ListTemperatureSamples(ctx, db.ListTemperatureSamplesParams{
		FromAt: tsAt(from),
		ToAt:   tsAt(to),
	})
	if err != nil {
		return nil, err
	}
	out := make([]TemperatureReadRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, TemperatureReadRow{
			At:      r.SampledAt.Time,
			CentiC:  int(r.CentiC),
			OnWrist: r.OnWrist,
		})
	}
	return out, nil
}

func (s *Store) ListTemperatureCompact(ctx context.Context, from, to string) ([]CompactDay, error) {
	fromAt, toAt, err := dayRange(from, to)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.TemperatureCompact(ctx, db.TemperatureCompactParams{FromAt: tsAt(fromAt), ToAt: tsAt(toAt)})
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

func (v *TxView) RecomputeTemperatureWearProxy(ctx context.Context, from, to time.Time) (Mutation, error) {
	b := newMutation()
	rows, err := v.q.RefreshTemperatureWearProxy(ctx, db.RefreshTemperatureWearProxyParams{
		FromAt: tsAt(from),
		ToAt:   tsAt(to),
	})
	if err != nil {
		return Mutation{}, err
	}
	for _, r := range rows {
		b.addPoint(r.SampledAt.Time)
	}
	return b.result(), nil
}
