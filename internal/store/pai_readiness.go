package store

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// UpsertPaiScores and UpsertReadinessScores persist the single per-day value
// parsed from the device's raw PAI (0x0D) / readiness (0x39) blobs. Both go
// through execBatch for uniform chunked batching, same write path as every
// other bulk upsert in this package.
const upsertRawPaiScoreSQL = `
	INSERT INTO raw_pai_scores (day_key, score, source_session_id, updated_at)
	VALUES ($1::date, $2, $3, NOW())
	ON CONFLICT (day_key) DO UPDATE SET
		score = EXCLUDED.score,
		source_session_id = EXCLUDED.source_session_id,
		updated_at = NOW()`

const upsertRawReadinessSQL = `
	INSERT INTO raw_readiness (day_key, score, source_session_id, updated_at)
	VALUES ($1::date, $2, $3, NOW())
	ON CONFLICT (day_key) DO UPDATE SET
		score = EXCLUDED.score,
		source_session_id = EXCLUDED.source_session_id,
		updated_at = NOW()`

func (s *Store) UpsertPaiScores(ctx context.Context, sid string, scores map[string]int) error {
	if sid == "" {
		return errRequired("raw_pai_scores.source_session_id")
	}
	rows := make([]queuedRow, 0, len(scores))
	for day, score := range scores {
		rows = append(rows, queuedRow{day, score, sid})
	}
	return s.execBatch(ctx, upsertRawPaiScoreSQL, rows)
}

func (s *Store) UpsertReadinessScores(ctx context.Context, sid string, scores map[string]int) error {
	if sid == "" {
		return errRequired("raw_readiness.source_session_id")
	}
	rows := make([]queuedRow, 0, len(scores))
	for day, score := range scores {
		rows = append(rows, queuedRow{day, score, sid})
	}
	return s.execBatch(ctx, upsertRawReadinessSQL, rows)
}

// UpsertPaiScoresTx is UpsertPaiScores run against an already-open
// transaction, for use inside Store.WithTx.
func (s *Store) UpsertPaiScoresTx(ctx context.Context, tx pgx.Tx, sid string, scores map[string]int) error {
	if sid == "" {
		return errRequired("raw_pai_scores.source_session_id")
	}
	rows := make([]queuedRow, 0, len(scores))
	for day, score := range scores {
		rows = append(rows, queuedRow{day, score, sid})
	}
	return execBatchTx(ctx, tx, upsertRawPaiScoreSQL, rows)
}

// UpsertReadinessScoresTx is UpsertReadinessScores run against an
// already-open transaction, for use inside Store.WithTx.
func (s *Store) UpsertReadinessScoresTx(ctx context.Context, tx pgx.Tx, sid string, scores map[string]int) error {
	if sid == "" {
		return errRequired("raw_readiness.source_session_id")
	}
	rows := make([]queuedRow, 0, len(scores))
	for day, score := range scores {
		rows = append(rows, queuedRow{day, score, sid})
	}
	return execBatchTx(ctx, tx, upsertRawReadinessSQL, rows)
}
