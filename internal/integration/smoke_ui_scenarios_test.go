//go:build heliolytics_smoke

package integration

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Anshuman115/heliolytics/internal/testutil/disposable"
	"github.com/jackc/pgx/v5/pgconn"
)

// uiWindowDays is the inclusive IST window every scenario date stays inside.
const uiWindowDays = 30

// Synthetic night constants. Every baseline night repeats these inputs so the
// expected aggregates stay exact and readable.
const (
	uiBaselineBPM    = 58
	uiBaselineHRV    = 45
	uiBaselineResp   = 14
	uiBaselineTemp   = 3620
	uiNightScore     = 78
	uiNightStartHour = 2
	uiNightEndHour   = 6
	uiActivityCount  = 720
	uiWorkoutSport   = 49
	uiWorkoutName    = "High Intensity Interval Training"
)

// TestSeedUIScenarios uploads labeled synthetic scenarios over signed phone
// HTTP and asserts the API returns the expected values. It runs after
// TestSeedLocalSmoke on the same heliolytics_smoke database.
func TestSeedUIScenarios(t *testing.T) {
	config := smokeEnv(t)
	manifestPath := strings.TrimSpace(os.Getenv("HELIOLYTICS_SMOKE_MANIFEST"))
	if manifestPath == "" {
		t.Fatal("HELIOLYTICS_SMOKE_MANIFEST is required for the UI scenario seed")
	}

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
	if smokeCount(t, config.databaseURL, "sync_sessions") == 0 {
		t.Fatal("TestSeedLocalSmoke must run before TestSeedUIScenarios")
	}

	client := &smokeClient{base: config.apiURL, secret: config.secret}
	today := uiIstToday()

	// Training load needs a birth date and a sex, so the profile goes first.
	profile := []byte(`{"name":null,"birthDate":"1991-02-03","sex":"male","heightCm":176.0,"weightKg":70.0}`)
	if status, payload := client.do(t, http.MethodPut, "/api/profile", "", profile, "application/json"); status != http.StatusNoContent {
		t.Fatalf("profile status %d: %s", status, payload)
	}

	uploads := uiUploads(t, today)
	for _, upload := range uploads {
		uiSendUpload(t, client, upload)
	}
	t.Logf("seeded %d synthetic uploads", len(uploads))

	seeded := uiAssertStack(t, client, today)
	uiWriteManifest(t, manifestPath, today, len(uploads), seeded)
}

// uiIstToday is the current IST calendar date at midnight.
func uiIstToday() time.Time {
	now := time.Now().In(istZone)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, istZone)
}

// uiDate renders the YYYY-MM-DD label of a day.
func uiDate(day time.Time) string { return day.In(istZone).Format("2006-01-02") }

// uiAt returns the instant hour:minute on one IST day.
func uiAt(day time.Time, hour, minute int) time.Time {
	return day.Add(time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute)
}

// uiOffset returns today minus n IST days.
func uiOffset(today time.Time, n int) time.Time { return today.AddDate(0, 0, -n) }

// uiSleepMinute is one instant in whole minutes since the previous midnight,
// the base a 0x48 record uses.
func uiSleepMinute(sleepDay, at time.Time) int {
	base := sleepDay.AddDate(0, 0, -1)
	return int(at.Sub(base) / time.Minute)
}

type uiActivityMinute struct{ steps, bpm byte }

// uiConstantActivity returns count minutes of one repeated activity value.
func uiConstantActivity(count int, steps, bpm byte) []uiActivityMinute {
	out := make([]uiActivityMinute, count)
	for i := range out {
		out[i] = uiActivityMinute{steps: steps, bpm: bpm}
	}
	return out
}

// uiActivityBlob builds one anchored 0x01 payload from per-minute values.
func uiActivityBlob(minutes []uiActivityMinute) []byte {
	out := make([]byte, 0, len(minutes)*8)
	for _, minute := range minutes {
		out = append(out, activityMinute(minute.bpm, minute.steps)...)
	}
	return out
}

// uiTemperatureBlob builds count contiguous 0x2E minutes.
func uiTemperatureBlob(count int, hundredths int16) []byte {
	out := make([]byte, 0, count*8)
	for range count {
		out = append(out, temperatureMinute(hundredths)...)
	}
	return out
}

// uiScalarBlob builds absolute-timestamped scalar samples for 0x49 and 0x38.
func uiScalarBlob(stride, count int, start time.Time, every time.Duration, value byte) []byte {
	out := make([]byte, 0, count*stride)
	for i := range count {
		record := make([]byte, stride)
		binary.LittleEndian.PutUint32(record, uint32(start.Add(time.Duration(i)*every).Unix()))
		record[4] = 22 // IST is 22 quarter hours east of UTC.
		record[5] = value
		out = append(out, record...)
	}
	return out
}

// uiStressBlob builds count contiguous 0x13 minutes.
func uiStressBlob(count int, value byte) []byte {
	out := make([]byte, count)
	for i := range out {
		out[i] = value
	}
	return out
}

type uiSpo2Sample struct {
	at   time.Time
	pct  int
	auto bool
}

// uiSpo2Blob builds one version 2 0x25 round of absolute samples.
func uiSpo2Blob(samples []uiSpo2Sample) []byte {
	out := make([]byte, 0, 1+len(samples)*65)
	out = append(out, 0x02)
	for _, sample := range samples {
		record := make([]byte, 65)
		binary.LittleEndian.PutUint32(record, uint32(sample.at.Unix()))
		value := byte(sample.pct)
		if sample.auto {
			value |= 0x80
		}
		record[4] = value
		out = append(out, record...)
	}
	return out
}

// uiSleepStage is one half-open stage on the record minute grid. end is
// inclusive, which is how the protocol stores it.
type uiSleepStage struct {
	start int
	end   int
	kind  byte
}

// uiNap is one nap table entry with its own stages.
type uiNap struct {
	start   int
	end     int
	minutes int
	stages  []uiSleepStage
}

