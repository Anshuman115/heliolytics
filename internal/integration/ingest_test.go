package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Anshuman115/heliolytics/internal/auth"
	"github.com/jackc/pgx/v5"
)

// realWindow reads the committed session window.
func realWindow(t *testing.T) (time.Time, time.Time) {
	t.Helper()
	var envelope struct {
		StartedAt string `json:"startedAt"`
		EndedAt   string `json:"endedAt"`
	}
	if err := json.Unmarshal(realSession(t), &envelope); err != nil {
		t.Fatalf("decode session.json: %v", err)
	}
	started, err := time.Parse(time.RFC3339, envelope.StartedAt)
	if err != nil {
		t.Fatalf("startedAt %q: %v", envelope.StartedAt, err)
	}
	ended, err := time.Parse(time.RFC3339, envelope.EndedAt)
	if err != nil {
		t.Fatalf("endedAt %q: %v", envelope.EndedAt, err)
	}
	return started, ended
}

// catalogWithoutSessionID drops the optional sessionId the API cross-checks so
// the same bytes can be replayed under another session.
func catalogWithoutSessionID(t *testing.T, catalog []byte) []byte {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(catalog, &payload); err != nil {
		t.Fatalf("decode catalog: %v", err)
	}
	delete(payload, "sessionId")
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode catalog: %v", err)
	}
	return encoded
}

// queryStrings reads one text column from every returned row.
func (s *stack) queryStrings(t *testing.T, statement string, args ...any) []string {
	t.Helper()
	var out []string
	ctx := t.Context()
	if err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, statement, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var value string
			if err := rows.Scan(&value); err != nil {
				return err
			}
			out = append(out, value)
		}
		return rows.Err()
	}); err != nil {
		t.Fatalf("query %q: %v", statement, err)
	}
	return out
}

// rawHex returns this session's stored payloads keyed by type code.
func (s *stack) rawHex(t *testing.T, sessionID string) map[string]string {
	t.Helper()
	out := make(map[string]string)
	ctx := t.Context()
	if err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			"SELECT type_code, encode(payload, 'hex') FROM raw_blobs WHERE session_id = $1",
			sessionID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var code, payload string
			if err := rows.Scan(&code, &payload); err != nil {
				return err
			}
			out[code] = payload
		}
		return rows.Err()
	}); err != nil {
		t.Fatalf("read raw blobs: %v", err)
	}
	return out
}

// TestRealFixtureUploadPersistsApprovedBytesAndMetrics sends the committed June
// corpus through signed multipart HTTP and compares stored state with the
// approved goldens.
func TestRealFixtureUploadPersistsApprovedBytesAndMetrics(t *testing.T) {
	s := newStack(t)
	blobs := realBlobs(t)

	result := s.uploadOK(t, realSession(t), realCatalog(t), blobs)
	if result.SessionID != "fixture-dumpp-2026-06-22" {
		t.Fatalf("session id = %q", result.SessionID)
	}
	if !result.Complete {
		t.Fatalf("complete = false, types = %+v", result.Types)
	}

	activity := result.Types["0x01"]
	if activity.Status != "accepted" || activity.Rows != 4218 || activity.Dropped != 6 {
		t.Fatalf("0x01 = %+v", activity)
	}
	if activity.DataThrough == nil ||
		activity.DataThrough.UTC().Format(time.RFC3339) != "2026-06-22T16:58:00Z" {
		t.Fatalf("0x01 dataThrough = %v", activity.DataThrough)
	}

	spo2Sleep := result.Types["0x26"]
	if spo2Sleep.Status != "raw_only" || spo2Sleep.Rows != 0 {
		t.Fatalf("0x26 = %+v", spo2Sleep)
	}
	if len(spo2Sleep.Warnings) != 1 || spo2Sleep.DataThrough != nil {
		t.Fatalf("0x26 warnings = %v dataThrough = %v", spo2Sleep.Warnings, spo2Sleep.DataThrough)
	}
	if rows := result.Types["0x05"].Rows; rows != 1 {
		t.Fatalf("0x05 rows = %d, want 1", rows)
	}
	if rows := result.Types["0x48"].Rows; rows != 2 {
		t.Fatalf("0x48 rows = %d, want 2", rows)
	}
	if rows := result.Types["0x06"].Rows; rows != 0 {
		t.Fatalf("0x06 rows = %d, want 0", rows)
	}
	if _, ok := result.Types["0x39"]; ok {
		t.Fatal("0x39 must not be reported")
	}

	if sessions := s.count(t, "sync_sessions"); sessions != 1 {
		t.Fatalf("sync_sessions = %d, want 1", sessions)
	}
	if stored := s.count(t, "raw_blobs"); stored != len(fixtureCodes) {
		t.Fatalf("raw_blobs = %d, want %d", stored, len(fixtureCodes))
	}

	hexPayloads := s.rawHex(t, "fixture-dumpp-2026-06-22")
	for _, code := range fixtureCodes {
		if hexPayloads[code] != fmt.Sprintf("%x", blobs[code]) {
			t.Fatalf("%s payload was not stored byte for byte", code)
		}
	}

	if spo2 := s.count(t, "spo2_samples"); spo2 != 0 {
		t.Fatalf("spo2_samples = %d, want 0 for a raw-only 0x26", spo2)
	}
	if steps := s.count(t, "step_samples"); steps != 2109 {
		t.Fatalf("step_samples = %d, want 2109", steps)
	}
	// 0x01 minutes and 0x46 seconds share the table under distinct sources.
	if hr := s.countWhere(t, "heart_rate_samples", "source = '0x01'"); hr != 2109 {
		t.Fatalf("heart_rate_samples source 0x01 = %d, want 2109", hr)
	}
	if hr := s.countWhere(t, "heart_rate_samples", "source = '0x46'"); hr != 180 {
		t.Fatalf("heart_rate_samples source 0x46 = %d, want 180", hr)
	}
	if sleep := s.count(t, "sleep_sessions"); sleep != 2 {
		t.Fatalf("sleep_sessions = %d, want 2", sleep)
	}
	if workouts := s.count(t, "activities"); workouts != 1 {
		t.Fatalf("activities = %d, want 1", workouts)
	}

	first := s.daily(t, "2026-06-21")
	if first.Steps == nil || *first.Steps != 856 {
		t.Fatalf("2026-06-21 steps = %s, want 856", optionalIntText(first.Steps))
	}
	second := s.daily(t, "2026-06-22")
	if second.Steps == nil || *second.Steps != 3708 {
		t.Fatalf("2026-06-22 steps = %s, want 3708", optionalIntText(second.Steps))
	}
	if second.WorkoutCount != 1 {
		t.Fatalf("workoutCount = %d, want 1", second.WorkoutCount)
	}
	if second.SleepMins == nil || *second.SleepMins != 307 {
		t.Fatalf("sleepMins = %s, want 307", optionalIntText(second.SleepMins))
	}
	if second.NapCount != 1 {
		t.Fatalf("napCount = %d, want 1", second.NapCount)
	}
}

