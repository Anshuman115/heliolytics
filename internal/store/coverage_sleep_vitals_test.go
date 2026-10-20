package store

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestSameInstantTwoHeartRateSourcesCoexist(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 24, 20, 0, 0, 0, time.UTC)
	observedAt := at.Add(time.Hour)
	seedSession(t, st, "hr-sources", at.Add(-time.Hour), observedAt)

	write := func(source string, bpm int) {
		mustTx(t, st, func(tx pgx.Tx) error {
			_, err := st.WriteHeartRate(ctx, tx,
				[]HeartRateSample{{At: at, Source: source, BPM: bpm}}, "hr-sources", observedAt)
			return err
		})
	}
	write("0x01", 60)
	write("0x46", 120)

	if n := countRows(t, st, "heart_rate_samples"); n != 2 {
		t.Fatalf("rows = %d, want both sources at one instant", n)
	}

	days, err := st.ListHeartRateSamplesCompact(ctx, "2026-09-24", "2026-09-25")
	if err != nil {
		t.Fatalf("compact: %v", err)
	}
	if len(days) != 1 || len(days[0].Values) != 1 || days[0].Values[0] != 60 {
		t.Fatalf("compact hr = %+v, want source 0x01 only", days)
	}

	write("0x46", 120)
	if n := countRows(t, st, "heart_rate_samples"); n != 2 {
		t.Fatalf("rows after retry = %d, want 2", n)
	}
}

func TestTemperatureWearProxyUsesCanonicalHeartRate(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	at := time.Date(2026, 10, 1, 2, 30, 0, 0, time.UTC)
	observedAt := at.Add(time.Hour)
	seedSession(t, st, "wear-proxy", at.Add(-time.Hour), observedAt)

	mustTx(t, st, func(tx pgx.Tx) error {
		_, err := st.WriteTemperature(ctx, tx,
			[]TemperatureSample{{At: at, CentiC: 3627}}, "wear-proxy", observedAt)
		return err
	})
	if got := temperatureWear(t, st, at); got != false {
		t.Fatal("proxy set without any heart rate")
	}

	mustTx(t, st, func(tx pgx.Tx) error {
		_, err := st.WriteHeartRate(ctx, tx,
			[]HeartRateSample{{At: at, Source: "0x46", BPM: 90}}, "wear-proxy", observedAt)
		return err
	})
	refreshWearProxy(t, st, at)
	if got := temperatureWear(t, st, at); got {
		t.Fatal("0x46 alone must not qualify as the wear proxy heart rate")
	}

	mustTx(t, st, func(tx pgx.Tx) error {
		_, err := st.WriteHeartRate(ctx, tx,
			[]HeartRateSample{{At: at, Source: "0x01", BPM: 61}}, "wear-proxy", observedAt)
		return err
	})
	mut := refreshWearProxy(t, st, at)
	if !mut.Changed {
		t.Fatal("proxy flip did not report a change")
	}
	if got := temperatureWear(t, st, at); !got {
		t.Fatal("proxy not set with same-minute canonical 0x01 heart rate")
	}
	if got := storedCenti(t, st, at); got != 3627 {
		t.Fatalf("stored centi-degrees = %d, want 3627", got)
	}

	outOfDomain := at.Add(3 * time.Minute)
	mustTx(t, st, func(tx pgx.Tx) error {
		if _, err := st.WriteHeartRate(ctx, tx,
			[]HeartRateSample{{At: outOfDomain, Source: "0x01", BPM: 61}}, "wear-proxy", observedAt); err != nil {
			return err
		}
		_, err := st.WriteTemperature(ctx, tx,
			[]TemperatureSample{{At: outOfDomain, CentiC: 2410}}, "wear-proxy", observedAt)
		return err
	})
	refreshWearProxy(t, st, outOfDomain)
	if got := temperatureWear(t, st, outOfDomain); got {
		t.Fatal("out-of-domain temperature must not be counted as on-wrist")
	}
}

func refreshWearProxy(t *testing.T, st *Store, at time.Time) Mutation {
	t.Helper()
	var out Mutation
	mustTx(t, st, func(tx pgx.Tx) error {
		var err error
		out, err = NewTxView(tx).RecomputeTemperatureWearProxy(context.Background(), at, at.Add(time.Minute))
		return err
	})
	return out
}

func temperatureWear(t *testing.T, st *Store, at time.Time) bool {
	t.Helper()
	var onWrist bool
	if err := st.pool.QueryRow(context.Background(),
		`SELECT on_wrist FROM temperature_samples WHERE sampled_at = $1`, at).Scan(&onWrist); err != nil {
		t.Fatalf("read on_wrist: %v", err)
	}
	return onWrist
}

func storedCenti(t *testing.T, st *Store, at time.Time) int {
	t.Helper()
	var centi int
	if err := st.pool.QueryRow(context.Background(),
		`SELECT (celsius * 100)::int FROM temperature_samples WHERE sampled_at = $1`, at).Scan(&centi); err != nil {
		t.Fatalf("read celsius: %v", err)
	}
	return centi
}