// uiSleepRecord renders one 594-byte 0x48 record. Naps share the record.
func uiSleepRecord(t *testing.T, sleepDay time.Time, main bool, score int,
	startMin, endMin int, rem, light, deep, wake int,
	stages []uiSleepStage, naps []uiNap) []byte {
	t.Helper()
	record := make([]byte, 594)
	midnight := time.Date(sleepDay.Year(), sleepDay.Month(), sleepDay.Day(), 0, 0, 0, 0, istZone)
	binary.LittleEndian.PutUint32(record[4:], uint32(midnight.Unix()))
	if main {
		if score <= 0 || score > 100 {
			t.Fatalf("sleep score %d is outside 1..100", score)
		}
		record[8], record[9] = 1, 1
		record[0x16] = byte(score)
		if startMin <= 0 || endMin <= startMin || endMin > 0xffff {
			t.Fatalf("sleep window %d..%d does not fit the record", startMin, endMin)
		}
		if len(stages) > 51 {
			t.Fatalf("main stage count %d exceeds 51", len(stages))
		}
		binary.LittleEndian.PutUint16(record[0x0a:], uint16(startMin))
		binary.LittleEndian.PutUint16(record[0x0c:], uint16(endMin))
		record[0x54] = byte(len(stages))
		for i, stage := range stages {
			off := 0x56 + 5*i
			binary.LittleEndian.PutUint16(record[off:], uint16(stage.start))
			binary.LittleEndian.PutUint16(record[off+2:], uint16(stage.end))
			record[off+4] = stage.kind
		}
		binary.LittleEndian.PutUint16(record[0x24a:], uint16(rem))
		binary.LittleEndian.PutUint16(record[0x24c:], uint16(light))
		binary.LittleEndian.PutUint16(record[0x24e:], uint16(deep))
		binary.LittleEndian.PutUint16(record[0x250:], uint16(wake))
	}
	if len(naps) > 10 {
		t.Fatalf("nap count %d exceeds the 10-entry table", len(naps))
	}
	stageOff := 0x155
	for i, nap := range naps {
		off := 0x18 + 6*i
		binary.LittleEndian.PutUint16(record[off:], uint16(nap.start))
		binary.LittleEndian.PutUint16(record[off+2:], uint16(nap.end))
		binary.LittleEndian.PutUint16(record[off+4:], uint16(nap.minutes))
		for _, stage := range nap.stages {
			if stageOff+5 > 0x24a {
				t.Fatalf("nap stages overrun the record")
			}
			binary.LittleEndian.PutUint16(record[stageOff:], uint16(stage.start))
			binary.LittleEndian.PutUint16(record[stageOff+2:], uint16(stage.end))
			record[stageOff+4] = stage.kind
			stageOff += 5
		}
	}
	return record
}

// uiWorkoutRecord renders one confirmed 0x05 summary with calories and HR.
func uiWorkoutRecord(t *testing.T, start time.Time, durationSec, sport,
	calories, avgHR, maxHR, minHR int) []byte {
	t.Helper()
	var buffer bytes.Buffer
	buffer.Write([]byte{0x0a, 0x03, 0x32, 0x2e, 0x31})
	writeProtoMessage(&buffer, 2, protoVarint(1, uint64(start.Unix())))
	writeProtoMessage(&buffer, 7, protoVarint(1, uint64(durationSec)))
	writeProtoVarint(&buffer, 3, uint64(sport))
	writeProtoMessage(&buffer, 16, protoVarint(1, uint64(calories)))
	heart := append(protoVarint(1, uint64(avgHR)), protoVarint(2, uint64(maxHR))...)
	heart = append(heart, protoVarint(3, uint64(minHR))...)
	writeProtoMessage(&buffer, 19, heart)
	return buffer.Bytes()
}

// uiUpload is one signed ingest request.
type uiUpload struct {
	name    string
	session string
	started time.Time
	ended   time.Time
	catalog []byte
	blobs   map[string][]byte
}

// uiSendUpload posts one upload with the smoke phone key.
func uiSendUpload(t *testing.T, client *smokeClient, upload uiUpload) {
	t.Helper()
	session := sessionPart(t, upload.session, upload.started, upload.ended)
	body, contentType := multipartBody(t, session, upload.catalog, upload.blobs)
	status, payload := client.do(t, http.MethodPost, "/api/ingest", "", body, contentType)
	if status != http.StatusOK {
		t.Fatalf("%s ingest status %d: %s", upload.name, status, payload)
	}
	var result struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("%s decode: %v", upload.name, err)
	}
	if result.SessionID != upload.session {
		t.Fatalf("%s session id = %q, want %q", upload.name, result.SessionID, upload.session)
	}
}

// uiNightUpload builds one complete baseline night: a four hour main sleep
// with stages, HRV, respiration, temperature and twelve hours of steps and HR.
func uiNightUpload(t *testing.T, name string, day time.Time, score int) uiUpload {
	t.Helper()
	sleepStart := uiAt(day, uiNightStartHour, 0)
	sleepEnd := uiAt(day, uiNightEndHour, 0)
	base := uiSleepMinute(day, sleepStart)
	end := uiSleepMinute(day, sleepEnd)
	stages := []uiSleepStage{
		{start: base, end: base + 59, kind: 4},
		{start: base + 60, end: base + 119, kind: 5},
		{start: base + 120, end: base + 179, kind: 8},
		{start: base + 180, end: base + 209, kind: 4},
		{start: base + 210, end: end - 1, kind: 7},
	}
	record := uiSleepRecord(t, day, true, score, base, end, 60, 90, 60, 30, stages, nil)
	return uiUpload{
		name:    name,
		session: "ui-" + name + "-" + uiDate(day),
		started: uiAt(day, 0, 0),
		ended:   uiAt(day, 12, 5),
		catalog: catalogPart(t,
			catalogEntry{Code: "0x01", RoundStart: istWallClock(uiAt(day, 0, 0))},
			catalogEntry{Code: "0x2E", RoundStart: istWallClock(sleepStart)}),
		blobs: map[string][]byte{
			"0x01": uiActivityBlob(uiConstantActivity(uiActivityCount, 0, uiBaselineBPM)),
			"0x2E": uiTemperatureBlob(240, uiBaselineTemp),
			"0x49": uiScalarBlob(6, 120, sleepStart, 2*time.Minute, uiBaselineHRV),
			"0x38": uiScalarBlob(8, 60, sleepStart, 4*time.Minute, uiBaselineResp),
			"0x48": record,
		},
	}
}

