package store

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestWithTxRollsBackOnError(t *testing.T) {
	st, cleanup := testStore(t)
	defer cleanup()
	ctx := context.Background()
	const sid = "tx-rollback-test-session"

	clean := func() { st.pool.Exec(ctx, `DELETE FROM sync_sessions WHERE session_id=$1`, sid) }
	clean()
	t.Cleanup(clean)

	wantErr := errors.New("boom")
	err := st.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`INSERT INTO sync_sessions (session_id, started_at) VALUES ($1, NOW())`, sid); err != nil {
			return err
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err=%v, want wantErr", err)
	}

	var count int
	if err := st.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM sync_sessions WHERE session_id=$1`, sid).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("count=%d, want 0 (insert must have rolled back)", count)
	}
}

func TestWithTxCommitsOnSuccess(t *testing.T) {
	st, cleanup := testStore(t)
	defer cleanup()
	ctx := context.Background()
	const sid = "tx-commit-test-session"

	clean := func() { st.pool.Exec(ctx, `DELETE FROM sync_sessions WHERE session_id=$1`, sid) }
	clean()
	t.Cleanup(clean)

	err := st.WithTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`INSERT INTO sync_sessions (session_id, started_at) VALUES ($1, NOW())`, sid)
		return err
	})
	if err != nil {
		t.Fatalf("WithTx: %v", err)
	}

	var count int
	if err := st.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM sync_sessions WHERE session_id=$1`, sid).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("count=%d, want 1 (insert must have committed)", count)
	}
}
