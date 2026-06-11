-- name: DeleteSleepBySession :exec
DELETE FROM sleep_sessions WHERE sync_session_id = $1;

-- name: UpsertSleepSession :exec
INSERT INTO sleep_sessions (sync_session_id, day_key, started_at, score,
  total_mins, deep_mins, rem_mins, light_mins, wake_mins, is_nap, stages_json)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
ON CONFLICT (sync_session_id, started_at) DO UPDATE SET
  score = EXCLUDED.score,
  total_mins = EXCLUDED.total_mins,
  deep_mins = EXCLUDED.deep_mins,
  rem_mins = EXCLUDED.rem_mins,
  light_mins = EXCLUDED.light_mins,
  wake_mins = EXCLUDED.wake_mins,
  is_nap = EXCLUDED.is_nap,
  stages_json = EXCLUDED.stages_json;

-- name: ListSleep :many
SELECT day_key, started_at, score, total_mins, deep_mins, rem_mins, light_mins,
       wake_mins, is_nap, stages_json
FROM sleep_sessions
WHERE day_key >= $1 AND day_key <= $2
ORDER BY started_at DESC;
