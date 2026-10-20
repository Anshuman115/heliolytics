-- name: ListActivitiesRange :many
SELECT started_at, kind, ended_at, sport_type, sport_name, calories, avg_hr,
       max_hr, min_hr, steps, has_detail
FROM activities
WHERE day_key >= sqlc.arg(from_day) AND day_key <= sqlc.arg(to_day)
ORDER BY started_at ASC, kind COLLATE "C" ASC;

-- name: ListWorkoutSessionsOverlapping :many
SELECT started_at, ended_at, sport_type, sport_name, calories, avg_hr, max_hr,
       min_hr, steps, has_detail, observed_at, session_id
FROM activities
WHERE kind = 'workout'
  AND started_at < sqlc.arg(to_at) AND ended_at > sqlc.arg(from_at)
ORDER BY started_at ASC;

-- name: ListWorkoutIntervalsRange :many
SELECT started_at, ended_at
FROM activities
WHERE kind = 'workout'
  AND started_at < sqlc.arg(to_at) AND ended_at > sqlc.arg(from_at)
ORDER BY started_at ASC;

-- name: ListAutoActivities :many
SELECT started_at, ended_at, sport_type, sport_name, calories, avg_hr, max_hr,
       min_hr, steps, has_detail
FROM activities
WHERE kind = 'auto'
  AND started_at < sqlc.arg(to_at) AND ended_at > sqlc.arg(from_at)
ORDER BY started_at ASC;

-- name: DeleteAutoActivities :exec
DELETE FROM activities
WHERE kind = 'auto'
  AND started_at < sqlc.arg(to_at) AND ended_at > sqlc.arg(from_at);

-- name: InsertAutoActivities :exec
INSERT INTO activities (started_at, kind, ended_at, sport_type, sport_name,
  calories, avg_hr, max_hr, min_hr, steps, has_detail, observed_at, session_id)
SELECT v.started_at, 'auto', v.ended_at, v.sport_type, v.sport_name,
       v.calories::smallint, v.avg_hr::smallint, v.max_hr::smallint,
       v.min_hr::smallint, v.steps::int, v.has_detail, v.observed_at, v.session_id
FROM ROWS FROM (
  unnest(sqlc.arg(started_at)::timestamptz[]),
  unnest(sqlc.arg(ended_at)::timestamptz[]),
  unnest(sqlc.arg(sport_type)::smallint[]),
  unnest(sqlc.arg(sport_name)::text[]),
  unnest(sqlc.arg(calories)::numeric[]),
  unnest(sqlc.arg(avg_hr)::numeric[]),
  unnest(sqlc.arg(max_hr)::numeric[]),
  unnest(sqlc.arg(min_hr)::numeric[]),
  unnest(sqlc.arg(steps)::numeric[]),
  unnest(sqlc.arg(has_detail)::boolean[]),
  unnest(sqlc.arg(observed_at)::timestamptz[]),
  unnest(sqlc.arg(session_id)::text[])
) AS v(started_at, ended_at, sport_type, sport_name, calories, avg_hr, max_hr,
       min_hr, steps, has_detail, observed_at, session_id)
ON CONFLICT (started_at, kind) DO UPDATE SET
  ended_at = EXCLUDED.ended_at,
  sport_type = EXCLUDED.sport_type,
  sport_name = EXCLUDED.sport_name,
  calories = EXCLUDED.calories,
  avg_hr = EXCLUDED.avg_hr,
  max_hr = EXCLUDED.max_hr,
  min_hr = EXCLUDED.min_hr,
  steps = EXCLUDED.steps,
  has_detail = EXCLUDED.has_detail,
  observed_at = EXCLUDED.observed_at,
  session_id = EXCLUDED.session_id;

-- name: UpsertWorkoutSession :exec
INSERT INTO activities (started_at, kind, ended_at, sport_type, sport_name,
  calories, avg_hr, max_hr, min_hr, steps, has_detail, observed_at, session_id)
VALUES (
  sqlc.arg(started_at), 'workout', sqlc.arg(ended_at), sqlc.arg(sport_type),
  sqlc.arg(sport_name), sqlc.arg(calories), sqlc.arg(avg_hr), sqlc.arg(max_hr),
  sqlc.arg(min_hr), sqlc.arg(steps), sqlc.arg(has_detail), sqlc.arg(observed_at),
  sqlc.arg(session_id)
)
ON CONFLICT (started_at, kind) DO UPDATE SET
  ended_at = EXCLUDED.ended_at,
  sport_type = EXCLUDED.sport_type,
  sport_name = EXCLUDED.sport_name,
  calories = EXCLUDED.calories,
  avg_hr = EXCLUDED.avg_hr,
  max_hr = EXCLUDED.max_hr,
  min_hr = EXCLUDED.min_hr,
  steps = EXCLUDED.steps,
  has_detail = EXCLUDED.has_detail,
  observed_at = EXCLUDED.observed_at,
  session_id = EXCLUDED.session_id;

-- name: DeleteWorkoutSessionsIn :exec
DELETE FROM activities
WHERE kind = 'workout' AND started_at = ANY(sqlc.arg(keys)::timestamptz[]);

-- name: MaxWorkoutEndAt :one
SELECT ended_at FROM activities
WHERE kind = 'workout'
  AND (sqlc.narg(has_detail)::boolean IS NULL OR has_detail = sqlc.narg(has_detail))
  AND (sqlc.narg(session_id)::text IS NULL OR session_id = sqlc.narg(session_id)::text)
ORDER BY ended_at DESC
LIMIT 1;

-- name: ActivityDayBounds :many
(SELECT day_key FROM activities ORDER BY day_key ASC LIMIT 1)
UNION ALL
(SELECT day_key FROM activities ORDER BY day_key DESC LIMIT 1);
