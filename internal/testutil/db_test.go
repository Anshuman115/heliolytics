package testutil

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

const (
	missingEnvChildFlag = "TESTUTIL_MISSING_ENV_CHILD"
	fakeContainerID     = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	testDSN             = "postgres://heliolytics:secret@127.0.0.1:5445/heliolytics_test_admin?sslmode=disable"
)

func connect(t *testing.T, dsn string) *pgx.Conn {
	t.Helper()

	conn, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { conn.Close(context.Background()) })
	return conn
}

func adminConn(t *testing.T) *pgx.Conn {
	t.Helper()

	dsn := strings.TrimSpace(os.Getenv(envAdminDSN))
	if dsn == "" {
		t.Fatalf("testutil: %s is required", envAdminDSN)
	}
	return connect(t, dsn)
}

func databaseName(t *testing.T, dsn string) string {
	t.Helper()

	var name string
	if err := connect(t, dsn).QueryRow(context.Background(), "SELECT current_database()").Scan(&name); err != nil {
		t.Fatalf("query current_database: %v", err)
	}
	return name
}

func TestNewDBIsolation(t *testing.T) {
	firstDSN := NewDB(t)
	secondDSN := NewDB(t)
	if firstDSN == secondDSN {
		t.Fatalf("NewDB returned the same DSN twice")
	}

	firstName := databaseName(t, firstDSN)
	secondName := databaseName(t, secondDSN)
	if firstName == secondName {
		t.Fatalf("NewDB returned the same database %q twice", firstName)
	}
	if !validDatabaseName(firstName) || !validDatabaseName(secondName) {
		t.Fatalf("NewDB created %q and %q, want %s<16 hex digits>", firstName, secondName, databasePrefix)
	}

	first := connect(t, firstDSN)
	if _, err := first.Exec(context.Background(), "CREATE TABLE isolation_probe (id INT)"); err != nil {
		t.Fatalf("create probe table: %v", err)
	}
	if _, err := first.Exec(context.Background(), "INSERT INTO isolation_probe (id) VALUES (1)"); err != nil {
		t.Fatalf("insert probe row: %v", err)
	}

	var present bool
	if err := connect(t, secondDSN).QueryRow(context.Background(),
		"SELECT to_regclass('isolation_probe') IS NOT NULL").Scan(&present); err != nil {
		t.Fatalf("query probe table: %v", err)
	}
	if present {
		t.Fatalf("database %s sees the probe table created in %s", secondName, firstName)
	}
}

func TestNewDBParallel(t *testing.T) {
	for _, name := range []string{"alpha", "beta"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			dsn := NewDB(t)
			conn := connect(t, dsn)
			ctx := context.Background()

			if _, err := conn.Exec(ctx, "CREATE TABLE parallel_probe (label TEXT)"); err != nil {
				t.Fatalf("create probe table: %v", err)
			}
			if _, err := conn.Exec(ctx, "INSERT INTO parallel_probe (label) VALUES ($1)", name); err != nil {
				t.Fatalf("insert probe row: %v", err)
			}

			var rows int
			if err := conn.QueryRow(ctx, "SELECT count(*) FROM parallel_probe").Scan(&rows); err != nil {
				t.Fatalf("count probe rows: %v", err)
			}
			if rows != 1 {
				t.Fatalf("parallel_probe has %d rows, want 1", rows)
			}

			var label string
			if err := conn.QueryRow(ctx, "SELECT label FROM parallel_probe").Scan(&label); err != nil {
				t.Fatalf("read probe label: %v", err)
			}
			if label != name {
				t.Fatalf("probe label = %q, want %q", label, name)
			}

			var schemaApplied bool
			if err := conn.QueryRow(ctx, "SELECT to_regclass('public.sync_sessions') IS NOT NULL").Scan(&schemaApplied); err != nil {
				t.Fatalf("query schema object: %v", err)
			}
			if !schemaApplied {
				t.Fatalf("%s was not applied to %s", schemaFileName, name)
			}
		})
	}
}

func TestNewDBCleanup(t *testing.T) {
	var name string

	t.Run("create", func(t *testing.T) {
		name = databaseName(t, NewDB(t))
		if !validDatabaseName(name) {
			t.Fatalf("NewDB created %q, want %s<16 hex digits>", name, databasePrefix)
		}
	})

	if name == "" {
		t.Fatal("subtest did not record a database name")
	}

	var present bool
	if err := adminConn(t).QueryRow(context.Background(),
		"SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)", name).Scan(&present); err != nil {
		t.Fatalf("query pg_database: %v", err)
	}
	if present {
		t.Fatalf("database %s still exists after test cleanup", name)
	}
}

