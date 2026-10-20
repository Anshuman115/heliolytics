package store

import (
	"context"
	"time"

	"github.com/Anshuman115/heliolytics/internal/store/db"
	"github.com/jackc/pgx/v5"
)

// TxView binds the generated queries to one caller transaction, so rollup
// code can read and write through the same lock and atomicity as ingest.
type TxView struct {
	tx pgx.Tx
	q  *db.Queries
}

func NewTxView(tx pgx.Tx) *TxView { return &TxView{tx: tx, q: db.New(tx)} }

// WithTx is the only canonical transaction opener. Any callback error, commit
// failure, cancellation or panic leaves no partial canonical state.
func (s *Store) WithTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(rollbackCtx)
	}()

	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	committed = true
	return nil
}

// WithReadOnlyTx gives replay a stable snapshot: every source page and the
// profile are read from one repeatable-read, read-only transaction.
func (s *Store) WithReadOnlyTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(rollbackCtx)
	}()

	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	committed = true
	return nil
}

// LockCanonical takes the singleton advisory lock first in the caller's tx.
func (s *Store) LockCanonical(ctx context.Context, tx pgx.Tx) error {
	return db.New(tx).LockCanonical(ctx)
}
