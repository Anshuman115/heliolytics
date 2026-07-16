package store

import (
	"context"
	"time"

	"github.com/heliolytics/api/internal/store/db"
)

type HealthSample struct {
	Metric    string
	DayKey    string
	SampledAt time.Time
	Value     float64
}

// Mirrors sql/queries/health_samples.sql:UpsertHealthSample. Kept inline so the
// rows can be pipelined as one batch; sqlc only generates a per-row Exec.
const upsertHealthSampleSQL = `
	INSERT INTO health_samples (metric, day_key, sampled_at, value, source_session_id)
	VALUES ($1, $2, $3, $4, $5)
	ON CONFLICT (metric, sampled_at) DO UPDATE SET
	  value = EXCLUDED.value,
	  day_key = EXCLUDED.day_key,
	  source_session_id = EXCLUDED.source_session_id`

func (s *Store) UpsertHealthSamples(ctx context.Context, sid string, pts []HealthSample) error {
	if sid == "" {
		return errRequired("health_samples.source_session_id")
	}
	// Validate and convert everything up front: a bad row must fail the whole
	// write before any of it reaches the database.
	rows := make([]queuedRow, 0, len(pts))
	for _, p := range pts {
		if err := validateHealthSample(p); err != nil {
			return err
		}
		day, err := dateKey(p.DayKey)
		if err != nil {
			return err
		}
		ts, err := timestamptzRequired(p.SampledAt, "health_samples.sampled_at")
		if err != nil {
			return err
		}
		val, err := numericFromFloat(p.Value)
		if err != nil {
			return err
		}
		rows = append(rows, queuedRow{p.Metric, day, ts, val, sid})
	}
	return s.execBatch(ctx, upsertHealthSampleSQL, rows)
}

func (s *Store) ListHealthSamples(ctx context.Context, from, to string) ([]HealthSample, error) {
	fromD, err := dateKey(from)
	if err != nil {
		return nil, err
	}
	toD, err := dateKey(to)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListHealthSamples(ctx, db.ListHealthSamplesParams{DayKey: fromD, DayKey_2: toD})
	if err != nil {
		return nil, err
	}
	out := make([]HealthSample, 0, len(rows))
	for _, r := range rows {
		v := numericVal(r.Value)
		if v == nil {
			continue
		}
		out = append(out, HealthSample{
			Metric: r.Metric, DayKey: dateKeyString(r.DayKey),
			SampledAt: r.SampledAt.Time, Value: *v,
		})
	}
	return out, nil
}

type HealthCompactDay struct {
	Metric    string    `json:"metric"`
	DayKey    string    `json:"dayKey"`
	StartTime time.Time `json:"startTime"`
	Offsets   []int32   `json:"offsets"`
	Values    []float64 `json:"values"`
}

func (s *Store) ListHealthSamplesCompact(ctx context.Context, from, to string) ([]HealthCompactDay, error) {
	fromD, err := dateKey(from)
	if err != nil {
		return nil, err
	}
	toD, err := dateKey(to)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListHealthSamplesCompact(ctx, db.ListHealthSamplesCompactParams{DayKey: fromD, DayKey_2: toD})
	if err != nil {
		return nil, err
	}
	out := make([]HealthCompactDay, len(rows))
	for i, r := range rows {
		out[i] = HealthCompactDay{
			Metric:    r.Metric,
			DayKey:    dateKeyString(r.DayKey),
			StartTime: r.StartTime.Time,
			Offsets:   r.Offsets,
			Values:    r.Values,
		}
	}
	return out, nil
}
