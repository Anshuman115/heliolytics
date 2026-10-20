package api

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Anshuman115/heliolytics/internal/auth"
	"github.com/Anshuman115/heliolytics/internal/store"
)

// countingReader proves that a rejected request never touched its body.
type countingReader struct {
	reader io.Reader
	mu     sync.Mutex
	reads  int
}

func newCountingReader(data []byte) *countingReader {
	return &countingReader{reader: bytes.NewReader(data)}
}

func (c *countingReader) Read(p []byte) (int, error) {
	c.mu.Lock()
	c.reads++
	c.mu.Unlock()
	return c.reader.Read(p)
}

func (c *countingReader) ReadCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.reads
}

// blockingProfile holds one request inside the handler so its body slot stays
// taken.
type blockingProfile struct {
	entered chan struct{}
	release chan struct{}
}

func (b *blockingProfile) update(ctx context.Context, profile *store.Profile) error {
	close(b.entered)
	<-b.release
	return nil
}

const validProfileBody = `{"name":null,"birthDate":null,"sex":null,"heightCm":null,"weightKg":null}`

func TestProtectedRouteRejectsMissingAuth(t *testing.T) {
	nonces := newFakeNonces()
	mux := newMux(offlineStore(t), testConfig(testKeys()), (&stubIngest{}).run,
		(&stubProfile{}).update, nonces)
	recorder := serve(mux, httptest.NewRequest(http.MethodGet, "/api/v1/daily-metrics", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", recorder.Code)
	}
	if code := errorCode(t, recorder); code != "unauthorized" {
		t.Fatalf("code = %q", code)
	}
	if nonces.consumeCount() != 0 {
		t.Fatal("unauthenticated request consumed a nonce")
	}
}

func TestLegacyTokenHeaderIsNotAccepted(t *testing.T) {
	keys := testKeys()
	nonces := newFakeNonces()
	mux := newMux(offlineStore(t), testConfig(keys), (&stubIngest{}).run, (&stubProfile{}).update, nonces)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/daily-metrics", nil)
	req.Header.Set("X-Heliolytics-Token", "1760000000.00112233445566778899aabbccddeeff.deadbeef")
	recorder := serve(mux, req)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", recorder.Code)
	}
	if nonces.consumeCount() != 0 {
		t.Fatal("legacy token consumed a nonce")
	}
}

func TestAuthHeaderFailuresDoNotReadBody(t *testing.T) {
	keys := testKeys()
	body := []byte(validProfileBody)
	nonces := newFakeNonces()
	mux := newMux(offlineStore(t), testConfig(keys), (&stubIngest{}).run, (&stubProfile{}).update, nonces)

	mutations := map[string]func(*http.Request){
		"duplicate auth header": func(req *http.Request) {
			req.Header.Add(auth.HeaderAuth, req.Header.Get(auth.HeaderAuth))
		},
		"duplicate hash header": func(req *http.Request) {
			req.Header.Add(auth.HeaderBodySHA256, req.Header.Get(auth.HeaderBodySHA256))
		},
		"uppercase hash": func(req *http.Request) {
			req.Header.Set(auth.HeaderBodySHA256, strings.ToUpper(req.Header.Get(auth.HeaderBodySHA256)))
		},
		"v1 version": func(req *http.Request) {
			header := req.Header.Get(auth.HeaderAuth)
			req.Header.Set(auth.HeaderAuth, "v1"+strings.TrimPrefix(header, "v2"))
		},
		"unknown kid": func(req *http.Request) {
			parts := strings.Split(req.Header.Get(auth.HeaderAuth), ".")
			parts[1] = "admin"
			req.Header.Set(auth.HeaderAuth, strings.Join(parts, "."))
		},
		"uppercase signature": func(req *http.Request) {
			parts := strings.Split(req.Header.Get(auth.HeaderAuth), ".")
			parts[4] = strings.ToUpper(parts[4])
			req.Header.Set(auth.HeaderAuth, strings.Join(parts, "."))
		},
		"short nonce": func(req *http.Request) {
			parts := strings.Split(req.Header.Get(auth.HeaderAuth), ".")
			parts[3] = "abcd"
			req.Header.Set(auth.HeaderAuth, strings.Join(parts, "."))
		},
		"short signature": func(req *http.Request) {
			parts := strings.Split(req.Header.Get(auth.HeaderAuth), ".")
			parts[4] = parts[4][:32]
			req.Header.Set(auth.HeaderAuth, strings.Join(parts, "."))
		},
		"flipped signature": func(req *http.Request) {
			parts := strings.Split(req.Header.Get(auth.HeaderAuth), ".")
			signature := []byte(parts[4])
			if signature[0] == 'a' {
				signature[0] = 'b'
			} else {
				signature[0] = 'a'
			}
			parts[4] = string(signature)
			req.Header.Set(auth.HeaderAuth, strings.Join(parts, "."))
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			reader := newCountingReader(body)
			req := httptest.NewRequest(http.MethodPut, "/api/v1/profile", reader)
			req.Header.Set("Content-Type", "application/json")
			signRequest(t, keys, "phone", req, body)
			mutate(req)
			recorder := serve(mux, req)
			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
			}
			if code := errorCode(t, recorder); code != "unauthorized" {
				t.Fatalf("code = %q", code)
			}
			if reads := reader.ReadCount(); reads != 0 {
				t.Fatalf("body read %d times on an auth failure", reads)
			}
		})
	}
	if nonces.consumeCount() != 0 {
		t.Fatalf("auth failures consumed %d nonces", nonces.consumeCount())
	}
}

