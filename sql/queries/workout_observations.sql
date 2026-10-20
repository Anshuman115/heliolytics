-- name: InsertWorkoutObservations :exec
INSERT INTO workout_observations (session_id, started_at, kind, ended_at,
  sport_type, sport_name, calories, avg_hr, max_hr, min_hr, steps, has_detail,
  observed_at)
SELECT v.session_id, v.started_at, 'workout', v.ended_at, v.sport_type,
       v.sport_name, v.calories::smallint, v.avg_hr::smallint, v.max_hr::smallint,
       v.min_hr::smallint, v.steps::int, v.has_detail, v.observed_at
FROM ROWS FROM (
  unnest(sqlc.arg(session_id)::text[]),
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
  unnest(sqlc.arg(observed_at)::timestamptz[])
) AS v(session_id, started_at, ended_at, sport_type, sport_name, calories,
       avg_hr, max_hr, min_hr, steps, has_detail, observed_at)
ON CONFLICT (session_id, started_at) DO NOTHING;

-- name: ListWorkoutObservationsBySession :many
SELECT session_id, started_at, ended_at, sport_type, sport_name, calories,
       avg_hr, max_hr, min_hr, steps, has_detail, observed_at
FROM workout_observations
WHERE session_id = sqlc.arg(session_id)
ORDER BY started_at ASC;

-- name: ListWorkoutObservationsOverlapping :many
SELECT session_id, started_at, ended_at, sport_type, sport_name, calories,
       avg_hr, max_hr, min_hr, steps, has_detail, observed_at
FROM workout_observations
WHERE started_at < sqlc.arg(to_at) AND ended_at > sqlc.arg(from_at)
ORDER BY started_at ASC;
