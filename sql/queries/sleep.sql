-- name: ListSleepSessions :many
SELECT started_at, ended_at, day_key, is_nap, score, total_mins, deep_mins,
       rem_mins, light_mins, wake_mins, stages, stages_truncated
FROM sleep_sessions
WHERE day_key >= sqlc.arg(from_day) AND day_key <= sqlc.arg(to_day)
ORDER BY day_key ASC, started_at ASC;

-- name: ListSleepSessionsOverlapping :many
SELECT started_at, ended_at, day_key, is_nap, score, total_mins, deep_mins,
       rem_mins, light_mins, wake_mins, stages, stages_truncated, observed_at,
       session_id
FROM sleep_sessions
WHERE started_at < sqlc.arg(to_at) AND ended_at > sqlc.arg(from_at)
ORDER BY started_at ASC;

-- name: UpsertSleepSession :exec
INSERT INTO sleep_sessions (started_at, ended_at, day_key, is_nap, score,
  total_mins, deep_mins, rem_mins, light_mins, wake_mins, stages,
  stages_truncated, observed_at, session_id)
VALUES (
  sqlc.arg(started_at), sqlc.arg(ended_at), sqlc.arg(day_key), sqlc.arg(is_nap),
  sqlc.arg(score), sqlc.arg(total_mins), sqlc.arg(deep_mins), sqlc.arg(rem_mins),
  sqlc.arg(light_mins), sqlc.arg(wake_mins), sqlc.arg(stages),
  sqlc.arg(stages_truncated), sqlc.arg(observed_at), sqlc.arg(session_id)
)
ON CONFLICT (started_at) DO UPDATE SET
  ended_at = EXCLUDED.ended_at,
  day_key = EXCLUDED.day_key,
  is_nap = EXCLUDED.is_nap,
  score = EXCLUDED.score,
  total_mins = EXCLUDED.total_mins,
  deep_mins = EXCLUDED.deep_mins,
  rem_mins = EXCLUDED.rem_mins,
  light_mins = EXCLUDED.light_mins,
  wake_mins = EXCLUDED.wake_mins,
  stages = EXCLUDED.stages,
  stages_truncated = EXCLUDED.stages_truncated,
  observed_at = EXCLUDED.observed_at,
  session_id = EXCLUDED.session_id;

-- name: DeleteSleepSessionsIn :exec
DELETE FROM sleep_sessions WHERE started_at = ANY(sqlc.arg(keys)::timestamptz[]);

-- name: MaxSleepEndAt :one
SELECT ended_at FROM sleep_sessions
WHERE (sqlc.narg(session_id)::text IS NULL OR session_id = sqlc.narg(session_id)::text)
ORDER BY ended_at DESC
LIMIT 1;

-- name: SleepDayBounds :many
(SELECT day_key FROM sleep_sessions ORDER BY day_key ASC LIMIT 1)
UNION ALL
(SELECT day_key FROM sleep_sessions ORDER BY day_key DESC LIMIT 1);
