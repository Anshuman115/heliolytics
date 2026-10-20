package store

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestDayHelpersUseISTCalendarDates(t *testing.T) {
	cases := []struct {
		y    int
		m    time.Month
		d    int
		want string
	}{
		{2026, time.October, 19, "2026-10-19"},
		{2028, time.February, 29, "2028-02-29"},
		{2026, time.December, 31, "2026-12-31"},
		{2027, time.January, 1, "2027-01-01"},
	}
	for _, c := range cases {
		day := Day(c.y, c.m, c.d)
		if got := day.Format(dateLayout); got != c.want {
			t.Fatalf("Day(%d,%d,%d) = %s, want %s", c.y, c.m, c.d, got, c.want)
		}
		if _, offset := day.Zone(); offset != 19800 {
			t.Fatalf("IST offset = %d, want 19800", offset)
		}
		if !day.Equal(dayOf(day)) {
			t.Fatalf("dayOf(%s) moved the calendar date", c.want)
		}
	}
	if got := nextDay(Day(2028, time.February, 28)).Format(dateLayout); got != "2028-02-29" {
		t.Fatalf("nextDay over a leap day = %s", got)
	}
	if got := nextDay(Day(2026, time.December, 31)).Format(dateLayout); got != "2027-01-01" {
		t.Fatalf("nextDay over a year boundary = %s", got)
	}
}

func TestParseDayRejectsNonCalendarInput(t *testing.T) {
	for _, input := range []string{"", "2026-1-2", "2026-10-19T00:00:00Z", "20261019", "2026-13-01"} {
		if _, err := parseDay(input); err == nil {
			t.Fatalf("parseDay(%q) accepted invalid input", input)
		}
	}
	start, end, err := dayRange("2026-10-19", "2026-10-19")
	if err != nil {
		t.Fatalf("dayRange: %v", err)
	}
	if start.Format(time.RFC3339Nano) != "2026-10-19T00:00:00+05:30" {
		t.Fatalf("range start = %s", start.Format(time.RFC3339Nano))
	}
	if end.Format(time.RFC3339Nano) != "2026-10-20T00:00:00+05:30" {
		t.Fatalf("range end = %s", end.Format(time.RFC3339Nano))
	}
}

func TestISTDayBoundarySelectsExactSamples(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	instants := []time.Time{
		time.Date(2026, 10, 18, 18, 29, 59, 999999000, time.UTC),
		time.Date(2026, 10, 18, 18, 30, 0, 0, time.UTC),
		time.Date(2026, 10, 19, 18, 29, 59, 999999000, time.UTC),
		time.Date(2026, 10, 19, 18, 30, 0, 0, time.UTC),
	}
	seedSession(t, st, "ist-boundary", instants[0].Add(-time.Hour), instants[3])
	mustTx(t, st, func(tx pgx.Tx) error {
		rows := make([]HeartRateSample, 0, len(instants))
		for i, at := range instants {
			rows = append(rows, HeartRateSample{At: at, Source: "0x01", BPM: 60 + i})
		}
		_, err := st.WriteHeartRate(ctx, tx, rows, "ist-boundary", instants[3])
		return err
	})

	days, err := st.ListHeartRateSamplesCompact(ctx, "2026-10-19", "2026-10-19")
	if err != nil {
		t.Fatalf("compact: %v", err)
	}
	if len(days) != 1 {
		t.Fatalf("compact days = %d, want 1", len(days))
	}
	if days[0].DayKey != "2026-10-19" {
		t.Fatalf("dayKey = %s", days[0].DayKey)
	}
	if len(days[0].Values) != 2 {
		t.Fatalf("values = %v, want the two samples inside the IST day", days[0].Values)
	}
	if days[0].Offsets[1] != 86399 {
		t.Fatalf("last offset = %d, want 86399", days[0].Offsets[1])
	}
}

func TestISTDayKeysIgnoreSessionTimezone(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	at := time.Date(2026, 10, 18, 18, 30, 0, 0, time.UTC) // 2026-10-19 00:00 IST
	seedSession(t, st, "timezone-session", at.Add(-time.Hour), at)
	mustTx(t, st, func(tx pgx.Tx) error {
		_, err := st.WriteHeartRate(ctx, tx,
			[]HeartRateSample{{At: at, Source: "0x01", BPM: 61}}, "timezone-session", at)
		return err
	})

	conn, err := st.pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SET TIME ZONE 'America/New_York'`); err != nil {
		t.Fatalf("set time zone: %v", err)
	}
	var dayKey string
	if err := conn.QueryRow(ctx,
		`SELECT day_key::text FROM heart_rate_samples WHERE sampled_at = $1`, at).Scan(&dayKey); err != nil {
		t.Fatalf("day_key: %v", err)
	}
	if dayKey != "2026-10-19" {
		t.Fatalf("day_key under a foreign session timezone = %s, want 2026-10-19", dayKey)
	}

	mustTx(t, st, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SET LOCAL TIME ZONE 'America/New_York'`); err != nil {
			return err
		}
		supported, err := NewTxView(tx).ListCanonicalDayKeys(ctx, []time.Time{Day(2026, time.October, 19)})
		if err != nil {
			return err
		}
		if len(supported) != 1 || supported[0].Format(dateLayout) != "2026-10-19" {
			t.Fatalf("support probe under a foreign timezone = %v", supported)
		}
		return nil
	})
}
