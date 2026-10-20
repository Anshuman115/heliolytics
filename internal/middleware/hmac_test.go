package middleware

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Anshuman115/heliolytics/internal/auth"
)

type nonceRecorder struct {
	mu    sync.Mutex
	used  map[string]bool
	calls int
}

func newNonceRecorder() *nonceRecorder {
	return &nonceRecorder{used: make(map[string]bool)}
}

func (n *nonceRecorder) ConsumeNonce(ctx context.Context, kid, nonce string, expiresAt time.Time) (bool, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.calls++
	key := kid + "/" + nonce
	if n.used[key] {
		return false, nil
	}
	n.used[key] = true
	return true, nil
}

func (n *nonceRecorder) count() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.calls
}

func testKeySet() auth.KeySet {
	var phone, web [32]byte
	for i := range phone {
		phone[i] = byte(0x10 + i)
		web[i] = byte(0x60 + i)
	}
	return auth.NewKeySet(phone, web)
}

func signedRequest(t *testing.T, keys auth.KeySet, kid, method, target, contentType string, body []byte) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, target, bytes.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	key, err := keys.Lookup(kid)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	secret, err := hex.DecodeString(key.SecretHex())
	if err != nil {
		t.Fatalf("secret: %v", err)
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		t.Fatalf("nonce: %v", err)
	}
	nonce := hex.EncodeToString(raw)
	ts := time.Now().Unix()
	sum := sha256.Sum256(body)
	hash := hex.EncodeToString(sum[:])
	canonical := auth.CanonicalText(kid, ts, nonce, method, req.URL.EscapedPath(), req.URL.RawQuery, hash, contentType)
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(canonical))
	req.Header.Set(auth.HeaderAuth, fmt.Sprintf("%s.%d.%s.%s", kid, ts, nonce, hex.EncodeToString(mac.Sum(nil))))
	req.Header.Set(auth.HeaderBodySHA256, hash)
	return req
}

func testGuardConfig(keys auth.KeySet, nonces NonceStore) GuardConfig {
	return GuardConfig{
		Keys:     keys,
		Policy:   IPPolicy{},
		IPLimit:  NewLimiter(1000, 100),
		KeyLimit: NewLimiter(1000, 4),
		Slots:    NewSemaphore(2),
		Nonces:   nonces,
		Logger:   log.New(io.Discard, "", 0),
	}
}

type unreadBody struct {
	t *testing.T
}

func (u unreadBody) Read(p []byte) (int, error) {
	u.t.Error("rejected request read its body")
	return 0, io.EOF
}

func TestGuardPassesAuthenticatedRequest(t *testing.T) {
	keys := testKeySet()
	nonces := newNonceRecorder()
	guard := NewGuard(testGuardConfig(keys, nonces))
	body := []byte(`{"name":null,"birthDate":null,"sex":null,"heightCm":null,"weightKg":null}`)
	prepare := func(r *http.Request, captured []byte) (any, error) { return string(captured), nil }

	var gotKid string
	var gotBody []byte
	var gotPayload any
	handler := guard.Protect(64<<10, prepare, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKid = Kid(r)
		gotBody = RequestBody(r)
		gotPayload = Payload(r)
		w.WriteHeader(http.StatusNoContent)
	}))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, signedRequest(t, keys, "phone", http.MethodPut, "/api/profile", "application/json", body))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	if gotKid != "phone" {
		t.Fatalf("kid = %q", gotKid)
	}
	if !bytes.Equal(gotBody, body) {
		t.Fatalf("body = %q", gotBody)
	}
	if gotPayload != string(body) {
		t.Fatalf("payload = %v", gotPayload)
	}
	if nonces.count() != 1 {
		t.Fatalf("nonce calls = %d", nonces.count())
	}
}

func TestGuardRejectsBadSignatureWithoutReadingBody(t *testing.T) {
	keys := testKeySet()
	nonces := newNonceRecorder()
	guard := NewGuard(testGuardConfig(keys, nonces))
	handler := guard.Protect(64<<10, nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler ran on a bad signature")
	}))

	req := signedRequest(t, keys, "phone", http.MethodPut, "/api/profile", "application/json", []byte("{}"))
	req.Body = io.NopCloser(unreadBody{t: t})
	header := req.Header.Get(auth.HeaderAuth)
	parts := strings.Split(header, ".")
	signature := []byte(parts[3])
	if signature[0] == 'a' {
		signature[0] = 'b'
	} else {
		signature[0] = 'a'
	}
	parts[3] = string(signature)
	req.Header.Set(auth.HeaderAuth, strings.Join(parts, "."))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", recorder.Code)
	}
	if nonces.count() != 0 {
		t.Fatal("bad signature consumed a nonce")
	}
}

