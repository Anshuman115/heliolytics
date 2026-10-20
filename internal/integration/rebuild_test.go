package integration

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Anshuman115/heliolytics/internal/ingest"
	"github.com/Anshuman115/heliolytics/internal/store"
	"github.com/Anshuman115/heliolytics/internal/testutil"
	"github.com/Anshuman115/heliolytics/internal/testutil/disposable"
	"github.com/jackc/pgx/v5"
)

// openStore opens one store on a DSN and closes it before the database drop.
func openStore(t *testing.T, dsn string) *store.Store {
	t.Helper()
	db, err := store.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(db.Close)
	return db
}

// stackOn wraps one existing store in the test client.
func stackOn(db *store.Store) *stack {
	keys := testKeys()
	return &stack{handler: newHandler(db, keys), db: db, keys: keys}
}

// testLogger adapts testing.T to the rebuild log callback.
func testLogger(t *testing.T) func(string) {
	t.Helper()
	return func(message string) { t.Log(message) }
}

// scopeFor inspects one live resource and builds B05's disposable scope.
func scopeFor(t *testing.T, dsn string) ingest.DisposableDatabase {
	t.Helper()
	containerID := os.Getenv("HELIOLYTICS_TEST_DB_CONTAINER_ID")
	wantImage := os.Getenv("HELIOLYTICS_TEST_DB_IMAGE")
	if containerID == "" || wantImage == "" {
		t.Fatal("the disposable database identity is not exported")
	}
	inspected, err := disposable.Inspect(t.Context(), containerID, dsn, wantImage)
	if err != nil {
		t.Fatalf("disposable.Inspect: %v", err)
	}

	db := openStore(t, dsn)
	identity, err := db.Identity(t.Context())
	if err != nil {
		t.Fatalf("store.Identity: %v", err)
	}
	return ingest.DisposableDatabase{
		ContainerID: inspected.ContainerID,
		Project:     inspected.Project,
		Volume:      inspected.Volume,
		Host:        inspected.Host,
		Port:        uint16(inspected.Port),
		Identity:    identity,
	}
}

// seedSource uploads the committed corpus and one profile into a source.
func seedSource(t *testing.T, source *stack) {
	t.Helper()
	source.uploadOK(t, realSession(t), realCatalog(t), realBlobs(t))
	profile := []byte(`{"name":null,"birthDate":"1991-02-03","sex":"male","heightCm":176.0,"weightKg":70.0}`)
	recorder := source.serve(source.newRequest(t, phoneKid, "PUT", "/api/profile", "",
		profile, "application/json"))
	if recorder.Code != 204 {
		t.Fatalf("source profile status %d: %s", recorder.Code, recorder.Body.String())
	}
}

// execute runs one statement against a store in its own transaction.
func execute(t *testing.T, db *store.Store, statement string) {
	t.Helper()
	ctx := context.Background()
	if err := db.WithTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, statement)
		return err
	}); err != nil {
		t.Fatalf("execute %q: %v", statement, err)
	}
}

// TestRebuildReplaysIntoFreshTarget proves a disposable source replays into a
// different empty target and that the source stays unchanged.
func TestRebuildReplaysIntoFreshTarget(t *testing.T) {
	sourceDSN := testutil.NewDB(t)
	sourceStore := openStore(t, sourceDSN)
	source := stackOn(sourceStore)
	seedSource(t, source)
	sourceCounts := source.tabulated(t)
	sourceDays := source.dailyKeys(t)

	targetDSN := testutil.NewDB(t)
	targetStore := openStore(t, targetDSN)
	target := stackOn(targetStore)

	scope := ingest.RebuildTestScope{
		Source: scopeFor(t, sourceDSN),
		Target: scopeFor(t, targetDSN),
	}
	if scope.Source.Identity == scope.Target.Identity {
		t.Fatalf("two fresh databases share one identity: %+v", scope.Source.Identity)
	}
	if err := ingest.RebuildAll(t.Context(), sourceStore, targetStore, scope, testLogger(t)); err != nil {
		t.Fatalf("RebuildAll: %v", err)
	}

	for table, want := range sourceCounts {
		if table == "auth_nonces" {
			continue
		}
		if got := target.count(t, table); got != want {
			t.Fatalf("%s = %d on the target, want %d", table, got, want)
		}
	}
	// Replay never copies replay bookkeeping.
	if nonces := target.count(t, "auth_nonces"); nonces != 0 {
		t.Fatalf("target auth_nonces = %d, want 0", nonces)
	}

	targetDays := target.dailyKeys(t)
	if !equalStrings(targetDays, sourceDays) {
		t.Fatalf("target daily keys = %v, want %v", targetDays, sourceDays)
	}
	for _, day := range sourceDays {
		before := source.daily(t, day)
		after := target.daily(t, day)
		if !equalOptionalInt(before.Steps, after.Steps) {
			t.Fatalf("%s steps = %s, want %s", day, optionalIntText(after.Steps), optionalIntText(before.Steps))
		}
		if !equalOptionalInt(before.SleepMins, after.SleepMins) {
			t.Fatalf("%s sleepMins = %s, want %s", day, optionalIntText(after.SleepMins), optionalIntText(before.SleepMins))
		}
		if before.WorkoutCount != after.WorkoutCount {
			t.Fatalf("%s workoutCount = %d, want %d", day, after.WorkoutCount, before.WorkoutCount)
		}
	}

	stored, err := targetStore.GetProfile(t.Context())
	if err != nil {
		t.Fatalf("target profile: %v", err)
	}
	if stored.BirthDate == nil {
		t.Fatal("target birthDate is null")
	}
	if year, month, day := stored.BirthDate.Date(); year != 1991 || month != time.February || day != 3 {
		t.Fatalf("target birthDate = %v, want 1991-02-03", stored.BirthDate)
	}
	if stored.Sex == nil || *stored.Sex != "male" {
		t.Fatalf("target sex = %v", stored.Sex)
	}

	after := source.tabulated(t)
	for table, want := range sourceCounts {
		if got := after[table]; got != want {
			t.Fatalf("source %s changed from %d to %d", table, want, got)
		}
	}
}

