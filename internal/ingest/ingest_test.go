package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/Anshuman115/heliolytics/internal/parse"
	"github.com/Anshuman115/heliolytics/internal/store"
	"github.com/Anshuman115/heliolytics/internal/testutil"
)

const testMAC = "02:00:00:00:00:01"

// fixtureDir resolves the committed sanitized dump used for real decode tests.
func fixtureDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve the test source path")
	}
	return filepath.Join(filepath.Dir(file), "..", "parse", "testdata", "real-dumpp-2026-06")
}

type fixtureSession struct {
	SessionID      string `json:"sessionId"`
	StartedAt      string `json:"startedAt"`
	EndedAt        string `json:"endedAt"`
	DeviceMAC      string `json:"deviceMac"`
	BatteryPercent *int   `json:"batteryPercent"`
}

type fixtureEnvelope struct {
	meta    store.SessionMeta
	catalog []byte
	blobs   map[string][]byte
}

func loadFixture(t *testing.T) fixtureEnvelope {
	t.Helper()
	dir := fixtureDir(t)
	raw, err := os.ReadFile(filepath.Join(dir, "session.json"))
	if err != nil {
		t.Fatalf("read fixture session: %v", err)
	}
	var session fixtureSession
	if err := json.Unmarshal(raw, &session); err != nil {
		t.Fatalf("decode fixture session: %v", err)
	}
	started, err := parseRFC3339(session.StartedAt)
	if err != nil {
		t.Fatalf("fixture startedAt: %v", err)
	}
	ended, err := parseRFC3339(session.EndedAt)
	if err != nil {
		t.Fatalf("fixture endedAt: %v", err)
	}
	catalog, err := os.ReadFile(filepath.Join(dir, "catalog.json"))
	if err != nil {
		t.Fatalf("read fixture catalog: %v", err)
	}
	blobs := map[string][]byte{}
	for _, code := range []string{
		"0x01", "0x05", "0x06", "0x0D", "0x13", "0x26",
		"0x2E", "0x38", "0x46", "0x48", "0x49",
	} {
		payload, err := os.ReadFile(filepath.Join(dir, code+"_raw.bin"))
		if err != nil {
			t.Fatalf("read fixture %s: %v", code, err)
		}
		blobs[code] = payload
	}
	return fixtureEnvelope{
		meta: store.SessionMeta{
			ID:         session.SessionID,
			DeviceMAC:  session.DeviceMAC,
			StartedAt:  started,
			EndedAt:    ended,
			BatteryPct: session.BatteryPercent,
		},
		catalog: catalog,
		blobs:   blobs,
	}
}

func parseRFC3339(value string) (time.Time, error) {
	return time.Parse(time.RFC3339, value)
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	dsn := testutil.NewDB(t)
	st, err := store.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(st.Close)
	return st
}

func TestRunRejectsInvalidCatalogAtomically(t *testing.T) {
	st := openStore(t)
	fixture := loadFixture(t)

	_, err := Run(context.Background(), st, fixture.meta, []byte("{not json"), fixture.blobs)
	var parseErr *parse.ParseError
	if !errors.As(err, &parseErr) || parseErr.Code != parse.ErrCodeInvalidCatalog {
		t.Fatalf("error = %v, want invalid_catalog", err)
	}

	coverage, err := st.GetCoverage(context.Background())
	if err != nil {
		t.Fatalf("coverage: %v", err)
	}
	if coverage.LastIngestAt != nil || coverage.HasData {
		t.Fatalf("coverage = %+v, want an untouched empty state", coverage)
	}
	days, err := st.ListDays(context.Background(), "2026-06-20", "2026-06-23")
	if err != nil {
		t.Fatalf("list days: %v", err)
	}
	if len(days) != 0 {
		t.Fatalf("daily rows = %d, want none after a rejected upload", len(days))
	}
}

