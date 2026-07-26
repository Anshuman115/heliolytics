-- name: UpsertWorkout :exec
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
  updated_at = NOW();

-- name: ListWorkouts :many
SELECT day_key, started_at, sport_type, sport_name,
       duration_sec, calories, avg_hr, max_hr
FROM workouts
WHERE day_key >= $1 AND day_key <= $2
ORDER BY started_at DESC;