// TestIdenticalRetryAndNewerSessionKeepsLineage proves a retry cannot inflate
// counts and that the same bytes in a newer session keep their own envelope.
func TestIdenticalRetryAndNewerSessionKeepsLineage(t *testing.T) {
	s := newStack(t)
	blobs := realBlobs(t)
	session := realSession(t)
	catalog := realCatalog(t)

	first := s.uploadOK(t, session, catalog, blobs)
	before := s.daily(t, "2026-06-21")

	retry := s.uploadOK(t, session, catalog, blobs)
	if retry.SessionID != first.SessionID {
		t.Fatalf("retry session id = %q", retry.SessionID)
	}
	if sessions := s.count(t, "sync_sessions"); sessions != 1 {
		t.Fatalf("sync_sessions = %d after a retry, want 1", sessions)
	}
	if stored := s.count(t, "raw_blobs"); stored != len(fixtureCodes) {
		t.Fatalf("raw_blobs = %d after a retry, want %d", stored, len(fixtureCodes))
	}
	if steps := s.count(t, "step_samples"); steps != 2109 {
		t.Fatalf("step_samples = %d after a retry, want 2109", steps)
	}

	started, ended := realWindow(t)
	newer := sessionPart(t, "fixture-newer-2026-06-22", started, ended)
	s.uploadOK(t, newer, catalogWithoutSessionID(t, catalog), blobs)

	if sessions := s.count(t, "sync_sessions"); sessions != 2 {
		t.Fatalf("sync_sessions = %d, want 2", sessions)
	}
	if stored := s.count(t, "raw_blobs"); stored != 2*len(fixtureCodes) {
		t.Fatalf("raw_blobs = %d, want %d", stored, 2*len(fixtureCodes))
	}
	if steps := s.count(t, "step_samples"); steps != 2109 {
		t.Fatalf("step_samples = %d after a replay, want 2109", steps)
	}

	after := s.daily(t, "2026-06-21")
	if before.Steps == nil || after.Steps == nil || *before.Steps != *after.Steps {
		t.Fatalf("steps changed across a replay: %v then %v", before.Steps, after.Steps)
	}
	if !before.UpdatedAt.Equal(after.UpdatedAt) {
		t.Fatalf("replayed equal values recomputed the day: %s then %s",
			before.UpdatedAt, after.UpdatedAt)
	}
}

