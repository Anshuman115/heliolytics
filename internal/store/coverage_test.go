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
		t.Skip("TEST_DATABASE_URL not set")
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
	_, _ = st.pool.Exec(ctx, `DELETE FROM sleep_sessions WHERE source_session_id = $1`, coverageTestSessionID)
	_, _ = st.pool.Exec(ctx, `DELETE FROM workouts WHERE source_session_id = $1`, coverageTestSessionID)
	_, _ = st.pool.Exec(ctx, `DELETE FROM activity_sessions WHERE source_session_id = $1`, coverageTestSessionID)
	_, _ = st.pool.Exec(ctx, `DELETE FROM step_samples WHERE source_session_id = $1`, coverageTestSessionID)
	_, _ = st.pool.Exec(ctx, `DELETE FROM raw_pai_scores WHERE source_session_id = $1`, coverageTestSessionID)
	_, _ = st.pool.Exec(ctx, `DELETE FROM daily_metrics WHERE source_session_id = $1`, coverageTestSessionID)
	_, _ = st.pool.Exec(ctx, `DELETE FROM stress_samples WHERE source_session_id = $1`, coverageTestSessionID)
	_, _ = st.pool.Exec(ctx, `DELETE FROM spo2_samples WHERE source_session_id = $1`, coverageTestSessionID)
	_, _ = st.pool.Exec(ctx, `DELETE FROM sync_sessions WHERE session_id = $1`, coverageTestSessionID)
}

func TestGetCoverageIncludesWorkoutTypeKeys(t *testing.T) {
	st, cleanup := testStore(t)
	t.Cleanup(cleanup)

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

func TestGetCoverageKeepsWorkoutTypeWatermarksSeparate(t *testing.T) {
	st, cleanup := testStore(t)
	t.Cleanup(cleanup)

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
		INSERT INTO workouts (source_session_id, day_key, started_at, duration_sec, sport_type, sport_name, has_summary)
		VALUES ($1, '2026-06-06', $2, $3, 92, 'Badminton', true)`,
		coverageTestSessionID, started, durationSec,
	); err != nil {
		t.Fatalf("insert workout: %v", err)
	}

	cov, err := st.GetCoverage(ctx)
	if err != nil {
		t.Fatalf("GetCoverage: %v", err)
	}
	if ts := cov.Types["0x05"]; ts == nil || !ts.UTC().Equal(wantEnd) {
		t.Fatalf("Types[0x05] = %v want %v", ts, wantEnd)
	}
	if ts := cov.Types["0x06"]; ts != nil {
		t.Fatalf("Types[0x06] = %v want nil without detail provenance", ts)
	}
}

func TestGetCoverageIncludesDerivedActivityInDataThrough(t *testing.T) {
	st, cleanup := testStore(t)
	t.Cleanup(cleanup)

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
		INSERT INTO activity_sessions (source_session_id, day_key, started_at, duration_sec, sport_type)
		VALUES ($1, '2026-06-05', $2, $3, 1)`,
		coverageTestSessionID, started, durationSec,
	); err != nil {
		t.Fatalf("insert activity_session: %v", err)
	}

	cov, err := st.GetCoverage(ctx)
	if err != nil {
		t.Fatalf("GetCoverage: %v", err)
	}
	if cov.DataThrough == nil || !cov.DataThrough.UTC().Equal(wantEnd) {
		t.Fatalf("DataThrough = %v want derived activity end %v", cov.DataThrough, wantEnd)
	}
	if _, exists := cov.Types["0x3B"]; exists {
		t.Fatal("unparsed 0x3B must not be advertised as synced coverage")
	}
}
