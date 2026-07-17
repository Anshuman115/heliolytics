package store

import (
	"context"

	"github.com/heliolytics/api/internal/store/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

func New(ctx context.Context, dbURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return nil, err
	}
	return &Store{pool: pool, q: db.New(pool)}, nil
}

func (s *Store) Close() { s.pool.Close() }

func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

// Pool exposes the underlying connection pool so internal/rollup can run
// per-day recompute queries without store internals leaking further.
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// ExecDayKeys runs sql once per day in days, inside one transaction, using
// execBatch's existing chunking. Exported so internal/rollup can drive
// per-day UPDATE statements without reaching into store internals.
func (s *Store) ExecDayKeys(ctx context.Context, sql string, days []string) error {
	rows := make([]queuedRow, 0, len(days))
	for _, d := range days {
		rows = append(rows, queuedRow{d})
	}
	return s.execBatch(ctx, sql, rows)
}

// ensureDailyMetricsRowSQL inserts a daily_metrics row for a day if one
// doesn't already exist, so per-day recompute writers always have a row to
// update. Chunked via execBatchTx like every other bulk write, so a large
// backfill's touched-day set is one pipelined round trip, not N.
const ensureDailyMetricsRowSQL = `INSERT INTO daily_metrics (day_key) VALUES ($1::date) ON CONFLICT (day_key) DO NOTHING`

// EnsureDailyMetricsRowsTx runs ensureDailyMetricsRowSQL for each day in days
// against an already-open transaction, for use inside Store.WithTx.
func (s *Store) EnsureDailyMetricsRowsTx(ctx context.Context, tx pgx.Tx, days []string) error {
	rows := make([]queuedRow, 0, len(days))
	for _, day := range days {
		rows = append(rows, queuedRow{day})
	}
	return execBatchTx(ctx, tx, ensureDailyMetricsRowSQL, rows)
}
