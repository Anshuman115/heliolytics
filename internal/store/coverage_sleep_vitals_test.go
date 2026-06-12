package store

import (
	"context"
	"testing"
	"time"
)

func TestGetCoverageSleepTypesNonNullWhenParsedSleepExists(t *testing.T) {
	st, cleanup := testStore(t)
	defer cleanup()

	ctx := context.Background()
	cleanupCoverageFixture(ctx, st)
	t.Cleanup(func() { cleanupCoverageFixture(ctx, st) })

	napStart := time.Date(2026, 6, 7, 8, 39, 0, 0, time.UTC)
	napMins := 135
	napEnd := napStart.Add(time.Duration(napMins) * time.Minute)
	mainStart := time.Date(2026, 6, 7, 22, 0, 0, 0, time.UTC)
	mainMins := 420
	mainEnd := mainStart.Add(time.Duration(mainMins) * time.Minute)

	if _, err := st.pool.Exec(ctx, `
		INSERT INTO sync_sessions (session_id, device_mac, started_at, ingested_at)
		VALUES ($1, 'aa:bb:cc:dd:ee:ff', $2, $2)`,
		coverageTestSessionID, napStart,
	); err != nil {
		t.Fatalf("insert sync_session: %v", err)
	}
	for _, row := range []struct {
		start time.Time
		mins  int
		day   string
		nap   bool
	}{
		{napStart, napMins, "2026-06-07", true},
		{mainStart, mainMins, "2026-06-07", false},
	} {
		if _, err := st.pool.Exec(ctx, `
			INSERT INTO sleep_sessions (sync_session_id, day_key, started_at, total_mins, is_nap)
			VALUES ($1, $2, $3, $4, $5)`,
			coverageTestSessionID, row.day, row.start, row.mins, row.nap,
		); err != nil {
			t.Fatalf("insert sleep_session: %v", err)
		}
	}

	cov, err := st.GetCoverage(ctx)
	if err != nil {
		t.Fatalf("GetCoverage: %v", err)
	}
	if ts := cov.Types["0x4E"]; ts == nil || !ts.UTC().Equal(napEnd) {
		t.Fatalf("Types[0x4E] = %v want %v", ts, napEnd)
	}
	if ts := cov.Types["0x48"]; ts == nil || !ts.UTC().Equal(mainEnd) {
		t.Fatalf("Types[0x48] = %v want %v", ts, mainEnd)
	}
}

func TestGetCoverageVitalsAndDailyNonNullWhenSeeded(t *testing.T) {
	st, cleanup := testStore(t)
	defer cleanup()

	ctx := context.Background()
	cleanupCoverageFixture(ctx, st)
	t.Cleanup(func() { cleanupCoverageFixture(ctx, st) })

	seedAt := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
	if _, err := st.pool.Exec(ctx, `
		INSERT INTO sync_sessions (session_id, device_mac, started_at, ingested_at)
		VALUES ($1, 'aa:bb:cc:dd:ee:ff', $2, $2)`,
		coverageTestSessionID, seedAt,
	); err != nil {
		t.Fatalf("insert sync_session: %v", err)
	}
	if _, err := st.pool.Exec(ctx, `
		INSERT INTO daily_metrics (day_key, steps, pai_score, updated_at)
		VALUES ('2026-06-08', 5000, 42, $1)`,
		seedAt,
	); err != nil {
		t.Fatalf("insert daily_metrics: %v", err)
	}
	if _, err := st.pool.Exec(ctx, `
		INSERT INTO health_samples (sync_session_id, metric, day_key, sampled_at, value)
		VALUES ($1, 'stress', '2026-06-08', $2, 35)`,
		coverageTestSessionID, seedAt,
	); err != nil {
		t.Fatalf("insert health_sample: %v", err)
	}

	cov, err := st.GetCoverage(ctx)
	if err != nil {
		t.Fatalf("GetCoverage: %v", err)
	}
	if cov.Types["0x01"] == nil {
		t.Fatal("Types[0x01] want non-null")
	}
	if cov.Types["0x0D"] == nil {
		t.Fatal("Types[0x0D] want non-null")
	}
	if cov.Types["0x13"] == nil || !cov.Types["0x13"].UTC().Equal(seedAt) {
		t.Fatalf("Types[0x13] = %v want %v", cov.Types["0x13"], seedAt)
	}
}
