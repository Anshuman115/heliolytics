package integration

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/Anshuman115/heliolytics/internal/api"
	"github.com/Anshuman115/heliolytics/internal/auth"
	"github.com/Anshuman115/heliolytics/internal/config"
	"github.com/Anshuman115/heliolytics/internal/ingest"
	"github.com/Anshuman115/heliolytics/internal/store"
	"github.com/Anshuman115/heliolytics/internal/testutil"
	"github.com/jackc/pgx/v5"
)

const (
	fixturePackage = "parse"
	fixtureSet     = "real-dumpp-2026-06"

	syntheticMAC = "02:00:00:00:00:01"
	phoneKid     = "phone"
	webKid       = "web"

	istOffsetSeconds = 5*3600 + 30*60
)

var istZone = time.FixedZone("IST", istOffsetSeconds)

var fixtureCodes = []string{
	"0x01", "0x05", "0x06", "0x0D", "0x13", "0x26", "0x2E", "0x38", "0x46", "0x48", "0x49",
}

var appTables = []string{
	"sync_sessions", "raw_blobs",
	"heart_rate_samples", "step_samples", "temperature_samples", "hrv_samples",
	"stress_samples", "resp_samples", "spo2_samples",
	"sleep_sessions", "activities", "sleep_observations", "workout_observations",
	"pai_scores", "coverage", "daily_metrics", "profile", "auth_nonces",
}

func testKeys() auth.KeySet {
	var phone, web [32]byte
	for i := range phone {
		phone[i] = byte(0x30 + i)
	}
	for i := range web {
		web[i] = byte(0x70 + i)
	}
	return auth.NewKeySet(phone, web)
}

func testConfig(keys auth.KeySet) config.Config {
	return config.Config{
		Keys:                   keys,
		RateLimitPerMin:        2000,
		RateLimitPerKeyPerMin:  2000,
		RequestTimeout:         20 * time.Second,
		MaxConcurrentBodyReads: 4,
	}
}

type stack struct {
	handler http.Handler
	db      *store.Store
	keys    auth.KeySet
}

func newStack(t *testing.T) *stack {
	t.Helper()
	dsn := testutil.NewDB(t)
	db, err := store.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(db.Close)

	keys := testKeys()
	return &stack{handler: newHandler(db, keys), db: db, keys: keys}
}

func newHandler(db *store.Store, keys auth.KeySet) http.Handler {
	return api.NewMux(db, testConfig(keys),
		func(ctx context.Context, meta store.SessionMeta, catalog []byte,
			blobs map[string][]byte) (store.IngestResult, error) {
			return ingest.Run(ctx, db, meta, catalog, blobs)
		},
		func(ctx context.Context, profile *store.Profile) error {
			return ingest.UpdateProfile(ctx, db, profile)
		},
	)
}

func offlineStack(t *testing.T) *stack {
	t.Helper()
	db, err := store.New(t.Context(),
		"postgres://nobody:nobody@127.0.0.1:1/heliolytics_test?sslmode=disable")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(db.Close)
	keys := testKeys()
	return &stack{handler: newHandler(db, keys), db: db, keys: keys}
}

func httpBody(body []byte) io.Reader {
	if len(body) == 0 {
		return nil
	}
	return bytes.NewReader(body)
}

func readAll(response *http.Response) ([]byte, error) {
	defer response.Body.Close()
	return io.ReadAll(io.LimitReader(response.Body, 1<<20))
}

func (s *stack) newRequest(t *testing.T, kid, method, path, query string,
	body []byte, contentType string) *http.Request {
	t.Helper()
	target := "http://api.test" + path
	if query != "" {
		target += "?" + query
	}
	request, err := http.NewRequest(method, target, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	s.sign(t, kid, request, body, freshNonce(t), time.Now().Unix())
	return request
}

func (s *stack) sign(t *testing.T, kid string, request *http.Request,
	body []byte, nonce string, ts int64) {
	t.Helper()
	key, err := s.keys.Lookup(kid)
	if err != nil {
		t.Fatalf("Lookup(%q): %v", kid, err)
	}
	secret, err := hex.DecodeString(key.SecretHex())
	if err != nil {
		t.Fatalf("secret hex: %v", err)
	}
	sum := sha256.Sum256(body)
	hash := hex.EncodeToString(sum[:])
	canonical := auth.CanonicalText(kid, ts, nonce, request.Method,
		request.URL.EscapedPath(), request.URL.RawQuery, hash, request.Header.Get("Content-Type"))
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(canonical))
	request.Header.Set(auth.HeaderAuth,
		fmt.Sprintf("%s.%d.%s.%s", kid, ts, nonce, hex.EncodeToString(mac.Sum(nil))))
	request.Header.Set(auth.HeaderBodySHA256, hash)
}

func (s *stack) serve(request *http.Request) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	s.handler.ServeHTTP(recorder, request)
	return recorder
}