func TestGuardForbidsWebWritesBeforeBodyRead(t *testing.T) {
	keys := testKeySet()
	nonces := newNonceRecorder()
	guard := NewGuard(testGuardConfig(keys, nonces))
	handler := guard.Protect(64<<10, nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler ran for a web write")
	}))

	req := signedRequest(t, keys, "web", http.MethodPut, "/api/profile", "application/json", []byte("{}"))
	req.Body = io.NopCloser(unreadBody{t: t})

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d", recorder.Code)
	}
	if nonces.count() != 0 {
		t.Fatal("forbidden write consumed a nonce")
	}
}

func TestGuardPreAuthRateLimit(t *testing.T) {
	keys := testKeySet()
	nonces := newNonceRecorder()
	cfg := testGuardConfig(keys, nonces)
	fixed := time.Unix(1760000000, 0)
	cfg.Clock = func() time.Time { return fixed }
	cfg.IPLimit = NewLimiterWithClock(60, 10, func() time.Time { return fixed })
	guard := NewGuard(cfg)
	handler := guard.Protect(0, nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := range 60 {
		if allowed, _ := cfg.IPLimit.Allow("192.0.2.1"); !allowed {
			t.Fatalf("drain request %d denied", i)
		}
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, signedRequest(t, keys, "phone", http.MethodGet, "/api/daily-metrics", "", nil))
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d", recorder.Code)
	}
	if retry := recorder.Header().Get("Retry-After"); retry != "1" {
		t.Fatalf("Retry-After = %q, want 1", retry)
	}
	if code := recordedCode(t, recorder); code != CodeRateLimited {
		t.Fatalf("code = %q", code)
	}
	if nonces.count() != 0 {
		t.Fatal("a rate-limited request consumed a nonce")
	}
}

func TestGuardTrustedPeerHeaderFailure(t *testing.T) {
	keys := testKeySet()
	nonces := newNonceRecorder()
	cfg := testGuardConfig(keys, nonces)
	cfg.Policy = IPPolicy{TrustProxy: true, Trusted: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}}
	guard := NewGuard(cfg)
	handler := guard.Protect(0, nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := signedRequest(t, keys, "phone", http.MethodGet, "/api/daily-metrics", "", nil)
	req.RemoteAddr = "127.0.0.1:5555"
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", recorder.Code)
	}
	if code := recordedCode(t, recorder); code != CodeInvalidProxyHeader {
		t.Fatalf("code = %q", code)
	}

	withHeader := signedRequest(t, keys, "phone", http.MethodGet, "/api/daily-metrics", "", nil)
	withHeader.RemoteAddr = "127.0.0.1:5555"
	withHeader.Header.Set("CF-Connecting-IP", "198.51.100.7")
	ok := httptest.NewRecorder()
	handler.ServeHTTP(ok, withHeader)
	if ok.Code != http.StatusOK {
		t.Fatalf("valid proxy header status = %d", ok.Code)
	}
}

func TestGuardBodySlotBusy(t *testing.T) {
	keys := testKeySet()
	nonces := newNonceRecorder()
	cfg := testGuardConfig(keys, nonces)
	cfg.Slots = NewSemaphore(1)
	guard := NewGuard(cfg)

	entered := make(chan struct{})
	release := make(chan struct{})
	handler := guard.Protect(64<<10, nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		w.WriteHeader(http.StatusNoContent)
	}))
	body := []byte("{}")

	first := httptest.NewRecorder()
	go handler.ServeHTTP(first, signedRequest(t, keys, "phone", http.MethodPut, "/api/profile", "application/json", body))
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first request never reached the handler")
	}

	second := httptest.NewRecorder()
	handler.ServeHTTP(second, signedRequest(t, keys, "phone", http.MethodPut, "/api/profile", "application/json", body))
	if second.Code != http.StatusServiceUnavailable {
		t.Fatalf("second status = %d", second.Code)
	}
	if retry := second.Header().Get("Retry-After"); retry != "1" {
		t.Fatalf("Retry-After = %q", retry)
	}
	if code := recordedCode(t, second); code != CodeServerBusy {
		t.Fatalf("code = %q", code)
	}
	if nonces.count() != 1 {
		t.Fatalf("nonce calls = %d, want 1", nonces.count())
	}

	close(release)
}

func recordedCode(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode body %q: %v", recorder.Body.String(), err)
	}
	return envelope.Error.Code
}
