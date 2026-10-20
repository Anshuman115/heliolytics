-- name: UpsertRespSamples :exec
INSERT INTO resp_samples (sampled_at, br_min, session_id, observed_at)
SELECT v.sampled_at, v.br_min, v.session_id, v.observed_at
FROM ROWS FROM (
  unnest(sqlc.arg(sampled_at)::timestamptz[]),
  unnest(sqlc.arg(br_min)::real[]),
  unnest(sqlc.arg(session_id)::text[]),
  unnest(sqlc.arg(observed_at)::timestamptz[])
) AS v(sampled_at, br_min, session_id, observed_at)
ON CONFLICT (sampled_at) DO UPDATE SET
  br_min = EXCLUDED.br_min,
  session_id = EXCLUDED.session_id,
  observed_at = EXCLUDED.observed_at
WHERE (EXCLUDED.observed_at, EXCLUDED.session_id COLLATE "C")
      >= (resp_samples.observed_at, resp_samples.session_id COLLATE "C");

-- name: ListRespSamples :many
SELECT sampled_at, br_min::float8 AS br_min, session_id, observed_at
FROM resp_samples
WHERE sampled_at >= sqlc.arg(from_at) AND sampled_at < sqlc.arg(to_at)
ORDER BY sampled_at ASC;

-- name: MaxRespSampleAt :one
SELECT sampled_at FROM resp_samples
WHERE (sqlc.narg(session_id)::text IS NULL OR session_id = sqlc.narg(session_id)::text)
ORDER BY sampled_at DESC
LIMIT 1;

-- name: RespDayBounds :many
(SELECT day_key FROM resp_samples ORDER BY sampled_at ASC LIMIT 1)
UNION ALL
(SELECT day_key FROM resp_samples ORDER BY sampled_at DESC LIMIT 1);

-- name: ListRespSamplesByKeys :many
SELECT sampled_at, br_min::float8 AS br_min, session_id, observed_at
FROM resp_samples
WHERE sampled_at = ANY(sqlc.arg(keys)::timestamptz[])
ORDER BY sampled_at ASC;
