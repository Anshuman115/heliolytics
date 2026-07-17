package parse

import (
	"context"
	"time"

	"github.com/heliolytics/api/internal/store"
)

// RunIngest parses raw sync blobs and persists the result inside a single
// transaction, then recomputes daily_metrics for every touched day. See
// WriteBatch for why the rollup step must happen after commit, not during.
func RunIngest(
	ctx context.Context,
	st *store.Store,
	meta store.SessionMeta,
	blobs map[string][]byte,
	fetchEnd time.Time,
) error {
	parsed := ParseBlobs(meta.CatalogJSON, blobs, fetchEnd)
	agg := Aggregate(parsed)
	return WriteBatch(ctx, st, meta.ID, meta, blobs, agg)
}
