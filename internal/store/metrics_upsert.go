package store

import (
	"context"

	"github.com/heliolytics/api/internal/store/db"
	"github.com/jackc/pgx/v5"
)

// SQL consts mirroring sql/queries/sleep.sql, workouts.sql, and
// activity_sessions.sql's Upsert* statements. Inlined here (rather than
// reused from sqlc's per-row Exec) so rows can be pipelined as one batch via
// execBatchTx, matching the pattern used by every other bulk upsert in this
// package.
const upsertSleepSessionSQL = `
INSERT INTO sleep_sessions (source_session_id, day_key, started_at, score,
  total_mins, deep_mins, rem_mins, light_mins, wake_mins, is_nap, stages_json)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
ON CONFLICT (started_at, day_key) DO UPDATE SET
  source_session_id = EXCLUDED.source_session_id,
  score = EXCLUDED.score,
  total_mins = EXCLUDED.total_mins,
  deep_mins = EXCLUDED.deep_mins,
  rem_mins = EXCLUDED.rem_mins,
  light_mins = EXCLUDED.light_mins,
  wake_mins = EXCLUDED.wake_mins,
  is_nap = EXCLUDED.is_nap,
  stages_json = EXCLUDED.stages_json,
  updated_at = NOW()`

const upsertWorkoutSQL = `
INSERT INTO workouts (source_session_id, day_key, started_at, sport_type,
  sport_name, duration_sec, calories, avg_hr, max_hr, has_summary, has_detail)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
ON CONFLICT (day_key, started_at) DO UPDATE SET
  source_session_id = EXCLUDED.source_session_id,
  sport_type = CASE WHEN EXCLUDED.has_summary THEN EXCLUDED.sport_type ELSE workouts.sport_type END,
  sport_name = CASE WHEN EXCLUDED.has_summary THEN EXCLUDED.sport_name ELSE workouts.sport_name END,
  duration_sec = CASE WHEN EXCLUDED.has_summary OR NOT workouts.has_summary THEN EXCLUDED.duration_sec ELSE workouts.duration_sec END,
  calories = COALESCE(EXCLUDED.calories, workouts.calories),
  avg_hr = COALESCE(EXCLUDED.avg_hr, workouts.avg_hr),
  max_hr = COALESCE(EXCLUDED.max_hr, workouts.max_hr),
  has_summary = workouts.has_summary OR EXCLUDED.has_summary,
  has_detail = workouts.has_detail OR EXCLUDED.has_detail,
  updated_at = NOW()`

const upsertActivitySessionSQL = `
INSERT INTO activity_sessions (source_session_id, day_key, started_at, sport_type,
  sport_name, duration_sec, calories, avg_hr, max_hr)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (day_key, started_at) DO UPDATE SET
  source_session_id = EXCLUDED.source_session_id,
  sport_name = EXCLUDED.sport_name,
  duration_sec = EXCLUDED.duration_sec,
  calories = EXCLUDED.calories,
  avg_hr = EXCLUDED.avg_hr,
  max_hr = EXCLUDED.max_hr,
  updated_at = NOW()`

