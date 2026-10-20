package parse

import (
	"errors"
	"testing"
	"time"
)

func appendVarint(dst []byte, value uint64) []byte {
	for value >= 0x80 {
		dst = append(dst, byte(value)|0x80)
		value >>= 7
	}
	return append(dst, byte(value))
}

func protoBytesField(dst []byte, field int, payload []byte) []byte {
	dst = appendVarint(dst, uint64(field)<<3|2)
	dst = appendVarint(dst, uint64(len(payload)))
	return append(dst, payload...)
}

func protoVarintField(dst []byte, field int, value uint64) []byte {
	dst = appendVarint(dst, uint64(field)<<3)
	return appendVarint(dst, value)
}

type workoutSpec struct {
	start    int64
	duration int64
	sport    *int
	calories *int
	avgHR    *int
	maxHR    *int
	minHR    *int
}

func encodeWorkout(spec workoutSpec) []byte {
	message := protoBytesField(nil, 1, []byte("2.1"))
	message = append(message, protoBytesField(nil, 2, protoVarintField(nil, 1, uint64(spec.start)))...)
	if spec.sport != nil {
		message = append(message, protoBytesField(nil, 3, protoVarintField(nil, 1, uint64(*spec.sport)))...)
	}
	message = append(message, protoBytesField(nil, 7, protoVarintField(nil, 1, uint64(spec.duration)))...)
	if spec.calories != nil {
		message = append(message, protoBytesField(nil, 16, protoVarintField(nil, 1, uint64(*spec.calories)))...)
	}
	var hr []byte
	if spec.avgHR != nil {
		hr = protoVarintField(hr, 1, uint64(*spec.avgHR))
	}
	if spec.maxHR != nil {
		hr = protoVarintField(hr, 2, uint64(*spec.maxHR))
	}
	if spec.minHR != nil {
		hr = protoVarintField(hr, 3, uint64(*spec.minHR))
	}
	if len(hr) > 0 {
		message = append(message, protoBytesField(nil, 19, hr)...)
	}
	out := append([]byte{0x00, 0x80}, message...)
	return append(out, 0x00)
}

func detailStream(times ...time.Time) []byte {
	var out []byte
	for _, at := range times {
		out = append(out, workoutDetailHeader...)
		millis := at.UnixMilli()
		for index := range workoutDetailTimestampSize {
			out = append(out, byte(millis>>(8*index)))
		}
	}
	return out
}

func workoutWindow(center time.Time) Window {
	return Window{StartedAt: center.Add(-time.Hour), EndedAt: center.Add(4 * time.Hour)}
}

func TestWorkoutUnknownSportKeepsNumericID(t *testing.T) {
	start := time.Date(2026, 6, 20, 14, 39, 0, 0, time.UTC)
	raw := encodeWorkout(workoutSpec{start: start.Unix(), duration: 3600, sport: new(223)})
	rows, result, err := parseWorkoutSummaries(raw, workoutWindow(start))
	if err != nil {
		t.Fatalf("parse summary: %v", err)
	}
	if len(rows) != 1 || result.Rows != 1 {
		t.Fatalf("rows=%v result=%+v, want one workout", rows, result)
	}
	if rows[0].SportType != 223 || rows[0].SportName != "Activity 223" {
		t.Fatalf("sport = %d/%q, want 223/Activity 223", rows[0].SportType, rows[0].SportName)
	}
}

func TestWorkoutWithoutSportFallsBackToWorkout(t *testing.T) {
	start := time.Date(2026, 6, 20, 14, 39, 0, 0, time.UTC)
	raw := encodeWorkout(workoutSpec{start: start.Unix(), duration: 600})
	rows, _, err := parseWorkoutSummaries(raw, workoutWindow(start))
	if err != nil {
		t.Fatalf("parse summary: %v", err)
	}
	if len(rows) != 1 || rows[0].SportType != 0 || rows[0].SportName != "Workout" {
		t.Fatalf("rows = %+v, want sport 0 named Workout", rows)
	}
}

func TestWorkoutOptionalStatsOutsideDomainBecomeNil(t *testing.T) {
	start := time.Date(2026, 6, 20, 14, 39, 0, 0, time.UTC)
	raw := encodeWorkout(workoutSpec{
		start: start.Unix(), duration: 600, sport: new(92),
		calories: new(12000), avgHR: new(0),
	})
	rows, result, err := parseWorkoutSummaries(raw, workoutWindow(start))
	if err != nil {
		t.Fatalf("parse summary: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want the workout retained", len(rows))
	}
	if rows[0].Calories != nil || rows[0].AvgHR != nil {
		t.Fatalf("out of range stats survived: %+v", rows[0])
	}
	if result.Dropped != 2 {
		t.Fatalf("dropped = %d, want both out of range statistics", result.Dropped)
	}
	if len(result.Warnings) != 1 || result.Warnings[0].Code != "workout_stat_out_of_range" ||
		result.Warnings[0].Count != 2 {
		t.Fatalf("warnings = %v, want one workout_stat_out_of_range count=2", result.Warnings)
	}
}

func TestWorkoutSportOutsideInt16IsFatal(t *testing.T) {
	start := time.Date(2026, 6, 20, 14, 39, 0, 0, time.UTC)
	raw := encodeWorkout(workoutSpec{start: start.Unix(), duration: 600, sport: new(40000)})
	_, _, err := parseWorkoutSummaries(raw, workoutWindow(start))
	var parseErr *ParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("error = %v, want a structural *ParseError", err)
	}
}