// equalOptionalInt compares two optional integer values.
func equalOptionalInt(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// TestRebuildRefusesSameDatabaseAliases sends one empty database through two
// loopback endpoints and requires the stable identity refusal before mutation.
func TestRebuildRefusesSameDatabaseAliases(t *testing.T) {
	dsn := testutil.NewDB(t)
	ipv4 := openStore(t, dsn)

	ipv6DSN := strings.Replace(dsn, "127.0.0.1:", "[::1]:", 1)
	if ipv6DSN == dsn {
		t.Fatalf("test DSN %q does not use the IPv4 loopback host", dsn)
	}
	ipv6 := openStore(t, ipv6DSN)

	scope := ingest.RebuildTestScope{
		Source: scopeFor(t, dsn),
		Target: scopeFor(t, ipv6DSN),
	}
	if scope.Source.Identity != scope.Target.Identity {
		t.Fatalf("aliases reported different identities: %+v and %+v",
			scope.Source.Identity, scope.Target.Identity)
	}

	err := ingest.RebuildAll(t.Context(), ipv4, ipv6, scope, nil)
	if !errors.Is(err, ingest.ErrRebuildSameDatabase) {
		t.Fatalf("err = %v, want ErrRebuildSameDatabase", err)
	}
	for _, table := range appTables {
		if count := (&stack{db: ipv4}).count(t, table); count != 0 {
			t.Fatalf("%s = %d rows after a refused rebuild", table, count)
		}
	}
}

// TestRebuildRefusesNonEmptyTarget keeps a populated target untouched.
func TestRebuildRefusesNonEmptyTarget(t *testing.T) {
	sourceDSN := testutil.NewDB(t)
	sourceStore := openStore(t, sourceDSN)
	source := stackOn(sourceStore)
	source.uploadOK(t, realSession(t), realCatalog(t), realBlobs(t))
	sourceCounts := source.tabulated(t)

	targetDSN := testutil.NewDB(t)
	targetStore := openStore(t, targetDSN)
	target := stackOn(targetStore)
	minute := time.Date(2026, time.September, 26, 1, 0, 0, 0, time.UTC)
	target.uploadOK(t, sessionPart(t, "target-existing", minute, minute.Add(5*time.Minute)),
		catalogPart(t, catalogEntry{Code: "0x2E", RoundStart: istWallClock(minute)}),
		map[string][]byte{"0x2E": temperatureMinute(3250)})
	targetCounts := target.tabulated(t)

	scope := ingest.RebuildTestScope{
		Source: scopeFor(t, sourceDSN),
		Target: scopeFor(t, targetDSN),
	}
	if err := ingest.RebuildAll(t.Context(), sourceStore, targetStore, scope, nil); err == nil {
		t.Fatal("rebuild accepted a populated target")
	}

	for table, want := range targetCounts {
		if got := target.count(t, table); got != want {
			t.Fatalf("target %s changed from %d to %d", table, want, got)
		}
	}
	for table, want := range sourceCounts {
		if got := source.count(t, table); got != want {
			t.Fatalf("source %s changed from %d to %d", table, want, got)
		}
	}
}

// TestRebuildRefusesProductionScope covers every scope guard. A nil store pair
// proves the refusal happens before any resource is touched.
func TestRebuildRefusesProductionScope(t *testing.T) {
	valid := ingest.DisposableDatabase{
		ContainerID: strings.Repeat("a", 64),
		Project:     "v01",
		Volume:      "heliolytics_test_data",
		Host:        "127.0.0.1",
		Port:        5445,
		Identity: store.DatabaseIdentity{
			SystemIdentifier: "7000000000000000001",
			DatabaseOID:      16384,
			Database:         "heliolytics_test_0123456789abcdef",
		},
	}

	cases := []struct {
		name   string
		mutate func(*ingest.DisposableDatabase)
	}{
		{"production project", func(db *ingest.DisposableDatabase) { db.Project = "deploy" }},
		{"empty project", func(db *ingest.DisposableDatabase) { db.Project = "" }},
		{"production volume", func(db *ingest.DisposableDatabase) { db.Volume = "deploy_pgdata" }},
		{"empty volume", func(db *ingest.DisposableDatabase) { db.Volume = "" }},
		{"remote host", func(db *ingest.DisposableDatabase) { db.Host = "10.0.0.5" }},
		{"zero port", func(db *ingest.DisposableDatabase) { db.Port = 0 }},
		{"empty container", func(db *ingest.DisposableDatabase) { db.ContainerID = "" }},
		{"production database", func(db *ingest.DisposableDatabase) {
			db.Identity.Database = "heliolytics"
		}},
		{"unknown database", func(db *ingest.DisposableDatabase) {
			db.Identity.Database = "heliolytics_prod"
		}},
		{"missing identity", func(db *ingest.DisposableDatabase) {
			db.Identity = store.DatabaseIdentity{}
		}},
	}

	ctx := context.Background()
	for _, tc := range cases {
		t.Run("source "+tc.name, func(t *testing.T) {
			source := valid
			tc.mutate(&source)
			err := ingest.RebuildAll(ctx, nil, nil,
				ingest.RebuildTestScope{Source: source, Target: valid}, nil)
			if err == nil {
				t.Fatal("rebuild accepted a refused source scope")
			}
			if !strings.Contains(err.Error(), "source") {
				t.Fatalf("error %v does not name the source role", err)
			}
		})
		t.Run("target "+tc.name, func(t *testing.T) {
			target := valid
			tc.mutate(&target)
			err := ingest.RebuildAll(ctx, nil, nil,
				ingest.RebuildTestScope{Source: valid, Target: target}, nil)
			if err == nil {
				t.Fatal("rebuild accepted a refused target scope")
			}
			if !strings.Contains(err.Error(), "target") {
				t.Fatalf("error %v does not name the target role", err)
			}
		})
	}
}

// TestRebuildLateFailureLeavesTargetEmpty injects a failure on a late daily
// write, proves the target rollback and then retries cleanly.
func TestRebuildLateFailureLeavesTargetEmpty(t *testing.T) {
	sourceDSN := testutil.NewDB(t)
	sourceStore := openStore(t, sourceDSN)
	source := stackOn(sourceStore)
	seedSource(t, source)
	sourceCounts := source.tabulated(t)
	sourceDays := source.dailyKeys(t)

	targetDSN := testutil.NewDB(t)
	targetStore := openStore(t, targetDSN)
	target := stackOn(targetStore)

	execute(t, targetStore, `CREATE FUNCTION reject_daily_write() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'injected late failure';
END
$$`)
	execute(t, targetStore, `CREATE TRIGGER reject_daily_write
BEFORE INSERT OR UPDATE ON daily_metrics
FOR EACH ROW EXECUTE FUNCTION reject_daily_write()`)

	scope := ingest.RebuildTestScope{
		Source: scopeFor(t, sourceDSN),
		Target: scopeFor(t, targetDSN),
	}
	if err := ingest.RebuildAll(t.Context(), sourceStore, targetStore, scope, nil); err == nil {
		t.Fatal("rebuild succeeded despite the injected failure")
	}
	for _, table := range appTables {
		if count := target.count(t, table); count != 0 {
			t.Fatalf("target %s = %d rows after a rolled-back rebuild", table, count)
		}
	}

	execute(t, targetStore, `DROP TRIGGER reject_daily_write ON daily_metrics`)
	execute(t, targetStore, `DROP FUNCTION reject_daily_write()`)

	if err := ingest.RebuildAll(t.Context(), sourceStore, targetStore, scope, nil); err != nil {
		t.Fatalf("clean retry: %v", err)
	}
	if days := target.dailyKeys(t); !equalStrings(days, sourceDays) {
		t.Fatalf("retry daily keys = %v, want %v", days, sourceDays)
	}
	after := source.tabulated(t)
	for table, want := range sourceCounts {
		if got := after[table]; got != want {
			t.Fatalf("source %s changed from %d to %d", table, want, got)
		}
	}
}
