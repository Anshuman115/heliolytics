package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewMuxPanicsOnNilDependency(t *testing.T) {
	keys := testKeys()
	cfg := testConfig(keys)
	st := offlineStore(t)
	ingest := (&stubIngest{}).run
	profile := (&stubProfile{}).update

	cases := []struct {
		name  string
		build func()
	}{
		{"store", func() { NewMux(nil, cfg, ingest, profile) }},
		{"ingest", func() { NewMux(st, cfg, nil, profile) }},
		{"profile", func() { NewMux(st, cfg, ingest, nil) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				recovered := recover()
				if recovered == nil {
					t.Fatal("expected a panic for a nil dependency")
				}
				if message, ok := recovered.(string); !ok || message != "api.NewMux: nil dependency" {
					t.Fatalf("panic = %v, want api.NewMux: nil dependency", recovered)
				}
			}()
			tc.build()
		})
	}
}

func TestHealthIsPublicText(t *testing.T) {
	mux := newMux(offlineStore(t), testConfig(testKeys()), (&stubIngest{}).run, (&stubProfile{}).update, newFakeNonces())
	recorder := serve(mux, httptest.NewRequest(http.MethodGet, "/health", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if body := recorder.Body.String(); body != "ok" {
		t.Fatalf("body = %q, want ok", body)
	}
	if contentType := recorder.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/plain") {
		t.Fatalf("content type = %q", contentType)
	}
}

func TestVersionReportsNullsWithoutBuildMetadata(t *testing.T) {
	mux := newMux(offlineStore(t), testConfig(testKeys()), (&stubIngest{}).run, (&stubProfile{}).update, newFakeNonces())
	recorder := serve(mux, httptest.NewRequest(http.MethodGet, "/version", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	var body struct {
		Commit  *string `json:"commit"`
		BuiltAt *string `json:"builtAt"`
	}
	decodeJSON(t, recorder, &body)
	if body.Commit != nil || body.BuiltAt != nil {
		t.Fatalf("commit=%v builtAt=%v, want nulls", body.Commit, body.BuiltAt)
	}
}

func TestVersionReportsInjectedMetadata(t *testing.T) {
	cfg := testConfig(testKeys())
	cfg.Commit = "local-w2"
	cfg.BuiltAt = "2026-10-20T00:00:00Z"
	mux := newMux(offlineStore(t), cfg, (&stubIngest{}).run, (&stubProfile{}).update, newFakeNonces())
	recorder := serve(mux, httptest.NewRequest(http.MethodGet, "/version", nil))
	var body struct {
		Commit  *string `json:"commit"`
		BuiltAt *string `json:"builtAt"`
	}
	decodeJSON(t, recorder, &body)
	if body.Commit == nil || *body.Commit != "local-w2" {
		t.Fatalf("commit = %v", body.Commit)
	}
	if body.BuiltAt == nil || *body.BuiltAt != "2026-10-20T00:00:00Z" {
		t.Fatalf("builtAt = %v", body.BuiltAt)
	}
}

func TestUnknownAndDeletedPathsAreJSON404(t *testing.T) {
	nonces := newFakeNonces()
	mux := newMux(offlineStore(t), testConfig(testKeys()), (&stubIngest{}).run, (&stubProfile{}).update, nonces)

	paths := []string{
		"/api/v1/reparse", "/api/v1/strain", "/api/v1/recovery",
		"/api/v1/metrics/hrv", "/api/v1/metrics/rhr", "/api/v1/metrics/vo2max",
		"/api/v1/metrics/workouts", "/api/v1/metrics/activity-sessions",
		"/api/v1/daily-health-scores",
		"/api/v1/daily-metrics/", "/api/v1//daily-metrics", "/api/v1/../v1/daily-metrics",
		"/api/v1", "/", "/api/v2/daily-metrics",
	}
	for _, path := range paths {
		recorder := serve(mux, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d, want 404", path, recorder.Code)
		}
		if code := errorCode(t, recorder); code != "not_found" {
			t.Fatalf("%s code = %q, want not_found", path, code)
		}
	}
	if calls := nonces.consumeCount(); calls != 0 {
		t.Fatalf("nonce consumed %d times on unknown paths", calls)
	}
}

func TestWrongMethodIsJSON405WithAllow(t *testing.T) {
	nonces := newFakeNonces()
	mux := newMux(offlineStore(t), testConfig(testKeys()), (&stubIngest{}).run, (&stubProfile{}).update, nonces)

	cases := []struct {
		method string
		path   string
		allow  string
	}{
		{http.MethodPost, "/api/v1/daily-metrics", http.MethodGet},
		{http.MethodDelete, "/api/v1/sleep", http.MethodGet},
		{http.MethodOptions, "/api/v1/metrics/series", http.MethodGet},
		{http.MethodHead, "/api/v1/daily-metrics", http.MethodGet},
		{http.MethodGet, "/api/v1/ingest", http.MethodPost},
		{http.MethodGet, "/api/v1/profile", http.MethodPut},
		{http.MethodPost, "/api/v1/profile", http.MethodPut},
		{http.MethodPut, "/api/v1/metrics/coverage", http.MethodGet},
	}
	for _, tc := range cases {
		recorder := serve(mux, httptest.NewRequest(tc.method, tc.path, nil))
		if recorder.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s %s status = %d, want 405", tc.method, tc.path, recorder.Code)
		}
		if allow := recorder.Header().Get("Allow"); allow != tc.allow {
			t.Fatalf("%s %s allow = %q, want %q", tc.method, tc.path, allow, tc.allow)
		}
		if tc.method == http.MethodHead {
			continue
		}
		if code := errorCode(t, recorder); code != "method_not_allowed" {
			t.Fatalf("%s %s code = %q", tc.method, tc.path, code)
		}
	}
	if calls := nonces.consumeCount(); calls != 0 {
		t.Fatalf("nonce consumed %d times on rejected methods", calls)
	}
}

func TestRangeValidationRejectsBadInput(t *testing.T) {
	keys := testKeys()
	nonces := newFakeNonces()
	mux := newMux(offlineStore(t), testConfig(keys), (&stubIngest{}).run, (&stubProfile{}).update, nonces)

	cases := []struct {
		target string
		code   string
	}{
		{"/api/v1/daily-metrics?from=2026-10-01", "invalid_date"},
		{"/api/v1/daily-metrics?to=2026-10-01", "invalid_date"},
		{"/api/v1/daily-metrics?from=&to=", "invalid_date"},
		{"/api/v1/daily-metrics?from=2026-1-5&to=2026-10-01", "invalid_date"},
		{"/api/v1/daily-metrics?from=2026-02-30&to=2026-03-01", "invalid_date"},
		{"/api/v1/daily-metrics?from=2026-10-01&to=2026-10-01&from=2026-10-02", "invalid_date"},
		{"/api/v1/daily-metrics?from=2026-10-05&to=2026-10-01", "invalid_range"},
		{"/api/v1/daily-metrics?from=2024-01-01&to=2025-01-01", "range_too_large"},
		{"/api/v1/metrics/hr?from=2026-01-01&to=2026-02-01", "range_too_large"},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, tc.target, nil)
		signRequest(t, keys, "phone", req, nil)
		recorder := serve(mux, req)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d, want 400", tc.target, recorder.Code)
		}
		if code := errorCode(t, recorder); code != tc.code {
			t.Fatalf("%s code = %q, want %q", tc.target, code, tc.code)
		}
	}
	// C5 consumes the nonce before the handler, so each authenticated request
	// uses exactly one even when its range is rejected.
	if calls := nonces.consumeCount(); calls != len(cases) {
		t.Fatalf("nonce consumed %d times on invalid ranges, want %d", calls, len(cases))
	}
}

