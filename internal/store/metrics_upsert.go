package store

import (
	"context"

	"github.com/heliolytics/api/internal/store/db"
)

func (s *Store) UpsertDayMetrics(ctx context.Context, days []DayMetric) error {
	for _, d := range days {
		if err := validateDayMetric(d); err != nil {
			return err
		}
		temp, err := numericPtr(d.TempAvgC)
		if err != nil {
			return err
		}
		if err := s.q.UpsertDayMetric(ctx, db.UpsertDayMetricParams{
			DayKey:               d.DayKey,
			Steps:                int32(d.Steps),
			PaiScore:             int4Ptr(d.PaiScore),
			Readiness:            int4Ptr(d.Readiness),
			Spo2Avg:              int4Ptr(d.Spo2Avg),
			HrvRmssd:             int4Ptr(d.HrvRmssd),
			RestingHr:            int4Ptr(d.RestingHr),
			MaxHr:                int4Ptr(d.MaxHr),
			RespRateAvg:          int4Ptr(d.RespRateAvg),
			StressAvg:            int4Ptr(d.StressAvg),
			SleepScore:           int4Ptr(d.SleepScore),
			SleepMins:            int4Ptr(d.SleepMins),
			SleepDeepMins:        int4Ptr(d.SleepDeepMins),
			SleepRemMins:         int4Ptr(d.SleepRemMins),
			SleepLightMins:       int4Ptr(d.SleepLightMins),
			TempAvgC:             temp,
			NapCount:             int32(d.NapCount),
			WorkoutCount:         int32(d.WorkoutCount),
			ActivitySessionCount: int32(d.ActivitySessionCount),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ReplaceSleepSessions(ctx context.Context, sid string, rows []SleepRow) error {
	if sid == "" {
		return errRequired("sleep_sessions.sync_session_id")
	}
	if err := s.q.DeleteSleepBySession(ctx, sid); err != nil {
		return err
	}
	for _, r := range rows {
		r.SyncSessionID = sid
		if err := validateSleepRow(r); err != nil {
			return err
		}
		ts, err := timestamptzRequired(r.StartedAt, "sleep_sessions.started_at")
		if err != nil {
			return err
		}
		stagesJSON, err := encodeSleepStages(r.Stages)
		if err != nil {
			return err
		}
		if err := s.q.UpsertSleepSession(ctx, db.UpsertSleepSessionParams{
			SyncSessionID: sid,
			DayKey:        r.DayKey,
			StartedAt:     ts,
			Score:         int32(r.Score),
			TotalMins:     int32(r.TotalMins),
			DeepMins:      int32(r.DeepMins),
			RemMins:       int32(r.RemMins),
			LightMins:     int32(r.LightMins),
			WakeMins:      int32(r.WakeMins),
			IsNap:         r.IsNap,
			StagesJson:    stagesJSON,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ReplaceWorkouts(ctx context.Context, sid string, rows []WorkoutRow) error {
	if sid == "" {
		return errRequired("workouts.sync_session_id")
	}
	if err := s.q.DeleteWorkoutsBySession(ctx, sid); err != nil {
		return err
	}
	for _, r := range rows {
		r.SyncSessionID = sid
		if err := validateWorkoutRow(r); err != nil {
			return err
		}
		ts, err := timestamptzRequired(r.StartedAt, "workouts.started_at")
		if err != nil {
			return err
		}
		if err := s.q.UpsertWorkout(ctx, db.UpsertWorkoutParams{
			SyncSessionID: sid,
			DayKey:        r.DayKey,
			StartedAt:     ts,
			SportType:     int32(r.SportType),
			SportName:     textPtr(r.SportName),
			DurationSec:   int32(r.DurationSec),
			Calories:      int4Ptr(r.Calories),
			AvgHr:         int4Ptr(r.AvgHr),
			MaxHr:         int4Ptr(r.MaxHr),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ReplaceActivitySessions(ctx context.Context, sid string, rows []ActivitySessionRow) error {
	if sid == "" {
		return errRequired("activity_sessions.sync_session_id")
	}
	if err := s.q.DeleteActivitySessionsBySession(ctx, sid); err != nil {
		return err
	}
	for _, r := range rows {
		r.SyncSessionID = sid
		if err := validateActivityRow(r); err != nil {
			return err
		}
		ts, err := timestamptzRequired(r.StartedAt, "activity_sessions.started_at")
		if err != nil {
			return err
		}
		if err := s.q.UpsertActivitySession(ctx, db.UpsertActivitySessionParams{
			SyncSessionID: sid,
			DayKey:        r.DayKey,
			StartedAt:     ts,
			SportType:     int32(r.SportType),
			SportName:     textPtr(r.SportName),
			DurationSec:   int32(r.DurationSec),
			Calories:      int4Ptr(r.Calories),
			AvgHr:         int4Ptr(r.AvgHr),
			MaxHr:         int4Ptr(r.MaxHr),
		}); err != nil {
			return err
		}
	}
	return nil
}
