package store

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestHRVSamplesRoundTrip(t *testing.T) {
	st := newTestStore(t)
	at := time.Date(2026, 10, 1, 20, 0, 0, 0, time.UTC)
	seedSession(t, st, "hrv-round-trip", at.Add(-time.Hour), at)

	mut, err := writeHRVTx(t, st, []HRVSample{{At: at, RMSSDMS: 48.5}}, "hrv-round-trip", at)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if !mut.Changed {
		t.Fatal("new sample did not report a change")
	}

	rows, err := listHRV(context.Background(), st.q, at.Add(-time.Hour), at.Add(time.Hour))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(rows) != 1 || rows[0].RMSSDMS != 48.5 {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestHRVSamplesRejectOutOfRangeValues(t *testing.T) {
	st := newTestStore(t)
	at := time.Date(2026, 10, 1, 20, 0, 0, 0, time.UTC)
	seedSession(t, st, "hrv-range", at.Add(-time.Hour), at)

	for _, value := range []float64{0, -1, 301, math.NaN(), math.Inf(1)} {
		_, err := writeHRVTx(t, st, []HRVSample{{At: at, RMSSDMS: value}}, "hrv-range", at)
		var invalid *InvalidRowError
		if !errors.As(err, &invalid) {
			t.Fatalf("rmssd=%v err = %v, want an invalid row error", value, err)
		}
	}
	if n := countRows(t, st, "hrv_samples"); n != 0 {
		t.Fatalf("rows after rejected writes = %d, want 0", n)
	}
}

func writeHRVTx(t *testing.T, st *Store, rows []HRVSample, sessionID string, observedAt time.Time) (Mutation, error) {
	t.Helper()
	var out Mutation
	err := st.WithTx(context.Background(), func(tx pgx.Tx) error {
		var err error
		out, err = st.WriteHRV(context.Background(), tx, rows, sessionID, observedAt)
		return err
	})
	return out, err
}