// uiUploads builds every scenario upload oldest first. Readiness depends on
// older nights, so the order keeps each recompute consistent.
func uiUploads(t *testing.T, today time.Time) []uiUpload {
	t.Helper()
	off := func(n int) time.Time { return uiOffset(today, n) }
	out := make([]uiUpload, 0, uiWindowDays)

	// vitals-no-spo2: stress plus a few steps and no optical samples.
	stressed := off(29)
	out = append(out, uiUpload{
		name:    "vitals-no-spo2",
		session: "ui-vitals-no-spo2-" + uiDate(stressed),
		started: uiAt(stressed, 9, 0),
		ended:   uiAt(stressed, 10, 15),
		catalog: catalogPart(t,
			catalogEntry{Code: "0x13", RoundStart: istWallClock(uiAt(stressed, 9, 0))},
			catalogEntry{Code: "0x01", RoundStart: istWallClock(uiAt(stressed, 10, 0))}),
		blobs: map[string][]byte{
			"0x13": uiStressBlob(20, 30),
			"0x01": uiActivityBlob(uiConstantActivity(10, 2, 65)),
		},
	})

	// vitals-real-spo2, second day: three samples of a real 100.
	perfect := off(28)
	out = append(out, uiUpload{
		name:    "vitals-real-spo2-100",
		session: "ui-vitals-real-spo2-100-" + uiDate(perfect),
		started: uiAt(perfect, 8, 55),
		ended:   uiAt(perfect, 9, 10),
		catalog: catalogPart(t),
		blobs: map[string][]byte{
			"0x25": uiSpo2Blob([]uiSpo2Sample{
				{at: uiAt(perfect, 9, 0), pct: 100, auto: true},
				{at: uiAt(perfect, 9, 1), pct: 100, auto: true},
				{at: uiAt(perfect, 9, 2), pct: 100, auto: true},
			}),
		},
	})

	// vitals-real-spo2, first day: mean 97.33, minimum 95, three samples.
	sparse := off(27)
	out = append(out, uiUpload{
		name:    "vitals-real-spo2-mean",
		session: "ui-vitals-real-spo2-mean-" + uiDate(sparse),
		started: uiAt(sparse, 7, 55),
		ended:   uiAt(sparse, 8, 10),
		catalog: catalogPart(t),
		blobs: map[string][]byte{
			"0x25": uiSpo2Blob([]uiSpo2Sample{
				{at: uiAt(sparse, 8, 0), pct: 95, auto: true},
				{at: uiAt(sparse, 8, 1), pct: 98, auto: true},
				{at: uiAt(sparse, 8, 2), pct: 99, auto: true},
			}),
		},
	})

	// readiness-missing no_data: a supported day with steps and no HRV night.
	missing := off(26)
	out = append(out, uiUpload{
		name:    "readiness-missing-no-data",
		session: "ui-readiness-missing-no-data-" + uiDate(missing),
		started: uiAt(missing, 7, 55),
		ended:   uiAt(missing, 8, 25),
		catalog: catalogPart(t,
			catalogEntry{Code: "0x01", RoundStart: istWallClock(uiAt(missing, 8, 0))}),
		blobs: map[string][]byte{
			"0x01": uiActivityBlob(uiConstantActivity(20, 5, 70)),
		},
	})

	// readiness-missing no_baseline: a full HRV night with no earlier night.
	out = append(out, uiNightUpload(t, "readiness-missing-no-baseline", off(25), uiNightScore))
	// Offset 24 stays empty on purpose.

	// readiness-calibrating priors, then its target night.
	out = append(out, uiNightUpload(t, "readiness-baseline", off(23), uiNightScore))
	out = append(out, uiNightUpload(t, "readiness-baseline", off(22), uiNightScore))
	out = append(out, uiNightUpload(t, "readiness-baseline", off(21), uiNightScore))
	out = append(out, uiNightUpload(t, "readiness-calibrating", off(20), uiNightScore))

	// workout-and-auto: a threshold auto session plus one recorded workout.
	active := off(19)
	minutes := make([]uiActivityMinute, 0, 31)
	minutes = append(minutes, uiConstantActivity(10, 0, 70)...)
	minutes = append(minutes, uiConstantActivity(15, 100, 110)...)
	minutes = append(minutes, uiConstantActivity(6, 0, 70)...)
	out = append(out, uiUpload{
		name:    "workout-and-auto",
		session: "ui-workout-and-auto-" + uiDate(active),
		started: uiAt(active, 9, 45),
		ended:   uiAt(active, 17, 50),
		catalog: catalogPart(t,
			catalogEntry{Code: "0x01", RoundStart: istWallClock(uiAt(active, 9, 50))}),
		blobs: map[string][]byte{
			"0x01": uiActivityBlob(minutes),
			"0x05": uiWorkoutRecord(t, uiAt(active, 17, 0), 2400, uiWorkoutSport, 320, 130, 152, 96),
		},
	})

	// split-night: two disjoint non-nap sessions on one day.
	split := off(18)
	first := uiSleepRecord(t, split, true, 80, 1260, 1500, 0, 180, 60, 0, []uiSleepStage{
		{start: 1260, end: 1319, kind: 4},
		{start: 1320, end: 1379, kind: 5},
		{start: 1380, end: 1499, kind: 4},
	}, nil)
	second := uiSleepRecord(t, split, true, 76, 1560, 1800, 60, 100, 60, 20, []uiSleepStage{
		{start: 1560, end: 1619, kind: 5},
		{start: 1620, end: 1679, kind: 8},
		{start: 1680, end: 1779, kind: 4},
		{start: 1780, end: 1799, kind: 7},
	}, nil)
	out = append(out, uiUpload{
		name:    "split-night",
		session: "ui-split-night-" + uiDate(split),
		started: uiAt(split, 0, 0),
		ended:   uiAt(split, 6, 30),
		catalog: catalogPart(t),
		blobs:   map[string][]byte{"0x48": append(first, second...)},
	})

	// nap-only: no main sleep, one afternoon nap.
	napOnly := off(17)
	napRecord := uiSleepRecord(t, napOnly, false, 0, 0, 0, 0, 0, 0, 0, nil, []uiNap{
		{start: 2220, end: 2264, minutes: 45, stages: []uiSleepStage{
			{start: 2220, end: 2239, kind: 4},
			{start: 2240, end: 2264, kind: 5},
		}},
	})
	out = append(out, uiUpload{
		name:    "nap-only",
		session: "ui-nap-only-" + uiDate(napOnly),
		started: uiAt(napOnly, 12, 0),
		ended:   uiAt(napOnly, 14, 0),
		catalog: catalogPart(t),
		blobs:   map[string][]byte{"0x48": napRecord},
	})

	// main-and-nap: a truncated main sleep plus one complete nap.
	napDay := off(16)
	napStage := uiSleepMinute(napDay, uiAt(napDay, 14, 0))
	mainNap := uiSleepRecord(t, napDay, true, 84, 1380, 1800, 120, 180, 90, 30, []uiSleepStage{
		{start: 1380, end: 1499, kind: 4},
		{start: 1500, end: 1559, kind: 5},
	}, []uiNap{
		{start: napStage, end: napStage + 44, minutes: 45, stages: []uiSleepStage{
			{start: napStage, end: napStage + 19, kind: 4},
			{start: napStage + 20, end: napStage + 44, kind: 5},
		}},
	})
	out = append(out, uiUpload{
		name:    "main-and-nap",
		session: "ui-main-and-nap-" + uiDate(napDay),
		started: uiAt(napDay, 0, 0),
		ended:   uiAt(napDay, 15, 0),
		catalog: catalogPart(t),
		blobs:   map[string][]byte{"0x48": mainNap},
	})

	// Baseline nights, oldest first, so the scored target sees all of them.
	for offset := 15; offset >= 2; offset-- {
		out = append(out, uiNightUpload(t, "readiness-baseline", off(offset), uiNightScore))
	}
	out = append(out, uiNightUpload(t, "readiness-scored", off(1), uiNightScore))
	return out
}

