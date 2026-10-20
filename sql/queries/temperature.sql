-- name: UpsertTemperatureSamples :exec
-- on_wrist is written false for new keys and never changed here; the wear proxy
-- has its own maintenance statement.
INSERT INTO temperature_samples (sampled_at, celsius, on_wrist, session_id, observed_at)
SELECT v.sampled_at, v.celsius, false, v.session_id, v.observed_at
FROM ROWS FROM (
  unnest(sqlc.arg(sampled_at)::timestamptz[]),
  unnest(sqlc.arg(celsius)::numeric[]),
  unnest(sqlc.arg(session_id)::text[]),
  unnest(sqlc.arg(observed_at)::timestamptz[])
) AS v(sampled_at, celsius, session_id, observed_at)
ON CONFLICT (sampled_at) DO UPDATE SET
  celsius = EXCLUDED.celsius,
  session_id = EXCLUDED.session_id,
  observed_at = EXCLUDED.observed_at
WHERE (EXCLUDED.observed_at, EXCLUDED.session_id COLLATE "C")
      >= (temperature_samples.observed_at, temperature_samples.session_id COLLATE "C");

-- name: ListTemperatureSamples :many
SELECT sampled_at, (celsius * 100)::int AS centi_c, on_wrist, session_id, observed_at
FROM temperature_samples
WHERE sampled_at >= sqlc.arg(from_at) AND sampled_at < sqlc.arg(to_at)
ORDER BY sampled_at ASC;

-- name: MaxTemperatureSampleAt :one
SELECT sampled_at FROM temperature_samples
WHERE (sqlc.narg(session_id)::text IS NULL OR session_id = sqlc.narg(session_id)::text)
ORDER BY sampled_at DESC
LIMIT 1;

-- name: TemperatureCompact :many
WITH filtered AS (
  SELECT day_key, sampled_at, celsius
  FROM temperature_samples
  WHERE on_wrist
    AND sampled_at >= sqlc.arg(from_at) AND sampled_at < sqlc.arg(to_at)
), computed AS (
  SELECT day_key, sampled_at, celsius::float8 AS value,
         MIN(sampled_at) OVER (PARTITION BY day_key) AS day_start
  FROM filtered
)
SELECT day_key, day_start::timestamptz AS start_time,
       array_agg(floor(EXTRACT(EPOCH FROM (sampled_at - day_start)))::int ORDER BY sampled_at)::int[] AS offsets,
       array_agg(value ORDER BY sampled_at)::float8[] AS values
FROM computed
GROUP BY day_key, day_start
ORDER BY start_time ASC;

-- name: RefreshTemperatureWearProxy :many
-- Wear proxy is same UTC minute canonical 0x01 HR plus an in-domain temperature.
UPDATE temperature_samples t
SET on_wrist = candidate.valid
FROM (
  SELECT ts.sampled_at,
         (ts.celsius >= 30 AND ts.celsius <= 40 AND EXISTS (
            SELECT 1 FROM heart_rate_samples h
            WHERE h.source = '0x01'
              AND h.sampled_at >= date_trunc('minute', ts.sampled_at)
              AND h.sampled_at < date_trunc('minute', ts.sampled_at) + INTERVAL '1 minute'
         )) AS valid
  FROM temperature_samples ts
  WHERE ts.sampled_at >= sqlc.arg(from_at) AND ts.sampled_at < sqlc.arg(to_at)
) AS candidate
WHERE t.sampled_at = candidate.sampled_at
  AND t.on_wrist IS DISTINCT FROM candidate.valid
RETURNING t.sampled_at AS sampled_at, t.on_wrist AS on_wrist;

-- name: TemperatureDayBounds :many
(SELECT day_key FROM temperature_samples ORDER BY sampled_at ASC LIMIT 1)
UNION ALL
(SELECT day_key FROM temperature_samples ORDER BY sampled_at DESC LIMIT 1);

-- name: ListTemperatureSamplesByKeys :many
SELECT sampled_at, (celsius * 100)::int AS centi_c, session_id, observed_at
FROM temperature_samples
WHERE sampled_at = ANY(sqlc.arg(keys)::timestamptz[])
ORDER BY sampled_at ASC;
