//go:build heliolytics_smoke

package integration

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
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Anshuman115/heliolytics/internal/auth"
	"github.com/Anshuman115/heliolytics/internal/store"
	"github.com/Anshuman115/heliolytics/internal/testutil/disposable"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type smokeConfig struct {
	apiContainer string
	dbContainer  string
	apiURL       string
	databaseURL  string
	image        string
	secret       []byte
}

func smokeEnv(t *testing.T) smokeConfig {
	t.Helper()
	if os.Getenv("HELIOLYTICS_SMOKE_TEST_ONLY") != "true" {
		t.Fatal("HELIOLYTICS_SMOKE_TEST_ONLY=true is required")
	}
	required := func(name string) string {
		value := strings.TrimSpace(os.Getenv(name))
		if value == "" {
			t.Fatalf("%s is required for the smoke seed", name)
		}
		return value
	}

	key := required("HELIOLYTICS_SMOKE_API_KEY")
	secretHex, ok := strings.CutPrefix(key, "phone.")
	if !ok {
		t.Fatalf("HELIOLYTICS_SMOKE_API_KEY must have the form phone.<64 lowercase hex>")
	}
	secret, err := hex.DecodeString(secretHex)
	if err != nil || len(secret) != 32 {
		t.Fatalf("HELIOLYTICS_SMOKE_API_KEY is not 32 hex-encoded bytes")
	}

	return smokeConfig{
		apiContainer: required("HELIOLYTICS_SMOKE_API_CONTAINER_ID"),
		dbContainer:  required("HELIOLYTICS_SMOKE_DB_CONTAINER_ID"),
		apiURL:       required("HELIOLYTICS_SMOKE_API_URL"),
		databaseURL:  required("HELIOLYTICS_SMOKE_DATABASE_URL"),
		image:        required("HELIOLYTICS_TEST_DB_IMAGE"),
		secret:       secret,
	}
}

type smokeContainer struct {
	ID     string `json:"Id"`
	Name   string `json:"Name"`
	Config struct {
		Image  string            `json:"Image"`
		Labels map[string]string `json:"Labels"`
		Env    []string          `json:"Env"`
	} `json:"Config"`
	Mounts []struct {
		Name        string `json:"Name"`
		Destination string `json:"Destination"`
	} `json:"Mounts"`
	NetworkSettings struct {
		Ports map[string][]struct {
			HostIP   string `json:"HostIp"`
			HostPort string `json:"HostPort"`
		} `json:"Ports"`
		Networks map[string]json.RawMessage `json:"Networks"`
	} `json:"NetworkSettings"`
}

func inspectContainer(t *testing.T, containerID string) smokeContainer {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "docker", "inspect", containerID).Output()
	if err != nil {
		t.Fatalf("docker inspect %s: %v", containerID, err)
	}
	var containers []smokeContainer
	if err := json.Unmarshal(output, &containers); err != nil {
		t.Fatalf("decode docker inspect: %v", err)
	}
	if len(containers) != 1 {
		t.Fatalf("docker inspect returned %d containers", len(containers))
	}
	if containers[0].ID != containerID {
		t.Fatalf("docker inspect reported %s, want %s", containers[0].ID, containerID)
	}
	return containers[0]
}

func requireTestLabels(t *testing.T, container smokeContainer, role string) {
	t.Helper()
	if container.Config.Labels["heliolytics.test"] != "true" {
		t.Fatalf("%s container is missing the heliolytics.test=true label", role)
	}
	project := strings.TrimSpace(container.Config.Labels["heliolytics.test.project"])
	if project == "" {
		t.Fatalf("%s container is missing the heliolytics.test.project label", role)
	}
	if project == "deploy" {
		t.Fatalf("%s container belongs to the deploy project", role)
	}
}

func requireAPIScope(t *testing.T, config smokeConfig, api, database smokeContainer) {
	t.Helper()
	target, err := url.Parse(config.apiURL)
	if err != nil {
		t.Fatalf("parse %s: %v", config.apiURL, err)
	}
	if target.Scheme != "http" {
		t.Fatalf("API URL scheme = %q, want http", target.Scheme)
	}
	host := target.Hostname()
	if host != "127.0.0.1" && host != "::1" {
		t.Fatalf("API URL host %q is not loopback", host)
	}
	port := target.Port()
	if port == "" {
		t.Fatal("API URL must include its loopback port")
	}

	published := false
	for _, binding := range api.NetworkSettings.Ports["8080/tcp"] {
		if binding.HostPort == port && (binding.HostIP == "127.0.0.1" || binding.HostIP == "::1") {
			published = true
		}
	}
	if !published {
		t.Fatalf("API container publishes no loopback mapping for %s:8080", port)
	}

	shared := false
	for network := range api.NetworkSettings.Networks {
		if _, ok := database.NetworkSettings.Networks[network]; ok {
			shared = true
		}
	}
	if !shared {
		t.Fatal("the API container shares no network with the smoke database")
	}

	var databaseURL string
	for _, entry := range api.Config.Env {
		if value, ok := strings.CutPrefix(entry, "DATABASE_URL="); ok {
			databaseURL = value
		}
	}
	if databaseURL == "" {
		t.Fatal("the API container has no DATABASE_URL")
	}
	parsed, err := pgconn.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse the API container DATABASE_URL: %v", err)
	}
	if parsed.Database != "heliolytics_smoke" {
		t.Fatalf("API container database = %q, want heliolytics_smoke", parsed.Database)
	}
	name := strings.TrimPrefix(database.Name, "/")
	if parsed.Host != name && parsed.Host != database.ID {
		t.Fatalf("API container database host = %q, want the smoke database container", parsed.Host)
	}
}

