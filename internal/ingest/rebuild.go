package ingest

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/Anshuman115/heliolytics/internal/parse"
	"github.com/Anshuman115/heliolytics/internal/rollup"
	"github.com/Anshuman115/heliolytics/internal/store"
	"github.com/jackc/pgx/v5"
)

// ErrRebuildSameDatabase refuses a rebuild whose source and target are one
// database reached through two endpoints.
var ErrRebuildSameDatabase = errors.New("rebuild source and target are the same database")

// DisposableDatabase is one live-inspected disposable test database. The
// caller obtains these facts through Docker inspection and the owner-only
// identity query; a string prefix never authorizes anything on its own.
type DisposableDatabase struct {
	ContainerID string
	Project     string
	Volume      string
	Host        string
	Port        uint16
	Identity    store.DatabaseIdentity
}

// RebuildTestScope is the explicit test-only rebuild scope.
type RebuildTestScope struct {
	Source DisposableDatabase
	Target DisposableDatabase
}

const (
	productionDatabase = "heliolytics"
	productionProject  = "deploy"
	productionVolume   = "deploy_pgdata"
	smokeSource        = "heliolytics_smoke"
	replayPageSize     = 100
)

var (
	testDatabaseName    = regexp.MustCompile(`^heliolytics_test_[0-9a-f]{16}$`)
	rebuildSourceName   = regexp.MustCompile(`^heliolytics_rebuild_source_[0-9a-z]{1,32}$`)
	rebuildTargetName   = regexp.MustCompile(`^heliolytics_rebuild_target(_[0-9a-z]{1,32})?$`)
	loopbackHosts       = map[string]struct{}{"127.0.0.1": {}, "::1": {}}
	errTargetNotFresh   = errors.New("rebuild target is not a fresh database with the current schema")
	errIdentityMismatch = errors.New("rebuild resource identity mismatch")
)

// RebuildAll replays a disposable test source into a different, empty,
// same-schema disposable test target. It never populates production and has no
// server caller.
func RebuildAll(ctx context.Context, source, target *store.Store, scope RebuildTestScope,
	log func(string)) error {
	if err := scope.Source.validate("source"); err != nil {
		return err
	}
	if err := scope.Target.validate("target"); err != nil {
		return err
	}
	if scope.Source.Identity == scope.Target.Identity {
		return ErrRebuildSameDatabase
	}
	if err := verifyIdentity(ctx, source, scope.Source, "source"); err != nil {
		return err
	}
	if err := verifyIdentity(ctx, target, scope.Target, "target"); err != nil {
		return err
	}
	logf(log, "rebuild: scope verified, replaying source into the empty target")

	return source.WithReadOnlyTx(ctx, func(stx pgx.Tx) error {
		sourceView := store.NewTxView(stx)
		profile, err := sourceView.GetProfile(ctx)
		if err != nil {
			return err
		}
		return target.WithTx(ctx, func(ttx pgx.Tx) error {
			if err := target.LockCanonical(ctx, ttx); err != nil {
				return err
			}
			targetView := store.NewTxView(ttx)
			fresh, err := targetView.IsFreshTarget(ctx)
			if err != nil {
				return err
			}
			if !fresh {
				return errTargetNotFresh
			}
			if err := targetView.PutProfile(ctx, profile); err != nil {
				return err
			}
			return replay(ctx, sourceView, target, ttx, log)
		})
	})
}

