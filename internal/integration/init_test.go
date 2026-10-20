package integration

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/Anshuman115/heliolytics/internal/store"
)

// TestFreshSchemaInitHasZeroApplicationRows proves a fresh schema has no
// health or profile rows, web reads touch only nonces, and the first phone
// upload creates exactly its own lineage.
func TestFreshSchemaInitHasZeroApplicationRows(t *testing.T) {
	s := newStack(t)

	for _, table := range appTables {
		if count := s.count(t, table); count != 0 {
			t.Fatalf("%s has %d rows after schema init", table, count)
		}
	}

	// A signed web read may create its nonce and nothing else.
	recorder := s.get(t, webKid, "/api/metrics/coverage", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("coverage status %d: %s", recorder.Code, recorder.Body.String())
	}
	var coverage struct {
		HasData      bool               `json:"hasData"`
		LastIngestAt *string            `json:"lastIngestAt"`
		DataThrough  *string            `json:"dataThrough"`
		Types        map[string]*string `json:"types"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &coverage); err != nil {
		t.Fatalf("decode coverage: %v", err)
	}
	if coverage.HasData || coverage.LastIngestAt != nil || coverage.DataThrough != nil {
		t.Fatalf("empty coverage reported data: %s", recorder.Body.String())
	}
	if len(coverage.Types) != len(store.MeasuredTypeCodes) {
		t.Fatalf("coverage keys = %d, want %d", len(coverage.Types), len(store.MeasuredTypeCodes))
	}
	for code, value := range coverage.Types {
		if value != nil {
			t.Fatalf("coverage %s = %v, want null", code, *value)
		}
	}

	if nonces := s.count(t, "auth_nonces"); nonces != 1 {
		t.Fatalf("auth_nonces = %d after one signed read, want 1", nonces)
	}
	for _, table := range appTables {
		if table == "auth_nonces" {
			continue
		}
		if count := s.count(t, table); count != 0 {
			t.Fatalf("%s has %d rows after a web read", table, count)
		}
	}

	// A web write is refused before the body is read and consumes no nonce.
	body := &failReader{}
	request, err := http.NewRequest(http.MethodPut, "http://api.test/api/profile", body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	s.sign(t, webKid, request, nil, freshNonce(t), time.Now().Unix())
	recorder = s.serve(request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("web profile status %d, want 403", recorder.Code)
	}
	if code := errorCode(t, recorder); code != "forbidden" {
		t.Fatalf("web profile code = %q", code)
	}
	if body.read {
		t.Fatal("the refused web write read the request body")
	}

	session := sessionPart(t, "web-write-2026-09-21",
		time.Date(2026, time.September, 21, 1, 0, 0, 0, time.UTC),
		time.Date(2026, time.September, 21, 1, 10, 0, 0, time.UTC))
	recorder = s.upload(t, webKid, session, catalogPart(t), map[string][]byte{
		"0x01": activityMinute(70, 12),
	})
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("web ingest status %d, want 403", recorder.Code)
	}
	if nonces := s.count(t, "auth_nonces"); nonces != 1 {
		t.Fatalf("auth_nonces = %d after refused web writes, want 1", nonces)
	}
	if sessions := s.count(t, "sync_sessions"); sessions != 0 {
		t.Fatalf("sync_sessions = %d after refused web writes", sessions)
	}

	// A phone profile-only write stores the profile and no derived day rows.
	profile := []byte(`{"name":null,"birthDate":"1990-05-01","sex":"male","heightCm":175.0,"weightKg":70.0}`)
	recorder = s.serve(s.newRequest(t, phoneKid, http.MethodPut, "/api/profile", "",
		profile, "application/json"))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("profile status %d: %s", recorder.Code, recorder.Body.String())
	}
	if recorder.Body.Len() != 0 {
		t.Fatalf("profile body = %q, want empty", recorder.Body.String())
	}
	if profiles := s.count(t, "profile"); profiles != 1 {
		t.Fatalf("profile = %d rows, want 1", profiles)
	}
	if days := s.count(t, "daily_metrics"); days != 0 {
		t.Fatalf("daily_metrics = %d after a profile-only write", days)
	}

	// The first accepted phone upload creates exactly its own lineage.
	minute := time.Date(2026, time.September, 21, 1, 5, 0, 0, time.UTC)
	firstSession := sessionPart(t, "first-phone-upload-2026-09-21",
		minute, minute.Add(10*time.Minute))
	activity := activityMinute(70, 12)
	recorder = s.upload(t, phoneKid, firstSession,
		catalogPart(t, catalogEntry{Code: "0x01", RoundStart: istWallClock(minute)}),
		map[string][]byte{"0x01": activity})
	if recorder.Code != http.StatusOK {
		t.Fatalf("first upload status %d: %s", recorder.Code, recorder.Body.String())
	}
	result := decodeResult(t, recorder)
	if result.SessionID != "first-phone-upload-2026-09-21" {
		t.Fatalf("session id = %q", result.SessionID)
	}
	if rows := result.Types["0x01"].Rows; rows != 2 {
		t.Fatalf("0x01 rows = %d, want 2", rows)
	}

	if sessions := s.count(t, "sync_sessions"); sessions != 1 {
		t.Fatalf("sync_sessions = %d, want 1", sessions)
	}
	if blobs := s.count(t, "raw_blobs"); blobs != 1 {
		t.Fatalf("raw_blobs = %d, want 1", blobs)
	}
	stored := s.queryText(t, "SELECT encode(payload, 'hex') FROM raw_blobs WHERE type_code = '0x01'")
	if stored != "00000c4600000000" {
		t.Fatalf("stored 0x01 payload = %s", stored)
	}
	if device := s.queryText(t, "SELECT device_mac FROM sync_sessions"); device != syntheticMAC {
		t.Fatalf("stored device MAC = %s", device)
	}
	if steps := s.count(t, "step_samples"); steps != 1 {
		t.Fatalf("step_samples = %d, want 1", steps)
	}
	if hr := s.count(t, "heart_rate_samples"); hr != 1 {
		t.Fatalf("heart_rate_samples = %d, want 1", hr)
	}
	if codes := s.count(t, "coverage"); codes != 1 {
		t.Fatalf("coverage = %d rows, want 1", codes)
	}

	day := s.daily(t, "2026-09-21")
	if day.Steps == nil || *day.Steps != 12 {
		t.Fatalf("steps = %s, want 12", optionalIntText(day.Steps))
	}
	if day.AvgHr == nil || *day.AvgHr != 70 {
		t.Fatalf("avgHr = %s, want 70", optionalFloatText(day.AvgHr))
	}
	if got := s.dailyKeys(t); len(got) != 1 || got[0] != "2026-09-21" {
		t.Fatalf("daily keys = %v", got)
	}
}

// TestProfileWriteIsStrictAndReplaces checks the profile envelope rules and a
// committed clear without inventing values.
func TestProfileWriteIsStrictAndReplaces(t *testing.T) {
	s := newStack(t)

	missing := []byte(`{"name":null,"birthDate":null,"sex":null,"heightCm":null}`)
	recorder := s.serve(s.newRequest(t, phoneKid, http.MethodPut, "/api/profile", "",
		missing, "application/json"))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("partial profile status %d, want 400", recorder.Code)
	}

	invalid := []byte(`{"name":null,"birthDate":null,"sex":"other","heightCm":null,"weightKg":null}`)
	recorder = s.serve(s.newRequest(t, phoneKid, http.MethodPut, "/api/profile", "",
		invalid, "application/json"))
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid profile status %d, want 422", recorder.Code)
	}
	if profiles := s.count(t, "profile"); profiles != 0 {
		t.Fatalf("profile = %d after refused writes", profiles)
	}

	complete := []byte(`{"name":"Owner","birthDate":"1990-05-01","sex":"female","heightCm":160.5,"weightKg":55.0}`)
	recorder = s.serve(s.newRequest(t, phoneKid, http.MethodPut, "/api/profile", "",
		complete, "application/json"))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("valid profile status %d: %s", recorder.Code, recorder.Body.String())
	}

	cleared := []byte(`{"name":null,"birthDate":null,"sex":null,"heightCm":null,"weightKg":null}`)
	recorder = s.serve(s.newRequest(t, phoneKid, http.MethodPut, "/api/profile", "",
		cleared, "application/json"))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("cleared profile status %d: %s", recorder.Code, recorder.Body.String())
	}

	profile, err := s.db.GetProfile(t.Context())
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if profile.Name != nil || profile.BirthDate != nil || profile.Sex != nil ||
		profile.HeightCm != nil || profile.WeightKg != nil {
		t.Fatalf("cleared profile kept values: %+v", profile)
	}
	if profiles := s.count(t, "profile"); profiles != 1 {
		t.Fatalf("profile = %d rows, want 1", profiles)
	}
}