type smokeClient struct {
	base   string
	secret []byte
}

func (c *smokeClient) do(t *testing.T, method, path, query string,
	body []byte, contentType string) (int, []byte) {
	t.Helper()
	target := c.base + path
	if query != "" {
		target += "?" + query
	}
	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequest(method, target, reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}

	sum := sha256.Sum256(body)
	hash := hex.EncodeToString(sum[:])
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		t.Fatalf("rand: %v", err)
	}
	ts := time.Now().Unix()
	canonical := auth.CanonicalText("phone", ts, hex.EncodeToString(nonce), request.Method,
		request.URL.EscapedPath(), request.URL.RawQuery, hash, request.Header.Get("Content-Type"))
	mac := hmac.New(sha256.New, c.secret)
	_, _ = mac.Write([]byte(canonical))
	request.Header.Set(auth.HeaderAuth,
		fmt.Sprintf("phone.%d.%s.%s", ts, hex.EncodeToString(nonce),
			hex.EncodeToString(mac.Sum(nil))))
	request.Header.Set(auth.HeaderBodySHA256, hash)

	client := &http.Client{Timeout: 60 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return response.StatusCode, payload
}

func smokeCount(t *testing.T, dsn, table string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	connection, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to the smoke database: %v", err)
	}
	defer connection.Close(ctx)
	var total int
	if err := connection.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&total); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return total
}

