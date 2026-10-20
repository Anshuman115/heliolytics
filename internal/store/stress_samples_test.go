package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestStressSamplesRoundTrip(t *testing.T) {
	st := newTestStore(t)
	at := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	seedSession(t, st, "stress-round-trip", at.Add(-time.Hour), at)

	if _, err := writeStressTx(t, st, []StressSample{{At: at, Stress: 38}}, "stress-round-trip", at); err != nil {
		t.Fatalf("write: %v", err)
	}
	rows, err := listStress(context.Background(), st.q, at.Add(-time.Hour), at.Add(time.Hour))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(rows) != 1 || rows[0].Stress != 38 {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestStressSamplesPreserveObservedZero(t *testing.T) {
	st := newTestStore(t)
	at := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	seedSession(t, st, "stress-zero", at.Add(-time.Hour), at)

	if _, err := writeStressTx(t, st, []StressSample{{At: at, Stress: 0}}, "stress-zero", at); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := scalarString(t, st,
		`SELECT stress::text FROM stress_samples WHERE sampled_at = $1`, at); got != "0" {
		t.Fatalf("stored stress = %s, want a measured zero", got)
	}
}

func TestStressSamplesRejectOutOfRange(t *testing.T) {
	st := newTestStore(t)
	at := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	seedSession(t, st, "stress-range", at.Add(-time.Hour), at)

	for _, value := range []int{-1, 101} {
		var invalid *InvalidRowError
		if _, err := writeStressTx(t, st, []StressSample{{At: at, Stress: value}}, "stress-range", at); !errors.As(err, &invalid) {
			t.Fatalf("stress=%d err = %v, want an invalid row error", value, err)
		}
	}
}

func writeStressTx(t *testing.T, st *Store, rows []StressSample, sessionID string, observedAt time.Time) (Mutation, error) {
	t.Helper()
	var out Mutation
	err := st.WithTx(context.Background(), func(tx pgx.Tx) error {
		var err error
		out, err = st.WriteStress(context.Background(), tx, rows, sessionID, observedAt)
		return err
	})
	return out, err
}
