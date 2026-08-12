package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/heliolytics/api/internal/auth"
	"github.com/heliolytics/api/internal/config"
	"github.com/heliolytics/api/internal/store"
)

const hrvTestSigningSecret = "test-secret-value"

// testAPIStore mirrors internal/store's own testStore helper (unexported
// there, so internal/api needs its own copy): skip unless a test database is
// configured, otherwise connect and hand back a cleanup func.
func testAPIStore(t *testing.T) (*store.Store, func()) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	st, err := store.New(ctx, url)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if err := st.Ping(ctx); err != nil {
		st.Close()
		t.Fatalf("Ping: %v", err)
	}
	return st, func() { st.Close() }
}

func TestHrvRouteReturnsPoints(t *testing.T) {
	st, cleanup := testAPIStore(t)
	defer cleanup()

	ctx := context.Background()
	const day = "2026-06-22"
	const sid = "hrv-route-test"
	clean := func() { st.Pool().Exec(ctx, `DELETE FROM hrv_samples WHERE day_key=$1::date`, day) }
	clean()
	t.Cleanup(clean)

	// Seed one sample so the route has real data to return.
	pts := []store.SampleValue{{
		DayKey:    day,
		SampledAt: time.Date(2026, 6, 22, 2, 0, 0, 0, time.UTC),
		Value:     45,
	}}
	if err := st.UpsertHrvSamples(ctx, sid, pts); err != nil {
		t.Fatalf("seed UpsertHrvSamples: %v", err)
	}

	mux := NewMux(st, config.Config{SigningSecret: hrvTestSigningSecret})

	token, err := auth.SignToken(hrvTestSigningSecret)
	if err != nil {
		t.Fatalf("SignToken: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics/hrv?from=2020-01-01&to=2030-01-01", nil)
	req.Header.Set("X-Heliolytics-Token", token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Points []struct {
			SampledAt string  `json:"sampledAt"`
			Value     float64 `json:"value"`
		} `json:"points"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	found := false
	for _, p := range body.Points {
		if p.Value == 45 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected a point with value 45 in response, got %+v", body.Points)
	}
}
