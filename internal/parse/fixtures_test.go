package parse

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

const (
	realFixtureDir  = "testdata/real-dumpp-2026-06"
	spo2FixtureDir  = "testdata/synthetic-spo2"
	fixtureTimeForm = "2006-01-02T15:04:05Z"
)

func istLocation() *time.Location {
	return time.FixedZone("IST", int(istOffsetSec))
}

func mustReadFixture(t testing.TB, name ...string) []byte {
	t.Helper()
	path := filepath.Join(append([]string{realFixtureDir}, name...)...)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("fixture %s is missing: %v", path, err)
	}
	return data
}

func fixtureBytes(t testing.TB, code string) []byte {
	t.Helper()
	data := mustReadFixture(t, code+"_raw.bin")
	if len(data) == 0 {
		t.Fatalf("%s fixture is empty", code)
	}
	return data
}

func mustParseFixtureTime(t testing.TB, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("fixture timestamp %q is not RFC3339: %v", value, err)
	}
	return parsed.UTC()
}

type fixtureSession struct {
	SessionID  string `json:"sessionId"`
	StartedAt  string `json:"startedAt"`
	EndedAt    string `json:"endedAt"`
	DeviceMAC  string `json:"deviceMac"`
	BatteryPct *int   `json:"batteryPercent"`
}

func loadFixtureSession(t testing.TB) fixtureSession {
	t.Helper()
	var session fixtureSession
	if err := json.Unmarshal(mustReadFixture(t, "session.json"), &session); err != nil {
		t.Fatalf("decode session.json: %v", err)
	}
	if session.SessionID == "" || session.StartedAt == "" || session.EndedAt == "" {
		t.Fatalf("session.json is incomplete: %+v", session)
	}
	if session.DeviceMAC != "02:00:00:00:00:01" {
		t.Fatalf("session.json must use the synthetic MAC, got %q", session.DeviceMAC)
	}
	return session
}

func loadFixtureWindow(t testing.TB) Window {
	t.Helper()
	session := loadFixtureSession(t)
	return Window{
		StartedAt: mustParseFixtureTime(t, session.StartedAt),
		EndedAt:   mustParseFixtureTime(t, session.EndedAt),
	}
}

func loadFixtureCatalog(t testing.TB) Catalog {
	t.Helper()
	catalog, err := ParseCatalog(mustReadFixture(t, "catalog.json"))
	if err != nil {
		t.Fatalf("decode catalog.json: %v", err)
	}
	if len(catalog.Chunked) == 0 {
		t.Fatal("catalog.json has no chunked entries")
	}
	return catalog
}

type expectedType struct {
	Status           string   `json:"status"`
	Rows             int      `json:"rows"`
	Dropped          int      `json:"dropped"`
	TrailingBytes    int      `json:"trailingBytes"`
	ProcessedThrough *string  `json:"processedThrough"`
	Warnings         []string `json:"warnings"`
}

type expectedSleep struct {
	DayKey          string `json:"day_key"`
	StartedAt       string `json:"started_at"`
	EndedAt         string `json:"ended_at"`
	IsNap           bool   `json:"is_nap"`
	Score           *int   `json:"score"`
	TotalMins       int    `json:"total_mins"`
	DeepMins        int    `json:"deep_mins"`
	REMMins         int    `json:"rem_mins"`
	LightMins       int    `json:"light_mins"`
	WakeMins        int    `json:"wake_mins"`
	Stages          int    `json:"stages"`
	StagesTruncated bool   `json:"stages_truncated"`
}

type expectedFixture struct {
	Types map[string]expectedType `json:"types"`
	Steps struct {
		NonzeroMinutes  int            `json:"nonzero_minutes"`
		ZeroMinutes     int            `json:"zero_minutes"`
		SentinelMinutes int            `json:"sentinel_minutes"`
		Sum             int            `json:"sum"`
		DaySums         map[string]int `json:"day_sums"`
		FirstMinute     string         `json:"first_minute"`
		LastMinute      string         `json:"last_minute"`
	} `json:"steps"`
	HeartRate struct {
		Rows     int                `json:"rows"`
		First    string             `json:"first"`
		Last     string             `json:"last"`
		DayMeans map[string]float64 `json:"day_means"`
	} `json:"heart_rate"`
	ContinuousHeartRate struct {
		Rows     int    `json:"rows"`
		First    string `json:"first"`
		Last     string `json:"last"`
		FirstBPM int    `json:"first_bpm"`
		LastBPM  int    `json:"last_bpm"`
	} `json:"continuous_heart_rate"`
	Temperature struct {
		FirstHundredths int `json:"first_hundredths_c"`
		MinHundredths   int `json:"min_hundredths_c"`
		MaxHundredths   int `json:"max_hundredths_c"`
	} `json:"temperature"`
	HRV struct {
		Rows       int `json:"rows"`
		MinRMSSD   int `json:"min_rmssd"`
		MaxRMSSD   int `json:"max_rmssd"`
		TimezoneID int `json:"timezone"`
	} `json:"hrv"`
	Respiration struct {
		Rows       int `json:"rows"`
		MinBreaths int `json:"min_br_min"`
		MaxBreaths int `json:"max_br_min"`
		TimezoneID int `json:"timezone"`
	} `json:"respiration"`
	Stress struct {
		Rows       int `json:"rows"`
		MinValue   int `json:"min"`
		MaxValue   int `json:"max"`
		SentinelIn int `json:"sentinel_bytes"`
	} `json:"stress"`
	PAI []struct {
		DayKey    string `json:"day_key"`
		Score     int    `json:"score"`
		SampledAt string `json:"sampled_at"`
	} `json:"pai"`
	Sleep struct {
		Main expectedSleep `json:"main"`
		Nap  expectedSleep `json:"nap"`
	} `json:"sleep"`
	Workout struct {
		SportType     int    `json:"sport_type"`
		SportName     string `json:"sport_name"`
		StartedAt     string `json:"started_at"`
		EndedAt       string `json:"ended_at"`
		DurationSec   int    `json:"duration_sec"`
		Calories      int    `json:"calories"`
		AvgHR         int    `json:"avg_hr"`
		MaxHR         int    `json:"max_hr"`
		MinHR         int    `json:"min_hr"`
		HasDetail     bool   `json:"has_detail"`
		DetailBlocks  int    `json:"detail_blocks"`
		FirstDetailAt string `json:"first_detail_at"`
		LastDetailAt  string `json:"last_detail_at"`
	} `json:"workout"`
}