func (s *Store) UpsertSleepSessions(ctx context.Context, sid string, rows []SleepRow) error {
	if sid == "" {
		return errRequired("sleep_sessions.source_session_id")
	}
	for _, r := range rows {
		if err := validateSleepRow(r); err != nil {
			return err
		}
		day, err := dateKey(r.DayKey)
		if err != nil {
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
			SourceSessionID: sid, DayKey: day, StartedAt: ts,
			Score: int32(r.Score), TotalMins: int32(r.TotalMins),
			DeepMins: int32(r.DeepMins), RemMins: int32(r.RemMins),
			LightMins: int32(r.LightMins), WakeMins: int32(r.WakeMins),
			IsNap: r.IsNap, StagesJson: stagesJSON,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) UpsertWorkouts(ctx context.Context, sid string, rows []WorkoutRow) error {
	if sid == "" {
		return errRequired("workouts.source_session_id")
	}
	for _, r := range rows {
		if err := validateWorkoutRow(r); err != nil {
			return err
		}
		day, err := dateKey(r.DayKey)
		if err != nil {
			return err
		}
		ts, err := timestamptzRequired(r.StartedAt, "workouts.started_at")
		if err != nil {
			return err
		}
		if err := s.q.UpsertWorkout(ctx, db.UpsertWorkoutParams{
			SourceSessionID: sid, DayKey: day, StartedAt: ts,
			SportType: int32(r.SportType), SportName: textPtr(r.SportName),
			DurationSec: int32(r.DurationSec), Calories: int4Ptr(r.Calories),
			AvgHr: int4Ptr(r.AvgHr), MaxHr: int4Ptr(r.MaxHr),
			HasSummary: r.HasSummary, HasDetail: r.HasDetail,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) UpsertActivitySessions(ctx context.Context, sid string, rows []ActivitySessionRow) error {
	if sid == "" {
		return errRequired("activity_sessions.source_session_id")
	}
	for _, r := range rows {
		if err := validateActivityRow(r); err != nil {
			return err
		}
		day, err := dateKey(r.DayKey)
		if err != nil {
			return err
		}
		ts, err := timestamptzRequired(r.StartedAt, "activity_sessions.started_at")
		if err != nil {
			return err
		}
		if err := s.q.UpsertActivitySession(ctx, db.UpsertActivitySessionParams{
			SourceSessionID: sid, DayKey: day, StartedAt: ts,
			SportType: int32(r.SportType), SportName: textPtr(r.SportName),
			DurationSec: int32(r.DurationSec), Calories: int4Ptr(r.Calories),
			AvgHr: int4Ptr(r.AvgHr), MaxHr: int4Ptr(r.MaxHr),
		}); err != nil {
			return err
		}
	}
	return nil
}

// UpsertSleepSessionsTx is UpsertSleepSessions run against an already-open
// transaction, for use inside Store.WithTx. Builds one []queuedRow and
// writes via execBatchTx instead of looping per-row tx.Exec, per the
// project's uniform-batching rule for multi-row writes.
func (s *Store) UpsertSleepSessionsTx(ctx context.Context, tx pgx.Tx, sid string, rows []SleepRow) error {
	if sid == "" {
		return errRequired("sleep_sessions.source_session_id")
	}
	qrows := make([]queuedRow, 0, len(rows))
	for _, r := range rows {
		if err := validateSleepRow(r); err != nil {
			return err
		}
		day, err := dateKey(r.DayKey)
		if err != nil {
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
		qrows = append(qrows, queuedRow{
			sid, day, ts, int32(r.Score), int32(r.TotalMins),
			int32(r.DeepMins), int32(r.RemMins), int32(r.LightMins),
			int32(r.WakeMins), r.IsNap, stagesJSON,
		})
	}
	return execBatchTx(ctx, tx, upsertSleepSessionSQL, qrows)
}

// UpsertWorkoutsTx is UpsertWorkouts run against an already-open transaction,
// for use inside Store.WithTx. Builds one []queuedRow and writes via
// execBatchTx instead of looping per-row tx.Exec, per the project's
// uniform-batching rule for multi-row writes.
func (s *Store) UpsertWorkoutsTx(ctx context.Context, tx pgx.Tx, sid string, rows []WorkoutRow) error {
	if sid == "" {
		return errRequired("workouts.source_session_id")
	}
	qrows := make([]queuedRow, 0, len(rows))
	for _, r := range rows {
		if err := validateWorkoutRow(r); err != nil {
			return err
		}
		day, err := dateKey(r.DayKey)
		if err != nil {
			return err
		}
		ts, err := timestamptzRequired(r.StartedAt, "workouts.started_at")
		if err != nil {
			return err
		}
		qrows = append(qrows, queuedRow{
			sid, day, ts, int32(r.SportType), textPtr(r.SportName),
			int32(r.DurationSec), int4Ptr(r.Calories), int4Ptr(r.AvgHr), int4Ptr(r.MaxHr),
			r.HasSummary, r.HasDetail,
		})
	}
	return execBatchTx(ctx, tx, upsertWorkoutSQL, qrows)
}

// UpsertActivitySessionsTx is UpsertActivitySessions run against an
// already-open transaction, for use inside Store.WithTx. Builds one
// []queuedRow and writes via execBatchTx instead of looping per-row
// tx.Exec, per the project's uniform-batching rule for multi-row writes.
func (s *Store) UpsertActivitySessionsTx(ctx context.Context, tx pgx.Tx, sid string, rows []ActivitySessionRow) error {
	if sid == "" {
		return errRequired("activity_sessions.source_session_id")
	}
	qrows := make([]queuedRow, 0, len(rows))
	for _, r := range rows {
		if err := validateActivityRow(r); err != nil {
			return err
		}
		day, err := dateKey(r.DayKey)
		if err != nil {
			return err
		}
		ts, err := timestamptzRequired(r.StartedAt, "activity_sessions.started_at")
		if err != nil {
			return err
		}
		qrows = append(qrows, queuedRow{
			sid, day, ts, int32(r.SportType), textPtr(r.SportName),
			int32(r.DurationSec), int4Ptr(r.Calories), int4Ptr(r.AvgHr), int4Ptr(r.MaxHr),
		})
	}
	return execBatchTx(ctx, tx, upsertActivitySessionSQL, qrows)
}
