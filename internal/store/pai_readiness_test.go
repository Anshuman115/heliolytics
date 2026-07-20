package store

import (
	"context"
	"testing"
)

func TestUpsertPaiScoresThenReadBack(t *testing.T) {
	st, cleanup := testStore(t)
	t.Cleanup(cleanup)
	ctx := context.Background()
	const day = "2026-06-22"

	clean := func() { st.pool.Exec(ctx, `DELETE FROM raw_pai_scores WHERE day_key=$1::date`, day) }
	clean()
	t.Cleanup(clean)

	if err := st.UpsertPaiScores(ctx, "sess-pai-1", map[string]int{day: 81}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	var score int
	if err := st.pool.QueryRow(ctx,
		`SELECT score FROM raw_pai_scores WHERE day_key=$1::date`, day).Scan(&score); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if score != 81 {
		t.Fatalf("score=%d, want 81", score)
	}
}

func TestUpsertReadinessScoresThenReadBack(t *testing.T) {
	st, cleanup := testStore(t)
	t.Cleanup(cleanup)
	ctx := context.Background()
	const day = "2026-06-23"

	clean := func() { st.pool.Exec(ctx, `DELETE FROM raw_readiness WHERE day_key=$1::date`, day) }
	clean()
	t.Cleanup(clean)

	if err := st.UpsertReadinessScores(ctx, "sess-rdy-1", map[string]int{day: 72}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	var score int
	if err := st.pool.QueryRow(ctx,
		`SELECT score FROM raw_readiness WHERE day_key=$1::date`, day).Scan(&score); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if score != 72 {
		t.Fatalf("score=%d, want 72", score)
	}
}
