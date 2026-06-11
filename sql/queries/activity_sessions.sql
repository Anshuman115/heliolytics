-- name: DeleteActivitySessionsBySession :exec
DELETE FROM activity_sessions WHERE sync_session_id = $1;

-- name: UpsertActivitySession :exec
INSERT INTO activity_sessions (sync_session_id, day_key, started_at, sport_type,
  sport_name, duration_sec, calories, avg_hr, max_hr)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (sync_session_id, started_at) DO UPDATE SET
  sport_name = EXCLUDED.sport_name,
  duration_sec = EXCLUDED.duration_sec,
  calories = EXCLUDED.calories,
  avg_hr = EXCLUDED.avg_hr,
  max_hr = EXCLUDED.max_hr;

-- name: ListActivitySessions :many
SELECT sync_session_id, day_key, started_at, sport_type, sport_name,
       duration_sec, calories, avg_hr, max_hr
FROM activity_sessions
WHERE day_key >= $1 AND day_key <= $2
ORDER BY started_at DESC;
