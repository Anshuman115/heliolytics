-- name: UpsertHrvSamples :exec
INSERT INTO hrv_samples (sampled_at, rmssd_ms, session_id, observed_at)
SELECT v.sampled_at, v.rmssd, v.session_id, v.observed_at
FROM ROWS FROM (
  unnest(sqlc.arg(sampled_at)::timestamptz[]),
  unnest(sqlc.arg(rmssd)::real[]),
  unnest(sqlc.arg(session_id)::text[]),
  unnest(sqlc.arg(observed_at)::timestamptz[])
) AS v(sampled_at, rmssd, session_id, observed_at)
ON CONFLICT (sampled_at) DO UPDATE SET
  rmssd_ms = EXCLUDED.rmssd_ms,
  session_id = EXCLUDED.session_id,
  observed_at = EXCLUDED.observed_at
WHERE (EXCLUDED.observed_at, EXCLUDED.session_id COLLATE "C")
      >= (hrv_samples.observed_at, hrv_samples.session_id COLLATE "C");

-- name: ListHrvSamples :many
SELECT sampled_at, rmssd_ms::float8 AS rmssd, session_id, observed_at
FROM hrv_samples
WHERE sampled_at >= sqlc.arg(from_at) AND sampled_at < sqlc.arg(to_at)
ORDER BY sampled_at ASC;

-- name: MaxHrvSampleAt :one
SELECT sampled_at FROM hrv_samples
WHERE (sqlc.narg(session_id)::text IS NULL OR session_id = sqlc.narg(session_id)::text)
ORDER BY sampled_at DESC
LIMIT 1;

-- name: HrvDayBounds :many
(SELECT day_key FROM hrv_samples ORDER BY sampled_at ASC LIMIT 1)
UNION ALL
(SELECT day_key FROM hrv_samples ORDER BY sampled_at DESC LIMIT 1);

-- name: ListHrvSamplesByKeys :many
SELECT sampled_at, rmssd_ms::float8 AS rmssd, session_id, observed_at
FROM hrv_samples
WHERE sampled_at = ANY(sqlc.arg(keys)::timestamptz[])
ORDER BY sampled_at ASC;