// TestNewerEqualValueAdvancesObservationWithoutRegressing checks the
// (observed_at, session_id) ordering rule for canonical samples.
func TestNewerEqualValueAdvancesObservationWithoutRegressing(t *testing.T) {
	s := newStack(t)
	minute := time.Date(2026, time.September, 22, 1, 0, 0, 0, time.UTC)
	catalog := catalogPart(t, catalogEntry{Code: "0x2E", RoundStart: istWallClock(minute)})

	oldest := sessionPart(t, "temp-oldest", minute.Add(-10*time.Minute), minute)
	older := sessionPart(t, "temp-old", minute, minute.Add(5*time.Minute))
	newer := sessionPart(t, "temp-new", minute, minute.Add(15*time.Minute))

	s.uploadOK(t, older, catalog, map[string][]byte{"0x2E": temperatureMinute(3212)})
	if got := s.queryText(t,
		"SELECT session_id FROM temperature_samples WHERE sampled_at = $1", minute); got != "temp-old" {
		t.Fatalf("first sample session = %q, want temp-old", got)
	}
	before := s.daily(t, "2026-09-22")

	s.uploadOK(t, newer, catalog, map[string][]byte{"0x2E": temperatureMinute(3212)})
	if got := s.queryText(t,
		"SELECT session_id FROM temperature_samples WHERE sampled_at = $1", minute); got != "temp-new" {
		t.Fatalf("equal newer sample session = %q, want temp-new", got)
	}
	after := s.daily(t, "2026-09-22")
	if !before.UpdatedAt.Equal(after.UpdatedAt) {
		t.Fatalf("an equal newer sample recomputed the day: %s then %s",
			before.UpdatedAt, after.UpdatedAt)
	}

	s.uploadOK(t, oldest, catalog, map[string][]byte{"0x2E": temperatureMinute(3100)})
	value := s.queryText(t,
		"SELECT celsius::text FROM temperature_samples WHERE sampled_at = $1", minute)
	if value != "32.12" {
		t.Fatalf("older different value regressed the sample: %s", value)
	}
	if got := s.queryText(t,
		"SELECT session_id FROM temperature_samples WHERE sampled_at = $1", minute); got != "temp-new" {
		t.Fatalf("older upload took ownership: %q", got)
	}
	if stored := s.count(t, "raw_blobs"); stored != 3 {
		t.Fatalf("raw_blobs = %d, want 3 retained envelopes", stored)
	}
}

// TestSessionConflictKeepsOriginalState proves a changed payload under an
// existing session id is refused without touching stored state.
func TestSessionConflictKeepsOriginalState(t *testing.T) {
	s := newStack(t)
	blobs := realBlobs(t)
	s.uploadOK(t, realSession(t), realCatalog(t), blobs)

	changed := make(map[string][]byte, len(blobs))
	for code, payload := range blobs {
		changed[code] = payload
	}
	mutated := append([]byte(nil), changed["0x46"]...)
	mutated[0] ^= 0xff
	changed["0x46"] = mutated

	recorder := s.upload(t, phoneKid, realSession(t), realCatalog(t), changed)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("changed payload status %d, want 409", recorder.Code)
	}
	if code := errorCode(t, recorder); code != "session_conflict" {
		t.Fatalf("error code = %q", code)
	}
	if sessions := s.count(t, "sync_sessions"); sessions != 1 {
		t.Fatalf("sync_sessions = %d, want 1", sessions)
	}
	if raw := s.count(t, "raw_blobs"); raw != len(fixtureCodes) {
		t.Fatalf("raw_blobs = %d, want %d", raw, len(fixtureCodes))
	}
	hexPayloads := s.rawHex(t, "fixture-dumpp-2026-06-22")
	if hexPayloads["0x46"] == fmt.Sprintf("%x", mutated) {
		t.Fatal("the conflicting payload replaced the stored bytes")
	}
}

// chainUpload is one prepared synthetic upload with its exact request bytes.
type chainUpload struct {
	session []byte
	catalog []byte
	blobs   map[string][]byte
	body    []byte
	content string
}

