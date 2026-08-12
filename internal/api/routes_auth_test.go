package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/heliolytics/api/internal/auth"
	"github.com/heliolytics/api/internal/config"
)

func TestProtectedRouteRequiresValidOneTimeToken(t *testing.T) {
	const secret = "route-auth-test-secret"
	mux := NewMux(nil, config.Config{
		SigningSecret:   secret,
		RateLimitPerMin: 100,
	})

	request := func(token string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/reparse", nil)
		if token != "" {
			req.Header.Set("X-Heliolytics-Token", token)
		}
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, req)
		return recorder
	}

	if recorder := request(""); recorder.Code != http.StatusUnauthorized {
		t.Fatalf("missing token status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}

	token, err := auth.SignToken(secret)
	if err != nil {
		t.Fatal(err)
	}
	if recorder := request(token); recorder.Code != http.StatusNotFound {
		t.Fatalf("valid token status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
	if recorder := request(token); recorder.Code != http.StatusUnauthorized {
		t.Fatalf("replayed token status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}
