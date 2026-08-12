package store

import (
	"context"

	"github.com/heliolytics/api/internal/store/db"
	"github.com/jackc/pgx/v5"
)

// TxView exposes the small store surface needed by rollups while an ingest
// transaction is still open.
type TxView struct {
	tx pgx.Tx
	q  *db.Queries
}

func NewTxView(tx pgx.Tx) *TxView {
	return &TxView{tx: tx, q: db.New(tx)}
}

func (s *Store) ExecSQL(ctx context.Context, sql string, args ...any) error {
	_, err := s.pool.Exec(ctx, sql, args...)
	return err
}

func (v *TxView) ExecSQL(ctx context.Context, sql string, args ...any) error {
	_, err := v.tx.Exec(ctx, sql, args...)
	return err
}

func (v *TxView) ExecDayKeys(ctx context.Context, sql string, days []string) error {
	rows := make([]queuedRow, 0, len(days))
	for _, day := range days {
		rows = append(rows, queuedRow{day})
	}
	return execBatchTx(ctx, v.tx, sql, rows)
}

func (v *TxView) ListSleep(ctx context.Context, from, to string) ([]SleepMetric, error) {
	return listSleep(ctx, v.q, from, to)
}

func (v *TxView) RecomputeReadiness(ctx context.Context, days []string) error {
	return recomputeReadiness(ctx, v.tx, days)
}