// TestOverlapChainAllUploadOrders replays A, B and C in every order and checks
// that the canonical set is A plus C.
func TestOverlapChainAllUploadOrders(t *testing.T) {
	day := time.Date(2026, time.September, 25, 0, 0, 0, 0, time.UTC)
	type candidate struct {
		id       string
		envelope [2]time.Time
		start    time.Time
		duration time.Duration
	}
	candidates := []candidate{
		{
			id: "chain-a",
			envelope: [2]time.Time{
				time.Date(2026, time.September, 25, 0, 30, 0, 0, time.UTC),
				time.Date(2026, time.September, 25, 2, 5, 0, 0, time.UTC),
			},
			start:    time.Date(2026, time.September, 25, 1, 0, 0, 0, time.UTC),
			duration: time.Hour,
		},
		{
			id: "chain-b",
			envelope: [2]time.Time{
				time.Date(2026, time.September, 25, 1, 0, 0, 0, time.UTC),
				time.Date(2026, time.September, 25, 2, 40, 0, 0, time.UTC),
			},
			start:    time.Date(2026, time.September, 25, 1, 30, 0, 0, time.UTC),
			duration: time.Hour,
		},
		{
			id: "chain-c",
			envelope: [2]time.Time{
				time.Date(2026, time.September, 25, 1, 30, 0, 0, time.UTC),
				time.Date(2026, time.September, 25, 3, 10, 0, 0, time.UTC),
			},
			start:    time.Date(2026, time.September, 25, 2, 0, 0, 0, time.UTC),
			duration: time.Hour,
		},
	}
	orders := [][3]int{
		{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0},
	}

	for _, order := range orders {
		name := fmt.Sprintf("%s-%s-%s",
			candidates[order[0]].id, candidates[order[1]].id, candidates[order[2]].id)
		t.Run(name, func(t *testing.T) {
			s := newStack(t)
			uploads := make([]chainUpload, len(candidates))
			for index, item := range candidates {
				uploads[index] = chainUpload{
					session: sessionPart(t, item.id, item.envelope[0], item.envelope[1]),
					catalog: catalogPart(t),
					blobs: map[string][]byte{
						"0x48": sleepRecord(t, day, item.start, item.start.Add(item.duration)),
						"0x05": workoutRecord(t, item.start, int(item.duration.Seconds()), 1),
					},
				}
			}
			send := func(index int) {
				item := uploads[index]
				s.uploadOK(t, item.session, item.catalog, item.blobs)
			}

			for _, index := range order {
				send(index)
			}
			assertChainResult(t, s, candidates[0].start, candidates[2].start)

			// Exact retries keep every candidate and the same canonical set.
			for _, index := range order {
				send(index)
			}
			assertChainResult(t, s, candidates[0].start, candidates[2].start)
		})
	}
}

// assertChainResult checks the retained candidates and the canonical set.
func assertChainResult(t *testing.T, s *stack, firstStart, lastStart time.Time) {
	t.Helper()
	if retained := s.count(t, "sleep_observations"); retained != 3 {
		t.Fatalf("sleep_observations = %d, want 3", retained)
	}
	if retained := s.count(t, "workout_observations"); retained != 3 {
		t.Fatalf("workout_observations = %d, want 3", retained)
	}
	if canonical := s.count(t, "sleep_sessions"); canonical != 2 {
		t.Fatalf("sleep_sessions = %d, want 2", canonical)
	}
	if canonical := s.count(t, "activities"); canonical != 2 {
		t.Fatalf("activities = %d, want 2", canonical)
	}

	want := []string{
		firstStart.UTC().Format(time.RFC3339),
		lastStart.UTC().Format(time.RFC3339),
	}
	gotSleep := s.queryStrings(t,
		`SELECT to_char(started_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		 FROM sleep_sessions ORDER BY started_at`)
	if !equalStrings(gotSleep, want) {
		t.Fatalf("canonical sleep = %v, want %v", gotSleep, want)
	}
	gotWorkouts := s.queryStrings(t,
		`SELECT to_char(started_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		 FROM activities WHERE kind = 'workout' ORDER BY started_at`)
	if !equalStrings(gotWorkouts, want) {
		t.Fatalf("canonical workouts = %v, want %v", gotWorkouts, want)
	}

	keys := s.dailyKeys(t)
	if len(keys) != 1 || keys[0] != "2026-09-25" {
		t.Fatalf("daily keys = %v, want [2026-09-25]", keys)
	}
	row := s.daily(t, "2026-09-25")
	if row.SleepMins == nil || *row.SleepMins != 120 {
		t.Fatalf("sleepMins = %s, want 120", optionalIntText(row.SleepMins))
	}
	if row.WorkoutCount != 2 {
		t.Fatalf("workoutCount = %d, want 2", row.WorkoutCount)
	}
	if row.SleepScore == nil || *row.SleepScore != 60 {
		t.Fatalf("sleepScore = %s, want 60", optionalIntText(row.SleepScore))
	}
}