// uiDayRow is the daily-metrics day shape this test asserts.
type uiDayRow struct {
	DayKey              string          `json:"dayKey"`
	Steps               *int            `json:"steps"`
	AvgHr               *float64        `json:"avgHr"`
	RestingHr           *float64        `json:"restingHr"`
	HrvRmssd            *float64        `json:"hrvRmssd"`
	RespRate            *float64        `json:"respRate"`
	Spo2Avg             *float64        `json:"spo2Avg"`
	Spo2Min             *int            `json:"spo2Min"`
	Spo2Samples         int             `json:"spo2Samples"`
	StressAvg           *float64        `json:"stressAvg"`
	SkinTempNightC      *float64        `json:"skinTempNightC"`
	SleepScore          *int            `json:"sleepScore"`
	SleepMins           *int            `json:"sleepMins"`
	SleepDeepMins       *int            `json:"sleepDeepMins"`
	SleepRemMins        *int            `json:"sleepRemMins"`
	SleepLightMins      *int            `json:"sleepLightMins"`
	SleepWakeMins       *int            `json:"sleepWakeMins"`
	TimeInBedMins       *int            `json:"timeInBedMins"`
	SleepEfficiencyPct  *int            `json:"sleepEfficiencyPct"`
	NapCount            int             `json:"napCount"`
	WorkoutCount        int             `json:"workoutCount"`
	AutoActivityCount   int             `json:"autoActivityCount"`
	WorkoutCalories     *int            `json:"workoutCalories"`
	TrainingLoad        *float64        `json:"trainingLoad"`
	Readiness           *int            `json:"readiness"`
	ReadinessStatus     *string         `json:"readinessStatus"`
	ReadinessBand       *string         `json:"readinessBand"`
	ReadinessComponents json.RawMessage `json:"readinessComponents"`
}

// uiSleepSession is one stored sleep session in the sleep route.
type uiSleepSession struct {
	StartedAt       string `json:"startedAt"`
	EndedAt         string `json:"endedAt"`
	Score           *int   `json:"score"`
	TotalMins       int    `json:"totalMins"`
	DeepMins        int    `json:"deepMins"`
	RemMins         int    `json:"remMins"`
	LightMins       int    `json:"lightMins"`
	WakeMins        int    `json:"wakeMins"`
	StagesTruncated bool   `json:"stagesTruncated"`
	Stages          []struct {
		Start string `json:"start"`
		End   string `json:"end"`
		Type  int    `json:"type"`
	} `json:"stages"`
}

// uiSleepDay is one day of the sleep route.
type uiSleepDay struct {
	DayKey string           `json:"dayKey"`
	Main   *uiSleepSession  `json:"main"`
	Naps   []uiSleepSession `json:"naps"`
}

// uiActivityRow is one unified activity row.
type uiActivityRow struct {
	Kind        string `json:"kind"`
	DayKey      string `json:"dayKey"`
	StartedAt   string `json:"startedAt"`
	EndedAt     string `json:"endedAt"`
	DurationSec int    `json:"durationSec"`
	SportType   int    `json:"sportType"`
	SportName   string `json:"sportName"`
	Calories    *int   `json:"calories"`
	AvgHr       *int   `json:"avgHr"`
	MaxHr       *int   `json:"maxHr"`
	MinHr       *int   `json:"minHr"`
	Steps       *int   `json:"steps"`
	HasDetail   bool   `json:"hasDetail"`
}

// uiSeriesDay is one compact metric series day.
type uiSeriesDay struct {
	Metric    string    `json:"metric"`
	DayKey    string    `json:"dayKey"`
	StartTime string    `json:"startTime"`
	Offsets   []int32   `json:"offsets"`
	Values    []float64 `json:"values"`
}

