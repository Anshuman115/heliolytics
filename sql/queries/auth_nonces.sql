-- name: ConsumeNonce :one
INSERT INTO auth_nonces (kid, nonce, expires_at)
VALUES (sqlc.arg(kid), sqlc.arg(nonce), sqlc.arg(expires_at))
ON CONFLICT (kid, nonce) DO UPDATE SET expires_at = EXCLUDED.expires_at
WHERE auth_nonces.expires_at < clock_timestamp()
RETURNING kid;

-- name: PurgeNonces :execrows
DELETE FROM auth_nonces WHERE expires_at < sqlc.arg(cutoff);