// equalStrings reports whether two slices hold the same ordered values.
func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// TestMalformedFramingRejectsWholeUpload proves a structural type failure
// writes nothing and leaves no poisoned state.
func TestMalformedFramingRejectsWholeUpload(t *testing.T) {
	s := newStack(t)
	started, ended := realWindow(t)
	session := sessionPart(t, "framing-2026-09-22", started, ended)
	catalog := catalogPart(t, catalogEntry{Code: "0x01", RoundStart: "2026-06-21T11:16:00.000"})

	recorder := s.upload(t, phoneKid, session, catalog, map[string][]byte{
		"0x01": {0x00, 0x00, 0x00},
	})
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status %d, want 422: %s", recorder.Code, recorder.Body.String())
	}
	if code := errorCode(t, recorder); code != "invalid_type" {
		t.Fatalf("error code = %q, want invalid_type", code)
	}

	for _, table := range appTables {
		if table == "auth_nonces" {
			continue
		}
		if count := s.count(t, table); count != 0 {
			t.Fatalf("%s = %d rows after a rejected upload", table, count)
		}
	}

	// The next valid request accepts its own rows; nothing was poisoned.
	s.uploadOK(t, realSession(t), realCatalog(t), realBlobs(t))
	if sessions := s.count(t, "sync_sessions"); sessions != 1 {
		t.Fatalf("sync_sessions = %d after the retry", sessions)
	}
	if steps := s.count(t, "step_samples"); steps != 2109 {
		t.Fatalf("step_samples = %d after the retry", steps)
	}
}

// TestNonceReplayRejectedOnceUnderConcurrency sends one signed nonce to many
// concurrent requests and requires exactly one authentication.
func TestNonceReplayRejectedOnceUnderConcurrency(t *testing.T) {
	s := newStack(t)

	signed, err := http.NewRequest(http.MethodGet, "http://api.test/api/metrics/coverage", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	s.sign(t, phoneKid, signed, nil, freshNonce(t), time.Now().Unix())
	header := signed.Header.Get(auth.HeaderAuth)
	hash := signed.Header.Get(auth.HeaderBodySHA256)

	const attempts = 8
	codes := make([]int, attempts)
	var wait sync.WaitGroup
	start := make(chan struct{})
	for index := range attempts {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			replay, err := http.NewRequest(http.MethodGet,
				"http://api.test/api/metrics/coverage", nil)
			if err != nil {
				codes[index] = -1
				return
			}
			replay.Header.Set(auth.HeaderAuth, header)
			replay.Header.Set(auth.HeaderBodySHA256, hash)
			<-start
			codes[index] = s.serve(replay).Code
		}(index)
	}
	close(start)
	wait.Wait()

	accepted := 0
	for _, code := range codes {
		switch code {
		case http.StatusOK:
			accepted++
		case http.StatusUnauthorized:
		default:
			t.Fatalf("unexpected status %d", code)
		}
	}
	if accepted != 1 {
		t.Fatalf("accepted %d concurrent replays, want 1: %v", accepted, codes)
	}
	if nonces := s.count(t, "auth_nonces"); nonces != 1 {
		t.Fatalf("auth_nonces = %d, want 1", nonces)
	}

	// A second mux over the same database is the multi-instance case: replay
	// stays rejected because the nonce lives in the database.
	second := newHandler(s.db, s.keys)
	replay, err := http.NewRequest(http.MethodGet, "http://api.test/api/metrics/coverage", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	replay.Header.Set(auth.HeaderAuth, header)
	replay.Header.Set(auth.HeaderBodySHA256, hash)
	recorder := httptest.NewRecorder()
	second.ServeHTTP(recorder, replay)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("replay against a second instance = %d, want 401", recorder.Code)
	}
}

// TestWebWriteForbiddenBeforeBodyRead proves the credential split rejects a web
// write before the body or nonce is touched.
func TestWebWriteForbiddenBeforeBodyRead(t *testing.T) {
	s := newStack(t)

	body := &failReader{}
	request, err := http.NewRequest(http.MethodPut, "http://api.test/api/profile", body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	s.sign(t, webKid, request, nil, freshNonce(t), time.Now().Unix())

	recorder := s.serve(request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status %d, want 403: %s", recorder.Code, recorder.Body.String())
	}
	if code := errorCode(t, recorder); code != "forbidden" {
		t.Fatalf("error code = %q", code)
	}
	if body.read {
		t.Fatal("the refused request read its body")
	}
	if nonces := s.count(t, "auth_nonces"); nonces != 0 {
		t.Fatalf("auth_nonces = %d, want 0", nonces)
	}
	if profiles := s.count(t, "profile"); profiles != 0 {
		t.Fatalf("profile = %d rows, want 0", profiles)
	}

	session := sessionPart(t, "web-ingest",
		time.Date(2026, time.September, 21, 1, 0, 0, 0, time.UTC),
		time.Date(2026, time.September, 21, 1, 10, 0, 0, time.UTC))
	recorder = s.upload(t, webKid, session, catalogPart(t), map[string][]byte{
		"0x01": activityMinute(60, 5),
	})
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("web ingest status %d, want 403", recorder.Code)
	}
	if nonces := s.count(t, "auth_nonces"); nonces != 0 {
		t.Fatalf("auth_nonces = %d after refused web ingest", nonces)
	}
	if sessions := s.count(t, "sync_sessions"); sessions != 0 {
		t.Fatalf("sync_sessions = %d, want 0", sessions)
	}
}

