package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Anshuman115/heliolytics/internal/store"
)

// seedSession inserts one immutable sync session used by sample FKs.
func seedSession(t *testing.T, st *store.Store, id string) {
	t.Helper()
	seed(t, st, `INSERT INTO sync_sessions (session_id, device_mac, started_at, ended_at, catalog)
		VALUES ($1, '02:00:00:00:00:01', '2026-09-30T00:00:00Z', '2026-10-02T00:00:00Z', '{"chunked":[]}'::jsonb)`, id)
}

func TestDailyMetricsKeepsNullsAndZeros(t *testing.T) {
	st := testStore(t)
	keys := testKeys()
	mux := newMux(st, testConfig(keys), (&stubIngest{}).run, (&stubProfile{}).update, newFakeNonces())

	seed(t, st, `INSERT INTO daily_metrics
		(day_key, steps, avg_hr, pai_score, workout_calories, trimp, spo2_n)
		VALUES ('2026-10-01', 0, NULL, 0, 0, 12.34, 0)`)
	seed(t, st, `INSERT INTO daily_metrics
		(day_key, readiness, readiness_components)
		VALUES ('2026-10-02', 42, '{"version":1,"status":"scored","score":42,"raw":50,"baselineNights":20,"components":[],"flags":[]}'::jsonb)`)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/daily-metrics?from=2026-10-01&to=2026-10-03", nil)
	signRequest(t, keys, "web", req, nil)
	recorder := serve(mux, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}

	var body struct {
		Days []map[string]any `json:"days"`
	}
	decodeJSON(t, recorder, &body)
	if len(body.Days) != 2 {
		t.Fatalf("days = %d, want 2", len(body.Days))
	}

	first := body.Days[0]
	if first["dayKey"] != "2026-10-01" {
		t.Fatalf("first day = %v", first["dayKey"])
	}
	if steps, ok := first["steps"].(float64); !ok || steps != 0 {
		t.Fatalf("steps = %v, want 0", first["steps"])
	}
	if first["avgHr"] != nil {
		t.Fatalf("avgHr = %v, want null", first["avgHr"])
	}
	if pai, ok := first["paiScore"].(float64); !ok || pai != 0 {
		t.Fatalf("paiScore = %v, want 0", first["paiScore"])
	}
	if calories, ok := first["workoutCalories"].(float64); !ok || calories != 0 {
		t.Fatalf("workoutCalories = %v, want 0", first["workoutCalories"])
	}
	if load, ok := first["trainingLoad"].(float64); !ok || load != 12.3 {
		t.Fatalf("trainingLoad = %v, want 12.3", first["trainingLoad"])
	}
	if samples, ok := first["spo2Samples"].(float64); !ok || samples != 0 {
		t.Fatalf("spo2Samples = %v, want 0", first["spo2Samples"])
	}
	for _, key := range []string{"readiness", "readinessStatus", "readinessBand", "readinessComponents"} {
		if value, present := first[key]; !present || value != nil {
			t.Fatalf("%s = %v (present=%v), want null", key, value, present)
		}
	}
	if _, present := first["timeInBedMins"]; !present {
		t.Fatal("timeInBedMins key missing")
	}

	second := body.Days[1]
	if second["readinessStatus"] != "scored" {
		t.Fatalf("readinessStatus = %v", second["readinessStatus"])
	}
	if second["readinessBand"] != "fair" {
		t.Fatalf("readinessBand = %v", second["readinessBand"])
	}
	if second["readinessComponents"] == nil {
		t.Fatal("readinessComponents dropped")
	}
}