func TestRunRejectsBadFramingAtomically(t *testing.T) {
	st := openStore(t)
	fixture := loadFixture(t)

	// A catalog is validated against its own payload, so a genuine framing
	// failure needs a catalog that describes the truncated 0x01 round.
	catalog := []byte(`{"sessionId":"fixture-dumpp-2026-06-22",` +
		`"chunked":[{"code":"0x01","status":"ok","roundStart":"2026-06-21T11:16:00.000"}],` +
		`"unsolicited":[]}`)

	_, err := Run(context.Background(), st, fixture.meta, catalog,
		map[string][]byte{"0x01": {0x00, 0x00, 0x00, 0x00}})
	var parseErr *parse.ParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("error = %v, want a parse error", err)
	}
	if parseErr.Code != parse.ErrCodeInvalidFraming || parseErr.TypeCode != "0x01" {
		t.Fatalf("error = %v, want invalid_framing for 0x01", err)
	}

	coverage, err := st.GetCoverage(context.Background())
	if err != nil {
		t.Fatalf("coverage: %v", err)
	}
	if coverage.LastIngestAt != nil || coverage.HasData {
		t.Fatalf("a rejected upload must not record a session: %+v", coverage)
	}

	// The rejection must not poison the session id, so the same envelope with
	// its real catalog is accepted afterwards.
	result, err := Run(context.Background(), st, fixture.meta, fixture.catalog, fixture.blobs)
	if err != nil {
		t.Fatalf("valid upload after a rejected one: %v", err)
	}
	if got := result.Types["0x01"]; got.Status != "accepted" || got.Rows != 4218 {
		t.Fatalf("0x01 = %+v, want the full accepted fixture", got)
	}
}

func TestRunStoresFixtureAndDerivesDailyRows(t *testing.T) {
	st := openStore(t)
	fixture := loadFixture(t)

	result, err := Run(context.Background(), st, fixture.meta, fixture.catalog, fixture.blobs)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.SessionID != fixture.meta.ID {
		t.Fatalf("session id = %s, want %s", result.SessionID, fixture.meta.ID)
	}
	if !result.Complete {
		t.Fatalf("complete = false, types = %+v", result.Types)
	}
	if got := result.Types["0x01"]; got.Status != "accepted" || got.Rows != 4218 || got.DataThrough == nil {
		t.Fatalf("0x01 = %+v, want 4218 accepted rows with a watermark", got)
	}
	spo2 := result.Types["0x26"]
	if spo2.Status != "raw_only" || spo2.DataThrough != nil {
		t.Fatalf("0x26 = %+v, want raw_only without a watermark", spo2)
	}
	if len(spo2.Warnings) == 0 {
		t.Fatal("0x26 must explain its firmware placeholders")
	}
	if result.Warnings == nil {
		t.Fatal("top-level warnings must be an empty array, not null")
	}

	ctx := context.Background()
	coverage, err := st.GetCoverage(ctx)
	if err != nil {
		t.Fatalf("coverage: %v", err)
	}
	if !coverage.HasData || coverage.LastIngestAt == nil {
		t.Fatalf("coverage = %+v, want recorded canonical data", coverage)
	}
	if coverage.Types["0x01"] == nil {
		t.Fatal("0x01 coverage watermark missing")
	}
	if coverage.Types["0x26"] != nil {
		t.Fatal("0x26 must never carry a coverage watermark")
	}

	days, err := st.ListDays(ctx, "2026-06-21", "2026-06-22")
	if err != nil {
		t.Fatalf("list days: %v", err)
	}
	if len(days) != 2 {
		t.Fatalf("daily rows = %d, want the exact canonical key set", len(days))
	}
	for _, day := range days {
		if len(day.ReadinessComponents) == 0 {
			t.Fatalf("day %s has no persisted readiness explanation", day.DayKey)
		}
		if day.ReadinessStatus == nil {
			t.Fatalf("day %s has no readiness status", day.DayKey)
		}
	}
}

func TestRunExactRetryAndChangedBytesConflict(t *testing.T) {
	st := openStore(t)
	fixture := loadFixture(t)
	ctx := context.Background()

	first, err := Run(ctx, st, fixture.meta, fixture.catalog, fixture.blobs)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	retry, err := Run(ctx, st, fixture.meta, fixture.catalog, fixture.blobs)
	if err != nil {
		t.Fatalf("exact retry: %v", err)
	}
	if len(retry.Types) != len(first.Types) {
		t.Fatalf("retry types = %d, want %d", len(retry.Types), len(first.Types))
	}
	for code, want := range first.Types {
		if got := retry.Types[code]; got.Rows != want.Rows || got.Status != want.Status {
			t.Fatalf("retry %s = %+v, want %+v", code, got, want)
		}
	}

	changed := map[string][]byte{}
	for code, payload := range fixture.blobs {
		changed[code] = payload
	}
	changed["0x01"] = append(append([]byte{}, fixture.blobs["0x01"]...), 0x00)
	if _, err := Run(ctx, st, fixture.meta, fixture.catalog, changed); !errors.Is(err, store.ErrSessionConflict) {
		t.Fatalf("changed bytes error = %v, want session_conflict", err)
	}

	days, err := st.ListDays(ctx, "2026-06-21", "2026-06-22")
	if err != nil {
		t.Fatalf("list days: %v", err)
	}
	if len(days) != 2 {
		t.Fatalf("daily rows = %d, want the original two days untouched", len(days))
	}
}
