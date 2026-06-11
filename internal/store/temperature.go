package store

import (
	"context"
	"time"

	"github.com/heliolytics/api/internal/store/db"
)

type TempPoint struct {
	DayKey    string
	SampledAt time.Time
	Celsius   float64
}

func (s *Store) ReplaceTemperature(ctx context.Context, sid string, pts []TempPoint) error {
	if sid == "" {
		return errRequired("temperature_samples.sync_session_id")
	}
	if err := s.q.DeleteTemperatureBySession(ctx, sid); err != nil {
		return err
	}
	for _, p := range pts {
		if err := validateTempPoint(p); err != nil {
			return err
		}
		ts, err := timestamptzRequired(p.SampledAt, "temperature_samples.sampled_at")
		if err != nil {
			return err
		}
		c, err := numericFromFloat(p.Celsius)
		if err != nil {
			return err
		}
		if err := s.q.InsertTemperatureSample(ctx, db.InsertTemperatureSampleParams{
			SyncSessionID: sid,
			DayKey:        p.DayKey,
			SampledAt:     ts,
			Celsius:       c,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ListTemperature(ctx context.Context, from, to string) ([]TempPoint, error) {
	rows, err := s.q.ListTemperature(ctx, db.ListTemperatureParams{DayKey: from, DayKey_2: to})
	if err != nil {
		return nil, err
	}
	out := make([]TempPoint, 0, len(rows))
	for _, r := range rows {
		c := numericVal(r.Celsius)
		if c == nil {
			continue
		}
		out = append(out, TempPoint{
			DayKey: r.DayKey, SampledAt: r.SampledAt.Time, Celsius: *c,
		})
	}
	return out, nil
}
