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

-- name: ListTemperatureCompact :many
WITH computed AS (
  SELECT
    day_key,
    sampled_at,
    celsius,
    (MIN(sampled_at) OVER(PARTITION BY day_key))::timestamptz as day_start
  FROM temperature_samples
  WHERE day_key >= $1 AND day_key <= $2
)
SELECT
  day_key,
  day_start as start_time,
  array_agg(EXTRACT(EPOCH FROM (sampled_at - day_start))::int ORDER BY sampled_at)::int[] as offsets,
  array_agg(celsius::float8 ORDER BY sampled_at)::float8[] as values
FROM computed
GROUP BY day_key, day_start
ORDER BY day_key ASC;

