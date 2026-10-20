package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestWriteSessionNanosecondEnvelopeRetry(t *testing.T) {
	st := newTestStore(t)
	started := time.Date(2026, 10, 1, 0, 0, 0, 123456789, time.UTC)
	meta := SessionMeta{ID: "precision-session", DeviceMAC: "02:00:00:00:00:01", StartedAt: started, EndedAt: started.Add(time.Hour)}
	blobs := []RawBlob{{TypeCode: "0x05", Payload: []byte{1, 2, 3}}}
	existing, err := writeSessionEnvelope(t, st, meta, `{}`, blobs)
	if err != nil || existing {
		t.Fatalf("first write existing=%v err=%v, want false and nil", existing, err)
	}
	var persistedStart, persistedEnd time.Time
	if err := st.pool.QueryRow(context.Background(), "SELECT started_at, ended_at FROM sync_sessions WHERE session_id=$1", meta.ID).Scan(&persistedStart, &persistedEnd); err != nil {
		t.Fatal(err)
	}
	if persistedStart.Nanosecond() != 123456000 || persistedEnd.Nanosecond() != 123456000 {
		t.Fatalf("persisted fractions = %d/%d, want truncation to 123456000", persistedStart.Nanosecond(), persistedEnd.Nanosecond())
	}
	existing, err = writeSessionEnvelope(t, st, meta, `{}`, blobs)
	if err != nil || !existing {
		t.Fatalf("identical retry existing=%v err=%v, want true and nil", existing, err)
	}
	for _, field := range []string{"start", "end"} {
		changed := meta
		if field == "start" {
			changed.StartedAt = changed.StartedAt.Add(time.Microsecond)
		} else {
			changed.EndedAt = changed.EndedAt.Add(time.Microsecond)
		}
		if _, err := writeSessionEnvelope(t, st, changed, `{}`, blobs); !errors.Is(err, ErrSessionConflict) {
			t.Errorf("changed %s error = %v, want session conflict", field, err)
		}
	}
}

func TestNanosecondObservationOrderMatchesSessionOrder(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	started := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	base := started.Add(2*time.Hour + 123456000*time.Nanosecond)
	row := workoutCandidate(started, started.Add(time.Hour))
	for _, candidate := range []struct {
		id     string
		nanos  time.Duration
		detail bool
	}{
		{id: "z-tied", nanos: 100, detail: true},
		{id: "a-tied", nanos: 789, detail: false},
	} {
		meta := SessionMeta{ID: candidate.id, DeviceMAC: "02:00:00:00:00:01", StartedAt: started, EndedAt: base.Add(candidate.nanos * time.Nanosecond)}
		row.HasDetail = candidate.detail
		mustTx(t, st, func(tx pgx.Tx) error {
			existing, err := st.WriteSession(ctx, tx, meta, []byte(`{}`), nil)
			if err != nil {
				return err
			}
			_, err = st.WriteWorkouts(ctx, tx, []ActivityRow{row}, meta.ID, meta.EndedAt, existing)
			return err
		})
	}
	rows := canonicalWorkouts(t, st)
	if len(rows) != 1 || !rows[0].HasDetail {
		t.Fatalf("canonical workouts = %+v, want z-tied detail winner at equal persisted microsecond", rows)
	}
}
