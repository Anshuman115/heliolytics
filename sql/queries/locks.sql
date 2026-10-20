-- name: LockCanonical :exec
-- One global singleton lock: ingest, profile recomputation and target rebuild
-- serialize on the same key. Acquired first, held until the transaction ends.
SELECT pg_advisory_xact_lock(72651019, 1);
