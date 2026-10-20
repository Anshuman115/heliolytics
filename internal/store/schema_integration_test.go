package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Anshuman115/heliolytics/internal/testutil"
	"github.com/jackc/pgx/v5"
)

// ---- shared test helpers ----

// newTestStore opens the store on a fresh disposable database. Close is
// registered after the helper's DROP so connections close first.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	dsn := testutil.NewDB(t)
	st, err := New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(st.Close)
	return st
}

func mustTx(t *testing.T, st *Store, fn func(pgx.Tx) error) {
	t.Helper()
	if err := st.WithTx(context.Background(), fn); err != nil {
		t.Fatalf("tx: %v", err)
	}
}

// seedSession stores one immutable envelope carrying a single raw type.
func seedSession(t *testing.T, st *Store, id string, startedAt, endedAt time.Time) {
	t.Helper()
	ctx := context.Background()
	catalog := []byte(`{"schemaVersion":1,"sessionId":` + quoteJSON(id) + `}`)
	meta := SessionMeta{
		ID:        id,
		DeviceMAC: "02:00:00:00:00:01",
		StartedAt: startedAt,
		EndedAt:   endedAt,
	}
	blob := RawBlob{TypeCode: "0x01", Payload: []byte{1, 2, 3}}
	mustTx(t, st, func(tx pgx.Tx) error {
		_, err := st.WriteSession(ctx, tx, meta, catalog, []RawBlob{blob})
		return err
	})
}

func quoteJSON(s string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + replacer.Replace(s) + `"`
}

