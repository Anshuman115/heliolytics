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

func (s *Store) ReplaceHealthSamples(ctx context.Context, sid string, pts []HealthSample) error {
	if sid == "" {
		return errRequired("health_samples.sync_session_id")
	}
	if err := s.q.DeleteHealthSamplesBySession(ctx, sid); err != nil {
		return err
	}
	for _, p := range pts {
		if err := validateHealthSample(sid, p); err != nil {
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
		if err := s.q.InsertHealthSample(ctx, db.InsertHealthSampleParams{
			SyncSessionID: sid,
			Metric:        p.Metric,
			DayKey:        p.DayKey,
			SampledAt:     ts,
			Value:         val,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ListHealthSamples(ctx context.Context, from, to string) ([]HealthSample, error) {
	rows, err := s.q.ListHealthSamples(ctx, db.ListHealthSamplesParams{DayKey: from, DayKey_2: to})
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
			Metric: r.Metric, DayKey: r.DayKey,
			SampledAt: r.SampledAt.Time, Value: *v,
		})
	}
	return out, nil
}