func (s *stack) get(t *testing.T, kid, path, query string) *httptest.ResponseRecorder {
	t.Helper()
	return s.serve(s.newRequest(t, kid, http.MethodGet, path, query, nil, ""))
}

func (s *stack) upload(t *testing.T, kid string, session, catalog []byte,
	blobs map[string][]byte) *httptest.ResponseRecorder {
	t.Helper()
	body, contentType := multipartBody(t, session, catalog, blobs)
	return s.serve(s.newRequest(t, kid, http.MethodPost, "/api/ingest", "", body, contentType))
}

func (s *stack) uploadOK(t *testing.T, session, catalog []byte, blobs map[string][]byte) store.IngestResult {
	t.Helper()
	recorder := s.upload(t, phoneKid, session, catalog, blobs)
	if recorder.Code != http.StatusOK {
		t.Fatalf("ingest status %d: %s", recorder.Code, recorder.Body.String())
	}
	return decodeResult(t, recorder)
}

func (s *stack) count(t *testing.T, table string) int {
	t.Helper()
	var total int
	ctx := context.Background()
	if err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&total)
	}); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return total
}

func (s *stack) countWhere(t *testing.T, table, clause string) int {
	t.Helper()
	var total int
	ctx := context.Background()
	if err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT count(*) FROM "+table+" WHERE "+clause).Scan(&total)
	}); err != nil {
		t.Fatalf("count %s where %s: %v", table, clause, err)
	}
	return total
}

func (s *stack) queryText(t *testing.T, statement string, args ...any) string {
	t.Helper()
	var value string
	ctx := context.Background()
	if err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, statement, args...).Scan(&value)
	}); err != nil {
		t.Fatalf("query %q: %v", statement, err)
	}
	return value
}

func (s *stack) tabulated(t *testing.T) map[string]int {
	t.Helper()
	out := make(map[string]int, len(appTables))
	for _, table := range appTables {
		out[table] = s.count(t, table)
	}
	return out
}

func (s *stack) daily(t *testing.T, day string) store.DayMetric {
	t.Helper()
	rows, err := s.db.ListDays(context.Background(), day, day)
	if err != nil {
		t.Fatalf("ListDays(%s): %v", day, err)
	}
	if len(rows) != 1 {
		t.Fatalf("ListDays(%s) returned %d rows, want 1", day, len(rows))
	}
	return rows[0]
}

func (s *stack) dailyKeys(t *testing.T) []string {
	t.Helper()
	var keys []string
	ctx := context.Background()
	if err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, "SELECT day_key::text FROM daily_metrics ORDER BY day_key")
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var key string
			if err := rows.Scan(&key); err != nil {
				return err
			}
			keys = append(keys, key)
		}
		return rows.Err()
	}); err != nil {
		t.Fatalf("daily keys: %v", err)
	}
	return keys
}

func multipartBody(t *testing.T, session, catalog []byte, blobs map[string][]byte) ([]byte, string) {
	t.Helper()
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	writePart(t, writer, "session", session)
	writePart(t, writer, "catalog", catalog)
	codes := make([]string, 0, len(blobs))
	for code := range blobs {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	for _, code := range codes {
		writePart(t, writer, code+"_raw.bin", blobs[code])
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	return buffer.Bytes(), writer.FormDataContentType()
}

func writePart(t *testing.T, writer *multipart.Writer, name string, data []byte) {
	t.Helper()
	part, err := writer.CreateFormField(name)
	if err != nil {
		t.Fatalf("create part %s: %v", name, err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write part %s: %v", name, err)
	}
}

func decodeResult(t *testing.T, recorder *httptest.ResponseRecorder) store.IngestResult {
	t.Helper()
	var result store.IngestResult
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode ingest result %q: %v", recorder.Body.String(), err)
	}
	return result
}

func errorCode(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode error %q: %v", recorder.Body.String(), err)
	}
	return envelope.Error.Code
}

func freshNonce(t *testing.T) string {
	t.Helper()
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return hex.EncodeToString(buffer)
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller did not report a file")
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(file)))
}

