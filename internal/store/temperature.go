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

func (s *Store) UpsertTemperature(ctx context.Context, sid string, pts []TempPoint) error {
	if sid == "" {
		return errRequired("temperature_samples.source_session_id")
	}
	for _, p := range pts {
		if err := validateTempPoint(p); err != nil {
			return err
		}
		day, err := dateKey(p.DayKey)
		if err != nil {
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
		if err := s.q.UpsertTemperatureSample(ctx, db.UpsertTemperatureSampleParams{
			SampledAt: ts, DayKey: day, Celsius: c, SourceSessionID: sid,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ListTemperature(ctx context.Context, from, to string) ([]TempPoint, error) {
	fromD, err := dateKey(from)
	if err != nil {
		return nil, err
	}
	toD, err := dateKey(to)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListTemperature(ctx, db.ListTemperatureParams{DayKey: fromD, DayKey_2: toD})
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
			DayKey: dateKeyString(r.DayKey), SampledAt: r.SampledAt.Time, Celsius: *c,
		})
	}
	return out, nil
}

type TempCompactDay struct {
	DayKey    string    `json:"dayKey"`
	StartTime time.Time `json:"startTime"`
	Offsets   []int32   `json:"offsets"`
	Values    []float64 `json:"values"`
}

func (s *Store) ListTemperatureCompact(ctx context.Context, from, to string) ([]TempCompactDay, error) {
	fromD, err := dateKey(from)
	if err != nil {
		return nil, err
	}
	toD, err := dateKey(to)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListTemperatureCompact(ctx, db.ListTemperatureCompactParams{DayKey: fromD, DayKey_2: toD})
	if err != nil {
		return nil, err
	}
	out := make([]TempCompactDay, len(rows))
	for i, r := range rows {
		out[i] = TempCompactDay{
			DayKey:    dateKeyString(r.DayKey),
			StartTime: r.StartTime.Time,
			Offsets:   r.Offsets,
			Values:    r.Values,
		}
	}
	return out, nil
}