// uiAssertStack reads every seeded route, compares the expected values and
// returns every supported day the daily route lists.
func uiAssertStack(t *testing.T, client *smokeClient, today time.Time) []string {
	t.Helper()
	from := uiDate(uiOffset(today, uiWindowDays-1))
	to := uiDate(today)
	days := uiDailyRows(t, client, from, to)

	nightOffsets := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 20, 21, 22, 23, 25}
	for _, offset := range nightOffsets {
		label := uiDate(uiOffset(today, offset))
		row, ok := days[label]
		if !ok {
			t.Fatalf("daily-metrics has no row for the seeded night %s", label)
		}
		uiAssertBaselineNight(t, label, row)
	}

	// readiness-scored: at least fourteen prior HRV nights and a score.
	scoredDay := uiDate(uiOffset(today, 1))
	scored, ok := days[scoredDay]
	if !ok {
		t.Fatalf("daily-metrics has no row for %s", scoredDay)
	}
	uiWantString(t, scoredDay+" readinessStatus", scored.ReadinessStatus, "scored")
	uiWantInt(t, scoredDay+" readinessBaselineNights", uiBaselineNights(t, scoredDay, scored.ReadinessComponents), 19)
	if scored.Readiness == nil {
		t.Fatalf("%s readiness is null", scoredDay)
	}
	uiWantString(t, scoredDay+" readinessBand", scored.ReadinessBand, uiBandOf(*scored.Readiness))
	uiAssertScoredComponents(t, scoredDay, scored.ReadinessComponents)

	// readiness-calibrating: four prior HRV nights and no score.
	calibratingDay := uiDate(uiOffset(today, 20))
	calibrating, ok := days[calibratingDay]
	if !ok {
		t.Fatalf("daily-metrics has no row for %s", calibratingDay)
	}
	uiWantString(t, calibratingDay+" readinessStatus", calibrating.ReadinessStatus, "calibrating")
	uiWantInt(t, calibratingDay+" readinessBaselineNights", uiBaselineNights(t, calibratingDay, calibrating.ReadinessComponents), 4)
	uiWantNullInt(t, calibratingDay+" readiness", calibrating.Readiness)
	uiWantNullString(t, calibratingDay+" readinessBand", calibrating.ReadinessBand)

	// readiness-missing no_baseline: HRV without an earlier night.
	noBaselineDay := uiDate(uiOffset(today, 25))
	noBaseline, ok := days[noBaselineDay]
	if !ok {
		t.Fatalf("daily-metrics has no row for %s", noBaselineDay)
	}
	uiWantString(t, noBaselineDay+" readinessStatus", noBaseline.ReadinessStatus, "no_baseline")
	uiWantInt(t, noBaselineDay+" readinessBaselineNights", uiBaselineNights(t, noBaselineDay, noBaseline.ReadinessComponents), 0)
	uiWantNullInt(t, noBaselineDay+" readiness", noBaseline.Readiness)

	// readiness-missing no_data: a supported day with no HRV night.
	noDataDay := uiDate(uiOffset(today, 26))
	noData, ok := days[noDataDay]
	if !ok {
		t.Fatalf("daily-metrics has no row for %s", noDataDay)
	}
	uiWantString(t, noDataDay+" readinessStatus", noData.ReadinessStatus, "no_data")
	uiWantInt(t, noDataDay+" readinessBaselineNights", uiBaselineNights(t, noDataDay, noData.ReadinessComponents), 0)
	uiWantNullInt(t, noDataDay+" readiness", noData.Readiness)
	uiWantInt(t, noDataDay+" steps", noData.Steps, 100)
	uiWantFloat(t, noDataDay+" avgHr", noData.AvgHr, 70)
	uiWantNullInt(t, noDataDay+" sleepMins", noData.SleepMins)
	uiWantNullFloat(t, noDataDay+" spo2Avg", noData.Spo2Avg)

	// vitals-no-spo2: stress present, optical samples absent.
	stressDay := uiDate(uiOffset(today, 29))
	stress, ok := days[stressDay]
	if !ok {
		t.Fatalf("daily-metrics has no row for %s", stressDay)
	}
	uiWantFloat(t, stressDay+" stressAvg", stress.StressAvg, 30)
	uiWantInt(t, stressDay+" steps", stress.Steps, 20)
	uiWantFloat(t, stressDay+" avgHr", stress.AvgHr, 65)
	uiWantNullFloat(t, stressDay+" spo2Avg", stress.Spo2Avg)
	uiWantNullInt(t, stressDay+" spo2Min", stress.Spo2Min)
	uiWantCount(t, stressDay+" spo2Samples", stress.Spo2Samples, 0)

	// vitals-real-spo2: mean 97.33 rounds to 97.3 with minimum 95.
	meanDay := uiDate(uiOffset(today, 27))
	mean, ok := days[meanDay]
	if !ok {
		t.Fatalf("daily-metrics has no row for %s", meanDay)
	}
	uiWantFloat(t, meanDay+" spo2Avg", mean.Spo2Avg, 97.3)
	uiWantInt(t, meanDay+" spo2Min", mean.Spo2Min, 95)
	uiWantCount(t, meanDay+" spo2Samples", mean.Spo2Samples, 3)
	uiWantNullInt(t, meanDay+" steps", mean.Steps)

	// A real 100 stays 100 and is never read as a placeholder.
	perfectDay := uiDate(uiOffset(today, 28))
	perfect, ok := days[perfectDay]
	if !ok {
		t.Fatalf("daily-metrics has no row for %s", perfectDay)
	}
	uiWantFloat(t, perfectDay+" spo2Avg", perfect.Spo2Avg, 100)
	uiWantInt(t, perfectDay+" spo2Min", perfect.Spo2Min, 100)
	uiWantCount(t, perfectDay+" spo2Samples", perfect.Spo2Samples, 3)

	// main-and-nap: truncated main stages plus one complete nap.
	uiAssertMainAndNap(t, days[uiDate(uiOffset(today, 16))], uiDate(uiOffset(today, 16)))
	// nap-only: a null main with one nap.
	uiAssertNapOnly(t, days[uiDate(uiOffset(today, 17))], uiDate(uiOffset(today, 17)))
	// split-night: both sessions counted, the longest reported as main.
	uiAssertSplitNight(t, days[uiDate(uiOffset(today, 18))], uiDate(uiOffset(today, 18)))

	// workout-and-auto: one threshold auto session and one recorded workout.
	uiAssertActivities(t, client, uiDate(uiOffset(today, 19)))
	uiAssertSeries(t, client, today, from, to)

	// empty: a window date with no data returns an empty list.
	emptyDay := uiDate(uiOffset(today, 24))
	if _, present := days[emptyDay]; present {
		t.Fatalf("daily-metrics lists %s, which the seed never writes", emptyDay)
	}
	status, payload := client.do(t, http.MethodGet, "/api/daily-metrics",
		"from="+emptyDay+"&to="+emptyDay, nil, "")
	if status != http.StatusOK {
		t.Fatalf("empty day status %d: %s", status, payload)
	}
	var empty struct {
		Days []uiDayRow `json:"days"`
	}
	if err := json.Unmarshal(payload, &empty); err != nil {
		t.Fatalf("decode empty day: %v", err)
	}
	if empty.Days == nil || len(empty.Days) != 0 {
		t.Fatalf("empty day response = %s, want []", payload)
	}

	uiAssertSleepRoute(t, client, today)

	supported := make([]string, 0, len(days))
	for day := range days {
		supported = append(supported, day)
	}
	sort.Strings(supported)
	return supported
}

// uiAssertBaselineNight checks the shared baseline night values.
func uiAssertBaselineNight(t *testing.T, label string, row uiDayRow) {
	t.Helper()
	uiWantInt(t, label+" steps", row.Steps, 0)
	uiWantFloat(t, label+" avgHr", row.AvgHr, uiBaselineBPM)
	uiWantFloat(t, label+" restingHr", row.RestingHr, uiBaselineBPM)
	uiWantFloat(t, label+" hrvRmssd", row.HrvRmssd, uiBaselineHRV)
	uiWantFloat(t, label+" respRate", row.RespRate, uiBaselineResp)
	uiWantFloat(t, label+" skinTempNightC", row.SkinTempNightC, float64(uiBaselineTemp)/100)
	uiWantNullFloat(t, label+" spo2Avg", row.Spo2Avg)
	uiWantNullFloat(t, label+" stressAvg", row.StressAvg)
	uiWantInt(t, label+" sleepScore", row.SleepScore, uiNightScore)
	uiWantInt(t, label+" sleepMins", row.SleepMins, 210)
	uiWantInt(t, label+" sleepDeepMins", row.SleepDeepMins, 60)
	uiWantInt(t, label+" sleepRemMins", row.SleepRemMins, 60)
	uiWantInt(t, label+" sleepLightMins", row.SleepLightMins, 90)
	uiWantInt(t, label+" sleepWakeMins", row.SleepWakeMins, 30)
	uiWantInt(t, label+" timeInBedMins", row.TimeInBedMins, 240)
	uiWantInt(t, label+" sleepEfficiencyPct", row.SleepEfficiencyPct, 88)
	uiWantCount(t, label+" napCount", row.NapCount, 0)
	uiWantCount(t, label+" workoutCount", row.WorkoutCount, 0)
	uiWantCount(t, label+" autoActivityCount", row.AutoActivityCount, 0)
	uiWantFloat(t, label+" trainingLoad", row.TrainingLoad, 0)
}

// uiAssertMainAndNap checks the truncated main sleep and its nap.
func uiAssertMainAndNap(t *testing.T, row uiDayRow, label string) {
	t.Helper()
	uiWantInt(t, label+" sleepScore", row.SleepScore, 84)
	uiWantInt(t, label+" sleepMins", row.SleepMins, 390)
	uiWantInt(t, label+" sleepDeepMins", row.SleepDeepMins, 90)
	uiWantInt(t, label+" sleepRemMins", row.SleepRemMins, 120)
	uiWantInt(t, label+" sleepLightMins", row.SleepLightMins, 180)
	uiWantInt(t, label+" sleepWakeMins", row.SleepWakeMins, 30)
	uiWantInt(t, label+" timeInBedMins", row.TimeInBedMins, 420)
	uiWantInt(t, label+" sleepEfficiencyPct", row.SleepEfficiencyPct, 93)
	uiWantCount(t, label+" napCount", row.NapCount, 1)
}

