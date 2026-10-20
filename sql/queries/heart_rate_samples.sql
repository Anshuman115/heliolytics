-- name: UpsertHeartRateSamples :exec
-- Newer observation tuples win; an older sync never regresses a canonical value.
INSERT INTO heart_rate_samples (sampled_at, source, bpm, session_id, observed_at)
SELECT v.sampled_at, v.source, v.bpm, v.session_id, v.observed_at
FROM ROWS FROM (
  unnest(sqlc.arg(sampled_at)::timestamptz[]),
  unnest(sqlc.arg(source)::text[]),
  unnest(sqlc.arg(bpm)::smallint[]),
  unnest(sqlc.arg(session_id)::text[]),
  unnest(sqlc.arg(observed_at)::timestamptz[])
) AS v(sampled_at, source, bpm, session_id, observed_at)
ON CONFLICT (sampled_at, source) DO UPDATE SET
  bpm = EXCLUDED.bpm,
  session_id = EXCLUDED.session_id,
  observed_at = EXCLUDED.observed_at
WHERE (EXCLUDED.observed_at, EXCLUDED.session_id COLLATE "C")
      >= (heart_rate_samples.observed_at, heart_rate_samples.session_id COLLATE "C");

-- name: ListHeartRateSamples :many
SELECT sampled_at, source, bpm, session_id, observed_at
FROM heart_rate_samples
WHERE sampled_at >= sqlc.arg(from_at) AND sampled_at < sqlc.arg(to_at)
ORDER BY sampled_at ASC;

-- name: MaxHeartRateSampleAt :one
SELECT sampled_at FROM heart_rate_samples
WHERE source = sqlc.arg(source)
  AND (sqlc.narg(session_id)::text IS NULL OR session_id = sqlc.narg(session_id)::text)
ORDER BY sampled_at DESC
LIMIT 1;

-- name: HeartRateCompact :many
WITH filtered AS (
  SELECT day_key, sampled_at, bpm
  FROM heart_rate_samples
  WHERE source = '0x01'
    AND sampled_at >= sqlc.arg(from_at) AND sampled_at < sqlc.arg(to_at)
), computed AS (
  SELECT day_key, sampled_at, bpm::float8 AS value,
         MIN(sampled_at) OVER (PARTITION BY day_key) AS day_start
  FROM filtered
)
SELECT day_key, day_start::timestamptz AS start_time,
       array_agg(floor(EXTRACT(EPOCH FROM (sampled_at - day_start)))::int ORDER BY sampled_at)::int[] AS offsets,
       array_agg(value ORDER BY sampled_at)::float8[] AS values
FROM computed
GROUP BY day_key, day_start
ORDER BY start_time ASC;

-- name: HeartRateDayBounds :many
(SELECT day_key FROM heart_rate_samples ORDER BY sampled_at ASC LIMIT 1)
UNION ALL
(SELECT day_key FROM heart_rate_samples ORDER BY sampled_at DESC LIMIT 1);

-- name: ListHeartRateSamplesByKeys :many
SELECT sampled_at, source, bpm, session_id, observed_at
FROM heart_rate_samples
WHERE sampled_at = ANY(sqlc.arg(keys)::timestamptz[])
  AND source = ANY(sqlc.arg(sources)::text[])
ORDER BY sampled_at ASC, source COLLATE "C" ASC;
