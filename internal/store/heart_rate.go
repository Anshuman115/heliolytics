package store

import (
	"context"
	"time"

	"github.com/heliolytics/api/internal/store/db"
	"github.com/jackc/pgx/v5"
)

type HeartRateSample struct {
	DayKey     string
	SampledAt  time.Time
	Bpm        int
	SourceType string
}

// Mirrors sql/queries/heart_rate_samples.sql:UpsertHeartRateSample. Inline so
// rows can be pipelined as one batch; sqlc only generates a per-row Exec.
const upsertHeartRateSQL = `
	INSERT INTO heart_rate_samples (sampled_at, day_key, bpm, source_session_id, source_type)
	VALUES ($1, $2, $3, $4, $5)
	ON CONFLICT (sampled_at) DO UPDATE SET
	  bpm = EXCLUDED.bpm,
	  day_key = EXCLUDED.day_key,
	  source_session_id = EXCLUDED.source_session_id,
	  source_type = EXCLUDED.source_type`

func (s *Store) UpsertHeartRateSamples(ctx context.Context, sid string, pts []HeartRateSample) error {
	if sid == "" {
		return errRequired("heart_rate_samples.source_session_id")
	}
	rows := make([]queuedRow, 0, len(pts))
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
		rows = append(rows, queuedRow{ts, day, int16(p.Bpm), sid, p.SourceType})
	}
	return s.execBatch(ctx, upsertHeartRateSQL, rows)
}

// UpsertHeartRateSamplesTx is UpsertHeartRateSamples run against an
// already-open transaction, for use inside Store.WithTx.
func (s *Store) UpsertHeartRateSamplesTx(ctx context.Context, tx pgx.Tx, sid string, pts []HeartRateSample) error {
	if sid == "" {
		return errRequired("heart_rate_samples.source_session_id")
	}
	rows := make([]queuedRow, 0, len(pts))
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
		rows = append(rows, queuedRow{ts, day, int16(p.Bpm), sid, p.SourceType})
	}
	return execBatchTx(ctx, tx, upsertHeartRateSQL, rows)
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

type HeartRateCompactDay struct {
	DayKey    string    `json:"dayKey"`
	StartTime time.Time `json:"startTime"`
	Offsets   []int32   `json:"offsets"`
	Values    []int32   `json:"values"`
}

func (s *Store) ListHeartRateSamplesCompact(ctx context.Context, from, to string) ([]HeartRateCompactDay, error) {
	fromD, err := dateKey(from)
	if err != nil {
		return nil, err
	}
	toD, err := dateKey(to)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListHeartRateSamplesCompact(ctx, db.ListHeartRateSamplesCompactParams{DayKey: fromD, DayKey_2: toD})
	if err != nil {
		return nil, err
	}
	out := make([]HeartRateCompactDay, len(rows))
	for i, r := range rows {
		out[i] = HeartRateCompactDay{
			DayKey:    dateKeyString(r.DayKey),
			StartTime: r.StartTime.Time,
			Offsets:   r.Offsets,
			Values:    r.Values,
		}
	}
	return out, nil
}
