package store

import (
	"context"
	"time"

	"github.com/heliolytics/api/internal/store/db"
)

func (s *Store) ListDays(ctx context.Context, from, to string) ([]DayMetric, error) {
	rows, err := s.q.ListDays(ctx, db.ListDaysParams{DayKey: from, DayKey_2: to})
	if err != nil {
		return nil, err
	}
	out := make([]DayMetric, 0, len(rows))
	for _, r := range rows {
		out = append(out, dayMetricFromDB(r))
	}
	return out, nil
}

func (s *Store) ListSleep(ctx context.Context, from, to string) ([]SleepMetric, error) {
	rows, err := s.q.ListSleep(ctx, db.ListSleepParams{DayKey: from, DayKey_2: to})
	if err != nil {
		return nil, err
	}
	out := make([]SleepMetric, 0, len(rows))
	for _, r := range rows {
		out = append(out, SleepMetric{
			DayKey: r.DayKey, StartedAt: r.StartedAt.Time, Score: int(r.Score),
			TotalMins: int(r.TotalMins), DeepMins: int(r.DeepMins),
			RemMins: int(r.RemMins), LightMins: int(r.LightMins),
			WakeMins: int(r.WakeMins), IsNap: r.IsNap,
			Stages: decodeSleepStages(r.StagesJson),
		})
	}
	return out, nil
}

func (s *Store) ListWorkouts(ctx context.Context, from, to string) ([]WorkoutRow, error) {
	rows, err := s.q.ListWorkouts(ctx, db.ListWorkoutsParams{DayKey: from, DayKey_2: to})
	if err != nil {
		return nil, err
	}
	out := make([]WorkoutRow, 0, len(rows))
	for _, r := range rows {
		name := ""
		if r.SportName.Valid {
			name = r.SportName.String
		}
		out = append(out, WorkoutRow{
			SyncSessionID: r.SyncSessionID, DayKey: r.DayKey,
			StartedAt: r.StartedAt.Time, SportType: int(r.SportType),
			SportName: name, DurationSec: int(r.DurationSec),
			Calories: int4Val(r.Calories), AvgHr: int4Val(r.AvgHr),
			MaxHr: int4Val(r.MaxHr),
		})
	}
	return out, nil
}

func (s *Store) ListActivitySessions(ctx context.Context, from, to string) ([]ActivitySessionRow, error) {
	rows, err := s.q.ListActivitySessions(ctx, db.ListActivitySessionsParams{DayKey: from, DayKey_2: to})
	if err != nil {
		return nil, err
	}
	out := make([]ActivitySessionRow, 0, len(rows))
	for _, r := range rows {
		name := ""
		if r.SportName.Valid {
			name = r.SportName.String
		}
		out = append(out, ActivitySessionRow{
			SyncSessionID: r.SyncSessionID, DayKey: r.DayKey,
			StartedAt: r.StartedAt.Time, SportType: int(r.SportType),
			SportName: name, DurationSec: int(r.DurationSec),
			Calories: int4Val(r.Calories), AvgHr: int4Val(r.AvgHr),
			MaxHr: int4Val(r.MaxHr),
		})
	}
	return out, nil
}

func dayMetricFromDB(r db.DailyMetric) DayMetric {
	updated := time.Time{}
	if r.UpdatedAt.Valid {
		updated = r.UpdatedAt.Time
	}
	return DayMetric{
		DayKey: r.DayKey, Steps: int(r.Steps),
		PaiScore: int4Val(r.PaiScore), Readiness: int4Val(r.Readiness),
		Spo2Avg: int4Val(r.Spo2Avg), HrvRmssd: int4Val(r.HrvRmssd),
		RestingHr: int4Val(r.RestingHr), MaxHr: int4Val(r.MaxHr),
		RespRateAvg: int4Val(r.RespRateAvg), StressAvg: int4Val(r.StressAvg),
		SleepScore: int4Val(r.SleepScore), SleepMins: int4Val(r.SleepMins),
		SleepDeepMins: int4Val(r.SleepDeepMins), SleepRemMins: int4Val(r.SleepRemMins),
		SleepLightMins: int4Val(r.SleepLightMins), TempAvgC: numericVal(r.TempAvgC),
		NapCount: int(r.NapCount), WorkoutCount: int(r.WorkoutCount),
		ActivitySessionCount: int(r.ActivitySessionCount), Updated: updated,
	}
}
