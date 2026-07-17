package store

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// WithTx runs fn inside one transaction: begins, runs fn, commits if fn
// returns nil, rolls back otherwise (including on panic — the deferred
// Rollback is a no-op after a successful Commit). This is what makes an
// ingest sync's writes all-or-nothing: any failure at any point in fn rolls
// back every write made so far in that sync, no partial day survives.
func (s *Store) WithTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// execBatchTx is execBatch's logic but against an already-open tx instead of
// opening its own — used by write methods when called from inside WithTx.
// Chunking still applies (see execBatch's comment on why), but no Begin/
// Commit here since the caller owns the transaction boundary.
func execBatchTx(ctx context.Context, tx pgx.Tx, sql string, rows []queuedRow) error {
	if len(rows) == 0 {
		return nil
	}
	for start := 0; start < len(rows); start += batchChunkSize {
		end := min(start+batchChunkSize, len(rows))
		batch := &pgx.Batch{}
		for _, args := range rows[start:end] {
			batch.Queue(sql, args...)
		}
		if err := tx.SendBatch(ctx, batch).Close(); err != nil {
			return err
		}
	}
	return nil
}