// TestUnknownPathAndWrongMethod checks the exact-path router contract.
func TestUnknownPathAndWrongMethod(t *testing.T) {
	s := newStack(t)

	cases := []struct {
		method string
		path   string
		status int
		code   string
		allow  string
	}{
		{"GET", "/api/unknown", http.StatusNotFound, "not_found", ""},
		{"GET", "/api/daily-metrics/", http.StatusNotFound, "not_found", ""},
		{"HEAD", "/api/daily-metrics", http.StatusMethodNotAllowed, "method_not_allowed", "GET"},
		{"POST", "/health", http.StatusMethodNotAllowed, "method_not_allowed", "GET"},
		{"HEAD", "/health", http.StatusMethodNotAllowed, "method_not_allowed", "GET"},
		{"GET", "/api/profile", http.StatusMethodNotAllowed, "method_not_allowed", "PUT"},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			request, err := http.NewRequest(tc.method, "http://api.test"+tc.path, nil)
			if err != nil {
				t.Fatalf("new request: %v", err)
			}
			recorder := s.serve(request)
			if recorder.Code != tc.status {
				t.Fatalf("status %d, want %d: %s", recorder.Code, tc.status, recorder.Body.String())
			}
			if code := errorCode(t, recorder); code != tc.code {
				t.Fatalf("error code = %q, want %q", code, tc.code)
			}
			if tc.allow != "" {
				if allow := recorder.Header().Get("Allow"); allow != tc.allow {
					t.Fatalf("Allow = %q, want %q", allow, tc.allow)
				}
			}
		})
	}
}

// TestDisplacedEarliestDayIsPruned crosses midnight with an older and a newer
// workout and requires the same final keys in both upload orders.
func TestDisplacedEarliestDayIsPruned(t *testing.T) {
	// IST 2026-10-01 23:50 on the older candidate and 2026-10-02 00:00 on the
	// newer one, so the older row starts on the earlier IST day.
	olderStart := time.Date(2026, time.October, 1, 18, 20, 0, 0, time.UTC)
	newerStart := time.Date(2026, time.October, 1, 18, 30, 0, 0, time.UTC)

	older := chainUpload{
		session: sessionPart(t, "displaced-older",
			olderStart.Add(-time.Minute), olderStart.Add(25*time.Minute)),
		catalog: catalogPart(t),
		blobs:   map[string][]byte{"0x05": workoutRecord(t, olderStart, 20*60, 1)},
	}
	newer := chainUpload{
		session: sessionPart(t, "displaced-newer",
			newerStart.Add(-time.Minute), newerStart.Add(25*time.Minute)),
		catalog: catalogPart(t),
		blobs:   map[string][]byte{"0x05": workoutRecord(t, newerStart, 20*60, 1)},
	}

	t.Run("older first", func(t *testing.T) {
		s := newStack(t)
		s.uploadOK(t, older.session, older.catalog, older.blobs)
		if keys := s.dailyKeys(t); !equalStrings(keys, []string{"2026-10-01"}) {
			t.Fatalf("daily keys after the older upload = %v", keys)
		}
		if row := s.daily(t, "2026-10-01"); row.WorkoutCount != 1 {
			t.Fatalf("workoutCount = %d, want 1", row.WorkoutCount)
		}

		s.uploadOK(t, newer.session, newer.catalog, newer.blobs)
		assertDisplacedResult(t, s)
	})

	t.Run("newer first", func(t *testing.T) {
		s := newStack(t)
		s.uploadOK(t, newer.session, newer.catalog, newer.blobs)
		s.uploadOK(t, older.session, older.catalog, older.blobs)

		// The older candidate never becomes canonical, so its day never exists.
		if keys := s.dailyKeys(t); !equalStrings(keys, []string{"2026-10-02"}) {
			t.Fatalf("daily keys = %v", keys)
		}
		assertDisplacedResult(t, s)
	})
}

// assertDisplacedResult checks the pruned domain and the surviving candidate.
func assertDisplacedResult(t *testing.T, s *stack) {
	t.Helper()
	if keys := s.dailyKeys(t); !equalStrings(keys, []string{"2026-10-02"}) {
		t.Fatalf("daily keys = %v, want [2026-10-02]", keys)
	}
	row := s.daily(t, "2026-10-02")
	if row.WorkoutCount != 1 {
		t.Fatalf("workoutCount = %d, want 1", row.WorkoutCount)
	}
	if retained := s.count(t, "workout_observations"); retained != 2 {
		t.Fatalf("workout_observations = %d, want 2 retained candidates", retained)
	}
	if canonical := s.count(t, "activities"); canonical != 1 {
		t.Fatalf("activities = %d, want 1 canonical workout", canonical)
	}
	start := s.queryText(t,
		`SELECT to_char(started_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		 FROM activities`)
	if start != "2026-10-01T18:30:00Z" {
		t.Fatalf("canonical workout start = %s, want the newer candidate", start)
	}
}

