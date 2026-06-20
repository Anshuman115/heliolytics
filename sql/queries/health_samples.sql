-- name: UpsertHealthSample :exec
INSERT INTO health_samples (metric, day_key, sampled_at, value, source_session_id)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (metric, sampled_at) DO UPDATE SET
  value = EXCLUDED.value,
  day_key = EXCLUDED.day_key,
  source_session_id = EXCLUDED.source_session_id;

-- name: ListHealthSamples :many
SELECT metric, day_key, sampled_at, value
FROM health_samples
WHERE day_key >= $1 AND day_key <= $2
ORDER BY sampled_at ASC;