// uiAssertNapOnly checks a day whose sleep is one nap and no main session.
func uiAssertNapOnly(t *testing.T, row uiDayRow, label string) {
	t.Helper()
	uiWantCount(t, label+" napCount", row.NapCount, 1)
	uiWantNullInt(t, label+" sleepScore", row.SleepScore)
	uiWantNullInt(t, label+" sleepMins", row.SleepMins)
	uiWantNullInt(t, label+" timeInBedMins", row.TimeInBedMins)
	uiWantNullInt(t, label+" sleepEfficiencyPct", row.SleepEfficiencyPct)
}

// uiAssertSplitNight checks both non-nap sessions of one night.
func uiAssertSplitNight(t *testing.T, row uiDayRow, label string) {
	t.Helper()
	uiWantInt(t, label+" sleepMins", row.SleepMins, 460)
	uiWantInt(t, label+" sleepDeepMins", row.SleepDeepMins, 120)
	uiWantInt(t, label+" sleepRemMins", row.SleepRemMins, 60)
	uiWantInt(t, label+" sleepLightMins", row.SleepLightMins, 280)
	uiWantInt(t, label+" sleepWakeMins", row.SleepWakeMins, 20)
	uiWantInt(t, label+" timeInBedMins", row.TimeInBedMins, 480)
	uiWantInt(t, label+" sleepEfficiencyPct", row.SleepEfficiencyPct, 96)
	uiWantInt(t, label+" sleepScore", row.SleepScore, 80)
	uiWantCount(t, label+" napCount", row.NapCount, 0)
}

// uiAssertScoredComponents checks the raw inputs the score was built from.
func uiAssertScoredComponents(t *testing.T, label string, raw json.RawMessage) {
	t.Helper()
	var document struct {
		Status         string `json:"status"`
		BaselineNights int    `json:"baselineNights"`
		Components     []struct {
			Name  string   `json:"name"`
			Value *float64 `json:"value"`
			N     *int     `json:"n"`
		} `json:"components"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("%s readinessComponents: %v", label, err)
	}
	if document.Status != "scored" {
		t.Fatalf("%s component status = %q", label, document.Status)
	}
	if document.BaselineNights != 19 {
		t.Fatalf("%s component baselineNights = %d, want 19", label, document.BaselineNights)
	}
	wanted := map[string]float64{
		"hrv":         uiBaselineHRV,
		"sleeping_hr": uiBaselineBPM,
		"sleep":       uiNightScore,
		"skin_temp":   float64(uiBaselineTemp) / 100,
		"resp_rate":   uiBaselineResp,
	}
	seen := make(map[string]bool, len(wanted))
	for _, component := range document.Components {
		want, ok := wanted[component.Name]
		if !ok {
			continue
		}
		seen[component.Name] = true
		if component.Value == nil {
			t.Fatalf("%s component %s value is null", label, component.Name)
		}
		uiWantFloat(t, label+" component "+component.Name, component.Value, want)
	}
	for name := range wanted {
		if !seen[name] {
			t.Fatalf("%s readinessComponents is missing %s", label, name)
		}
	}
}

// uiBaselineNights reads baselineNights from the stored explanation.
func uiBaselineNights(t *testing.T, label string, raw json.RawMessage) *int {
	t.Helper()
	var document struct {
		BaselineNights *int `json:"baselineNights"`
	}
	if err := json.Unmarshal(raw, &document); err != nil || document.BaselineNights == nil {
		t.Fatalf("%s readinessComponents: %s", label, raw)
	}
	return document.BaselineNights
}

// uiBandOf mirrors the documented score bands, never a stored label.
func uiBandOf(score int) string {
	switch {
	case score >= 67:
		return "good"
	case score >= 34:
		return "fair"
	default:
		return "low"
	}
}

// uiDailyRows reads the daily route into a map keyed by day.
func uiDailyRows(t *testing.T, client *smokeClient, from, to string) map[string]uiDayRow {
	t.Helper()
	status, payload := client.do(t, http.MethodGet, "/api/daily-metrics",
		"from="+from+"&to="+to, nil, "")
	if status != http.StatusOK {
		t.Fatalf("daily metrics status %d: %s", status, payload)
	}
	var body struct {
		Days []uiDayRow `json:"days"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		t.Fatalf("decode daily metrics: %v", err)
	}
	out := make(map[string]uiDayRow, len(body.Days))
	for _, row := range body.Days {
		out[row.DayKey] = row
	}
	return out
}