func fixturePath(t *testing.T, name string) string {
	t.Helper()
	root := repoRoot(t)
	path := filepath.Join(root, "internal", fixturePackage, "testdata", fixtureSet, name)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("required fixture %s is missing: %v", path, err)
	}
	return path
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(fixturePath(t, name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	if len(data) == 0 {
		t.Fatalf("fixture %s is empty", name)
	}
	return data
}

func realSession(t *testing.T) []byte {
	t.Helper()
	return readFixture(t, "session.json")
}

func realCatalog(t *testing.T) []byte {
	t.Helper()
	return readFixture(t, "catalog.json")
}

func realBlobs(t *testing.T) map[string][]byte {
	t.Helper()
	blobs := make(map[string][]byte, len(fixtureCodes))
	for _, code := range fixtureCodes {
		blobs[code] = readFixture(t, code+"_raw.bin")
	}
	return blobs
}

func sessionPart(t *testing.T, id string, startedAt, endedAt time.Time) []byte {
	t.Helper()
	payload := struct {
		SessionID      string `json:"sessionId"`
		StartedAt      string `json:"startedAt"`
		EndedAt        string `json:"endedAt"`
		DeviceMAC      string `json:"deviceMac"`
		BatteryPercent *int   `json:"batteryPercent"`
	}{
		SessionID: id,
		StartedAt: startedAt.UTC().Format(time.RFC3339),
		EndedAt:   endedAt.UTC().Format(time.RFC3339),
		DeviceMAC: syntheticMAC,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode session: %v", err)
	}
	return encoded
}

type catalogEntry struct {
	Code       string
	RoundStart string
}

func catalogPart(t *testing.T, entries ...catalogEntry) []byte {
	t.Helper()
	type entry struct {
		Code       string `json:"code"`
		Status     string `json:"status"`
		RoundStart string `json:"roundStart,omitempty"`
	}
	payload := struct {
		Chunked []entry `json:"chunked"`
	}{Chunked: make([]entry, 0, len(entries))}
	for _, item := range entries {
		payload.Chunked = append(payload.Chunked, entry{
			Code: item.Code, Status: "ok", RoundStart: item.RoundStart,
		})
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode catalog: %v", err)
	}
	return encoded
}

func istWallClock(at time.Time) string {
	return at.In(istZone).Format("2006-01-02T15:04:05.000")
}

func sleepRecord(t *testing.T, sleepDay, start, end time.Time) []byte {
	t.Helper()
	record := make([]byte, 594)
	midnight := time.Date(sleepDay.Year(), sleepDay.Month(), sleepDay.Day(), 0, 0, 0, 0, istZone)
	binary.LittleEndian.PutUint32(record[4:], uint32(midnight.Unix()))
	record[8] = 1
	record[9] = 1
	base := midnight.Unix() - 86400
	startMinute := (start.Unix() - base) / 60
	endMinute := (end.Unix() - base) / 60
	if startMinute <= 0 || endMinute <= startMinute || endMinute > 0xffff {
		t.Fatalf("sleep interval %s..%s does not fit the record", start, end)
	}
	binary.LittleEndian.PutUint16(record[0x0a:], uint16(startMinute))
	binary.LittleEndian.PutUint16(record[0x0c:], uint16(endMinute))
	record[0x16] = 60
	binary.LittleEndian.PutUint16(record[0x24a:], 20) // REM
	binary.LittleEndian.PutUint16(record[0x24c:], 30) // light
	binary.LittleEndian.PutUint16(record[0x24e:], 10) // deep
	binary.LittleEndian.PutUint16(record[0x250:], 0)  // wake
	return record
}

func workoutRecord(t *testing.T, start time.Time, durationSec, sport int) []byte {
	t.Helper()
	var buffer bytes.Buffer
	// Field 1 carries the supported version string "2.1", which is also the
	// parser's record marker.
	buffer.Write([]byte{0x0a, 0x03, 0x32, 0x2e, 0x31})
	writeProtoMessage(&buffer, 2, protoVarint(1, uint64(start.Unix())))
	writeProtoMessage(&buffer, 7, protoVarint(1, uint64(durationSec)))
	writeProtoVarint(&buffer, 3, uint64(sport))
	return buffer.Bytes()
}

func activityMinute(bpm, steps byte) []byte {
	record := make([]byte, 8)
	record[2] = steps
	record[3] = bpm
	return record
}

func temperatureMinute(hundredths int16) []byte {
	record := make([]byte, 8)
	binary.LittleEndian.PutUint16(record[2:], uint16(hundredths))
	return record
}

func protoVarint(field int, value uint64) []byte {
	out := appendUvarint(nil, uint64(field)<<3)
	return appendUvarint(out, value)
}

func writeProtoVarint(buffer *bytes.Buffer, field int, value uint64) {
	buffer.Write(protoVarint(field, value))
}

func writeProtoMessage(buffer *bytes.Buffer, field int, payload []byte) {
	buffer.Write(appendUvarint(nil, uint64(field)<<3|2))
	buffer.Write(appendUvarint(nil, uint64(len(payload))))
	buffer.Write(payload)
}

func appendUvarint(dst []byte, value uint64) []byte {
	for value >= 0x80 {
		dst = append(dst, byte(value)|0x80)
		value >>= 7
	}
	return append(dst, byte(value))
}

func optionalIntText(value *int) string {
	if value == nil {
		return "null"
	}
	return strconv.Itoa(*value)
}

func optionalFloatText(value *float64) string {
	if value == nil {
		return "null"
	}
	return strconv.FormatFloat(*value, 'g', -1, 64)
}

func optionalText(value *string) string {
	if value == nil {
		return "null"
	}
	return *value
}

func optionalDayText(value *time.Time) string {
	if value == nil {
		return "null"
	}
	return value.Format("2006-01-02")
}

type failReader struct {
	read bool
}

func (r *failReader) Read([]byte) (int, error) {
	r.read = true
	return 0, errors.New("body was read")
}
