-- name: UpsertStepSamples :exec
INSERT INTO step_samples (sampled_at, steps, session_id, observed_at)
SELECT v.sampled_at, v.steps, v.session_id, v.observed_at
FROM ROWS FROM (
  unnest(sqlc.arg(sampled_at)::timestamptz[]),
  unnest(sqlc.arg(steps)::smallint[]),
  unnest(sqlc.arg(session_id)::text[]),
  unnest(sqlc.arg(observed_at)::timestamptz[])
) AS v(sampled_at, steps, session_id, observed_at)
ON CONFLICT (sampled_at) DO UPDATE SET
  steps = EXCLUDED.steps,
  session_id = EXCLUDED.session_id,
  observed_at = EXCLUDED.observed_at
WHERE (EXCLUDED.observed_at, EXCLUDED.session_id COLLATE "C")
      >= (step_samples.observed_at, step_samples.session_id COLLATE "C");

-- name: ListStepSamples :many
SELECT sampled_at, steps, session_id, observed_at
FROM step_samples
WHERE sampled_at >= sqlc.arg(from_at) AND sampled_at < sqlc.arg(to_at)
ORDER BY sampled_at ASC;

-- name: MaxStepSampleAt :one
SELECT sampled_at FROM step_samples
WHERE (sqlc.narg(session_id)::text IS NULL OR session_id = sqlc.narg(session_id)::text)
ORDER BY sampled_at DESC
LIMIT 1;

-- name: StepDayBounds :many
(SELECT day_key FROM step_samples ORDER BY sampled_at ASC LIMIT 1)
UNION ALL
(SELECT day_key FROM step_samples ORDER BY sampled_at DESC LIMIT 1);

-- name: ListStepSamplesByKeys :many
SELECT sampled_at, steps, session_id, observed_at
FROM step_samples
WHERE sampled_at = ANY(sqlc.arg(keys)::timestamptz[])
ORDER BY sampled_at ASC;