// uiAssertSleepRoute checks the stored sleep sessions by day.
func uiAssertSleepRoute(t *testing.T, client *smokeClient, today time.Time) {
	t.Helper()
	from := uiDate(uiOffset(today, 25))
	to := uiDate(uiOffset(today, 1))
	status, payload := client.do(t, http.MethodGet, "/api/sleep",
		"from="+from+"&to="+to, nil, "")
	if status != http.StatusOK {
		t.Fatalf("sleep status %d: %s", status, payload)
	}
	var body struct {
		Days []uiSleepDay `json:"days"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		t.Fatalf("decode sleep: %v", err)
	}
	byDay := make(map[string]uiSleepDay, len(body.Days))
	for _, day := range body.Days {
		byDay[day.DayKey] = day
	}

	baseline := byDay[uiDate(uiOffset(today, 5))]
	if baseline.Main == nil {
		t.Fatalf("sleep has no main session for the baseline night")
	}
	uiWantCount(t, "baseline main totalMins", baseline.Main.TotalMins, 210)
	uiWantCount(t, "baseline main stages", len(baseline.Main.Stages), 5)
	if baseline.Main.StagesTruncated {
		t.Fatalf("baseline main stages report truncation")
	}
	if len(baseline.Naps) != 0 {
		t.Fatalf("baseline day naps = %d, want 0", len(baseline.Naps))
	}
	uiWantInt(t, "baseline main score", baseline.Main.Score, uiNightScore)

	mainNap := byDay[uiDate(uiOffset(today, 16))]
	if mainNap.Main == nil || len(mainNap.Naps) != 1 {
		t.Fatalf("main-and-nap sleep day = %+v", mainNap)
	}
	uiWantCount(t, "main-and-nap main totalMins", mainNap.Main.TotalMins, 390)
	if !mainNap.Main.StagesTruncated {
		t.Fatalf("main-and-nap main stages do not report truncation")
	}
	uiWantCount(t, "main-and-nap main stages", len(mainNap.Main.Stages), 2)
	uiWantCount(t, "main-and-nap nap totalMins", mainNap.Naps[0].TotalMins, 45)
	uiWantCount(t, "main-and-nap nap lightMins", mainNap.Naps[0].LightMins, 20)
	uiWantCount(t, "main-and-nap nap deepMins", mainNap.Naps[0].DeepMins, 25)
	if mainNap.Naps[0].StagesTruncated {
		t.Fatalf("main-and-nap nap stages report truncation")
	}
	if mainNap.Naps[0].Score != nil {
		t.Fatalf("main-and-nap nap exposes a score")
	}

	napOnly := byDay[uiDate(uiOffset(today, 17))]
	if napOnly.Main != nil {
		t.Fatalf("nap-only day reports a main session")
	}
	if len(napOnly.Naps) != 1 {
		t.Fatalf("nap-only day naps = %d, want 1", len(napOnly.Naps))
	}
	uiWantCount(t, "nap-only nap totalMins", napOnly.Naps[0].TotalMins, 45)

	split := byDay[uiDate(uiOffset(today, 18))]
	if split.Main == nil {
		t.Fatalf("split-night day reports no main session")
	}
	uiWantCount(t, "split-night main totalMins", split.Main.TotalMins, 240)
	if split.Main.StagesTruncated {
		t.Fatalf("split-night main stages report truncation")
	}
}

// uiAssertActivities checks the recorded workout and the auto session.
func uiAssertActivities(t *testing.T, client *smokeClient, day string) {
	t.Helper()
	status, payload := client.do(t, http.MethodGet, "/api/activities",
		"from="+day+"&to="+day, nil, "")
	if status != http.StatusOK {
		t.Fatalf("activities status %d: %s", status, payload)
	}
	var body struct {
		Activities []uiActivityRow `json:"activities"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		t.Fatalf("decode activities: %v", err)
	}
	if len(body.Activities) != 2 {
		t.Fatalf("%s activities = %d, want 2", day, len(body.Activities))
	}
	auto := body.Activities[0]
	if auto.Kind != "auto" {
		t.Fatalf("%s first activity kind = %q, want auto", day, auto.Kind)
	}
	if auto.DayKey != day {
		t.Fatalf("%s auto dayKey = %q", day, auto.DayKey)
	}
	if auto.SportName != "Walk" {
		t.Fatalf("%s auto label = %q, want Walk", day, auto.SportName)
	}
	if auto.DurationSec != 900 {
		t.Fatalf("%s auto durationSec = %d, want 900", day, auto.DurationSec)
	}
	uiWantInt(t, day+" auto steps", auto.Steps, 1500)
	uiWantInt(t, day+" auto avgHr", auto.AvgHr, 110)
	uiWantInt(t, day+" auto maxHr", auto.MaxHr, 110)
	uiWantInt(t, day+" auto minHr", auto.MinHr, 110)
	uiWantNullInt(t, day+" auto calories", auto.Calories)

	workout := body.Activities[1]
	if workout.Kind != "workout" {
		t.Fatalf("%s second activity kind = %q, want workout", day, workout.Kind)
	}
	if workout.SportType != uiWorkoutSport || workout.SportName != uiWorkoutName {
		t.Fatalf("%s workout sport = %d %q", day, workout.SportType, workout.SportName)
	}
	uiWantInt(t, day+" workout calories", workout.Calories, 320)
	uiWantInt(t, day+" workout avgHr", workout.AvgHr, 130)
	uiWantInt(t, day+" workout maxHr", workout.MaxHr, 152)
	uiWantInt(t, day+" workout minHr", workout.MinHr, 96)
	if workout.DurationSec != 2400 {
		t.Fatalf("%s workout durationSec = %d, want 2400", day, workout.DurationSec)
	}
	uiWantNullInt(t, day+" workout steps", workout.Steps)
}

