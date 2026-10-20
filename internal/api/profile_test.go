package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Anshuman115/heliolytics/internal/auth"
	"github.com/Anshuman115/heliolytics/internal/store"
)

// profileMux builds a mux with offline storage and the supplied callbacks.
func profileMux(t *testing.T, keys auth.KeySet, nonces *fakeNonces, profile *stubProfile) http.Handler {
	t.Helper()
	return newMux(offlineStore(t), testConfig(keys), (&stubIngest{}).run, profile.update, nonces)
}

// putProfile sends one signed profile replacement.
func putProfile(t *testing.T, handler http.Handler, keys auth.KeySet, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/profile", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	signRequest(t, keys, "phone", req, body)
	return serve(handler, req)
}

func TestProfileFullReplacementCallsCallbackOnce(t *testing.T) {
	keys := testKeys()
	nonces := newFakeNonces()
	profile := &stubProfile{}
	handler := profileMux(t, keys, nonces, profile)
	body := []byte(`{"name":"Anshuman","birthDate":"1995-04-12","sex":"male","heightCm":178.0,"weightKg":74.5}`)

	recorder := putProfile(t, handler, keys, body)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	if recorder.Body.Len() != 0 {
		t.Fatalf("204 carried a body: %q", recorder.Body.String())
	}
	if profile.calls != 1 {
		t.Fatalf("callback calls = %d, want 1", profile.calls)
	}
	got := profile.profile
	if got == nil || got.Name == nil || *got.Name != "Anshuman" {
		t.Fatalf("name = %v", got)
	}
	if got.BirthDate == nil || got.BirthDate.Format("2006-01-02") != "1995-04-12" {
		t.Fatalf("birthDate = %v", got.BirthDate)
	}
	if got.Sex == nil || *got.Sex != "male" {
		t.Fatalf("sex = %v", got.Sex)
	}
	if got.HeightCm == nil || *got.HeightCm != 178 || got.WeightKg == nil || *got.WeightKg != 74.5 {
		t.Fatalf("height/weight = %v/%v", got.HeightCm, got.WeightKg)
	}
	if nonces.consumeCount() != 1 {
		t.Fatalf("nonce consumed %d times", nonces.consumeCount())
	}
}

func TestProfileNullsClearEveryField(t *testing.T) {
	keys := testKeys()
	profile := &stubProfile{}
	handler := profileMux(t, keys, newFakeNonces(), profile)
	body := []byte(`{"name":null,"birthDate":null,"sex":null,"heightCm":null,"weightKg":null}`)

	recorder := putProfile(t, handler, keys, body)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	got := profile.profile
	if got == nil || got.Name != nil || got.BirthDate != nil || got.Sex != nil ||
		got.HeightCm != nil || got.WeightKg != nil {
		t.Fatalf("cleared profile = %+v", got)
	}
}

