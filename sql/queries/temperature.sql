-- name: UpsertTemperatureSample :exec
INSERT INTO temperature_samples (sampled_at, day_key, celsius, source_session_id)
VALUES ($1, $2, $3, $4)
ON CONFLICT (sampled_at) DO UPDATE SET
  celsius = EXCLUDED.celsius,
  day_key = EXCLUDED.day_key,
  source_session_id = EXCLUDED.source_session_id;

-- name: ListTemperature :many
SELECT day_key, sampled_at, celsius
FROM temperature_samples
WHERE day_key >= $1 AND day_key <= $2
ORDER BY sampled_at ASC;