func TestDefaultRangeIsThirtyISTDays(t *testing.T) {
	// 18:45Z on 2026-10-19 is 00:15 IST on 2026-10-20, so the IST day is the
	// 20th even though the UTC day is still the 19th.
	now := time.Date(2026, 10, 19, 18, 45, 0, 0, time.UTC)
	handler := &metricsHandler{now: func() time.Time { return now }}
	from, to, failure := handler.rangeBounds(
		httptest.NewRequest(http.MethodGet, "/api/v1/daily-metrics", nil), maxDailyRangeDays)
	if failure != nil {
		t.Fatalf("default range rejected: %v", failure)
	}
	if from != "2026-09-21" || to != "2026-10-20" {
		t.Fatalf("from=%s to=%s, want 2026-09-21 and 2026-10-20", from, to)
	}
}

func TestRangeCapsAcceptExactBoundaries(t *testing.T) {
	handler := &metricsHandler{now: time.Now}
	cases := []struct {
		name    string
		target  string
		maxDays int
	}{
		{"same day", "/api/v1/daily-metrics?from=2026-10-20&to=2026-10-20", maxDailyRangeDays},
		{"leap day", "/api/v1/daily-metrics?from=2024-02-28&to=2024-03-01", maxDailyRangeDays},
		{"366 days", "/api/v1/daily-metrics?from=2024-01-01&to=2024-12-31", maxDailyRangeDays},
		{"31 samples", "/api/v1/metrics/hr?from=2026-10-01&to=2026-10-31", maxSampleRangeDays},
	}
	for _, tc := range cases {
		_, _, failure := handler.rangeBounds(httptest.NewRequest(http.MethodGet, tc.target, nil), tc.maxDays)
		if failure != nil {
			t.Fatalf("%s rejected: %s", tc.name, failure.Code)
		}
	}
}