// TestLateWriteFailureRollsBackIngest injects a failure on the final daily write
// and requires no partial ingest state, with the consumed nonce as the one
// documented exception.
func TestLateWriteFailureRollsBackIngest(t *testing.T) {
	s := newStack(t)

	execute(t, s.db, `CREATE FUNCTION reject_ingest_daily() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'injected late failure';
END
$$`)
	execute(t, s.db, `CREATE TRIGGER reject_ingest_daily
BEFORE INSERT OR UPDATE ON daily_metrics
FOR EACH ROW EXECUTE FUNCTION reject_ingest_daily()`)

	recorder := s.upload(t, phoneKid, realSession(t), realCatalog(t), realBlobs(t))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status %d, want 500: %s", recorder.Code, recorder.Body.String())
	}
	if code := errorCode(t, recorder); code != "internal" {
		t.Fatalf("error code = %q, want internal", code)
	}

	for _, table := range appTables {
		if table == "auth_nonces" {
			continue
		}
		if count := s.count(t, table); count != 0 {
			t.Fatalf("%s = %d rows after a rolled-back ingest", table, count)
		}
	}
	if nonces := s.count(t, "auth_nonces"); nonces != 1 {
		t.Fatalf("auth_nonces = %d, want the one consumed by authentication", nonces)
	}

	execute(t, s.db, `DROP TRIGGER reject_ingest_daily ON daily_metrics`)
	execute(t, s.db, `DROP FUNCTION reject_ingest_daily()`)

	s.uploadOK(t, realSession(t), realCatalog(t), realBlobs(t))
	if steps := s.count(t, "step_samples"); steps != 2109 {
		t.Fatalf("step_samples = %d after the clean retry", steps)
	}
}

// concurrentCall is one signed request built per stack.
type concurrentCall struct {
	name   string
	status int
	build  func(t *testing.T, s *stack) *http.Request
}

// serveConcurrently starts every request behind one barrier and returns the
// status codes. The mux request timeout bounds each request.
func serveConcurrently(t *testing.T, s *stack, calls []concurrentCall) []int {
	t.Helper()
	requests := make([]*http.Request, len(calls))
	for index, call := range calls {
		requests[index] = call.build(t, s)
	}
	codes := make([]int, len(requests))
	var wait sync.WaitGroup
	start := make(chan struct{})
	for index := range requests {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			codes[index] = s.serve(requests[index]).Code
		}(index)
	}
	close(start)
	wait.Wait()
	return codes
}

// stateSnapshot is the comparable final state, without timestamps or replay
// bookkeeping.
type stateSnapshot struct {
	counts        map[string]int
	sessions      []string
	dailyKeys     []string
	days          []string
	sleepStarts   []string
	workoutStarts []string
	profile       []string
}

// captureState reads the semantic state of one database.
func captureState(t *testing.T, s *stack) stateSnapshot {
	t.Helper()
	snapshot := stateSnapshot{
		counts:    s.tabulated(t),
		sessions:  s.queryStrings(t, "SELECT session_id FROM sync_sessions ORDER BY session_id"),
		dailyKeys: s.dailyKeys(t),
		sleepStarts: s.queryStrings(t,
			`SELECT to_char(started_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
			 FROM sleep_sessions ORDER BY started_at`),
		workoutStarts: s.queryStrings(t,
			`SELECT to_char(started_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
			 FROM activities ORDER BY started_at`),
	}
	delete(snapshot.counts, "auth_nonces")
	for _, day := range snapshot.dailyKeys {
		row := s.daily(t, day)
		snapshot.days = append(snapshot.days, fmt.Sprintf("%s|steps=%s|sleep=%s|workouts=%d|load=%s|readiness=%s",
			day, optionalIntText(row.Steps), optionalIntText(row.SleepMins), row.WorkoutCount,
			optionalFloatText(row.TrainingLoad), optionalIntText(row.Readiness)))
	}
	profile, err := s.db.GetProfile(t.Context())
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	snapshot.profile = []string{
		optionalText(profile.Name),
		optionalDayText(profile.BirthDate),
		optionalText(profile.Sex),
		optionalFloatText(profile.HeightCm),
		optionalFloatText(profile.WeightKg),
	}
	return snapshot
}

