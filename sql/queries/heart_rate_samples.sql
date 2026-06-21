-- name: UpsertHeartRateSample :exec
INSERT INTO heart_rate_samples (sampled_at, day_key, bpm, source_session_id)
VALUES ($1, $2, $3, $4)
ON CONFLICT (sampled_at) DO UPDATE SET
  bpm = EXCLUDED.bpm,
  day_key = EXCLUDED.day_key,
  source_session_id = EXCLUDED.source_session_id;

-- name: ListHeartRateSamples :many
SELECT day_key, sampled_at, bpm
FROM heart_rate_samples
WHERE day_key >= $1 AND day_key <= $2
ORDER BY sampled_at ASC;

-- name: ListHeartRateSamplesCompact :many
WITH computed AS (
  SELECT
    day_key,
    sampled_at,
    bpm,
    (MIN(sampled_at) OVER(PARTITION BY day_key))::timestamptz as day_start
  FROM heart_rate_samples
  WHERE day_key >= $1 AND day_key <= $2
)
SELECT
  day_key,
  day_start as start_time,
  array_agg(EXTRACT(EPOCH FROM (sampled_at - day_start))::int ORDER BY sampled_at)::int[] as offsets,
  array_agg(bpm::int ORDER BY sampled_at)::int[] as values
FROM computed
GROUP BY day_key, day_start
ORDER BY day_key ASC;

