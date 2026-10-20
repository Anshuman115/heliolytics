package integration

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/Anshuman115/heliolytics/internal/testutil"
)

// newRequestTo builds one signed request for an absolute base URL.
func (s *stack) newRequestTo(t *testing.T, base, kid, method, path, query string,
	body []byte, contentType string) *http.Request {
	t.Helper()
	target := base + path
	if query != "" {
		target += "?" + query
	}
	request, err := http.NewRequest(method, target, httpBody(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	s.sign(t, kid, request, body, freshNonce(t), time.Now().Unix())
	return request
}

// TestStackServesSignedRequestsOverTCP runs the real mux on a real listener so
// the wire path, headers and public routes are exercised without shortcuts.
func TestStackServesSignedRequestsOverTCP(t *testing.T) {
	s := newStack(t)
	server := httptest.NewServer(s.handler)
	t.Cleanup(server.Close)
	client := server.Client()

	health, err := client.Get(server.URL + "/health")
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	body, err := readAll(health)
	if err != nil {
		t.Fatalf("read health: %v", err)
	}
	if health.StatusCode != http.StatusOK || string(body) != "ok" {
		t.Fatalf("health = %d %q", health.StatusCode, body)
	}

	version, err := client.Get(server.URL + "/version")
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	versionBody, err := readAll(version)
	if err != nil {
		t.Fatalf("read version: %v", err)
	}
	if version.StatusCode != http.StatusOK ||
		!strings.Contains(string(versionBody), `"commit":null`) {
		t.Fatalf("version = %d %s", version.StatusCode, versionBody)
	}

	minute := time.Date(2026, time.September, 24, 1, 0, 0, 0, time.UTC)
	session := sessionPart(t, "stack-2026-09-24", minute, minute.Add(10*time.Minute))
	uploadBody, contentType := multipartBody(t, session,
		catalogPart(t, catalogEntry{Code: "0x01", RoundStart: istWallClock(minute)}),
		map[string][]byte{"0x01": activityMinute(88, 30)})
	request := s.newRequestTo(t, server.URL, phoneKid, http.MethodPost, "/api/v1/ingest", "",
		uploadBody, contentType)
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	ingestBody, err := readAll(response)
	if err != nil {
		t.Fatalf("read ingest: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("ingest = %d %s", response.StatusCode, ingestBody)
	}

	request = s.newRequestTo(t, server.URL, phoneKid, http.MethodGet,
		"/api/v1/daily-metrics", "from=2026-09-24&to=2026-09-24", nil, "")
	response, err = client.Do(request)
	if err != nil {
		t.Fatalf("daily metrics: %v", err)
	}
	dailyBody, err := readAll(response)
	if err != nil {
		t.Fatalf("read daily metrics: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("daily metrics = %d %s", response.StatusCode, dailyBody)
	}
	if !strings.Contains(string(dailyBody), `"dayKey":"2026-09-24"`) {
		t.Fatalf("daily metrics body = %s", dailyBody)
	}
}

// TestStackLivenessWithoutDatabase proves /health stays a liveness check while
// a data route fails closed on an unreachable database.
func TestStackLivenessWithoutDatabase(t *testing.T) {
	s := offlineStack(t)
	server := httptest.NewServer(s.handler)
	t.Cleanup(server.Close)
	client := server.Client()

	request := s.newRequestTo(t, server.URL, phoneKid, http.MethodGet,
		"/api/v1/metrics/coverage", "", nil, "")
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("coverage: %v", err)
	}
	if _, err := readAll(response); err != nil {
		t.Fatalf("read coverage: %v", err)
	}
	if response.StatusCode == http.StatusOK {
		t.Fatal("a data route answered 200 with an unreachable database")
	}

	health, err := client.Get(server.URL + "/health")
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	body, err := readAll(health)
	if err != nil {
		t.Fatalf("read health: %v", err)
	}
	if health.StatusCode != http.StatusOK || string(body) != "ok" {
		t.Fatalf("health after a failed data route = %d %q", health.StatusCode, body)
	}
}

// lockedBuffer collects subprocess output.
type lockedBuffer struct {
	mu   sync.Mutex
	text string
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.text += string(p)
	return len(p), nil
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.text
}

// processEnv copies the environment with the supplied variables replaced.
func processEnv(overrides map[string]string) []string {
	env := make([]string, 0, len(os.Environ())+len(overrides))
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if _, replaced := overrides[name]; replaced {
			continue
		}
		env = append(env, entry)
	}
	for name, value := range overrides {
		env = append(env, name+"="+value)
	}
	return env
}

// freeLoopbackAddress reserves and releases one loopback address.
func freeLoopbackAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return address
}

// TestCompiledServerLifecycle builds the real binary, checks the injected build
// metadata, sends one signed write and requires a clean SIGTERM exit.
func TestCompiledServerLifecycle(t *testing.T) {
	root := repoRoot(t)
	const commit = "subprocess-commit"
	const builtAt = "2026-10-20T00:00:00Z"
	binary := filepath.Join(t.TempDir(), "heliolytics-api")

	build := exec.Command("go", "build", "-ldflags",
		"-X main.commit="+commit+" -X main.builtAt="+builtAt,
		"-o", binary, "./cmd/server")
	build.Dir = root
	build.Env = processEnv(map[string]string{"CGO_ENABLED": "0"})
	var buildLog lockedBuffer
	build.Stdout, build.Stderr = &buildLog, &buildLog
	if err := build.Run(); err != nil {
		t.Fatalf("go build: %v\n%s", err, buildLog.String())
	}

	keys := testKeys()
	phoneKey, err := keys.Lookup(phoneKid)
	if err != nil {
		t.Fatalf("lookup phone: %v", err)
	}
	webKey, err := keys.Lookup(webKid)
	if err != nil {
		t.Fatalf("lookup web: %v", err)
	}
	keysSpec := "phone." + phoneKey.SecretHex() + ",web." + webKey.SecretHex()

	address := freeLoopbackAddress(t)
	base := "http://" + address
	dsn := testutil.NewDB(t)

	var output lockedBuffer
	command := exec.Command(binary)
	command.Env = processEnv(map[string]string{
		"ADDR":                 address,
		"DATABASE_URL":         dsn,
		"HELIOLYTICS_API_KEYS": keysSpec,
		"TRUST_PROXY":          "false",
	})
	command.Stdout, command.Stderr = &output, &output
	if err := command.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	var waitErr error
	done := make(chan struct{})
	go func() {
		waitErr = command.Wait()
		close(done)
	}()
	t.Cleanup(func() {
		select {
		case <-done:
		default:
			_ = command.Process.Kill()
			<-done
		}
	})

	client := &http.Client{Timeout: 5 * time.Second}
	deadline := time.Now().Add(60 * time.Second)
	for {
		response, err := client.Get(base + "/health")
		if err == nil {
			body, readErr := readAll(response)
			if readErr == nil && response.StatusCode == http.StatusOK && string(body) == "ok" {
				break
			}
		}
		select {
		case <-done:
			t.Fatalf("server exited before health: %v\n%s", waitErr, output.String())
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("health never became ready\n%s", output.String())
		}
		time.Sleep(25 * time.Millisecond)
	}

	version, err := client.Get(base + "/version")
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	versionBody, err := readAll(version)
	if err != nil {
		t.Fatalf("read version: %v", err)
	}
	var reported struct {
		Commit  *string `json:"commit"`
		BuiltAt *string `json:"builtAt"`
	}
	if err := json.Unmarshal(versionBody, &reported); err != nil {
		t.Fatalf("decode version %s: %v", versionBody, err)
	}
	if reported.Commit == nil || *reported.Commit != commit ||
		reported.BuiltAt == nil || *reported.BuiltAt != builtAt {
		t.Fatalf("version = %s", versionBody)
	}

	signer := &stack{keys: keys}
	minute := time.Date(2026, time.September, 27, 1, 0, 0, 0, time.UTC)
	session := sessionPart(t, "subprocess-2026-09-27", minute, minute.Add(10*time.Minute))
	uploadBody, contentType := multipartBody(t, session,
		catalogPart(t, catalogEntry{Code: "0x01", RoundStart: istWallClock(minute)}),
		map[string][]byte{"0x01": activityMinute(75, 40)})
	request := signer.newRequestTo(t, base, phoneKid, http.MethodPost, "/api/v1/ingest", "",
		uploadBody, contentType)

	writeClient := &http.Client{Timeout: 30 * time.Second}
	response, err := writeClient.Do(request)
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	ingestBody, err := readAll(response)
	if err != nil {
		t.Fatalf("read ingest: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("ingest = %d %s", response.StatusCode, ingestBody)
	}

	request = signer.newRequestTo(t, base, phoneKid, http.MethodGet,
		"/api/v1/daily-metrics", "from=2026-09-27&to=2026-09-27", nil, "")
	response, err = writeClient.Do(request)
	if err != nil {
		t.Fatalf("daily metrics: %v", err)
	}
	dailyBody, err := readAll(response)
	if err != nil {
		t.Fatalf("read daily metrics: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("daily metrics = %d %s", response.StatusCode, dailyBody)
	}
	if !strings.Contains(string(dailyBody), `"dayKey":"2026-09-27"`) {
		t.Fatalf("daily metrics body = %s", dailyBody)
	}

	if err := command.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("signal: %v", err)
	}
	select {
	case <-done:
		if waitErr != nil {
			t.Fatalf("server exit: %v\n%s", waitErr, output.String())
		}
	case <-time.After(30 * time.Second):
		t.Fatal("server did not exit after SIGTERM")
	}
}
