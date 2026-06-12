package store

import (
	"context"
	"os"
	"testing"
	"time"
)

const coverageTestSessionID = "coverage-integration-test"

func testStore(t *testing.T) (*Store, func()) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = os.Getenv("DATABASE_URL")
	}
	if url == "" {
		t.Skip("TEST_DATABASE_URL or DATABASE_URL not set")
	}
	ctx := context.Background()
	st, err := New(ctx, url)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := st.Ping(ctx); err != nil {
		st.Close()
		t.Fatalf("Ping: %v", err)
	}
	return st, func() { st.Close() }
}

func cleanupCoverageFixture(ctx context.Context, st *Store) {
	_, _ = st.pool.Exec(ctx, `DELETE FROM sync_sessions WHERE session_id = $1`, coverageTestSessionID)
}

func TestGetCoverageIncludesWorkoutTypeKeys(t *testing.T) {
	st, cleanup := testStore(t)
	defer cleanup()

	cov, err := st.GetCoverage(context.Background())
	if err != nil {
		t.Fatalf("GetCoverage: %v", err)
	}
	for _, key := range SyncedBLETypeCodes {
		if _, ok := cov.Types[key]; !ok {
			t.Fatalf("Types missing key %s", key)
		}
	}
}

func TestGetCoverageWorkoutTypesNonNullWhenParsedWorkoutExists(t *testing.T) {
	st, cleanup := testStore(t)
	defer cleanup()

	ctx := context.Background()
	cleanupCoverageFixture(ctx, st)
	t.Cleanup(func() { cleanupCoverageFixture(ctx, st) })

	started := time.Date(2026, 6, 6, 14, 0, 0, 0, time.UTC)
	durationSec := 10786
	wantEnd := started.Add(time.Duration(durationSec) * time.Second)

	if _, err := st.pool.Exec(ctx, `
		INSERT INTO sync_sessions (session_id, device_mac, started_at, ingested_at)
		VALUES ($1, 'aa:bb:cc:dd:ee:ff', $2, $2)`,
		coverageTestSessionID, started,
	); err != nil {
		t.Fatalf("insert sync_session: %v", err)
	}
	if _, err := st.pool.Exec(ctx, `
		INSERT INTO workouts (sync_session_id, day_key, started_at, duration_sec, sport_type, sport_name)
		VALUES ($1, '2026-06-06', $2, $3, 92, 'Badminton')`,
		coverageTestSessionID, started, durationSec,
	); err != nil {
		t.Fatalf("insert workout: %v", err)
	}

	cov, err := st.GetCoverage(ctx)
	if err != nil {
		t.Fatalf("GetCoverage: %v", err)
	}
	for _, key := range []string{"0x05", "0x06"} {
		ts := cov.Types[key]
		if ts == nil {
			t.Fatalf("Types[%s] want non-null", key)
		}
		if !ts.UTC().Equal(wantEnd) {
			t.Fatalf("Types[%s] = %v want %v", key, ts.UTC(), wantEnd)
		}
	}
}

func TestGetCoverageActivityTypeNonNullWhenParsedSessionExists(t *testing.T) {
	st, cleanup := testStore(t)
	defer cleanup()

	ctx := context.Background()
	cleanupCoverageFixture(ctx, st)
	t.Cleanup(func() { cleanupCoverageFixture(ctx, st) })

	started := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
	durationSec := 3600
	wantEnd := started.Add(time.Duration(durationSec) * time.Second)

	if _, err := st.pool.Exec(ctx, `
		INSERT INTO sync_sessions (session_id, device_mac, started_at, ingested_at)
		VALUES ($1, 'aa:bb:cc:dd:ee:ff', $2, $2)`,
		coverageTestSessionID, started,
	); err != nil {
		t.Fatalf("insert sync_session: %v", err)
	}
	if _, err := st.pool.Exec(ctx, `
		INSERT INTO activity_sessions (sync_session_id, day_key, started_at, duration_sec, sport_type)
		VALUES ($1, '2026-06-05', $2, $3, 1)`,
		coverageTestSessionID, started, durationSec,
	); err != nil {
		t.Fatalf("insert activity_session: %v", err)
	}

	cov, err := st.GetCoverage(ctx)
	if err != nil {
		t.Fatalf("GetCoverage: %v", err)
	}
	ts := cov.Types["0x3B"]
	if ts == nil {
		t.Fatal("Types[0x3B] want non-null")
	}
	if !ts.UTC().Equal(wantEnd) {
		t.Fatalf("Types[0x3B] = %v want %v", ts.UTC(), wantEnd)
	}
}