func countRows(t *testing.T, st *Store, table string) int64 {
	t.Helper()
	var n int64
	if err := st.pool.QueryRow(context.Background(), "SELECT count(*) FROM "+table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func scalarString(t *testing.T, st *Store, sql string, args ...any) string {
	t.Helper()
	var out string
	if err := st.pool.QueryRow(context.Background(), sql, args...).Scan(&out); err != nil {
		t.Fatalf("query %q: %v", sql, err)
	}
	return out
}

// ---- fresh schema ----

var applicationTables = []string{
	"sync_sessions", "raw_blobs",
	"heart_rate_samples", "step_samples", "temperature_samples", "hrv_samples",
	"stress_samples", "resp_samples", "spo2_samples",
	"sleep_sessions", "activities", "sleep_observations", "workout_observations",
	"pai_scores", "daily_metrics", "coverage", "profile", "auth_nonces",
}

var legacyTables = []string{
	"raw_type_blobs", "rhr_samples", "raw_readiness", "raw_pai_scores",
	"workouts", "activity_sessions", "profiles", "computed_readiness",
	"health_samples", "migrations", "schema_migrations",
}

func TestFreshSchemaApplicationInventoryIsEmpty(t *testing.T) {
	st := newTestStore(t)

	for _, table := range applicationTables {
		if n := countRows(t, st, table); n != 0 {
			t.Fatalf("fresh schema: %s has %d rows, want 0", table, n)
		}
	}
	for _, table := range legacyTables {
		got := scalarString(t, st,
			`SELECT coalesce(to_regclass('public.' || $1)::text, '')`, table)
		if got != "" {
			t.Fatalf("legacy relation %s still exists", table)
		}
	}
	if got := scalarString(t, st,
		`SELECT coalesce(to_regclass('public.canonical_day_keys')::text, '')`); got == "" {
		t.Fatal("canonical_day_keys view missing")
	}
}

func TestFreshSchemaTimescaleShape(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	rows, err := st.pool.Query(ctx, `
		SELECT hypertable_name, compression_enabled
		FROM timescaledb_information.hypertables
		WHERE hypertable_schema = 'public'
		ORDER BY hypertable_name`)
	if err != nil {
		t.Fatalf("hypertables: %v", err)
	}
	defer rows.Close()
	found := map[string]bool{}
	for rows.Next() {
		var name string
		var compressed bool
		if err := rows.Scan(&name, &compressed); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if !compressed {
			t.Fatalf("hypertable %s has no compression", name)
		}
		found[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	want := []string{"heart_rate_samples", "step_samples", "temperature_samples",
		"hrv_samples", "stress_samples", "resp_samples", "spo2_samples"}
	if len(found) != len(want) {
		t.Fatalf("hypertables = %v, want %v", found, want)
	}
	for _, name := range want {
		if !found[name] {
			t.Fatalf("hypertable %s missing", name)
		}
	}

	dimensions := scalarString(t, st, `
		SELECT string_agg(DISTINCT time_interval::text, ',' ORDER BY time_interval::text)
		FROM timescaledb_information.dimensions
		WHERE hypertable_schema = 'public' AND dimension_number = 1`)
	if dimensions != "90 days" {
		t.Fatalf("chunk interval = %q, want 90 days", dimensions)
	}

	policies := scalarString(t, st, `
		SELECT string_agg(DISTINCT config->>'compress_after', ',' ORDER BY config->>'compress_after')
		FROM timescaledb_information.jobs
		WHERE proc_name = 'policy_compression'`)
	if policies != "30 days" {
		t.Fatalf("compression policies = %q, want 30 days", policies)
	}
}

func TestFreshSchemaWithoutAppRoleStillInitializes(t *testing.T) {
	st := newTestStore(t)
	if got := scalarString(t, st, `SELECT count(*)::text FROM pg_roles WHERE rolname = 'heliolytics_app'`); got == "0" {
		// The schema grants nothing when the role is absent and still applies.
		for _, table := range applicationTables {
			if countRows(t, st, table) != 0 {
				t.Fatalf("%s not empty", table)
			}
		}
	}
}

func TestFreshSchemaAppRolePrivileges(t *testing.T) {
	ctx := context.Background()
	seed := newTestStore(t)
	if _, err := seed.pool.Exec(ctx,
		`DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'heliolytics_app')
		 THEN CREATE ROLE heliolytics_app LOGIN PASSWORD 'disposable-only'; END IF; END $$;`); err != nil {
		t.Fatalf("create role: %v", err)
	}

	st := newTestStore(t)
	checks := []struct {
		sql  string
		want bool
		why  string
	}{
		{`SELECT has_table_privilege('heliolytics_app', 'heart_rate_samples', 'UPDATE')`, true, "sample update"},
		{`SELECT has_table_privilege('heliolytics_app', 'daily_metrics', 'DELETE')`, true, "derived daily delete"},
		{`SELECT has_table_privilege('heliolytics_app', 'sleep_observations', 'INSERT')`, true, "candidate insert"},
		{`SELECT has_table_privilege('heliolytics_app', 'sleep_observations', 'UPDATE')`, false, "candidate update forbidden"},
		{`SELECT has_table_privilege('heliolytics_app', 'sync_sessions', 'UPDATE')`, false, "immutable envelope update forbidden"},
		{`SELECT has_table_privilege('heliolytics_app', 'raw_blobs', 'DELETE')`, false, "raw delete forbidden"},
		{`SELECT has_table_privilege('heliolytics_app', 'heart_rate_samples', 'DELETE')`, false, "sample delete forbidden"},
		{`SELECT has_table_privilege('heliolytics_app', 'canonical_day_keys', 'SELECT')`, true, "view select"},
		{`SELECT has_function_privilege('heliolytics_app', 'public.rebuild_database_identity()', 'EXECUTE')`, false, "identity helper owner-only"},
	}
	for _, check := range checks {
		var got bool
		if err := st.pool.QueryRow(ctx, check.sql).Scan(&got); err != nil {
			t.Fatalf("%s: %v", check.why, err)
		}
		if got != check.want {
			t.Fatalf("%s: got %v, want %v", check.why, got, check.want)
		}
	}

	if err := appRoleExecErr(t, st, `INSERT INTO heart_rate_samples
		(sampled_at, source, bpm, session_id, observed_at)
		VALUES (now(), '0x01', 60, 'missing-session', now())`); err == nil {
		t.Fatal("app role wrote an orphan sample")
	}
	if err := appRoleExecErr(t, st, `UPDATE sync_sessions SET device_mac = device_mac`); err == nil {
		t.Fatal("app role updated an immutable envelope")
	}
}

// appRoleExecErr runs one statement as the app role and returns its error. The
// transaction is rolled back so a rejected statement cannot poison the next.
func appRoleExecErr(t *testing.T, st *Store, sql string) error {
	t.Helper()
	ctx := context.Background()
	tx, err := st.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SET LOCAL ROLE heliolytics_app`); err != nil {
		t.Fatalf("set app role: %v", err)
	}
	_, execErr := tx.Exec(ctx, sql)
	return execErr
}

func TestSchemaConstraintsRejectInvalidRows(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	at := time.Date(2026, 10, 19, 6, 0, 0, 0, time.UTC)
	seedSession(t, st, "constraint-session", at.Add(-time.Hour), at)

	invalid := []string{
		`INSERT INTO heart_rate_samples (sampled_at, source, bpm, session_id, observed_at)
		 VALUES ('2026-10-19 06:00:00+00', '0x01', 24, 'constraint-session', now())`,
		`INSERT INTO heart_rate_samples (sampled_at, source, bpm, session_id, observed_at)
		 VALUES ('2026-10-19 06:00:00+00', '0x99', 60, 'constraint-session', now())`,
		`INSERT INTO step_samples (sampled_at, steps, session_id, observed_at)
		 VALUES ('2026-10-19 06:01:00+00', -1, 'constraint-session', now())`,
		`INSERT INTO spo2_samples (sampled_at, source, pct, mode, session_id, observed_at)
		 VALUES ('2026-10-19 06:02:00+00', '0x26', 97, 'auto', 'constraint-session', now())`,
		`INSERT INTO hrv_samples (sampled_at, rmssd_ms, session_id, observed_at)
		 VALUES ('2026-10-19 06:03:00+00', 0, 'constraint-session', now())`,
		`INSERT INTO resp_samples (sampled_at, br_min, session_id, observed_at)
		 VALUES ('2026-10-19 06:04:00+00', 3, 'constraint-session', now())`,
		`INSERT INTO temperature_samples (sampled_at, celsius, on_wrist, session_id, observed_at)
		 VALUES ('2026-10-19 06:05:00+00', 25.00, true, 'constraint-session', now())`,
		`INSERT INTO sync_sessions (session_id, device_mac, started_at, ended_at, catalog, battery_pct)
		 VALUES ('bad-battery', '02:00:00:00:00:01', now(), now(), '{}'::jsonb, 101)`,
		`INSERT INTO sleep_sessions (started_at, ended_at, day_key, is_nap, total_mins,
		   deep_mins, rem_mins, light_mins, wake_mins, stages, stages_truncated, observed_at, session_id)
		 VALUES ('2026-10-19 01:00:00+00', '2026-10-19 02:00:00+00', '2026-10-19', false, 60,
		   10, 10, 10, 0, '[]'::jsonb, true, now(), 'constraint-session')`,
		`INSERT INTO profile (singleton, name) VALUES (false, 'no')`,
	}
	for _, sql := range invalid {
		if _, err := st.pool.Exec(ctx, sql); err == nil {
			t.Fatalf("constraint accepted invalid row: %s", strings.TrimSpace(sql))
		}
	}
}

func writeSessionEnvelope(t *testing.T, st *Store, meta SessionMeta, catalog string,
	blobs []RawBlob) (bool, error) {
	t.Helper()
	var existing bool
	err := st.WithTx(context.Background(), func(tx pgx.Tx) error {
		var err error
		existing, err = st.WriteSession(context.Background(), tx, meta, []byte(catalog), blobs)
		return err
	})
	return existing, err
}

func TestWriteSessionEnvelopeIsImmutable(t *testing.T) {
	st := newTestStore(t)
	started := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	ended := started.Add(time.Hour)
	meta := SessionMeta{
		ID:         "envelope-session",
		DeviceMAC:  "02:00:00:00:00:01",
		StartedAt:  started,
		EndedAt:    ended,
		BatteryPct: new(80),
	}
	catalog := `{"schemaVersion":1,"sessionId":"envelope-session","chunked":[]}`
	blobs := []RawBlob{
		{TypeCode: "0x01", Payload: []byte{1, 2, 3}},
		{TypeCode: "0x2E", Payload: []byte{4, 5}},
	}

	existing, err := writeSessionEnvelope(t, st, meta, catalog, blobs)
	if err != nil || existing {
		t.Fatalf("first write: existing=%v err=%v", existing, err)
	}
	if n := countRows(t, st, "raw_blobs"); n != 2 {
		t.Fatalf("raw rows = %d, want 2", n)
	}

	existing, err = writeSessionEnvelope(t, st, meta, catalog, blobs)
	if err != nil || !existing {
		t.Fatalf("identical retry: existing=%v err=%v", existing, err)
	}

	// Property order alone is not a conflict.
	reordered := `{"chunked":[],"sessionId":"envelope-session","schemaVersion":1}`
	if _, err := writeSessionEnvelope(t, st, meta, reordered, blobs); err != nil {
		t.Fatalf("reordered catalog was treated as a conflict: %v", err)
	}

	changedPayload := []RawBlob{
		{TypeCode: "0x01", Payload: []byte{9}},
		{TypeCode: "0x2E", Payload: []byte{4, 5}},
	}
	if _, err := writeSessionEnvelope(t, st, meta, catalog, changedPayload); !errors.Is(err, ErrSessionConflict) {
		t.Fatalf("changed payload err = %v, want ErrSessionConflict", err)
	}
	if _, err := writeSessionEnvelope(t, st, meta,
		`{"schemaVersion":1,"sessionId":"envelope-session"}`, blobs); !errors.Is(err, ErrSessionConflict) {
		t.Fatalf("changed catalog err = %v, want ErrSessionConflict", err)
	}
	if _, err := writeSessionEnvelope(t, st, meta, catalog, blobs[:1]); !errors.Is(err, ErrSessionConflict) {
		t.Fatalf("subset type list err = %v, want ErrSessionConflict", err)
	}
	superset := append(append([]RawBlob{}, blobs...), RawBlob{TypeCode: "0x13", Payload: []byte{7}})
	if _, err := writeSessionEnvelope(t, st, meta, catalog, superset); !errors.Is(err, ErrSessionConflict) {
		t.Fatalf("superset type list err = %v, want ErrSessionConflict", err)
	}
	if _, err := writeSessionEnvelope(t, st, meta, catalog, []RawBlob{
		{TypeCode: "0x01", Payload: []byte{1}},
		{TypeCode: "0x01", Payload: []byte{2}},
	}); err == nil {
		t.Fatal("duplicate RawBlob type accepted")
	}

	// A new session ID accepts the same bytes.
	next := meta
	next.ID = "envelope-session-2"
	if _, err := writeSessionEnvelope(t, st, next, catalog, blobs); err != nil {
		t.Fatalf("new session id with identical bytes: %v", err)
	}
	if n := countRows(t, st, "sync_sessions"); n != 2 {
		t.Fatalf("sessions = %d, want 2 immutable envelopes", n)
	}
}

func TestWriteSessionRequiresEndedAt(t *testing.T) {
	st := newTestStore(t)
	started := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	meta := SessionMeta{ID: "no-end", DeviceMAC: "02:00:00:00:00:01", StartedAt: started}
	var invalid *InvalidRowError
	if _, err := writeSessionEnvelope(t, st, meta, `{}`,
		[]RawBlob{{TypeCode: "0x01", Payload: []byte{1}}}); !errors.As(err, &invalid) {
		t.Fatalf("err = %v, want an invalid row error", err)
	}
	if n := countRows(t, st, "sync_sessions"); n != 0 {
		t.Fatalf("sessions = %d, want 0", n)
	}
}
