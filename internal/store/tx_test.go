package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestWithTxRollsBackOnError(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	wantErr := errors.New("boom")

	err := st.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`INSERT INTO coverage (type_code, data_through) VALUES ('0x13', now())`); err != nil {
			return err
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want the callback error", err)
	}
	if n := countRows(t, st, "coverage"); n != 0 {
		t.Fatalf("coverage rows after rollback = %d, want 0", n)
	}
}

func TestWithTxCommitsOnSuccess(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	if err := st.WithTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`INSERT INTO coverage (type_code, data_through) VALUES ('0x13', now())`)
		return err
	}); err != nil {
		t.Fatalf("WithTx: %v", err)
	}
	if n := countRows(t, st, "coverage"); n != 1 {
		t.Fatalf("coverage rows after commit = %d, want 1", n)
	}
}

func TestWithTxPreservesPanic(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	panicked := func() (recovered bool) {
		defer func() { recovered = recover() != nil }()
		_ = st.WithTx(ctx, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx,
				`INSERT INTO coverage (type_code, data_through) VALUES ('0x13', now())`); err != nil {
				return err
			}
			panic("boom")
		})
		return false
	}()
	if !panicked {
		t.Fatal("panic did not propagate out of WithTx")
	}
	if n := countRows(t, st, "coverage"); n != 0 {
		t.Fatalf("coverage rows after panic = %d, want 0", n)
	}
}

func TestWithTxCanceledContextRollsBack(t *testing.T) {
	st := newTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())

	err := st.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`INSERT INTO coverage (type_code, data_through) VALUES ('0x13', now())`); err != nil {
			return err
		}
		cancel()
		return context.Canceled
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if n := countRows(t, st, "coverage"); n != 0 {
		t.Fatalf("coverage rows after cancellation = %d, want 0", n)
	}
}

func TestWithTxReleasesCanonicalLock(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	if err := st.WithTx(ctx, func(tx pgx.Tx) error {
		if err := st.LockCanonical(ctx, tx); err != nil {
			return err
		}
		return errors.New("rollback with the lock held")
	}); err == nil {
		t.Fatal("expected the callback error")
	}

	locked := make(chan error, 1)
	go func() {
		lockCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		locked <- st.WithTx(lockCtx, func(tx pgx.Tx) error {
			return st.LockCanonical(lockCtx, tx)
		})
	}()
	select {
	case err := <-locked:
		if err != nil {
			t.Fatalf("re-acquiring the lock: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("canonical lock was not released by the rolled back transaction")
	}
}

func TestWithReadOnlyTxRejectsWrites(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	err := st.WithReadOnlyTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`INSERT INTO coverage (type_code, data_through) VALUES ('0x13', now())`)
		return err
	})
	if err == nil {
		t.Fatal("read-only transaction accepted a write")
	}
}
