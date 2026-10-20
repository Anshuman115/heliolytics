package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Anshuman115/heliolytics/internal/store"
)

func TestCoverageCarriesEveryMeasuredType(t *testing.T) {
	keys := testKeys()
	mux := newMux(testStore(t), testConfig(keys), (&stubIngest{}).run, (&stubProfile{}).update, newFakeNonces())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics/coverage", nil)
	signRequest(t, keys, "web", req, nil)
	recorder := serve(mux, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}

	var body struct {
		HasData      bool               `json:"hasData"`
		LastIngestAt *string            `json:"lastIngestAt"`
		DataThrough  *string            `json:"dataThrough"`
		Types        map[string]*string `json:"types"`
	}
	decodeJSON(t, recorder, &body)

	if body.HasData {
		t.Fatal("hasData true on an empty database")
	}
	if body.LastIngestAt != nil || body.DataThrough != nil {
		t.Fatalf("lastIngestAt=%v dataThrough=%v, want nulls", body.LastIngestAt, body.DataThrough)
	}
	if len(body.Types) != len(store.MeasuredTypeCodes) {
		t.Fatalf("types = %d keys, want %d", len(body.Types), len(store.MeasuredTypeCodes))
	}
	for _, code := range store.MeasuredTypeCodes {
		value, present := body.Types[code]
		if !present {
			t.Fatalf("types missing %s", code)
		}
		if value != nil {
			t.Fatalf("types[%s] = %v, want null", code, *value)
		}
	}
	if _, present := body.Types["0x26"]; present {
		t.Fatal("raw-only 0x26 must never appear in coverage")
	}
}

func TestCoverageReportsStoredWatermark(t *testing.T) {
	st := testStore(t)
	keys := testKeys()
	mux := newMux(st, testConfig(keys), (&stubIngest{}).run, (&stubProfile{}).update, newFakeNonces())

	seed(t, st, `INSERT INTO sync_sessions (session_id, device_mac, started_at, ended_at, catalog)
		VALUES ('coverage-session', '02:00:00:00:00:01', '2026-10-01T00:00:00Z', '2026-10-01T01:00:00Z',
		'{"chunked":[]}'::jsonb)`)
	seed(t, st, `INSERT INTO coverage (type_code, data_through)
		VALUES ('0x01', '2026-10-01T00:55:00Z')`)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics/coverage", nil)
	signRequest(t, keys, "web", req, nil)
	recorder := serve(mux, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}

	var body struct {
		HasData      bool               `json:"hasData"`
		LastIngestAt *string            `json:"lastIngestAt"`
		DataThrough  *string            `json:"dataThrough"`
		Types        map[string]*string `json:"types"`
	}
	decodeJSON(t, recorder, &body)
	if !body.HasData {
		t.Fatal("hasData false with a stored watermark")
	}
	if body.DataThrough == nil || *body.DataThrough != "2026-10-01T00:55:00Z" {
		t.Fatalf("dataThrough = %v", body.DataThrough)
	}
	if body.LastIngestAt == nil {
		t.Fatal("lastIngestAt null after a committed session")
	}
	if body.Types["0x01"] == nil || *body.Types["0x01"] != "2026-10-01T00:55:00Z" {
		t.Fatalf("types[0x01] = %v", body.Types["0x01"])
	}
	if body.Types["0x46"] != nil {
		t.Fatalf("types[0x46] = %v, want null", *body.Types["0x46"])
	}
}
