package store

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestReadReplaySessionsPagesInEndedAtOrder(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	started := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	ended := started.Add(12 * time.Hour)

	// Equal ended_at forces the session_id bytewise tie break.
	for _, id := range []string{"replay-c", "replay-a", "replay-b"} {
		seedSession(t, st, id, started, ended)
	}

	var collected []SessionReplay
	var after *ReplayCursor
	for range 5 {
		var page []SessionReplay
		if err := st.WithReadOnlyTx(ctx, func(tx pgx.Tx) error {
			var err error
			page, err = NewTxView(tx).ReadReplaySessions(ctx, after, 1)
			return err
		}); err != nil {
			t.Fatalf("replay page: %v", err)
		}
		if len(page) == 0 {
			break
		}
		collected = append(collected, page...)
		last := page[len(page)-1]
		after = &ReplayCursor{EndedAt: last.Meta.EndedAt, SessionID: last.Meta.ID}
	}
	if len(collected) != 3 {
		t.Fatalf("collected %d sessions, want 3", len(collected))
	}
	order := []string{collected[0].Meta.ID, collected[1].Meta.ID, collected[2].Meta.ID}
	want := []string{"replay-a", "replay-b", "replay-c"}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("replay order = %v, want %v", order, want)
		}
	}
	for _, session := range collected {
		if len(session.Blobs) != 1 || session.Blobs[0].TypeCode != "0x01" {
			t.Fatalf("session %s blobs = %+v", session.Meta.ID, session.Blobs)
		}
		if len(session.Catalog) == 0 {
			t.Fatalf("session %s catalog missing", session.Meta.ID)
		}
		if session.Meta.DeviceMAC != "02:00:00:00:00:01" {
			t.Fatalf("session %s device mac = %s", session.Meta.ID, session.Meta.DeviceMAC)
		}
	}

	var batch []SessionReplay
	if err := st.WithReadOnlyTx(ctx, func(tx pgx.Tx) error {
		var err error
		batch, err = NewTxView(tx).ReadReplaySessions(ctx, nil, 2)
		return err
	}); err != nil {
		t.Fatalf("replay batch: %v", err)
	}
	if len(batch) != 2 || batch[0].Meta.ID != "replay-a" || batch[1].Meta.ID != "replay-b" {
		t.Fatalf("batch = %+v", batch)
	}
}

func TestReadReplaySessionsRejectsNonPositiveLimit(t *testing.T) {
	st := newTestStore(t)
	err := st.WithReadOnlyTx(context.Background(), func(tx pgx.Tx) error {
		_, err := NewTxView(tx).ReadReplaySessions(context.Background(), nil, 0)
		return err
	})
	if err == nil {
		t.Fatal("zero page limit accepted")
	}
}

func TestIsFreshTargetRequiresEmptyV4Database(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	fresh := false
	if err := st.WithReadOnlyTx(ctx, func(tx pgx.Tx) error {
		var err error
		fresh, err = NewTxView(tx).IsFreshTarget(ctx)
		return err
	}); err != nil {
		t.Fatalf("fresh target: %v", err)
	}
	if !fresh {
		t.Fatal("new database is not reported as a fresh target")
	}

	started := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	seedSession(t, st, "fresh-check", started, started.Add(time.Hour))

	if err := st.WithReadOnlyTx(ctx, func(tx pgx.Tx) error {
		var err error
		fresh, err = NewTxView(tx).IsFreshTarget(ctx)
		return err
	}); err != nil {
		t.Fatalf("fresh target: %v", err)
	}
	if fresh {
		t.Fatal("database with a stored session is not a fresh target")
	}
}

func TestIdentityIsStableAndDistinguishesDatabases(t *testing.T) {
	first := newTestStore(t)
	second := newTestStore(t)
	ctx := context.Background()

	one, err := first.Identity(ctx)
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	two, err := second.Identity(ctx)
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	if one.SystemIdentifier == "" || one.Database == "" || one.DatabaseOID == 0 {
		t.Fatalf("incomplete identity: %+v", one)
	}
	if one.SystemIdentifier != two.SystemIdentifier {
		t.Fatalf("system identifiers differ within one cluster: %s vs %s",
			one.SystemIdentifier, two.SystemIdentifier)
	}
	if one.DatabaseOID == two.DatabaseOID {
		t.Fatalf("two databases share OID %d", one.DatabaseOID)
	}
}