// TestConcurrentWritesMatchSerialExecution runs two overlapping uploads and one
// profile write together and compares the result with the same requests applied
// one at a time.
func TestConcurrentWritesMatchSerialExecution(t *testing.T) {
	day := time.Date(2026, time.October, 3, 0, 0, 0, 0, time.UTC)
	candidates := []struct {
		id       string
		envelope [2]time.Time
		start    time.Time
	}{
		{
			id: "concurrent-a",
			envelope: [2]time.Time{
				time.Date(2026, time.October, 3, 0, 30, 0, 0, time.UTC),
				time.Date(2026, time.October, 3, 2, 5, 0, 0, time.UTC),
			},
			start: time.Date(2026, time.October, 3, 1, 0, 0, 0, time.UTC),
		},
		{
			id: "concurrent-b",
			envelope: [2]time.Time{
				time.Date(2026, time.October, 3, 1, 0, 0, 0, time.UTC),
				time.Date(2026, time.October, 3, 2, 40, 0, 0, time.UTC),
			},
			start: time.Date(2026, time.October, 3, 1, 30, 0, 0, time.UTC),
		},
	}
	profile := []byte(`{"name":null,"birthDate":"1990-02-03","sex":"male","heightCm":175.0,"weightKg":70.0}`)

	uploads := make([]chainUpload, len(candidates))
	for index, candidate := range candidates {
		session := sessionPart(t, candidate.id, candidate.envelope[0], candidate.envelope[1])
		blobs := map[string][]byte{
			"0x48": sleepRecord(t, day, candidate.start, candidate.start.Add(time.Hour)),
			"0x05": workoutRecord(t, candidate.start, 3600, 1),
		}
		body, content := multipartBody(t, session, catalogPart(t), blobs)
		uploads[index] = chainUpload{
			session: session, catalog: catalogPart(t), blobs: blobs, body: body, content: content,
		}
	}

	// Serial reference: the same requests, one at a time.
	serial := newStack(t)
	for _, upload := range uploads {
		serial.uploadOK(t, upload.session, upload.catalog, upload.blobs)
	}
	recorder := serial.serve(serial.newRequest(t, phoneKid, http.MethodPut,
		"/api/profile", "", profile, "application/json"))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("serial profile status %d: %s", recorder.Code, recorder.Body.String())
	}
	want := captureState(t, serial)
	if len(want.dailyKeys) == 0 {
		t.Fatal("the serial reference stored no daily row")
	}

	// Concurrent run on a different database with the same requests.
	concurrent := newStack(t)
	calls := make([]concurrentCall, 0, len(uploads)+1)
	for _, upload := range uploads {
		item := upload
		calls = append(calls, concurrentCall{
			name:   "upload",
			status: http.StatusOK,
			build: func(t *testing.T, s *stack) *http.Request {
				return s.newRequest(t, phoneKid, http.MethodPost, "/api/ingest", "",
					item.body, item.content)
			},
		})
	}
	calls = append(calls, concurrentCall{
		name:   "profile",
		status: http.StatusNoContent,
		build: func(t *testing.T, s *stack) *http.Request {
			return s.newRequest(t, phoneKid, http.MethodPut, "/api/profile", "",
				profile, "application/json")
		},
	})

	codes := serveConcurrently(t, concurrent, calls)
	for index, call := range calls {
		switch codes[index] {
		case call.status:
		case http.StatusServiceUnavailable:
			// The documented retryable response: retry once and require success.
			if retry := concurrent.serve(call.build(t, concurrent)); retry.Code != call.status {
				t.Fatalf("%s retry status %d: %s", call.name, retry.Code, retry.Body.String())
			}
		default:
			t.Fatalf("%s status %d, want %d", call.name, codes[index], call.status)
		}
	}

	got := captureState(t, concurrent)
	for table, wantCount := range want.counts {
		if gotCount := got.counts[table]; gotCount != wantCount {
			t.Fatalf("%s = %d after concurrent writes, want %d", table, gotCount, wantCount)
		}
	}
	if !equalStrings(got.sessions, want.sessions) {
		t.Fatalf("sessions = %v, want %v", got.sessions, want.sessions)
	}
	if len(got.sessions) != len(uploads) {
		t.Fatalf("stored %d sessions, want %d", len(got.sessions), len(uploads))
	}
	if !equalStrings(got.dailyKeys, want.dailyKeys) {
		t.Fatalf("daily keys = %v, want %v", got.dailyKeys, want.dailyKeys)
	}
	if !equalStrings(got.days, want.days) {
		t.Fatalf("daily values = %v, want %v", got.days, want.days)
	}
	if !equalStrings(got.sleepStarts, want.sleepStarts) {
		t.Fatalf("canonical sleep = %v, want %v", got.sleepStarts, want.sleepStarts)
	}
	if !equalStrings(got.workoutStarts, want.workoutStarts) {
		t.Fatalf("canonical workouts = %v, want %v", got.workoutStarts, want.workoutStarts)
	}
	if !equalStrings(got.profile, want.profile) {
		t.Fatalf("profile = %v, want %v", got.profile, want.profile)
	}
}
