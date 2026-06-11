-- name: DeleteTemperatureBySession :exec
DELETE FROM temperature_samples WHERE sync_session_id = $1;

-- name: InsertTemperatureSample :exec
INSERT INTO temperature_samples (sync_session_id, day_key, sampled_at, celsius)
VALUES ($1, $2, $3, $4);

-- name: ListTemperature :many
SELECT day_key, sampled_at, celsius
FROM temperature_samples
WHERE day_key >= $1 AND day_key <= $2
ORDER BY sampled_at ASC;
