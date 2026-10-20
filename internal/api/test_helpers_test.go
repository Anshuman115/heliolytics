package api

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Anshuman115/heliolytics/internal/auth"
	"github.com/Anshuman115/heliolytics/internal/config"
	"github.com/Anshuman115/heliolytics/internal/store"
	"github.com/Anshuman115/heliolytics/internal/testutil"
	"github.com/jackc/pgx/v5"
)

// testKeys are fixed, non-published secrets so tests never reuse the public
// signing fixture.
func testKeys() auth.KeySet {
	var phone, web [32]byte
	for i := range phone {
		phone[i] = byte(0x40 + i)
		web[i] = byte(0x80 + i)
	}
	return auth.NewKeySet(phone, web)
}

// testConfig is a valid configuration with generous limits.
func testConfig(keys auth.KeySet) config.Config {
	return config.Config{
		Keys:                   keys,
		RateLimitPerMin:        1000,
		RateLimitPerKeyPerMin:  1000,
		RequestTimeout:         5 * time.Second,
		MaxConcurrentBodyReads: 2,
	}
}

// fakeNonces is a replay store with the same semantics as the real one but no
// database.
type fakeNonces struct {
	mu    sync.Mutex
	used  map[string]struct{}
	fail  error
	calls int
}

func newFakeNonces() *fakeNonces { return &fakeNonces{used: make(map[string]struct{})} }

func (f *fakeNonces) ConsumeNonce(ctx context.Context, kid, nonce string, expiresAt time.Time) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.fail != nil {
		return false, f.fail
	}
	key := kid + "\x00" + nonce
	if _, seen := f.used[key]; seen {
		return false, nil
	}
	f.used[key] = struct{}{}
	return true, nil
}

func (f *fakeNonces) consumeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// offlineStore cannot reach a database, so a route that queries fails the test.
func offlineStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.New(context.Background(),
		"postgres://heliolytics:heliolytics@127.0.0.1:1/heliolytics_test?sslmode=disable")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(st.Close)
	return st
}

// testStore returns a store on a fresh database. The pool closes before the
// helper drops the database because cleanups run last-in first-out.
func testStore(t *testing.T) *store.Store {
	t.Helper()
	dsn := testutil.NewDB(t)
	st, err := store.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(st.Close)
	return st
}

// seed executes test fixture SQL in its own transaction.
func seed(t *testing.T, st *store.Store, statement string, args ...any) {
	t.Helper()
	ctx := context.Background()
	if err := st.WithTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, statement, args...)
		return err
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

// signRequest signs a request with a fresh nonce and the current time.
func signRequest(t *testing.T, keys auth.KeySet, kid string, req *http.Request, body []byte) {
	t.Helper()
	signRequestWithNonce(t, keys, kid, req, body, randomNonce(t), time.Now().Unix())
}

// signRequestWithNonce signs a request with the supplied nonce and timestamp.
func signRequestWithNonce(t *testing.T, keys auth.KeySet, kid string, req *http.Request,
	body []byte, nonce string, ts int64) {
	t.Helper()
	key, err := keys.Lookup(kid)
	if err != nil {
		t.Fatalf("Lookup(%q): %v", kid, err)
	}
	secret, err := hex.DecodeString(key.SecretHex())
	if err != nil {
		t.Fatalf("secret hex: %v", err)
	}
	sum := sha256.Sum256(body)
	hash := hex.EncodeToString(sum[:])
	canonical := auth.CanonicalText(kid, ts, nonce, req.Method,
		req.URL.EscapedPath(), req.URL.RawQuery, hash, req.Header.Get("Content-Type"))
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(canonical))
	req.Header.Set(auth.HeaderAuth, fmt.Sprintf("v2.%s.%d.%s.%s", kid, ts, nonce, hex.EncodeToString(mac.Sum(nil))))
	req.Header.Set(auth.HeaderBodySHA256, hash)
}

func randomNonce(t *testing.T) string {
	t.Helper()
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return hex.EncodeToString(buf)
}

// stubIngest records the single call the API makes and answers with result.
type stubIngest struct {
	calls   int
	meta    store.SessionMeta
	catalog []byte
	blobs   map[string][]byte
	result  store.IngestResult
	err     error
}

func (s *stubIngest) run(ctx context.Context, meta store.SessionMeta,
	catalog []byte, blobs map[string][]byte) (store.IngestResult, error) {
	s.calls++
	s.meta, s.catalog, s.blobs = meta, catalog, blobs
	return s.result, s.err
}

// stubProfile records the single callback and answers with err.
type stubProfile struct {
	calls   int
	profile *store.Profile
	err     error
}

func (s *stubProfile) update(ctx context.Context, profile *store.Profile) error {
	s.calls++
	s.profile = profile
	return s.err
}

// serve runs one request through a handler.
func serve(handler http.Handler, req *http.Request) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder
}

// decodeJSON decodes a recorded body into target.
func decodeJSON(t *testing.T, recorder *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(recorder.Body.Bytes(), target); err != nil {
		t.Fatalf("decode body %q: %v", recorder.Body.String(), err)
	}
}

// errorCode returns the C3 error code carried by a response.
func errorCode(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	decodeJSON(t, recorder, &envelope)
	return envelope.Error.Code
}
