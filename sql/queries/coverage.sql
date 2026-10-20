-- name: ListCoverage :many
SELECT type_code, data_through FROM coverage ORDER BY type_code COLLATE "C";

-- name: UpsertCoverage :exec
INSERT INTO coverage (type_code, data_through, updated_at)
VALUES (sqlc.arg(type_code), sqlc.arg(data_through), clock_timestamp())
ON CONFLICT (type_code) DO UPDATE SET
  data_through = EXCLUDED.data_through,
  updated_at = clock_timestamp()
WHERE coverage.data_through IS DISTINCT FROM EXCLUDED.data_through;

-- name: DeleteCoverage :exec
DELETE FROM coverage WHERE type_code = sqlc.arg(type_code);
