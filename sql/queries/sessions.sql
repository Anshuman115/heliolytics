-- name: GetSessionEnvelope :one
SELECT session_id, device_mac, started_at, ended_at, battery_pct, catalog, received_at
FROM sync_sessions
WHERE session_id = sqlc.arg(session_id);

-- name: InsertSession :exec
INSERT INTO sync_sessions (session_id, device_mac, started_at, ended_at, battery_pct, catalog)
VALUES (
  sqlc.arg(session_id),
  sqlc.arg(device_mac),
  sqlc.arg(started_at),
  sqlc.arg(ended_at),
  sqlc.arg(battery_pct),
  sqlc.arg(catalog)
);

-- name: LastSessionReceivedAt :one
SELECT received_at FROM sync_sessions ORDER BY received_at DESC LIMIT 1;

-- name: ListSessionsByIDs :many
SELECT session_id, device_mac, started_at, ended_at, battery_pct, catalog
FROM sync_sessions
WHERE session_id = ANY(sqlc.arg(ids)::text[]);
