package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestSpo2SamplesRoundTrip(t *testing.T) {
	st := newTestStore(t)
	at := time.Date(2026, 10, 1, 3, 0, 0, 0, time.UTC)
	seedSession(t, st, "spo2-round-trip", at.Add(-time.Hour), at)

	mut, err := writeSpo2Tx(t, st, []SpO2Sample{{At: at, Source: "0x25", Mode: "auto", Pct: 97}}, "spo2-round-trip", at)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if !mut.Changed {
		t.Fatal("new sample did not report a change")
	}

	rows, err := listSpO2(context.Background(), st.q, at.Add(-time.Hour), at.Add(time.Hour))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(rows) != 1 || rows[0].Pct != 97 || rows[0].Mode != "auto" || rows[0].Source != "0x25" {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestSpo2SamplesRejectPlaceholderSource(t *testing.T) {
	st := newTestStore(t)
	at := time.Date(2026, 10, 1, 3, 0, 0, 0, time.UTC)
	seedSession(t, st, "spo2-source", at.Add(-time.Hour), at)

	_, err := writeSpo2Tx(t, st,
		[]SpO2Sample{{At: at, Source: "0x26", Mode: "auto", Pct: 97}}, "spo2-source", at)
	var invalid *InvalidRowError
	if !errors.As(err, &invalid) {
		t.Fatalf("err = %v, want an invalid row error", err)
	}
	if n := countRows(t, st, "spo2_samples"); n != 0 {
		t.Fatalf("rows = %d, want 0", n)
	}
}

func TestSpo2SamplesRejectInvalidModeAndPct(t *testing.T) {
	st := newTestStore(t)
	at := time.Date(2026, 10, 1, 3, 0, 0, 0, time.UTC)
	seedSession(t, st, "spo2-invalid", at.Add(-time.Hour), at)

	bad := []SpO2Sample{
		{At: at, Source: "0x25", Mode: "sleep", Pct: 97},
		{At: at, Source: "0x25", Mode: "auto", Pct: 69},
		{At: at, Source: "0x25", Mode: "manual", Pct: 101},
	}
	for _, sample := range bad {
		var invalid *InvalidRowError
		if _, err := writeSpo2Tx(t, st, []SpO2Sample{sample}, "spo2-invalid", at); !errors.As(err, &invalid) {
			t.Fatalf("sample %+v err = %v, want an invalid row error", sample, err)
		}
	}
}

func writeSpo2Tx(t *testing.T, st *Store, rows []SpO2Sample, sessionID string, observedAt time.Time) (Mutation, error) {
	t.Helper()
	var out Mutation
	err := st.WithTx(context.Background(), func(tx pgx.Tx) error {
		var err error
		out, err = st.WriteSpO2(context.Background(), tx, rows, sessionID, observedAt)
		return err
	})
	return out, err
}
