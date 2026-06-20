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

func (s *Store) UpsertHealthSamples(ctx context.Context, sid string, pts []HealthSample) error {
	if sid == "" {
		return errRequired("health_samples.source_session_id")
	}
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
		if err := s.q.UpsertHealthSample(ctx, db.UpsertHealthSampleParams{
			Metric: p.Metric, DayKey: day, SampledAt: ts, Value: val, SourceSessionID: sid,
		}); err != nil {
			return err
		}
	}
	return nil
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