func TestProfileStructuralFailuresAre400(t *testing.T) {
	keys := testKeys()
	cases := map[string]string{
		"empty body":      ``,
		"array":           `[]`,
		"trailing":        `{"name":null,"birthDate":null,"sex":null,"heightCm":null,"weightKg":null} {}`,
		"unknown key":     `{"name":null,"birthDate":null,"sex":null,"heightCm":null,"weightKg":null,"age":30}`,
		"missing key":     `{"name":null,"birthDate":null,"sex":null,"heightCm":null}`,
		"duplicate key":   `{"name":null,"name":null,"birthDate":null,"sex":null,"heightCm":null,"weightKg":null}`,
		"null root":       `null`,
		"name wrong type": `{"name":7,"birthDate":null,"sex":null,"heightCm":null,"weightKg":null}`,
		"number as text":  `{"name":null,"birthDate":null,"sex":null,"heightCm":"178","weightKg":null}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			nonces := newFakeNonces()
			profile := &stubProfile{}
			handler := profileMux(t, keys, nonces, profile)
			recorder := putProfile(t, handler, keys, []byte(body))
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
			}
			if code := errorCode(t, recorder); code != "invalid_body" {
				t.Fatalf("code = %q", code)
			}
			if profile.calls != 0 {
				t.Fatalf("callback called %d times", profile.calls)
			}
			if nonces.consumeCount() != 0 {
				t.Fatalf("structural failure consumed a nonce")
			}
		})
	}
}

func TestProfileSemanticFailuresAre422(t *testing.T) {
	keys := testKeys()
	longName := strings.Repeat("a", 101)
	cases := map[string]string{
		"future birth date": `{"name":null,"birthDate":"2999-01-01","sex":null,"heightCm":null,"weightKg":null}`,
		"1900 boundary":     `{"name":null,"birthDate":"1900-01-01","sex":null,"heightCm":null,"weightKg":null}`,
		"bad date":          `{"name":null,"birthDate":"12/04/1995","sex":null,"heightCm":null,"weightKg":null}`,
		"sex typo":          `{"name":null,"birthDate":null,"sex":"m","heightCm":null,"weightKg":null}`,
		"short height":      `{"name":null,"birthDate":null,"sex":null,"heightCm":49.9,"weightKg":null}`,
		"tall height":       `{"name":null,"birthDate":null,"sex":null,"heightCm":250.1,"weightKg":null}`,
		"light weight":      `{"name":null,"birthDate":null,"sex":null,"heightCm":null,"weightKg":19.9}`,
		"heavy weight":      `{"name":null,"birthDate":null,"sex":null,"heightCm":null,"weightKg":300.1}`,
		"name control":      `{"name":"a\u0000b","birthDate":null,"sex":null,"heightCm":null,"weightKg":null}`,
		"name too long":     `{"name":"` + longName + `","birthDate":null,"sex":null,"heightCm":null,"weightKg":null}`,
		"nonfinite height":  `{"name":null,"birthDate":null,"sex":null,"heightCm":1e999,"weightKg":null}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			nonces := newFakeNonces()
			profile := &stubProfile{}
			handler := profileMux(t, keys, nonces, profile)
			recorder := putProfile(t, handler, keys, []byte(body))
			if recorder.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
			}
			if code := errorCode(t, recorder); code != "invalid_profile" {
				t.Fatalf("code = %q", code)
			}
			if profile.calls != 0 {
				t.Fatalf("callback called %d times", profile.calls)
			}
			if nonces.consumeCount() != 1 {
				t.Fatalf("semantic failure consumed %d nonces, want 1", nonces.consumeCount())
			}
		})
	}
}

func TestProfileOversizeIs413(t *testing.T) {
	keys := testKeys()
	nonces := newFakeNonces()
	profile := &stubProfile{}
	handler := profileMux(t, keys, nonces, profile)
	body := []byte(strings.Repeat("a", int(maxProfileBody)+1))

	recorder := putProfile(t, handler, keys, body)
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d", recorder.Code)
	}
	if code := errorCode(t, recorder); code != "payload_too_large" {
		t.Fatalf("code = %q", code)
	}
	if profile.calls != 0 || nonces.consumeCount() != 0 {
		t.Fatalf("oversize request reached the service or the nonce")
	}
}

func TestProfileCallbackFailureIsNot204(t *testing.T) {
	keys := testKeys()
	body := []byte(`{"name":null,"birthDate":null,"sex":null,"heightCm":null,"weightKg":null}`)
	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"store", errors.New("db down"), http.StatusInternalServerError, "internal"},
		{"invalid row", &store.InvalidRowError{Table: "profile", Reason: "rejected"},
			http.StatusUnprocessableEntity, "invalid_profile"},
		{"deadline", context.DeadlineExceeded, http.StatusServiceUnavailable, "request_timeout"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			profile := &stubProfile{err: tc.err}
			handler := profileMux(t, keys, newFakeNonces(), profile)
			recorder := putProfile(t, handler, keys, body)
			if recorder.Code != tc.status {
				t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
			}
			if code := errorCode(t, recorder); code != tc.code {
				t.Fatalf("code = %q, want %q", code, tc.code)
			}
		})
	}
}