func TestNewDBMissingEnv(t *testing.T) {
	if os.Getenv(missingEnvChildFlag) == "1" {
		NewDB(t)
		t.Fatalf("NewDB returned without %s and %s, want a fatal error", envAdminDSN, envContainer)
	}

	tests := []struct {
		name      string
		unset     []string
		set       []string
		wantError string
	}{
		{
			name:      "admin DSN missing",
			unset:     []string{envAdminDSN},
			set:       []string{envContainer + "=" + fakeContainerID, envImage + "=image"},
			wantError: envAdminDSN + " is required",
		},
		{
			name:      "container ID missing",
			unset:     []string{envContainer},
			set:       []string{envAdminDSN + "=" + testDSN, envImage + "=image"},
			wantError: envContainer + " is required",
		},
		{
			name:      "image missing",
			unset:     []string{envImage},
			set:       []string{envAdminDSN + "=" + testDSN, envContainer + "=" + fakeContainerID},
			wantError: envImage + " is required",
		},
		{
			name:      "no DATABASE_URL fallback",
			unset:     []string{envAdminDSN, envContainer, envImage},
			set:       []string{"DATABASE_URL=postgres://heliolytics:secret@127.0.0.1:5445/heliolytics?sslmode=disable"},
			wantError: envAdminDSN + " is required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestNewDBMissingEnv$", "-test.count=1")
			cmd.Env = childEnv(tc.unset, tc.set)
			out, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("NewDB with %v unset passed, want a fatal error; output:\n%s", tc.unset, out)
			}

			text := string(out)
			if !strings.Contains(text, tc.wantError) {
				t.Fatalf("child output does not mention %q:\n%s", tc.wantError, text)
			}
			if strings.Contains(text, "--- SKIP") {
				t.Fatalf("child skipped instead of failing:\n%s", text)
			}
			if !strings.Contains(text, "--- FAIL") {
				t.Fatalf("child did not fail the test:\n%s", text)
			}
		})
	}
}

// childEnv builds the environment for a missing-variable child run.
func childEnv(unset, set []string) []string {
	blocked := make(map[string]bool, len(unset)+len(set))
	for _, entry := range append(append([]string{}, unset...), set...) {
		key, _, _ := strings.Cut(entry, "=")
		blocked[key] = true
	}

	env := make([]string, 0, len(os.Environ())+len(set)+1)
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if blocked[key] {
			continue
		}
		env = append(env, entry)
	}
	env = append(env, missingEnvChildFlag+"=1")
	return append(env, set...)
}

func TestValidateAdminDSN(t *testing.T) {
	tests := []struct {
		name      string
		dsn       string
		wantError string
	}{
		{name: "url", dsn: testDSN},
		{
			name: "keyword form",
			dsn:  "host=127.0.0.1 port=5445 user=heliolytics password=secret dbname=heliolytics_test_admin sslmode=disable",
		},
		{
			name:      "application database",
			dsn:       "postgres://heliolytics:secret@127.0.0.1:5445/heliolytics?sslmode=disable",
			wantError: "is not the maintenance database",
		},
		{
			name:      "generated database",
			dsn:       "postgres://heliolytics:secret@127.0.0.1:5445/heliolytics_test_0123456789abcdef?sslmode=disable",
			wantError: "is not the maintenance database",
		},
		{
			name:      "unparseable",
			dsn:       "not a dsn",
			wantError: "parse database URL",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateAdminDSN(tc.dsn)
			if tc.wantError == "" {
				if err != nil {
					t.Fatalf("validateAdminDSN(%q): %v", tc.dsn, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validateAdminDSN(%q) = nil, want error containing %q", tc.dsn, tc.wantError)
			}
			if !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("validateAdminDSN(%q) error = %v, want containing %q", tc.dsn, err, tc.wantError)
			}
		})
	}
}

func TestReplaceDatabase(t *testing.T) {
	const generated = "heliolytics_test_0123456789abcdef"

	tests := []struct {
		name      string
		dsn       string
		database  string
		want      string
		wantError string
	}{
		{
			name:     "url",
			dsn:      testDSN,
			database: generated,
			want:     "postgres://heliolytics:secret@127.0.0.1:5445/" + generated + "?sslmode=disable",
		},
		{
			name:     "keyword form",
			dsn:      "host=127.0.0.1 port=5445 user=heliolytics dbname=heliolytics_test_admin sslmode=disable",
			database: generated,
			want:     "host=127.0.0.1 port=5445 user=heliolytics dbname=" + generated + " sslmode=disable",
		},
		{
			name:     "keyword form without dbname",
			dsn:      "host=127.0.0.1 port=5445",
			database: generated,
			want:     "host=127.0.0.1 port=5445 dbname=" + generated,
		},
		{
			name:      "uppercase suffix",
			dsn:       testDSN,
			database:  "heliolytics_test_0123456789ABCDEF",
			wantError: "not heliolytics_test_",
		},
		{
			name:      "short suffix",
			dsn:       testDSN,
			database:  "heliolytics_test_0123",
			wantError: "not heliolytics_test_",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := replaceDatabase(tc.dsn, tc.database)
			if tc.wantError != "" {
				if err == nil {
					t.Fatalf("replaceDatabase = %q, want error containing %q", got, tc.wantError)
				}
				if !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("replaceDatabase error = %v, want containing %q", err, tc.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("replaceDatabase: %v", err)
			}
			if got != tc.want {
				t.Fatalf("replaceDatabase = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNewDatabaseName(t *testing.T) {
	seen := make(map[string]bool, 100)
	for range 100 {
		name, err := newDatabaseName()
		if err != nil {
			t.Fatalf("newDatabaseName: %v", err)
		}
		if !validDatabaseName(name) {
			t.Fatalf("newDatabaseName = %q, want %s<16 hex digits>", name, databasePrefix)
		}
		if seen[name] {
			t.Fatalf("newDatabaseName repeated %q", name)
		}
		seen[name] = true
	}
}
