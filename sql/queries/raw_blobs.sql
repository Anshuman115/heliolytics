-- name: ListRawBlobsBySession :many
SELECT type_code, sha256, payload
FROM raw_blobs
WHERE session_id = sqlc.arg(session_id)
ORDER BY type_code COLLATE "C";

-- name: ListRawBlobsForSessions :many
SELECT session_id, type_code, sha256, payload
FROM raw_blobs
WHERE session_id = ANY(sqlc.arg(ids)::text[])
ORDER BY session_id COLLATE "C", type_code COLLATE "C";

-- name: InsertRawBlobs :exec
INSERT INTO raw_blobs (session_id, type_code, sha256, payload)
SELECT v.session_id, v.type_code, v.sha256, v.payload
FROM ROWS FROM (
  unnest(sqlc.arg(session_id)::text[]),
  unnest(sqlc.arg(type_code)::text[]),
  unnest(sqlc.arg(sha256)::bytea[]),
  unnest(sqlc.arg(payload)::bytea[])
) AS v(session_id, type_code, sha256, payload);
