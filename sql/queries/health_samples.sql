-- name: DeleteHealthSamplesBySession :exec
DELETE FROM health_samples WHERE sync_session_id = $1;

-- name: InsertHealthSample :exec
INSERT INTO health_samples (sync_session_id, metric, day_key, sampled_at, value)
VALUES ($1, $2, $3, $4, $5);

-- name: ListHealthSamples :many
SELECT metric, day_key, sampled_at, value
FROM health_samples
WHERE day_key >= $1 AND day_key <= $2
ORDER BY sampled_at ASC;
