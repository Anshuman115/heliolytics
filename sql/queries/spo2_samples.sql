-- name: UpsertSpo2Samples :exec
INSERT INTO spo2_samples (sampled_at, source, pct, mode, session_id, observed_at)
SELECT v.sampled_at, v.source, v.pct, v.mode, v.session_id, v.observed_at
FROM ROWS FROM (
  unnest(sqlc.arg(sampled_at)::timestamptz[]),
  unnest(sqlc.arg(source)::text[]),
  unnest(sqlc.arg(pct)::smallint[]),
  unnest(sqlc.arg(mode)::text[]),
  unnest(sqlc.arg(session_id)::text[]),
  unnest(sqlc.arg(observed_at)::timestamptz[])
) AS v(sampled_at, source, pct, mode, session_id, observed_at)
ON CONFLICT (sampled_at, source) DO UPDATE SET
  pct = EXCLUDED.pct,
  mode = EXCLUDED.mode,
  session_id = EXCLUDED.session_id,
  observed_at = EXCLUDED.observed_at
WHERE (EXCLUDED.observed_at, EXCLUDED.session_id COLLATE "C")
      >= (spo2_samples.observed_at, spo2_samples.session_id COLLATE "C");

-- name: ListSpo2Samples :many
SELECT sampled_at, source, pct, mode, session_id, observed_at
FROM spo2_samples
WHERE sampled_at >= sqlc.arg(from_at) AND sampled_at < sqlc.arg(to_at)
ORDER BY sampled_at ASC;

-- name: MaxSpo2SampleAt :one
SELECT sampled_at FROM spo2_samples
WHERE (sqlc.narg(session_id)::text IS NULL OR session_id = sqlc.narg(session_id)::text)
ORDER BY sampled_at DESC
LIMIT 1;

-- name: Spo2DayBounds :many
(SELECT day_key FROM spo2_samples ORDER BY sampled_at ASC LIMIT 1)
UNION ALL
(SELECT day_key FROM spo2_samples ORDER BY sampled_at DESC LIMIT 1);

-- name: ListSpo2SamplesByKeys :many
SELECT sampled_at, source, pct, mode, session_id, observed_at
FROM spo2_samples
WHERE sampled_at = ANY(sqlc.arg(keys)::timestamptz[])
  AND source = ANY(sqlc.arg(sources)::text[])
ORDER BY sampled_at ASC, source COLLATE "C" ASC;
