-- name: UpsertWorkout :exec
INSERT INTO workouts (source_session_id, day_key, started_at, sport_type,
  sport_name, duration_sec, calories, avg_hr, max_hr)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (day_key, started_at) DO UPDATE SET
  source_session_id = EXCLUDED.source_session_id,
  sport_name = EXCLUDED.sport_name,
  duration_sec = EXCLUDED.duration_sec,
  calories = EXCLUDED.calories,
  avg_hr = EXCLUDED.avg_hr,
  max_hr = EXCLUDED.max_hr,
  updated_at = NOW();

-- name: ListWorkouts :many
SELECT day_key, started_at, sport_type, sport_name,
       duration_sec, calories, avg_hr, max_hr
FROM workouts
WHERE day_key >= $1 AND day_key <= $2
ORDER BY started_at DESC;
