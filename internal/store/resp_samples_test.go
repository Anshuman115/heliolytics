package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestRespSamplesRoundTrip(t *testing.T) {
	st := newTestStore(t)
	at := time.Date(2026, 10, 1, 2, 0, 0, 0, time.UTC)
	seedSession(t, st, "resp-round-trip", at.Add(-time.Hour), at)

	if _, err := writeRespTx(t, st, []RespSample{{At: at, BRMin: 14.5}}, "resp-round-trip", at); err != nil {
		t.Fatalf("write: %v", err)
	}
	rows, err := listResp(context.Background(), st.q, at.Add(-time.Hour), at.Add(time.Hour))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(rows) != 1 || rows[0].BRMin != 14.5 {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestRespSamplesRejectOutOfRange(t *testing.T) {
	st := newTestStore(t)
	at := time.Date(2026, 10, 1, 2, 0, 0, 0, time.UTC)
	seedSession(t, st, "resp-range", at.Add(-time.Hour), at)

	for _, value := range []float64{3.9, 40.1} {
		var invalid *InvalidRowError
		if _, err := writeRespTx(t, st, []RespSample{{At: at, BRMin: value}}, "resp-range", at); !errors.As(err, &invalid) {
			t.Fatalf("br_min=%v err = %v, want an invalid row error", value, err)
		}
	}
	if n := countRows(t, st, "resp_samples"); n != 0 {
		t.Fatalf("rows after rejected writes = %d, want 0", n)
	}
}

func writeRespTx(t *testing.T, st *Store, rows []RespSample, sessionID string, observedAt time.Time) (Mutation, error) {
	t.Helper()
	var out Mutation
	err := st.WithTx(context.Background(), func(tx pgx.Tx) error {
		var err error
		out, err = st.WriteResp(context.Background(), tx, rows, sessionID, observedAt)
		return err
	})
	return out, err
}