// replay copies every immutable source session through the normal parse and
// candidate writers, then derives the rollups once.
func replay(ctx context.Context, source *store.TxView, target *store.Store, ttx pgx.Tx,
	log func(string)) error {
	var (
		days     []time.Time
		spans    []store.InstantInterval
		changed  bool
		cursor   *store.ReplayCursor
		last     *store.SessionMeta
		replayed int
	)
	for {
		page, err := source.ReadReplaySessions(ctx, cursor, replayPageSize)
		if err != nil {
			return err
		}
		if len(page) == 0 {
			break
		}
		for _, session := range page {
			session.Meta = session.Meta.Normalized()
			blobs := make(map[string][]byte, len(session.Blobs))
			raw := make([]store.RawBlob, 0, len(session.Blobs))
			for _, blob := range session.Blobs {
				blobs[blob.TypeCode] = blob.Payload
				raw = append(raw, blob)
			}
			catalog, err := parse.ParseCatalog(session.Catalog)
			if err != nil {
				return err
			}
			batch, err := parse.ParseBlobs(catalog, blobs,
				parse.Window{StartedAt: session.Meta.StartedAt, EndedAt: session.Meta.EndedAt})
			if err != nil {
				return err
			}
			existing, err := target.WriteSession(ctx, ttx, session.Meta, session.Catalog, raw)
			if err != nil {
				return err
			}
			mutation, err := applyBatch(ctx, target, ttx, session.Meta, batch, existing)
			if err != nil {
				return err
			}
			if mutation.Changed {
				changed = true
				days = append(days, mutation.Days...)
				spans = append(spans, mutation.Intervals...)
			}
			if err := target.RefreshTypeCoverage(ctx, ttx, submittedCodes(batch)); err != nil {
				return err
			}
			ended := session.Meta
			last = &ended
			replayed++
		}
		tail := page[len(page)-1]
		cursor = &store.ReplayCursor{EndedAt: tail.Meta.EndedAt, SessionID: tail.Meta.ID}
	}
	logf(log, fmt.Sprintf("rebuild: replayed %d sessions", replayed))

	if !changed || last == nil {
		return nil
	}
	return rollup.Run(ctx, target, ttx, rollup.RunInput{
		TouchedDays:       days,
		MutationIntervals: spans,
		Auto:              &rollup.AutoTrigger{SessionID: last.ID, ObservedAt: last.EndedAt},
	})
}

// validate refuses every production, unknown or non-local resource.
func (d DisposableDatabase) validate(role string) error {
	if d.ContainerID == "" {
		return fmt.Errorf("rebuild %s: disposable container id required", role)
	}
	if d.Project == "" || d.Project == productionProject {
		return fmt.Errorf("rebuild %s: disposable test project label required", role)
	}
	if d.Volume == "" || d.Volume == productionVolume {
		return fmt.Errorf("rebuild %s: disposable data volume required", role)
	}
	if _, ok := loopbackHosts[d.Host]; !ok {
		return fmt.Errorf("rebuild %s: loopback host required", role)
	}
	if d.Port == 0 {
		return fmt.Errorf("rebuild %s: mapped loopback port required", role)
	}
	if d.Identity.SystemIdentifier == "" || d.Identity.DatabaseOID == 0 || d.Identity.Database == "" {
		return fmt.Errorf("rebuild %s: cluster identity required", role)
	}
	if !allowedDatabaseName(role, d.Identity.Database) {
		return fmt.Errorf("rebuild %s: database %q is not an allowed disposable name", role, d.Identity.Database)
	}
	return nil
}

func allowedDatabaseName(role, name string) bool {
	if name == productionDatabase {
		return false
	}
	if testDatabaseName.MatchString(name) {
		return true
	}
	if role == "source" {
		return rebuildSourceName.MatchString(name) || name == smokeSource
	}
	return rebuildTargetName.MatchString(name)
}

// verifyIdentity compares the live owner-only identity with the inspected
// scope before any mutation.
func verifyIdentity(ctx context.Context, st *store.Store, want DisposableDatabase, role string) error {
	got, err := st.Identity(ctx)
	if err != nil {
		return fmt.Errorf("rebuild %s identity: %w", role, err)
	}
	if got != want.Identity {
		return fmt.Errorf("%w: %s", errIdentityMismatch, role)
	}
	return nil
}

func logf(log func(string), message string) {
	if log != nil {
		log(message)
	}
}
