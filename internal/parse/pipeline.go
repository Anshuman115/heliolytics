package parse

import (
	"context"
	"fmt"
	"time"

	"github.com/heliolytics/api/internal/store"
)

// RunIngest parses raw sync blobs, then persists rows and recomputes touched
// daily metrics inside one transaction.
func RunIngest(
	ctx context.Context,
	st *store.Store,
	meta store.SessionMeta,
	blobs map[string][]byte,
	fetchEnd time.Time,
) error {
	parsed, err := ParseBlobs(meta.CatalogJSON, blobs, fetchEnd)
	if err != nil {
		return fmt.Errorf("parse blobs: %w", err)
	}
	agg := Aggregate(parsed)
	return WriteBatch(ctx, st, meta.ID, meta, blobs, agg)
}
