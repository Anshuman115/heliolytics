-- name: UpsertRhrSample :exec
INSERT INTO rhr_samples (sampled_at, day_key, value, source_session_id)
VALUES ($1, $2, $3, $4)
ON CONFLICT (sampled_at) DO UPDATE SET
  value = EXCLUDED.value,
  day_key = EXCLUDED.day_key,
  source_session_id = EXCLUDED.source_session_id;

-- name: ListRhrSamplesRange :many
SELECT sampled_at, day_key, value
FROM rhr_samples
WHERE day_key >= $1 AND day_key <= $2
ORDER BY sampled_at ASC;

-- name: MeanRhrInWindow :one
SELECT AVG(value)::float8 AS avg_value, COUNT(*) AS n
FROM rhr_samples
WHERE sampled_at >= $1 AND sampled_at <= $2;

-- name: LatestRhrOnDay :one
SELECT value FROM rhr_samples
WHERE day_key = $1
ORDER BY sampled_at DESC
LIMIT 1;