func TestSleepChoosesLongestMainAndSeparatesNaps(t *testing.T) {
	st := testStore(t)
	seedSession(t, st, "sleep-session")
	seed(t, st, `INSERT INTO sleep_sessions
		(started_at, ended_at, day_key, is_nap, score, total_mins, deep_mins, rem_mins, light_mins, wake_mins,
		 stages, stages_truncated, observed_at, session_id)
		VALUES
		('2026-09-30T20:00:00Z', '2026-10-01T00:32:00Z', '2026-10-01', false, 70, 272, 60, 40, 172, 0,
		 '[]'::jsonb, false, '2026-10-01T01:00:00Z', 'sleep-session'),
		('2026-10-01T13:00:00Z', '2026-10-01T19:40:00Z', '2026-10-01', false, 88, 400, 100, 100, 200, 10,
		 '[{"start":"2026-10-01T13:00:00Z","end":"2026-10-01T14:00:00Z","type":5}]'::jsonb, true,
		 '2026-10-01T20:00:00Z', 'sleep-session'),
		('2026-10-01T07:00:00Z', '2026-10-01T08:08:00Z', '2026-10-01', true, 55, 68, 10, 10, 48, 0,
		 '[]'::jsonb, false, '2026-10-01T09:00:00Z', 'sleep-session')`)

	keys := testKeys()
	mux := newMux(st, testConfig(keys), (&stubIngest{}).run, (&stubProfile{}).update, newFakeNonces())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sleep?from=2026-10-01&to=2026-10-01", nil)
	signRequest(t, keys, "phone", req, nil)
	recorder := serve(mux, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}

	var body struct {
		Days []struct {
			DayKey string `json:"dayKey"`
			Main   *struct {
				TotalMins       float64 `json:"totalMins"`
				StagesTruncated bool    `json:"stagesTruncated"`
				Stages          []struct {
					Type float64 `json:"type"`
				} `json:"stages"`
			} `json:"main"`
			Naps []struct {
				Score     *float64 `json:"score"`
				TotalMins float64  `json:"totalMins"`
			} `json:"naps"`
		} `json:"days"`
	}
	decodeJSON(t, recorder, &body)
	if len(body.Days) != 1 {
		t.Fatalf("days = %d, want 1", len(body.Days))
	}
	day := body.Days[0]
	if day.Main == nil || day.Main.TotalMins != 400 {
		t.Fatalf("main = %+v, want the 400-minute session", day.Main)
	}
	if !day.Main.StagesTruncated || len(day.Main.Stages) != 1 || day.Main.Stages[0].Type != 5 {
		t.Fatalf("stages = %+v truncated=%v", day.Main.Stages, day.Main.StagesTruncated)
	}
	if len(day.Naps) != 1 || day.Naps[0].TotalMins != 68 {
		t.Fatalf("naps = %+v", day.Naps)
	}
	if day.Naps[0].Score != nil {
		t.Fatalf("nap score = %v, want null", *day.Naps[0].Score)
	}
}

func TestActivitiesAreOrderedAndKeepNulls(t *testing.T) {
	st := testStore(t)
	seedSession(t, st, "activity-session")
	seed(t, st, `INSERT INTO activities
		(started_at, kind, ended_at, sport_type, sport_name, calories, avg_hr, max_hr, min_hr, steps,
		 has_detail, observed_at, session_id)
		VALUES
		('2026-10-01T09:00:00Z', 'workout', '2026-10-01T10:00:00Z', 1, 'Running', NULL, NULL, NULL, NULL, NULL,
		 true, '2026-10-01T11:00:00Z', 'activity-session'),
		('2026-10-01T06:00:00Z', 'auto', '2026-10-01T06:45:00Z', 0, 'Walk', 0, 90, 110, 80, 4100,
		 false, '2026-10-01T11:00:00Z', 'activity-session')`)

	keys := testKeys()
	mux := newMux(st, testConfig(keys), (&stubIngest{}).run, (&stubProfile{}).update, newFakeNonces())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/activities?from=2026-10-01&to=2026-10-01", nil)
	signRequest(t, keys, "web", req, nil)
	recorder := serve(mux, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}

	var body struct {
		Activities []struct {
			Kind        string   `json:"kind"`
			SportName   string   `json:"sportName"`
			DurationSec float64  `json:"durationSec"`
			Calories    *float64 `json:"calories"`
			AvgHr       *float64 `json:"avgHr"`
			Steps       *float64 `json:"steps"`
			HasDetail   bool     `json:"hasDetail"`
		} `json:"activities"`
	}
	decodeJSON(t, recorder, &body)
	if len(body.Activities) != 2 {
		t.Fatalf("activities = %d, want 2", len(body.Activities))
	}
	auto, workout := body.Activities[0], body.Activities[1]
	if auto.Kind != "auto" || auto.DurationSec != 2700 {
		t.Fatalf("auto = %+v", auto)
	}
	if auto.Calories == nil || *auto.Calories != 0 {
		t.Fatalf("auto calories = %v, want 0", auto.Calories)
	}
	if workout.Kind != "workout" || workout.DurationSec != 3600 || !workout.HasDetail {
		t.Fatalf("workout = %+v", workout)
	}
	if workout.Calories != nil || workout.AvgHr != nil || workout.Steps != nil {
		t.Fatalf("workout nulls lost: %+v", workout)
	}
}

