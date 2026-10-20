// Package testutil provides an isolated disposable database per test.
package testutil

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Anshuman115/heliolytics/internal/testutil/disposable"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	adminDatabase  = "heliolytics_test_admin"
	databasePrefix = "heliolytics_test_"
	schemaFileName = "schema.sql"
	randomSuffix   = 8
	createAttempts = 5
	setupTimeout   = 2 * time.Minute
	cleanupTimeout = 30 * time.Second
)

const (
	envAdminDSN  = "TEST_DATABASE_URL"
	envContainer = "HELIOLYTICS_TEST_DB_CONTAINER_ID"
	envImage     = "HELIOLYTICS_TEST_DB_IMAGE"
)

// NewDB creates a fresh schema-applied database on the disposable test
// cluster and returns its DSN. The database is dropped during test cleanup.
func NewDB(t testing.TB) string {
	t.Helper()

	adminDSN := requireEnv(t, envAdminDSN)
	containerID := requireEnv(t, envContainer)
	wantImage := requireEnv(t, envImage)

	if err := validateAdminDSN(adminDSN); err != nil {
		t.Fatalf("testutil: %s: %v", envAdminDSN, err)
	}

	schema := readSchema(t)

	ctx, cancel := context.WithTimeout(context.Background(), setupTimeout)
	defer cancel()

	if _, err := disposable.Inspect(ctx, containerID, adminDSN, wantImage); err != nil {
		t.Fatalf("testutil: disposable database rejected: %v", err)
	}

	name, dsn := createDatabase(ctx, t, adminDSN)
	t.Cleanup(func() {
		dropDatabase(t, containerID, adminDSN, wantImage, name)
	})

	applySchema(ctx, t, dsn, schema)
	return dsn
}

// requireEnv reads a mandatory environment variable, never a fallback.
func requireEnv(t testing.TB, key string) string {
	t.Helper()

	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		t.Fatalf("testutil: %s is required for disposable database tests", key)
	}
	return value
}

// validateAdminDSN accepts only the disposable maintenance database.
func validateAdminDSN(dsn string) error {
	cfg, err := pgconn.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("parse database URL: %w", err)
	}
	if cfg.Database != adminDatabase {
		return fmt.Errorf("database %q is not the maintenance database %q", cfg.Database, adminDatabase)
	}
	return nil
}

// readSchema loads the repository schema from the module root.
func readSchema(t testing.TB) string {
	t.Helper()

	path, err := schemaPath()
	if err != nil {
		t.Fatalf("testutil: locate %s: %v", schemaFileName, err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("testutil: read %s: %v", path, err)
	}
	if strings.TrimSpace(string(content)) == "" {
		t.Fatalf("testutil: %s is empty", path)
	}
	return string(content)
}

// schemaPath walks up from this source file to the repository root.
func schemaPath() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("runtime.Caller did not report a file")
	}

	start := filepath.Dir(file)
	for dir := start; ; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, schemaFileName)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
		if filepath.Dir(dir) == dir {
			return "", fmt.Errorf("no %s above %s", schemaFileName, start)
		}
	}
}

// createDatabase creates one uniquely named database and returns its DSN.
func createDatabase(ctx context.Context, t testing.TB, adminDSN string) (string, string) {
	t.Helper()

	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("testutil: connect to %s: %v", adminDatabase, err)
	}

	name, createErr := createUniqueDatabase(ctx, admin)
	closeErr := admin.Close(context.WithoutCancel(ctx))
	if createErr != nil {
		t.Fatalf("testutil: create database: %v", createErr)
	}
	if closeErr != nil {
		t.Fatalf("testutil: close %s: %v", adminDatabase, closeErr)
	}

	dsn, err := replaceDatabase(adminDSN, name)
	if err != nil {
		t.Fatalf("testutil: build DSN for %s: %v", name, err)
	}
	return name, dsn
}

// applySchema loads schema.sql into the freshly created database.
func applySchema(ctx context.Context, t testing.TB, dsn, schema string) {
	t.Helper()

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("testutil: connect to new database: %v", err)
	}

	_, applyErr := conn.Exec(ctx, schema)
	closeErr := conn.Close(context.WithoutCancel(ctx))
	if applyErr != nil {
		t.Fatalf("testutil: apply %s: %v", schemaFileName, applyErr)
	}
	if closeErr != nil {
		t.Fatalf("testutil: close new database: %v", closeErr)
	}
}

// createUniqueDatabase retries on the rare generated-name collision.
func createUniqueDatabase(ctx context.Context, admin *pgx.Conn) (string, error) {
	for range createAttempts {
		name, err := newDatabaseName()
		if err != nil {
			return "", err
		}
		_, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize())
		if err == nil {
			return name, nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "42P04" {
			continue
		}
		return "", err
	}
	return "", fmt.Errorf("no unique name after %d attempts", createAttempts)
}

// dropDatabase removes the per-test database after re-checking the resource.
func dropDatabase(t testing.TB, containerID, adminDSN, wantImage, name string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()

	if _, err := disposable.Inspect(ctx, containerID, adminDSN, wantImage); err != nil {
		t.Errorf("testutil: not dropping %s, disposable database no longer verified: %v", name, err)
		return
	}

	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Errorf("testutil: connect to %s for cleanup: %v", adminDatabase, err)
		return
	}

	_, dropErr := admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)")
	closeErr := admin.Close(context.WithoutCancel(ctx))
	if dropErr != nil {
		t.Errorf("testutil: drop database %s: %v", name, dropErr)
	}
	if closeErr != nil {
		t.Errorf("testutil: close %s after cleanup: %v", adminDatabase, closeErr)
	}
}

// newDatabaseName returns a collision-safe name in the allowed grammar.
func newDatabaseName() (string, error) {
	var raw [randomSuffix]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate database name: %w", err)
	}
	return databasePrefix + hex.EncodeToString(raw[:]), nil
}

// validDatabaseName matches heliolytics_test_ plus 16 lowercase hex digits.
func validDatabaseName(name string) bool {
	suffix, ok := strings.CutPrefix(name, databasePrefix)
	if !ok || len(suffix) != 2*randomSuffix {
		return false
	}
	for _, r := range suffix {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

// replaceDatabase points a validated DSN at another generated database.
func replaceDatabase(dsn, name string) (string, error) {
	if !validDatabaseName(name) {
		return "", fmt.Errorf("database name %q is not %s<16 hex digits>", name, databasePrefix)
	}

	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		parsed, err := url.Parse(dsn)
		if err != nil {
			return "", fmt.Errorf("parse database URL: %w", err)
		}
		parsed.Path = "/" + name
		return parsed.String(), nil
	}

	fields := strings.Fields(dsn)
	replaced := false
	for i, field := range fields {
		if key, _, ok := strings.Cut(field, "="); ok && key == "dbname" {
			fields[i] = "dbname=" + name
			replaced = true
		}
	}
	if !replaced {
		fields = append(fields, "dbname="+name)
	}
	return strings.Join(fields, " "), nil
}