func loadExpected(t testing.TB) expectedFixture {
	t.Helper()
	var expected expectedFixture
	if err := json.Unmarshal(mustReadFixture(t, "expected.json"), &expected); err != nil {
		t.Fatalf("decode expected.json: %v", err)
	}
	if len(expected.Types) != 11 {
		t.Fatalf("expected.json covers %d types, want the 11 measured codes", len(expected.Types))
	}
	return expected
}

func readZeppCSV(t testing.TB, name string) [][]string {
	t.Helper()
	path := filepath.Join(realFixtureDir, "zepp", name)
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("fixture %s is missing: %v", path, err)
	}
	defer file.Close()
	rows, err := csv.NewReader(file).ReadAll()
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if len(rows) < 2 {
		t.Fatalf("%s holds no data rows", path)
	}
	return rows
}

func zeppSeries(t testing.TB, name string) map[int64]int {
	t.Helper()
	path := filepath.Join(realFixtureDir, "zepp", name)
	rows := readZeppCSV(t, name)
	out := make(map[int64]int, len(rows)-1)
	for _, row := range rows[1:] {
		if len(row) < 3 {
			t.Fatalf("%s row has %d columns, want 3", path, len(row))
		}
		at, err := time.ParseInLocation("2006-01-02 15:04", row[0]+" "+row[1], istLocation())
		if err != nil {
			t.Fatalf("%s row %v: %v", path, row, err)
		}
		value, err := strconv.Atoi(row[2])
		if err != nil {
			t.Fatalf("%s row %v: %v", path, row, err)
		}
		out[at.Unix()] = value
	}
	return out
}

func formatFixtureTime(at time.Time) string {
	return at.UTC().Format(fixtureTimeForm)
}

type spo2Boundaries struct {
	Files []struct {
		Name    string `json:"name"`
		Bytes   int    `json:"bytes"`
		SHA256  string `json:"sha256"`
		Offsets []int  `json:"offsets"`
	} `json:"files"`
	HexCases []struct {
		Name    string `json:"name"`
		Hex     string `json:"hex"`
		Offsets []int  `json:"offsets"`
	} `json:"hex_cases"`
}

func loadSpo2Boundaries(t testing.TB) spo2Boundaries {
	t.Helper()
	path := filepath.Join(spo2FixtureDir, "boundaries.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("fixture %s is missing: %v", path, err)
	}
	var boundaries spo2Boundaries
	if err := json.Unmarshal(data, &boundaries); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	if len(boundaries.Files) == 0 || len(boundaries.HexCases) == 0 {
		t.Fatalf("%s describes no cases", path)
	}
	return boundaries
}

func (b spo2Boundaries) file(t testing.TB, name string) []byte {
	t.Helper()
	path := filepath.Join(spo2FixtureDir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("fixture %s is missing: %v", path, err)
	}
	for _, file := range b.Files {
		if file.Name != name {
			continue
		}
		if len(data) != file.Bytes {
			t.Fatalf("%s is %d bytes, boundaries.json says %d", name, len(data), file.Bytes)
		}
		if got := sha256Hex(data); got != file.SHA256 {
			t.Fatalf("%s sha256 = %s, boundaries.json says %s", name, got, file.SHA256)
		}
		return data
	}
	t.Fatalf("%s is not listed in boundaries.json", name)
	return nil
}

func (b spo2Boundaries) hexCase(t testing.TB, name string) []byte {
	t.Helper()
	for _, testCase := range b.HexCases {
		if testCase.Name == name {
			data, err := hex.DecodeString(testCase.Hex)
			if err != nil {
				t.Fatalf("case %s holds invalid hex: %v", name, err)
			}
			return data
		}
	}
	t.Fatalf("case %s is not described in boundaries.json", name)
	return nil
}

func (b spo2Boundaries) offsets(t testing.TB, name string) []int {
	t.Helper()
	for _, file := range b.Files {
		if file.Name == name {
			return file.Offsets
		}
	}
	for _, testCase := range b.HexCases {
		if testCase.Name == name {
			return testCase.Offsets
		}
	}
	return nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func segmentCatalog(code string, offsets []int) Catalog {
	if len(offsets) == 0 {
		return Catalog{}
	}
	segments := make([]RoundSegment, len(offsets))
	for i, offset := range offsets {
		segments[i] = RoundSegment{ByteOffset: offset}
	}
	return Catalog{Chunked: []CatalogEntry{{Code: code, RoundSegments: segments}}}
}
