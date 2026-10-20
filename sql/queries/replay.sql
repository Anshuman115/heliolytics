-- name: ReadReplaySessionIDs :many
SELECT session_id, ended_at
FROM sync_sessions
WHERE sqlc.narg(after_ended_at)::timestamptz IS NULL
   OR (ended_at, session_id COLLATE "C")
      > (sqlc.narg(after_ended_at)::timestamptz, sqlc.narg(after_session_id)::text COLLATE "C")
ORDER BY ended_at ASC, session_id COLLATE "C" ASC
LIMIT sqlc.arg(page_limit)::int;

-- name: AnyApplicationRows :many
SELECT EXISTS (SELECT 1 FROM sync_sessions) AS present
UNION ALL SELECT EXISTS (SELECT 1 FROM raw_blobs) AS present
UNION ALL SELECT EXISTS (SELECT 1 FROM heart_rate_samples) AS present
UNION ALL SELECT EXISTS (SELECT 1 FROM step_samples) AS present
UNION ALL SELECT EXISTS (SELECT 1 FROM temperature_samples) AS present
UNION ALL SELECT EXISTS (SELECT 1 FROM hrv_samples) AS present
UNION ALL SELECT EXISTS (SELECT 1 FROM stress_samples) AS present
UNION ALL SELECT EXISTS (SELECT 1 FROM resp_samples) AS present
UNION ALL SELECT EXISTS (SELECT 1 FROM spo2_samples) AS present
UNION ALL SELECT EXISTS (SELECT 1 FROM sleep_sessions) AS present
UNION ALL SELECT EXISTS (SELECT 1 FROM activities) AS present
UNION ALL SELECT EXISTS (SELECT 1 FROM sleep_observations) AS present
UNION ALL SELECT EXISTS (SELECT 1 FROM workout_observations) AS present
UNION ALL SELECT EXISTS (SELECT 1 FROM pai_scores) AS present
UNION ALL SELECT EXISTS (SELECT 1 FROM daily_metrics) AS present
UNION ALL SELECT EXISTS (SELECT 1 FROM coverage) AS present
UNION ALL SELECT EXISTS (SELECT 1 FROM profile) AS present
UNION ALL SELECT EXISTS (SELECT 1 FROM auth_nonces) AS present;

-- name: CountRequiredRelations :one
SELECT count(*)
FROM pg_catalog.pg_class c
JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = 'public'
  AND c.relname = ANY(sqlc.arg(relations)::text[]);