// TestSeedLocalSmoke seeds a verified disposable stack with the committed June
// corpus plus one labelable synthetic day. It refuses any unproved scope before
// its first write and never truncates.
func TestSeedLocalSmoke(t *testing.T) {
	config := smokeEnv(t)

	api := inspectContainer(t, config.apiContainer)
	requireTestLabels(t, api, "api")
	database := inspectContainer(t, config.dbContainer)
	requireTestLabels(t, database, "database")
	requireAPIScope(t, config, api, database)

	if _, err := disposable.Inspect(t.Context(), config.dbContainer,
		config.databaseURL, config.image); err != nil {
		t.Fatalf("smoke database rejected: %v", err)
	}
	parsed, err := pgconn.ParseConfig(config.databaseURL)
	if err != nil {
		t.Fatalf("parse the smoke database URL: %v", err)
	}
	if parsed.Database != "heliolytics_smoke" {
		t.Fatalf("smoke database = %q, want heliolytics_smoke", parsed.Database)
	}

	for _, table := range appTables {
		if count := smokeCount(t, config.databaseURL, table); count != 0 {
			t.Fatalf("%s has %d rows; the smoke database must be freshly initialized", table, count)
		}
	}

	client := &smokeClient{base: config.apiURL, secret: config.secret}

	profile := []byte(`{"name":null,"birthDate":"1991-02-03","sex":"male","heightCm":176.0,"weightKg":70.0}`)
	status, payload := client.do(t, http.MethodPut, "/api/profile", "", profile, "application/json")
	if status != http.StatusNoContent {
		t.Fatalf("profile status %d: %s", status, payload)
	}
	if count := smokeCount(t, config.databaseURL, "daily_metrics"); count != 0 {
		t.Fatalf("daily_metrics = %d after a profile-only write", count)
	}

	uploadBody, contentType := multipartBody(t, realSession(t), realCatalog(t), realBlobs(t))
	status, payload = client.do(t, http.MethodPost, "/api/ingest", "", uploadBody, contentType)
	if status != http.StatusOK {
		t.Fatalf("June ingest status %d: %s", status, payload)
	}
	var result struct {
		SessionID string `json:"sessionId"`
		Types     map[string]struct {
			Status   string   `json:"status"`
			Rows     int      `json:"rows"`
			Warnings []string `json:"warnings"`
		} `json:"types"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("decode ingest response: %v", err)
	}
	if result.SessionID != "fixture-dumpp-2026-06-22" {
		t.Fatalf("session id = %q", result.SessionID)
	}
	if result.Types["0x26"].Status != "raw_only" || result.Types["0x26"].Rows != 0 {
		t.Fatalf("0x26 outcome = %+v", result.Types["0x26"])
	}
	if result.Types["0x01"].Rows != 4218 {
		t.Fatalf("0x01 rows = %d, want 4218", result.Types["0x01"].Rows)
	}

	status, payload = client.do(t, http.MethodGet, "/api/daily-metrics",
		"from=2026-06-21&to=2026-06-22", nil, "")
	if status != http.StatusOK {
		t.Fatalf("daily metrics status %d: %s", status, payload)
	}
	var days struct {
		Days []struct {
			DayKey *string `json:"dayKey"`
			Steps  *int    `json:"steps"`
		} `json:"days"`
	}
	if err := json.Unmarshal(payload, &days); err != nil {
		t.Fatalf("decode daily metrics: %v", err)
	}
	stepsByDay := make(map[string]int, len(days.Days))
	for _, day := range days.Days {
		if day.DayKey == nil || day.Steps == nil {
			continue
		}
		stepsByDay[*day.DayKey] = *day.Steps
	}
	if stepsByDay["2026-06-21"] != 856 || stepsByDay["2026-06-22"] != 3708 {
		t.Fatalf("June step totals = %v", stepsByDay)
	}

	now := time.Now().In(istZone)
	syntheticMinute := time.Date(now.Year(), now.Month(), now.Day(), 6, 30, 0, 0, istZone).UTC()
	syntheticDay := syntheticMinute.In(istZone).Format("2006-01-02")
	session := sessionPart(t, "synthetic-"+syntheticDay,
		syntheticMinute.Add(-time.Minute), syntheticMinute.Add(30*time.Minute))
	activity := append(activityMinute(70, 200), activityMinute(70, 0)...)
	uploadBody, contentType = multipartBody(t, session,
		catalogPart(t, catalogEntry{Code: "0x01", RoundStart: istWallClock(syntheticMinute)}),
		map[string][]byte{"0x01": activity})
	status, payload = client.do(t, http.MethodPost, "/api/ingest", "", uploadBody, contentType)
	if status != http.StatusOK {
		t.Fatalf("synthetic ingest status %d: %s", status, payload)
	}

	status, payload = client.do(t, http.MethodGet, "/api/daily-metrics",
		"from="+syntheticDay+"&to="+syntheticDay, nil, "")
	if status != http.StatusOK {
		t.Fatalf("synthetic daily status %d: %s", status, payload)
	}
	days.Days = nil
	if err := json.Unmarshal(payload, &days); err != nil {
		t.Fatalf("decode synthetic daily metrics: %v", err)
	}
	if len(days.Days) != 1 || days.Days[0].Steps == nil || *days.Days[0].Steps != 200 {
		t.Fatalf("synthetic day = %s", payload)
	}

	status, payload = client.do(t, http.MethodGet, "/api/metrics/coverage", "", nil, "")
	if status != http.StatusOK {
		t.Fatalf("coverage status %d: %s", status, payload)
	}
	var coverage struct {
		HasData bool               `json:"hasData"`
		Types   map[string]*string `json:"types"`
	}
	if err := json.Unmarshal(payload, &coverage); err != nil {
		t.Fatalf("decode coverage: %v", err)
	}
	if !coverage.HasData {
		t.Fatal("coverage reports no data after the seed")
	}
	if _, present := coverage.Types["0x26"]; present {
		t.Fatal("coverage includes 0x26; raw_only types are never coverage keys")
	}
	if len(coverage.Types) != len(store.MeasuredTypeCodes) {
		t.Fatalf("coverage keys = %d, want %d", len(coverage.Types), len(store.MeasuredTypeCodes))
	}
	for _, code := range store.MeasuredTypeCodes {
		if _, present := coverage.Types[code]; !present {
			t.Fatalf("coverage missing measured code %s", code)
		}
	}

	manifest := map[string]any{
		"seedIstDay": now.Format("2006-01-02"),
		"fetchWindow": []string{
			now.AddDate(0, 0, -29).Format("2006-01-02"), now.Format("2006-01-02"),
		},
		"syntheticDays":   []string{syntheticDay},
		"juneGoldenDates": []string{"2026-06-21", "2026-06-22"},
		"expected": map[string]any{
			"juneSteps":       map[string]int{"2026-06-21": 856, "2026-06-22": 3708},
			"syntheticSteps":  200,
			"spo2SleepStatus": "raw_only",
			"coverageCodes":   len(store.MeasuredTypeCodes),
		},
		"provenance": map[string]string{
			"2026-06-21": "real sensor bytes from the committed June corpus",
			syntheticDay: "synthetic observation, not capture or clinical evidence",
		},
	}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("encode manifest: %v", err)
	}
	t.Logf("smoke manifest:\n%s", encoded)
}