// uiAssertSeries checks the compact series for HRV, SpO2 and stress.
func uiAssertSeries(t *testing.T, client *smokeClient, today time.Time, from, to string) {
	t.Helper()
	status, payload := client.do(t, http.MethodGet, "/api/metrics/series",
		"from="+from+"&to="+to, nil, "")
	if status != http.StatusOK {
		t.Fatalf("series status %d: %s", status, payload)
	}
	var body struct {
		Days []uiSeriesDay `json:"days"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		t.Fatalf("decode series: %v", err)
	}
	series := make(map[string]uiSeriesDay, len(body.Days))
	for _, day := range body.Days {
		series[day.Metric+"/"+day.DayKey] = day
	}

	night := series["hrv/"+uiDate(uiOffset(today, 5))]
	uiWantCount(t, "baseline hrv series values", len(night.Values), 120)
	uiWantCount(t, "baseline hrv series offsets", len(night.Offsets), 120)
	if night.Offsets[0] != 0 || night.Offsets[1] != 120 {
		t.Fatalf("baseline hrv offsets start = %v", night.Offsets[:2])
	}
	uiWantFloat(t, "baseline hrv first value", &night.Values[0], uiBaselineHRV)

	mean := series["spo2/"+uiDate(uiOffset(today, 27))]
	uiWantCount(t, "spo2 mean series values", len(mean.Values), 3)
	uiWantFloat(t, "spo2 mean first value", &mean.Values[0], 95)
	uiWantFloat(t, "spo2 mean last value", &mean.Values[2], 99)

	perfect := series["spo2/"+uiDate(uiOffset(today, 28))]
	uiWantCount(t, "spo2 100 series values", len(perfect.Values), 3)
	uiWantFloat(t, "spo2 100 first value", &perfect.Values[0], 100)

	stress := series["stress/"+uiDate(uiOffset(today, 29))]
	uiWantCount(t, "stress series values", len(stress.Values), 20)
	uiWantFloat(t, "stress first value", &stress.Values[0], 30)
	if _, present := series["spo2/"+uiDate(uiOffset(today, 29))]; present {
		t.Fatal("vitals-no-spo2 day carries a spo2 series")
	}
}

func uiWantInt(t *testing.T, label string, got *int, want int) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s is null, want %d", label, want)
	}
	if *got != want {
		t.Fatalf("%s = %d, want %d", label, *got, want)
	}
}

func uiWantNullInt(t *testing.T, label string, got *int) {
	t.Helper()
	if got != nil {
		t.Fatalf("%s = %d, want null", label, *got)
	}
}

func uiWantCount(t *testing.T, label string, got, want int) {
	t.Helper()
	if got != want {
		t.Fatalf("%s = %d, want %d", label, got, want)
	}
}

func uiWantFloat(t *testing.T, label string, got *float64, want float64) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s is null, want %v", label, want)
	}
	diff := *got - want
	if diff < 0 {
		diff = -diff
	}
	if diff > 1e-6 {
		t.Fatalf("%s = %v, want %v", label, *got, want)
	}
}

func uiWantNullFloat(t *testing.T, label string, got *float64) {
	t.Helper()
	if got != nil {
		t.Fatalf("%s = %v, want null", label, *got)
	}
}

func uiWantString(t *testing.T, label string, got *string, want string) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s is null, want %q", label, want)
	}
	if *got != want {
		t.Fatalf("%s = %q, want %q", label, *got, want)
	}
}

func uiWantNullString(t *testing.T, label string, got *string) {
	t.Helper()
	if got != nil {
		t.Fatalf("%s = %q, want null", label, *got)
	}
}

// uiManifestScenario is one scenario entry for the browser worker.
type uiManifestScenario struct {
	Date     string         `json:"date"`
	Expected map[string]any `json:"expected"`
}

// uiManifest is the file the browser worker reads.
type uiManifest struct {
	GeneratedAt    string                        `json:"generatedAt"`
	Provenance     string                        `json:"provenance"`
	IstDay         string                        `json:"istDay"`
	Window         []string                      `json:"window"`
	Uploads        int                           `json:"uploads"`
	Scenarios      map[string]uiManifestScenario `json:"scenarios"`
	Unreachable    map[string]string             `json:"unreachable"`
	EmptyDate      string                        `json:"emptyDate"`
	SupportNights  []string                      `json:"supportNights"`
	SupportedDates []string                      `json:"supportedDates"`
	Detector       map[string]any                `json:"detector"`
	Notes          []string                      `json:"notes"`
}

// uiWriteManifest stores the scenario dates and expected values outside the
// repository. The caller owns the directory.
func uiWriteManifest(t *testing.T, path string, today time.Time, uploads int,
	supported []string) {
	t.Helper()
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("create manifest directory: %v", err)
		}
	}
	night := map[string]any{
		"steps":                0,
		"avgHr":                uiBaselineBPM,
		"restingHr":            uiBaselineBPM,
		"hrvRmssd":             uiBaselineHRV,
		"respRate":             uiBaselineResp,
		"skinTempNightC":       float64(uiBaselineTemp) / 100,
		"sleepScore":           uiNightScore,
		"sleepMins":            210,
		"sleepDeepMins":        60,
		"sleepRemMins":         60,
		"sleepLightMins":       90,
		"sleepWakeMins":        30,
		"timeInBedMins":        240,
		"sleepEfficiencyPct":   88,
		"napCount":             0,
		"trainingLoad":         0,
		"sleepStagesTruncated": false,
	}
	support := make([]string, 0, 20)
	for offset := 15; offset >= 2; offset-- {
		support = append(support, uiDate(uiOffset(today, offset)))
	}
	for _, offset := range []int{20, 21, 22, 23, 25} {
		support = append(support, uiDate(uiOffset(today, offset)))
	}
	scenarios := map[string]uiManifestScenario{
		"readiness-scored": {
			Date: uiDate(uiOffset(today, 1)),
			Expected: map[string]any{
				"night":           night,
				"readinessStatus": "scored",
				"readinessBand":   "fair",
				"baselineNights":  19,
				"componentInputs": map[string]any{"hrv": uiBaselineHRV, "sleeping_hr": uiBaselineBPM, "sleep": uiNightScore, "skin_temp": float64(uiBaselineTemp) / 100, "resp_rate": uiBaselineResp},
			},
		},
		"main-and-nap": {
			Date: uiDate(uiOffset(today, 16)),
			Expected: map[string]any{
				"sleepScore":          84,
				"sleepMins":           390,
				"sleepDeepMins":       90,
				"sleepRemMins":        120,
				"sleepLightMins":      180,
				"sleepWakeMins":       30,
				"timeInBedMins":       420,
				"sleepEfficiencyPct":  93,
				"napCount":            1,
				"mainStagesTruncated": true,
				"mainStageCount":      2,
				"napTotalMins":        45,
				"wakeStageMinutes":    30,
			},
		},
		"nap-only": {
			Date: uiDate(uiOffset(today, 17)),
			Expected: map[string]any{
				"napCount":     1,
				"main":         nil,
				"napTotalMins": 45,
				"sleepMins":    nil,
			},
		},
		"split-night": {
			Date: uiDate(uiOffset(today, 18)),
			Expected: map[string]any{
				"sleepMins":          460,
				"sleepDeepMins":      120,
				"sleepRemMins":       120,
				"sleepLightMins":     220,
				"sleepWakeMins":      20,
				"timeInBedMins":      480,
				"sleepEfficiencyPct": 96,
				"reportedMainMins":   240,
				"sessions":           2,
			},
		},
		"workout-and-auto": {
			Date: uiDate(uiOffset(today, 19)),
			Expected: map[string]any{
				"steps":              1500,
				"avgHr":              89.4,
				"workoutCount":       1,
				"autoActivityCount":  1,
				"workoutCalories":    320,
				"autoLabel":          "Walk",
				"autoDurationSec":    900,
				"autoSteps":          1500,
				"workoutSportType":   uiWorkoutSport,
				"workoutSportName":   uiWorkoutName,
				"workoutDurationSec": 2400,
			},
		},
		"readiness-calibrating": {
			Date: uiDate(uiOffset(today, 20)),
			Expected: map[string]any{
				"night":           night,
				"readinessStatus": "calibrating",
				"baselineNights":  4,
				"readiness":       nil,
			},
		},
		"vitals-real-spo2": {
			Date: uiDate(uiOffset(today, 27)) + "," + uiDate(uiOffset(today, 28)),
			Expected: map[string]any{
				"meanDay":    map[string]any{"spo2Avg": 97.3, "spo2Min": 95, "spo2Samples": 3},
				"perfectDay": map[string]any{"spo2Avg": 100, "spo2Min": 100, "spo2Samples": 3},
			},
		},
		"vitals-no-spo2": {
			Date: uiDate(uiOffset(today, 29)),
			Expected: map[string]any{
				"stressAvg":   30,
				"steps":       20,
				"avgHr":       65,
				"spo2Avg":     nil,
				"spo2Samples": 0,
			},
		},
		"readiness-missing": {
			Date: uiDate(uiOffset(today, 25)) + "," + uiDate(uiOffset(today, 26)),
			Expected: map[string]any{
				"no_baseline": map[string]any{"date": uiDate(uiOffset(today, 25)), "baselineNights": 0},
				"no_data":     map[string]any{"date": uiDate(uiOffset(today, 26)), "steps": 100, "avgHr": 70},
			},
		},
		"empty": {
			Date:     uiDate(uiOffset(today, 24)),
			Expected: map[string]any{"dailyRows": 0},
		},
	}

	manifest := uiManifest{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Provenance:  "synthetic",
		IstDay:      uiDate(today),
		Window:      []string{uiDate(uiOffset(today, uiWindowDays-1)), uiDate(today)},
		Uploads:     uploads,
		Scenarios:   scenarios,
		Unreachable: map[string]string{
			"zero-minute-wake-stage": "a 0x48 stage stores inclusive minute bounds, so the shortest stage is one minute and a zero minute wake stage cannot be encoded; the baseline nights carry a 30 minute wake stage and the main-and-nap main sleep is deliberately stage truncated",
		},
		EmptyDate:      uiDate(uiOffset(today, 24)),
		SupportNights:  support,
		SupportedDates: supported,
		Detector: map[string]any{
			"cadenceMin":  60,
			"gapMax":      3,
			"durationMin": 10,
			"runCadence":  130,
		},
		Notes: []string{
			"Every date is computed inside the inclusive IST window at run time.",
			"Today has only the two step minutes TestSeedLocalSmoke writes, so pick the manifest dates.",
			"Support nights are every seeded HRV night; the scored date counts nineteen of them and the calibrating date four.",
			"The calibrating and no_baseline dates are HRV nights too, so they also feed later baselines.",
		},
	}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("encode manifest: %v", err)
	}
	if err := os.WriteFile(path, append(encoded, '\n'), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	t.Logf("UI scenario manifest written to %s", path)
}