func TestWorkoutWithoutVersionMarkerIsRejected(t *testing.T) {
	raw := []byte{0x00, 0x80, 0x08, 0x01, 0x00}
	_, _, err := parseWorkoutSummaries(raw, loadFixtureWindow(t))
	var parseErr *ParseError
	if !errors.As(err, &parseErr) || parseErr.Code != ErrCodeUnsupportedVersion {
		t.Fatalf("error = %v, want unsupported_version", err)
	}
}

func TestDetailOnlyInputCreatesNoWorkout(t *testing.T) {
	start := time.Date(2026, 6, 22, 2, 20, 16, 0, time.UTC)
	batch, err := ParseBlobs(Catalog{}, map[string][]byte{
		typeWorkoutDetail: detailStream(start.Add(280 * time.Millisecond)),
	}, workoutWindow(start))
	if err != nil {
		t.Fatalf("parse detail: %v", err)
	}
	if len(batch.Workouts) != 0 {
		t.Fatalf("detail only input produced %d workouts", len(batch.Workouts))
	}
	result := batch.Types[typeWorkoutDetail]
	if result.Rows != 0 || result.Dropped != 1 {
		t.Fatalf("result = %+v, want one unmatched block and no rows", result)
	}
}

func TestDetailBlockAttachesInsideTheSummaryWindow(t *testing.T) {
	start := time.Date(2026, 6, 22, 2, 20, 16, 0, time.UTC)
	summary := encodeWorkout(workoutSpec{start: start.Unix(), duration: 600, sport: new(92)})
	batch, err := ParseBlobs(Catalog{}, map[string][]byte{
		typeWorkout:       summary,
		typeWorkoutDetail: detailStream(start.Add(280 * time.Millisecond)),
	}, workoutWindow(start))
	if err != nil {
		t.Fatalf("parse blobs: %v", err)
	}
	if len(batch.Workouts) != 1 || !batch.Workouts[0].HasDetail {
		t.Fatalf("workouts = %+v, want one summary with detail", batch.Workouts)
	}
	if result := batch.Types[typeWorkoutDetail]; result.ProcessedThrough == nil ||
		!result.ProcessedThrough.Equal(start.Add(600*time.Second)) {
		t.Fatalf("0x06 processedThrough = %v, want the attached workout end", result.ProcessedThrough)
	}
}

func TestDetailTrailerBlocksKeepTheWorkoutCount(t *testing.T) {
	start := time.Date(2026, 6, 22, 2, 20, 16, 0, time.UTC)
	summary := encodeWorkout(workoutSpec{start: start.Unix(), duration: 600, sport: new(92)})
	batch, err := ParseBlobs(Catalog{}, map[string][]byte{
		typeWorkout: summary,
		typeWorkoutDetail: detailStream(
			start.Add(time.Second),
			start.Add(700*time.Second),
		),
	}, workoutWindow(start))
	if err != nil {
		t.Fatalf("parse blobs: %v", err)
	}
	if len(batch.Workouts) != 1 || !batch.Workouts[0].HasDetail {
		t.Fatalf("workouts = %+v, want one summary with attached detail", batch.Workouts)
	}
	result := batch.Types[typeWorkoutDetail]
	if result.Dropped != 1 || len(result.Warnings) != 1 ||
		result.Warnings[0].String() != "unmatched_workout_detail offset=12 count=1" {
		t.Fatalf("result = %+v, want the trailer block warned at offset 12", result)
	}
}

func TestAmbiguousDetailBlockIsNotAttached(t *testing.T) {
	start := time.Date(2026, 6, 22, 2, 20, 16, 0, time.UTC)
	first := encodeWorkout(workoutSpec{start: start.Unix(), duration: 600, sport: new(92)})
	second := encodeWorkout(workoutSpec{start: start.Add(300 * time.Second).Unix(), duration: 600, sport: new(92)})
	batch, err := ParseBlobs(Catalog{}, map[string][]byte{
		typeWorkout:       append(first, second...),
		typeWorkoutDetail: detailStream(start.Add(400 * time.Second)),
	}, workoutWindow(start))
	if err != nil {
		t.Fatalf("parse blobs: %v", err)
	}
	if len(batch.Workouts) != 2 {
		t.Fatalf("workouts = %d, want both overlapping summaries", len(batch.Workouts))
	}
	for _, workout := range batch.Workouts {
		if workout.HasDetail {
			t.Fatalf("ambiguous block attached to %+v", workout)
		}
	}
	if result := batch.Types[typeWorkoutDetail]; result.Dropped != 1 {
		t.Fatalf("result = %+v, want the ambiguous block counted", result)
	}
}

func TestWorkoutSummaryWithoutDurationIsNotAWorkout(t *testing.T) {
	start := time.Date(2026, 6, 20, 14, 39, 0, 0, time.UTC)
	raw := encodeWorkout(workoutSpec{start: start.Unix(), duration: 0, sport: new(92)})
	rows, result, err := parseWorkoutSummaries(raw, workoutWindow(start))
	if err != nil {
		t.Fatalf("parse summary: %v", err)
	}
	if len(rows) != 0 || result.Rows != 0 || result.Dropped != 1 {
		t.Fatalf("rows=%v result=%+v, want the summary dropped", rows, result)
	}
}