func TestSeriesHasStableLabelsAndEqualArrays(t *testing.T) {
	st := testStore(t)
	seedSession(t, st, "series-session")
	seed(t, st, `INSERT INTO hrv_samples (sampled_at, rmssd_ms, session_id, observed_at)
		VALUES ('2026-10-01T02:00:00Z', 45.5, 'series-session', '2026-10-02T00:00:00Z')`)
	seed(t, st, `INSERT INTO stress_samples (sampled_at, stress, session_id, observed_at)
		VALUES ('2026-10-01T03:00:00Z', 30, 'series-session', '2026-10-02T00:00:00Z')`)

	keys := testKeys()
	mux := newMux(st, testConfig(keys), (&stubIngest{}).run, (&stubProfile{}).update, newFakeNonces())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics/series?from=2026-10-01&to=2026-10-01", nil)
	signRequest(t, keys, "web", req, nil)
	recorder := serve(mux, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}

	var body struct {
		Days []struct {
			DayKey  string    `json:"dayKey"`
			Metric  string    `json:"metric"`
			Offsets []float64 `json:"offsets"`
			Values  []float64 `json:"values"`
		} `json:"days"`
	}
	decodeJSON(t, recorder, &body)
	if len(body.Days) != 2 {
		t.Fatalf("days = %d, want two metric rows", len(body.Days))
	}
	if body.Days[0].Metric != "hrv" || body.Days[1].Metric != "stress" {
		t.Fatalf("metric order = %q, %q", body.Days[0].Metric, body.Days[1].Metric)
	}
	for _, day := range body.Days {
		if len(day.Offsets) != len(day.Values) || len(day.Offsets) == 0 {
			t.Fatalf("%s arrays offsets=%d values=%d", day.Metric, len(day.Offsets), len(day.Values))
		}
		if day.DayKey != "2026-10-01" {
			t.Fatalf("dayKey = %q", day.DayKey)
		}
	}
	if body.Days[0].Values[0] != 45.5 {
		t.Fatalf("hrv value = %v", body.Days[0].Values[0])
	}
}

func TestSleepRouteEmptyDatabaseReturnsEmptyList(t *testing.T) {
	keys := testKeys()
	mux := newMux(testStore(t), testConfig(keys), (&stubIngest{}).run, (&stubProfile{}).update, newFakeNonces())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sleep?from=2026-10-01&to=2026-10-01", nil)
	signRequest(t, keys, "web", req, nil)
	recorder := serve(mux, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	if body := recorder.Body.String(); body == "{\"days\":null}\n" || body == "{\"days\":null}" {
		t.Fatalf("empty list serialized as null: %s", body)
	}
	var decoded struct {
		Days []any `json:"days"`
	}
	decodeJSON(t, recorder, &decoded)
	if decoded.Days == nil || len(decoded.Days) != 0 {
		t.Fatalf("days = %v, want []", decoded.Days)
	}
}
