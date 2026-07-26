package store

import (
	"context"
	"testing"
	"time"
)

func TestGetCoverageSleepTypesNonNullWhenParsedSleepExists(t *testing.T) {
	st, cleanup := testStore(t)
	t.Cleanup(cleanup)

	ctx := context.Background()
	cleanupCoverageFixture(ctx, st)
	t.Cleanup(func() { cleanupCoverageFixture(ctx, st) })

	napStart := time.Date(2026, 6, 7, 8, 39, 0, 0, time.UTC)
	napMins := 135
	mainStart := time.Date(2026, 6, 7, 22, 0, 0, 0, time.UTC)
	mainMins := 420
	mainStageStart := time.Date(2026, 6, 6, 20, 0, 0, 0, time.UTC)
	mainEnd := time.Date(2026, 6, 7, 4, 0, 0, 0, time.UTC)

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
			INSERT INTO sleep_sessions (source_session_id, day_key, started_at, total_mins, is_nap)
			VALUES ($1, $2, $3, $4, $5)`,
			coverageTestSessionID, row.day, row.start, row.mins, row.nap,
		); err != nil {
			t.Fatalf("insert sleep_session: %v", err)
		}
	}
	if _, err := st.pool.Exec(ctx, `
		UPDATE sleep_sessions SET stages_json = jsonb_build_array(
		  jsonb_build_object('start', $1::timestamptz, 'end', $2::timestamptz, 'type', 4)
		) WHERE source_session_id=$3 AND is_nap=false`,
		mainStageStart, mainEnd, coverageTestSessionID,
	); err != nil {
		t.Fatalf("add sleep stages: %v", err)
	}

	cov, err := st.GetCoverage(ctx)
	if err != nil {
		t.Fatalf("GetCoverage: %v", err)
	}
	if ts := cov.Types["0x48"]; ts == nil || !ts.UTC().Equal(mainEnd) {
		t.Fatalf("Types[0x48] = %v want %v", ts, mainEnd)
	}
}

func TestGetCoverageVitalsAndDailyNonNullWhenSeeded(t *testing.T) {
	st, cleanup := testStore(t)
	t.Cleanup(cleanup)

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
		INSERT INTO daily_metrics (day_key, steps, pai_score, updated_at, source_session_id)
		VALUES ('2026-06-08', 5000, 42, $1, $2)`,
		seedAt.Add(12*time.Hour), coverageTestSessionID,
	); err != nil {
		t.Fatalf("insert daily_metrics: %v", err)
	}
	if _, err := st.pool.Exec(ctx, `
		INSERT INTO step_samples (sampled_at, day_key, steps, source_session_id)
		VALUES ($1, '2026-06-08', 10, $2)`,
		seedAt, coverageTestSessionID,
	); err != nil {
		t.Fatalf("insert step_sample: %v", err)
	}
	if _, err := st.pool.Exec(ctx, `
		INSERT INTO raw_pai_scores (day_key, score, source_session_id)
		VALUES ('2026-06-08', 42, $1)
		ON CONFLICT (day_key) DO UPDATE SET score=EXCLUDED.score, source_session_id=EXCLUDED.source_session_id`,
		coverageTestSessionID,
	); err != nil {
		t.Fatalf("insert pai score: %v", err)
	}
	if _, err := st.pool.Exec(ctx, `
		INSERT INTO stress_samples (sampled_at, day_key, value, source_session_id)
		VALUES ($1, '2026-06-08', 35, $2)`,
		seedAt, coverageTestSessionID,
	); err != nil {
		t.Fatalf("insert stress_sample: %v", err)
	}

	cov, err := st.GetCoverage(ctx)
	if err != nil {
		t.Fatalf("GetCoverage: %v", err)
	}
	if cov.Types["0x01"] == nil || !cov.Types["0x01"].UTC().Equal(seedAt) {
		t.Fatalf("Types[0x01] = %v want sample time %v", cov.Types["0x01"], seedAt)
	}
	if cov.Types["0x0D"] == nil {
		t.Fatal("Types[0x0D] want non-null")
	}
	if cov.Types["0x13"] == nil || !cov.Types["0x13"].UTC().Equal(seedAt) {
		t.Fatalf("Types[0x13] = %v want %v", cov.Types["0x13"], seedAt)
	}
}
