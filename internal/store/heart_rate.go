package store

import (
	"context"
	"time"

	"github.com/heliolytics/api/internal/store/db"
)

type HeartRateSample struct {
	DayKey    string
	SampledAt time.Time
	Bpm       int
}

func (s *Store) UpsertHeartRateSamples(ctx context.Context, sid string, pts []HeartRateSample) error {
	if sid == "" {
		return errRequired("heart_rate_samples.source_session_id")
	}
	for _, p := range pts {
		if err := validateHeartRateSample(p); err != nil {
			return err
		}
		day, err := dateKey(p.DayKey)
		if err != nil {
			return err
		}
		ts, err := timestamptzRequired(p.SampledAt, "heart_rate_samples.sampled_at")
		if err != nil {
			return err
		}
		if err := s.q.UpsertHeartRateSample(ctx, db.UpsertHeartRateSampleParams{
			SampledAt: ts, DayKey: day, Bpm: int16(p.Bpm), SourceSessionID: sid,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ListHeartRateSamples(ctx context.Context, from, to string) ([]HeartRateSample, error) {
	fromD, err := dateKey(from)
	if err != nil {
		return nil, err
	}
	toD, err := dateKey(to)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListHeartRateSamples(ctx, db.ListHeartRateSamplesParams{DayKey: fromD, DayKey_2: toD})
	if err != nil {
		return nil, err
	}
	out := make([]HeartRateSample, 0, len(rows))
	for _, r := range rows {
		out = append(out, HeartRateSample{
			DayKey: dateKeyString(r.DayKey), SampledAt: r.SampledAt.Time, Bpm: int(r.Bpm),
		})
	}
	return out, nil
}
