-- name: UpsertRawPaiScore :exec
INSERT INTO raw_pai_scores (day_key, score, source_session_id, updated_at)
VALUES ($1, $2, $3, NOW())
ON CONFLICT (day_key) DO UPDATE SET
  score = EXCLUDED.score,
  source_session_id = EXCLUDED.source_session_id,
  updated_at = NOW();

-- name: GetRawPaiScore :one
SELECT day_key, score FROM raw_pai_scores WHERE day_key = $1;