func TestTimestampOutsideWindowIsRejected(t *testing.T) {
	keys := testKeys()
	nonces := newFakeNonces()
	mux := newMux(offlineStore(t), testConfig(keys), (&stubIngest{}).run, (&stubProfile{}).update, nonces)
	body := []byte(validProfileBody)
	now := time.Now().Unix()

	cases := map[string]int64{
		"too old":       now - 301,
		"too far ahead": now + 61,
	}
	for name, ts := range cases {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPut, "/api/v1/profile", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			signRequestWithNonce(t, keys, "phone", req, body, randomNonce(t), ts)
			recorder := serve(mux, req)
			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d", recorder.Code)
			}
		})
	}
	if nonces.consumeCount() != 0 {
		t.Fatalf("out-of-window requests consumed %d nonces", nonces.consumeCount())
	}
}

func TestTimestampWindowEdgesAreAccepted(t *testing.T) {
	keys := testKeys()
	nonces := newFakeNonces()
	profile := &stubProfile{}
	mux := newMux(offlineStore(t), testConfig(keys), (&stubIngest{}).run, profile.update, nonces)
	body := []byte(validProfileBody)
	now := time.Now().Unix()

	for _, ts := range []int64{now - 300, now + 60} {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/profile", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		signRequestWithNonce(t, keys, "phone", req, body, randomNonce(t), ts)
		recorder := serve(mux, req)
		if recorder.Code != http.StatusNoContent {
			t.Fatalf("ts %d status = %d body = %s", ts, recorder.Code, recorder.Body.String())
		}
	}
}

