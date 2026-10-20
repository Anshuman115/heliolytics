package ingest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Anshuman115/heliolytics/internal/store"
	"github.com/jackc/pgx/v5"
)

func TestRunSummaryReplacementRefreshesDetailCoverage(t *testing.T) {
	ctx := context.Background()
	source := openStore(t)
	fixture := loadFixture(t)
	detailed := fixture.meta
	detailed.ID = "detailed-workout"
	catalog := []byte(`{"chunked":[{"code":"0x05","status":"ok"},{"code":"0x06","status":"ok"}]}`)
	blobs := map[string][]byte{"0x05": fixture.blobs["0x05"], "0x06": fixture.blobs["0x06"]}
	if _, err := Run(ctx, source, detailed, catalog, blobs); err != nil {
		t.Fatal(err)
	}
	before, err := source.GetCoverage(ctx)
	if err != nil || before.Types["0x06"] == nil {
		t.Fatalf("detailed coverage = %+v, err=%v", before, err)
	}
	summary := detailed
	summary.ID = "summary-workout"
	summary.EndedAt = summary.EndedAt.Add(time.Hour)
	result, err := Run(ctx, source, summary,
		[]byte(`{"chunked":[{"code":"0x05","status":"ok"}]}`),
		map[string][]byte{"0x05": fixture.blobs["0x05"]})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Types) != 1 || result.Types["0x05"].DataThrough == nil {
		t.Fatalf("summary response types = %+v, want only surviving 0x05", result.Types)
	}
	assertCoverage := func(t *testing.T, st *store.Store) {
		t.Helper()
		coverage, err := st.GetCoverage(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if coverage.Types["0x06"] != nil {
			t.Errorf("detail coverage = %s, want nil after summary-only replacement", coverage.Types["0x06"])
		}
		if coverage.Types["0x05"] == nil || !coverage.Types["0x05"].Equal(*before.Types["0x05"]) {
			t.Errorf("summary coverage = %v, want %v", coverage.Types["0x05"], before.Types["0x05"])
		}
	}
	t.Run("live", func(t *testing.T) { assertCoverage(t, source) })
	t.Run("replay", func(t *testing.T) {
		target := openStore(t)
		if err := source.WithReadOnlyTx(ctx, func(stx pgx.Tx) error {
			return target.WithTx(ctx, func(ttx pgx.Tx) error {
				return replay(ctx, store.NewTxView(stx), target, ttx, nil)
			})
		}); err != nil {
			t.Fatal(err)
		}
		assertCoverage(t, target)
	})
}

func TestRunNanosecondEnvelopeRetryAndReplay(t *testing.T) {
	ctx := context.Background()
	source := openStore(t)
	fixture := loadFixture(t)
	meta := fixture.meta
	meta.StartedAt = meta.StartedAt.Add(123456789 * time.Nanosecond)
	meta.EndedAt = meta.EndedAt.Add(123456789 * time.Nanosecond)
	catalog := []byte(`{"chunked":[{"code":"0x05","status":"ok"}]}`)
	blobs := map[string][]byte{"0x05": fixture.blobs["0x05"]}
	if _, err := Run(ctx, source, meta, catalog, blobs); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(ctx, source, meta, catalog, blobs); err != nil {
		t.Fatalf("identical nanosecond envelope retry: %v", err)
	}
	changed := meta
	changed.EndedAt = changed.EndedAt.Add(time.Microsecond)
	if _, err := Run(ctx, source, changed, catalog, blobs); !errors.Is(err, store.ErrSessionConflict) {
		t.Fatalf("changed envelope error = %v, want session conflict", err)
	}
	target := openStore(t)
	if err := source.WithReadOnlyTx(ctx, func(stx pgx.Tx) error {
		return target.WithTx(ctx, func(ttx pgx.Tx) error {
			return replay(ctx, store.NewTxView(stx), target, ttx, nil)
		})
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(ctx, target, meta, catalog, blobs); err != nil {
		t.Fatalf("original envelope retry after replay: %v", err)
	}
}
