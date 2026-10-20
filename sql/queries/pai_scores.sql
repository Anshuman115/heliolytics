-- name: UpsertPaiScores :exec
INSERT INTO pai_scores (day_key, score, sampled_at, observed_at, session_id)
SELECT v.day_key, v.score::smallint, v.sampled_at, v.observed_at, v.session_id
FROM ROWS FROM (
  unnest(sqlc.arg(day_key)::date[]),
  unnest(sqlc.arg(score)::smallint[]),
  unnest(sqlc.arg(sampled_at)::timestamptz[]),
  unnest(sqlc.arg(observed_at)::timestamptz[]),
  unnest(sqlc.arg(session_id)::text[])
) AS v(day_key, score, sampled_at, observed_at, session_id)
ON CONFLICT (day_key) DO UPDATE SET
  score = EXCLUDED.score,
  sampled_at = EXCLUDED.sampled_at,
  observed_at = EXCLUDED.observed_at,
  session_id = EXCLUDED.session_id
WHERE (EXCLUDED.observed_at, EXCLUDED.session_id COLLATE "C")
      >= (pai_scores.observed_at, pai_scores.session_id COLLATE "C");

-- name: ListPaiScoresByDays :many
SELECT day_key, score, sampled_at, observed_at, session_id
FROM pai_scores
WHERE day_key = ANY(sqlc.arg(days)::date[])
ORDER BY day_key ASC;

-- name: ListPaiScoresRange :many
SELECT day_key, score, sampled_at
FROM pai_scores
WHERE day_key >= sqlc.arg(from_day) AND day_key <= sqlc.arg(to_day)
ORDER BY day_key ASC;

-- name: MaxPaiSampledAt :one
SELECT sampled_at FROM pai_scores
WHERE (sqlc.narg(session_id)::text IS NULL OR session_id = sqlc.narg(session_id)::text)
ORDER BY sampled_at DESC
LIMIT 1;

-- name: PaiDayBounds :many
(SELECT day_key FROM pai_scores ORDER BY day_key ASC LIMIT 1)
UNION ALL
(SELECT day_key FROM pai_scores ORDER BY day_key DESC LIMIT 1);
