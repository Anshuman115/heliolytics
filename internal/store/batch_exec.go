package store

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// queuedRow is one statement's worth of arguments for a bulk upsert.
type queuedRow []any

// Statements per pipelined batch.
//
// Bounded on purpose. SendBatch writes every queued statement before reading
// any reply, so an unbounded batch can fill the socket both ways at once — the
// client still writing while the server blocks on a full send buffer. Chunking
// keeps each round trip's payload well inside those buffers. All chunks share
// one transaction, so the write stays all-or-nothing regardless.
const batchChunkSize = 500

// execBatch runs sql once per row in [rows], pipelined in chunks, inside a
// single transaction.
//
// Why this exists: one Exec per row costs a network round trip AND an implicit
// transaction commit (an fsync) per row. At ~16k rows per sync that measured
// ~4.5 ms/row, so a single upload spent over a minute in the database and could
// outrun the client's timeout — which cancelled the request mid-write and left
// samples stored without their daily rollups.
//
// One transaction plus chunked pipelining turns that into ~16 round trips and a
// single commit, and makes the write all-or-nothing: a cancelled request now
// rolls back cleanly instead of leaving a partial day behind.
func (s *Store) execBatch(ctx context.Context, sql string, rows []queuedRow) error {
	if len(rows) == 0 {
		return nil
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	// No-op once committed; guards every early return below.
	defer func() { _ = tx.Rollback(ctx) }()

	for start := 0; start < len(rows); start += batchChunkSize {
		end := min(start+batchChunkSize, len(rows))

		batch := &pgx.Batch{}
		for _, args := range rows[start:end] {
			batch.Queue(sql, args...)
		}

		// Close reports the first failure among the queued statements, and must
		// happen before the transaction can be committed.
		if err := tx.SendBatch(ctx, batch).Close(); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}
