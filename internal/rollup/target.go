package rollup

import (
	"context"

	"github.com/heliolytics/api/internal/store"
)

// Target is implemented by the normal store and by the current ingest
// transaction. Rollups therefore use the same code in both contexts.
type Target interface {
	ExecDayKeys(context.Context, string, []string) error
	ExecSQL(context.Context, string, ...any) error
	ListSleep(context.Context, string, string) ([]store.SleepMetric, error)
	RecomputeReadiness(context.Context, []string) error
}

var _ Target = (*store.Store)(nil)
var _ Target = (*store.TxView)(nil)
