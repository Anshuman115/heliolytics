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

-- name: ListHealthSamplesCompact :many
WITH computed AS (
  SELECT
    metric,
    day_key,
    sampled_at,
    value,
    (MIN(sampled_at) OVER(PARTITION BY metric, day_key))::timestamptz as day_start
  FROM health_samples
  WHERE day_key >= $1 AND day_key <= $2
)
SELECT
  metric,
  day_key,
  day_start as start_time,
  array_agg(EXTRACT(EPOCH FROM (sampled_at - day_start))::int ORDER BY sampled_at)::int[] as offsets,
  array_agg(value::float8 ORDER BY sampled_at)::float8[] as values
FROM computed
GROUP BY metric, day_key, day_start
ORDER BY metric, day_key ASC;

