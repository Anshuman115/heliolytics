package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Anshuman115/heliolytics/internal/auth"
	"github.com/Anshuman115/heliolytics/internal/config"
	"github.com/Anshuman115/heliolytics/internal/store"
)

func testKeys() auth.KeySet {
	var phone, web [32]byte
	for i := range phone {
		phone[i] = byte(0x60 + i)
		web[i] = byte(0xa0 + i)
	}
	return auth.NewKeySet(phone, web)
}

func offlineStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.New(context.Background(),
		"postgres://nobody:nobody@127.0.0.1:1/heliolytics_test?sslmode=disable")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(st.Close)
	return st
}

type syncBuffer struct {
	mu   sync.Mutex
	text string
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.text += string(p)
	return len(p), nil
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.text
}

func newLogger(w io.Writer) *log.Logger {
	return log.New(w, "", log.LstdFlags)
}

func awaitText(t *testing.T, buffer *syncBuffer, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(buffer.String(), want) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("log never contained %q: %q", want, buffer.String())
}

func serveHealth(t *testing.T, status int, body string) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	return "http://" + listener.Addr().String() + "/health"
}

func TestCheckHealthAcceptsDocumentedBody(t *testing.T) {
	for _, body := range []string{"ok", "ok\n"} {
		target := serveHealth(t, http.StatusOK, body)
		if err := checkHealth(context.Background(), target); err != nil {
			t.Fatalf("body %q: %v", body, err)
		}
	}
}

func TestCheckHealthRejectsFailures(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"server error", http.StatusInternalServerError, "ok"},
		{"empty body", http.StatusOK, ""},
		{"wrong body", http.StatusOK, "ready"},
		{"second newline", http.StatusOK, "ok\n\n"},
		{"trailing space", http.StatusOK, "ok "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			target := serveHealth(t, tc.status, tc.body)
			if err := checkHealth(context.Background(), target); err == nil {
				t.Fatalf("status %d body %q was accepted", tc.status, tc.body)
			}
		})
	}

	t.Run("refused", func(t *testing.T) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		target := "http://" + listener.Addr().String() + "/health"
		if err := listener.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
		if err := checkHealth(context.Background(), target); err == nil {
			t.Fatal("a closed listener was accepted")
		}
	})

	t.Run("deadline", func(t *testing.T) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		blocked := make(chan struct{})
		server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			<-blocked
		})}
		go func() { _ = server.Serve(listener) }()
		t.Cleanup(func() {
			close(blocked)
			_ = server.Close()
		})

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		target := "http://" + listener.Addr().String() + "/health"
		if err := checkHealth(ctx, target); err == nil {
			t.Fatal("a stalled handler was accepted")
		}
	})
}

func TestApplyBuildInfoReachesVersion(t *testing.T) {
	previousCommit, previousBuiltAt := commit, builtAt
	commit, builtAt = "0123456789abcdef", "2026-10-20T00:00:00Z"
	t.Cleanup(func() { commit, builtAt = previousCommit, previousBuiltAt })

	cfg := config.Config{Keys: testKeys()}
	applyBuildInfo(&cfg)
	if cfg.Commit != commit || cfg.BuiltAt != builtAt {
		t.Fatalf("build info not applied: %+v", cfg)
	}

	handler := newHandler(offlineStore(t), cfg)
	response := getJSON(t, handler, "/version")
	if response["commit"] != commit || response["builtAt"] != builtAt {
		t.Fatalf("version response = %v", response)
	}
}

func TestApplyBuildInfoUninjectedReportsNull(t *testing.T) {
	previousCommit, previousBuiltAt := commit, builtAt
	commit, builtAt = "", ""
	t.Cleanup(func() { commit, builtAt = previousCommit, previousBuiltAt })

	cfg := config.Config{Keys: testKeys()}
	applyBuildInfo(&cfg)

	response := getJSON(t, newHandler(offlineStore(t), cfg), "/version")
	if value, ok := response["commit"]; !ok || value != nil {
		t.Fatalf("commit = %v, want null", value)
	}
	if value, ok := response["builtAt"]; !ok || value != nil {
		t.Fatalf("builtAt = %v, want null", value)
	}
}

func getJSON(t *testing.T, handler http.Handler, path string) map[string]any {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := &http.Server{Handler: handler}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })

	url := "http://" + listener.Addr().String() + path
	response, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status %d", path, response.StatusCode)
	}
	var decoded map[string]any
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return decoded
}

func TestNewHandlerRejectsNilStore(t *testing.T) {
	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("newHandler accepted a nil store")
		}
		if text := fmt.Sprint(recovered); text != "api.NewMux: nil dependency" {
			t.Fatalf("panic = %q", text)
		}
	}()
	_ = newHandler(nil, config.Config{Keys: testKeys()})
}

func TestRunRejectsInvalidConfig(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	var stderr syncBuffer
	if code := run(nil, &stderr); code == 0 {
		t.Fatal("run accepted an empty DATABASE_URL")
	}
	if !strings.Contains(stderr.String(), "DATABASE_URL") {
		t.Fatalf("error did not name the variable: %q", stderr.String())
	}
}

func TestRunRejectsPositionalArguments(t *testing.T) {
	var stderr syncBuffer
	if code := run([]string{"unexpected"}, &stderr); code == 0 {
		t.Fatal("run accepted a positional argument")
	}
}

func TestRunHealthcheckUsesProbeWithoutConfig(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("HELIOLYTICS_WEB_API_KEY", "")
	t.Setenv("HELIOLYTICS_PHONE_API_KEY", "")

	target := serveHealth(t, http.StatusOK, "ok")
	previous := probeTarget
	probeTarget = target
	t.Cleanup(func() { probeTarget = previous })

	var stderr syncBuffer
	if code := run([]string{"-healthcheck"}, &stderr); code != 0 {
		t.Fatalf("healthcheck exit = %d, stderr = %q", code, stderr.String())
	}
}

func TestRunHealthcheckFailsWithoutAPI(t *testing.T) {
	previous := probeTarget
	probeTarget = serveHealth(t, http.StatusServiceUnavailable, "ok")
	t.Cleanup(func() { probeTarget = previous })

	var stderr syncBuffer
	if code := run([]string{"-healthcheck"}, &stderr); code == 0 {
		t.Fatal("healthcheck accepted a non-200 response")
	}
}

func TestServeStartsAndStopsBoundedly(t *testing.T) {
	t.Setenv("ADDR", "127.0.0.1:0")
	t.Setenv("DATABASE_URL", "postgres://nobody:nobody@127.0.0.1:1/heliolytics_test?sslmode=disable")
	t.Setenv("HELIOLYTICS_WEB_API_KEY", "web."+strings.Repeat("22", 32))
	t.Setenv("HELIOLYTICS_PHONE_API_KEY", "phone."+strings.Repeat("11", 32))

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var stderr syncBuffer
	exited := make(chan int, 1)
	go func() { exited <- serve(ctx, cfg, newLogger(&stderr)) }()

	awaitText(t, &stderr, "api listening on")
	cancel()

	select {
	case code := <-exited:
		if code != 0 {
			t.Fatalf("serve exit = %d, log = %q", code, stderr.String())
		}
	case <-time.After(15 * time.Second):
		t.Fatal("serve did not stop after cancellation")
	}
}
