-- name: InsertSleepObservations :exec
INSERT INTO sleep_observations (session_id, started_at, ended_at, day_key, is_nap,
  score, total_mins, deep_mins, rem_mins, light_mins, wake_mins, stages,
  stages_truncated, observed_at)
SELECT v.session_id, v.started_at, v.ended_at, v.day_key, v.is_nap,
       v.score::smallint, v.total_mins, v.deep_mins, v.rem_mins, v.light_mins,
       v.wake_mins,
       v.stages, v.stages_truncated, v.observed_at
FROM ROWS FROM (
  unnest(sqlc.arg(session_id)::text[]),
  unnest(sqlc.arg(started_at)::timestamptz[]),
  unnest(sqlc.arg(ended_at)::timestamptz[]),
  unnest(sqlc.arg(day_key)::date[]),
  unnest(sqlc.arg(is_nap)::boolean[]),
  unnest(sqlc.arg(score)::numeric[]),
  unnest(sqlc.arg(total_mins)::smallint[]),
  unnest(sqlc.arg(deep_mins)::smallint[]),
  unnest(sqlc.arg(rem_mins)::smallint[]),
  unnest(sqlc.arg(light_mins)::smallint[]),
  unnest(sqlc.arg(wake_mins)::smallint[]),
  unnest(sqlc.arg(stages)::jsonb[]),
  unnest(sqlc.arg(stages_truncated)::boolean[]),
  unnest(sqlc.arg(observed_at)::timestamptz[])
) AS v(session_id, started_at, ended_at, day_key, is_nap, score, total_mins,
       deep_mins, rem_mins, light_mins, wake_mins, stages, stages_truncated,
       observed_at)
ON CONFLICT (session_id, started_at) DO NOTHING;

-- name: ListSleepObservationsBySession :many
SELECT session_id, started_at, ended_at, day_key, is_nap, score, total_mins,
       deep_mins, rem_mins, light_mins, wake_mins, stages, stages_truncated,
       observed_at
FROM sleep_observations
WHERE session_id = sqlc.arg(session_id)
ORDER BY started_at ASC;

-- name: ListSleepObservationsOverlapping :many
SELECT session_id, started_at, ended_at, day_key, is_nap, score, total_mins,
       deep_mins, rem_mins, light_mins, wake_mins, stages, stages_truncated,
       observed_at
FROM sleep_observations
WHERE started_at < sqlc.arg(to_at) AND ended_at > sqlc.arg(from_at)
ORDER BY started_at ASC;