func TestTamperedBodyIsRejectedWithoutNonce(t *testing.T) {
	keys := testKeys()
	nonces := newFakeNonces()
	profile := &stubProfile{}
	mux := newMux(offlineStore(t), testConfig(keys), (&stubIngest{}).run, profile.update, nonces)

	original := []byte(validProfileBody)
	// A trailing space keeps the JSON valid while changing the signed bytes.
	tampered := append(append([]byte(nil), original...), ' ')
	req := httptest.NewRequest(http.MethodPut, "/api/v1/profile", bytes.NewReader(tampered))
	req.Header.Set("Content-Type", "application/json")
	// Sign the original bytes so the declared hash no longer matches.
	signRequest(t, keys, "phone", req, original)

	recorder := serve(mux, req)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	if profile.calls != 0 || nonces.consumeCount() != 0 {
		t.Fatal("tampered body reached the service or the nonce")
	}

	resigned := httptest.NewRequest(http.MethodPut, "/api/v1/profile", bytes.NewReader(tampered))
	resigned.Header.Set("Content-Type", "application/json")
	signRequest(t, keys, "phone", resigned, tampered)
	if recorder := serve(mux, resigned); recorder.Code != http.StatusNoContent {
		t.Fatalf("re-signed tampered body status = %d body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestNonceReplayIsRejected(t *testing.T) {
	keys := testKeys()
	nonces := newFakeNonces()
	mux := newMux(offlineStore(t), testConfig(keys), (&stubIngest{}).run, (&stubProfile{}).update, nonces)
	body := []byte(validProfileBody)
	nonce := randomNonce(t)
	ts := time.Now().Unix()

	build := func() *http.Request {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/profile", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		signRequestWithNonce(t, keys, "phone", req, body, nonce, ts)
		return req
	}
	if recorder := serve(mux, build()); recorder.Code != http.StatusNoContent {
		t.Fatalf("first status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	replay := serve(mux, build())
	if replay.Code != http.StatusUnauthorized {
		t.Fatalf("replay status = %d", replay.Code)
	}
	if code := errorCode(t, replay); code != "unauthorized" {
		t.Fatalf("replay code = %q", code)
	}
}

func TestWebWriteIsForbiddenBeforeBodyRead(t *testing.T) {
	keys := testKeys()
	nonces := newFakeNonces()
	profile := &stubProfile{}
	mux := newMux(offlineStore(t), testConfig(keys), (&stubIngest{}).run, profile.update, nonces)
	body := []byte(validProfileBody)

	reader := newCountingReader(body)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/profile", reader)
	req.Header.Set("Content-Type", "application/json")
	signRequest(t, keys, "web", req, body)
	recorder := serve(mux, req)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	if code := errorCode(t, recorder); code != "forbidden" {
		t.Fatalf("code = %q", code)
	}
	if reads := reader.ReadCount(); reads != 0 {
		t.Fatalf("web write read the body %d times", reads)
	}
	if profile.calls != 0 || nonces.consumeCount() != 0 {
		t.Fatal("web write reached the service or the nonce")
	}
}

func TestBothKidsMayRead(t *testing.T) {
	st := testStore(t)
	keys := testKeys()
	mux := newMux(st, testConfig(keys), (&stubIngest{}).run, (&stubProfile{}).update, newFakeNonces())
	for _, kid := range []string{"phone", "web"} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics/coverage", nil)
		signRequest(t, keys, kid, req, nil)
		recorder := serve(mux, req)
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s status = %d body = %s", kid, recorder.Code, recorder.Body.String())
		}
	}
}

func TestGetWithBodyIsInvalidBody(t *testing.T) {
	keys := testKeys()
	nonces := newFakeNonces()
	mux := newMux(offlineStore(t), testConfig(keys), (&stubIngest{}).run, (&stubProfile{}).update, nonces)
	body := []byte("surprise")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics/coverage", bytes.NewReader(body))
	signRequest(t, keys, "phone", req, body)
	recorder := serve(mux, req)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", recorder.Code)
	}
	if code := errorCode(t, recorder); code != "invalid_body" {
		t.Fatalf("code = %q", code)
	}
	if nonces.consumeCount() != 0 {
		t.Fatal("invalid body consumed a nonce")
	}
}

func TestBodySlotsReturn503WhenFull(t *testing.T) {
	keys := testKeys()
	nonces := newFakeNonces()
	blocking := &blockingProfile{entered: make(chan struct{}), release: make(chan struct{})}
	cfg := testConfig(keys)
	cfg.MaxConcurrentBodyReads = 1
	mux := newMux(offlineStore(t), cfg, (&stubIngest{}).run, blocking.update, nonces)
	body := []byte(validProfileBody)

	first := httptest.NewRequest(http.MethodPut, "/api/v1/profile", bytes.NewReader(body))
	first.Header.Set("Content-Type", "application/json")
	signRequest(t, keys, "phone", first, body)

	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- serve(mux, first) }()
	select {
	case <-blocking.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first request never reached the handler")
	}

	second := httptest.NewRequest(http.MethodPut, "/api/v1/profile", bytes.NewReader(body))
	second.Header.Set("Content-Type", "application/json")
	signRequest(t, keys, "phone", second, body)
	rejected := serve(mux, second)
	if rejected.Code != http.StatusServiceUnavailable {
		t.Fatalf("second status = %d body = %s", rejected.Code, rejected.Body.String())
	}
	if code := errorCode(t, rejected); code != "server_busy" {
		t.Fatalf("code = %q", code)
	}
	if retry := rejected.Header().Get("Retry-After"); retry != "1" {
		t.Fatalf("Retry-After = %q", retry)
	}

	close(blocking.release)
	select {
	case recorder := <-done:
		if recorder.Code != http.StatusNoContent {
			t.Fatalf("first status = %d body = %s", recorder.Code, recorder.Body.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("first request never finished")
	}
	if nonces.consumeCount() != 1 {
		t.Fatalf("nonce consumed %d times, want 1", nonces.consumeCount())
	}
}

func TestNonceStoreFailureIsInternal(t *testing.T) {
	keys := testKeys()
	nonces := newFakeNonces()
	nonces.fail = errors.New("database unavailable")
	profile := &stubProfile{}
	mux := newMux(offlineStore(t), testConfig(keys), (&stubIngest{}).run, profile.update, nonces)
	body := []byte(validProfileBody)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/profile", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	signRequest(t, keys, "phone", req, body)
	recorder := serve(mux, req)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	if code := errorCode(t, recorder); code != "internal" {
		t.Fatalf("code = %q", code)
	}
	if profile.calls != 0 {
		t.Fatal("handler ran without replay protection")
	}
}

func TestNonceIsSharedAcrossMuxInstances(t *testing.T) {
	st := testStore(t)
	keys := testKeys()
	cfg := testConfig(keys)
	first := NewMux(st, cfg, (&stubIngest{}).run, (&stubProfile{}).update)
	second := NewMux(st, cfg, (&stubIngest{}).run, (&stubProfile{}).update)
	body := []byte(validProfileBody)
	nonce := randomNonce(t)
	ts := time.Now().Unix()

	build := func() *http.Request {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/profile", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		signRequestWithNonce(t, keys, "phone", req, body, nonce, ts)
		return req
	}
	if recorder := serve(first, build()); recorder.Code != http.StatusNoContent {
		t.Fatalf("first instance status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	if recorder := serve(second, build()); recorder.Code != http.StatusUnauthorized {
		t.Fatalf("second instance status = %d, want 401", recorder.Code)
	}
}
