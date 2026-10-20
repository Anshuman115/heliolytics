package parse

import (
	"math"
	"testing"
	"time"
)

func TestRealFixtureReplay(t *testing.T) {
	catalog := loadFixtureCatalog(t)
	window := loadFixtureWindow(t)
	expected := loadExpected(t)
	blobs := make(map[string][]byte, len(expected.Types))
	for code := range expected.Types {
		blobs[code] = fixtureBytes(t, code)
	}
	batch, err := ParseBlobs(catalog, blobs, window)
	if err != nil {
		t.Fatalf("parse real fixtures: %v", err)
	}
	if len(batch.Types) != len(expected.Types) {
		t.Fatalf("parsed %d types, want %d", len(batch.Types), len(expected.Types))
	}
	for code, want := range expected.Types {
		got, ok := batch.Types[code]
		if !ok {
			t.Fatalf("%s has no type result", code)
		}
		if got.Status != want.Status || got.Rows != want.Rows ||
			got.Dropped != want.Dropped || got.TrailingBytes != want.TrailingBytes {
			t.Errorf("%s result = status=%s rows=%d dropped=%d trailing=%d, want status=%s rows=%d dropped=%d trailing=%d",
				code, got.Status, got.Rows, got.Dropped, got.TrailingBytes,
				want.Status, want.Rows, want.Dropped, want.TrailingBytes)
		}
		switch {
		case got.ProcessedThrough == nil && want.ProcessedThrough == nil:
		case got.ProcessedThrough == nil || want.ProcessedThrough == nil:
			t.Errorf("%s processedThrough = %v, want %v", code, got.ProcessedThrough, want.ProcessedThrough)
		default:
			if !got.ProcessedThrough.Equal(mustParseFixtureTime(t, *want.ProcessedThrough)) {
				t.Errorf("%s processedThrough = %s, want %s",
					code, formatFixtureTime(*got.ProcessedThrough), *want.ProcessedThrough)
			}
		}
		if len(got.Warnings) != len(want.Warnings) {
			t.Fatalf("%s warnings = %v, want %v", code, got.Warnings, want.Warnings)
		}
		for i := range want.Warnings {
			if got.Warnings[i].String() != want.Warnings[i] {
				t.Errorf("%s warning %d = %q, want %q", code, i, got.Warnings[i].String(), want.Warnings[i])
			}
		}
	}

	checkRealSteps(t, batch, expected)
	checkRealHeartRate(t, batch, expected)
	checkRealContinuousHeartRate(t, batch, expected)
	checkRealTemperature(t, batch, expected)
	checkRealScalarSeries(t, batch, expected)
	checkRealPAI(t, batch, expected)
	checkRealSleep(t, batch, expected)
	checkRealWorkout(t, batch, expected)
}

func checkRealSteps(t *testing.T, batch ParsedBatch, expected expectedFixture) {
	t.Helper()
	wantRows := expected.Steps.NonzeroMinutes + expected.Steps.ZeroMinutes
	if len(batch.Steps) != wantRows {
		t.Fatalf("step rows = %d, want %d", len(batch.Steps), wantRows)
	}
	zepp := zeppSeries(t, "activity-minute.csv")
	nonzero, sum := 0, 0
	daySums := map[string]int{}
	for _, row := range batch.Steps {
		if row.Steps > 0 {
			nonzero++
			if want, ok := zepp[row.SampledAt.Unix()]; !ok || want != row.Steps {
				t.Errorf("step minute %s = %d, Zepp %d (present=%t)",
					formatFixtureTime(row.SampledAt), row.Steps, want, ok)
			}
		}
		sum += row.Steps
		daySums[IstDayKey(row.SampledAt.Unix())] += row.Steps
	}
	if nonzero != expected.Steps.NonzeroMinutes {
		t.Errorf("nonzero step minutes = %d, want %d", nonzero, expected.Steps.NonzeroMinutes)
	}
	if len(zepp) != expected.Steps.NonzeroMinutes {
		t.Errorf("Zepp activity rows = %d, want %d", len(zepp), expected.Steps.NonzeroMinutes)
	}
	if sum != expected.Steps.Sum {
		t.Errorf("step sum = %d, want %d", sum, expected.Steps.Sum)
	}
	if dropped := batch.Types[typeActivity].Dropped; dropped != 2*expected.Steps.SentinelMinutes {
		t.Errorf("0x01 dropped = %d, want %d for the sentinel minutes", dropped, 2*expected.Steps.SentinelMinutes)
	}
	for day, want := range expected.Steps.DaySums {
		if daySums[day] != want {
			t.Errorf("steps on %s = %d, want %d", day, daySums[day], want)
		}
	}
	if got := formatFixtureTime(batch.Steps[0].SampledAt); got != expected.Steps.FirstMinute {
		t.Errorf("first step minute = %s, want %s", got, expected.Steps.FirstMinute)
	}
	if got := formatFixtureTime(batch.Steps[len(batch.Steps)-1].SampledAt); got != expected.Steps.LastMinute {
		t.Errorf("last step minute = %s, want %s", got, expected.Steps.LastMinute)
	}
}

func checkRealHeartRate(t *testing.T, batch ParsedBatch, expected expectedFixture) {
	t.Helper()
	var minutes []HeartRateRow
	for _, row := range batch.HeartRate {
		if row.Source == typeActivity {
			minutes = append(minutes, row)
		}
	}
	if len(minutes) != expected.HeartRate.Rows {
		t.Fatalf("0x01 HR rows = %d, want %d", len(minutes), expected.HeartRate.Rows)
	}
	zepp := zeppSeries(t, "heartrate-auto.csv")
	sums, counts := map[string]int{}, map[string]int{}
	for _, row := range minutes {
		if want, ok := zepp[row.SampledAt.Unix()]; !ok || want != row.BPM {
			t.Errorf("HR minute %s = %d, Zepp %d (present=%t)",
				formatFixtureTime(row.SampledAt), row.BPM, want, ok)
		}
		day := IstDayKey(row.SampledAt.Unix())
		sums[day] += row.BPM
		counts[day]++
	}
	for day, want := range expected.HeartRate.DayMeans {
		if counts[day] == 0 {
			t.Fatalf("no HR minutes on %s", day)
		}
		mean := math.Round(float64(sums[day])/float64(counts[day])*10) / 10
		if mean != want {
			t.Errorf("mean HR on %s = %.1f, want %.1f", day, mean, want)
		}
	}
	if got := formatFixtureTime(minutes[0].SampledAt); got != expected.HeartRate.First {
		t.Errorf("first HR minute = %s, want %s", got, expected.HeartRate.First)
	}
	if got := formatFixtureTime(minutes[len(minutes)-1].SampledAt); got != expected.HeartRate.Last {
		t.Errorf("last HR minute = %s, want %s", got, expected.HeartRate.Last)
	}
}

func checkRealContinuousHeartRate(t *testing.T, batch ParsedBatch, expected expectedFixture) {
	t.Helper()
	var seconds []HeartRateRow
	for _, row := range batch.HeartRate {
		if row.Source == typeContinuousHR {
			seconds = append(seconds, row)
		}
	}
	want := expected.ContinuousHeartRate
	if len(seconds) != want.Rows {
		t.Fatalf("0x46 rows = %d, want %d", len(seconds), want.Rows)
	}
	if got := formatFixtureTime(seconds[0].SampledAt); got != want.First {
		t.Errorf("first 0x46 sample = %s, want %s", got, want.First)
	}
	if got := formatFixtureTime(seconds[len(seconds)-1].SampledAt); got != want.Last {
		t.Errorf("last 0x46 sample = %s, want %s", got, want.Last)
	}
	if seconds[0].BPM != want.FirstBPM || seconds[len(seconds)-1].BPM != want.LastBPM {
		t.Errorf("0x46 edge BPM = %d/%d, want %d/%d",
			seconds[0].BPM, seconds[len(seconds)-1].BPM, want.FirstBPM, want.LastBPM)
	}
	for i, row := range seconds {
		drift := row.SampledAt.Sub(seconds[0].SampledAt) - time.Duration(i)*time.Second
		if drift != 0 {
			t.Fatalf("0x46 sample %d is %v off the one-second cadence", i, drift)
		}
	}
}

func checkRealTemperature(t *testing.T, batch ParsedBatch, expected expectedFixture) {
	t.Helper()
	want := expected.Temperature
	rows := batch.Temperature
	if len(rows) == 0 {
		t.Fatal("no temperature rows")
	}
	if rows[0].HundredthsC != want.FirstHundredths {
		t.Errorf("first temperature = %d hundredths, want %d", rows[0].HundredthsC, want.FirstHundredths)
	}
	minimum, maximum := rows[0].HundredthsC, rows[0].HundredthsC
	for _, row := range rows {
		minimum = min(minimum, row.HundredthsC)
		maximum = max(maximum, row.HundredthsC)
	}
	if minimum != want.MinHundredths || maximum != want.MaxHundredths {
		t.Errorf("temperature range = %d..%d, want %d..%d",
			minimum, maximum, want.MinHundredths, want.MaxHundredths)
	}
}

func checkRealScalarSeries(t *testing.T, batch ParsedBatch, expected expectedFixture) {
	t.Helper()
	checkScalarRange(t, "hrv", batch.HRV, expected.HRV.Rows, expected.HRV.MinRMSSD, expected.HRV.MaxRMSSD)
	checkScalarRange(t, "respiration", batch.Respiration,
		expected.Respiration.Rows, expected.Respiration.MinBreaths, expected.Respiration.MaxBreaths)
	checkScalarRange(t, "stress", batch.Stress,
		expected.Stress.Rows, expected.Stress.MinValue, expected.Stress.MaxValue)
}

func checkScalarRange(t *testing.T, name string, rows []ScalarRow, wantRows, wantMin, wantMax int) {
	t.Helper()
	if len(rows) != wantRows {
		t.Fatalf("%s rows = %d, want %d", name, len(rows), wantRows)
	}
	minimum, maximum := int(rows[0].Value), int(rows[0].Value)
	for _, row := range rows {
		minimum = min(minimum, int(row.Value))
		maximum = max(maximum, int(row.Value))
	}
	if minimum != wantMin || maximum != wantMax {
		t.Errorf("%s range = %d..%d, want %d..%d", name, minimum, maximum, wantMin, wantMax)
	}
}

func checkRealPAI(t *testing.T, batch ParsedBatch, expected expectedFixture) {
	t.Helper()
	if len(batch.PAI) != len(expected.PAI) {
		t.Fatalf("PAI rows = %d, want %d", len(batch.PAI), len(expected.PAI))
	}
	for i, want := range expected.PAI {
		got := batch.PAI[i]
		if got.DayKey != want.DayKey || got.Score != want.Score ||
			!got.SampledAt.Equal(mustParseFixtureTime(t, want.SampledAt)) {
			t.Errorf("PAI %d = %+v, want day=%s score=%d at=%s",
				i, got, want.DayKey, want.Score, want.SampledAt)
		}
	}
}

func checkRealSleep(t *testing.T, batch ParsedBatch, expected expectedFixture) {
	t.Helper()
	var main, nap *SleepRow
	for i := range batch.Sleep {
		switch {
		case batch.Sleep[i].IsNap && nap == nil:
			nap = &batch.Sleep[i]
		case !batch.Sleep[i].IsNap && main == nil:
			main = &batch.Sleep[i]
		}
	}
	if main == nil || nap == nil {
		t.Fatalf("sleep rows = %+v, want one main and one nap", batch.Sleep)
	}
	checkSleepRow(t, "main", *main, expected.Sleep.Main)
	checkSleepRow(t, "nap", *nap, expected.Sleep.Nap)
	checkSleepStagesAgainstZepp(t, *main, *nap)
}

func checkSleepRow(t *testing.T, name string, got SleepRow, want expectedSleep) {
	t.Helper()
	if got.DayKey != want.DayKey || got.IsNap != want.IsNap ||
		!got.StartedAt.Equal(mustParseFixtureTime(t, want.StartedAt)) ||
		!got.EndedAt.Equal(mustParseFixtureTime(t, want.EndedAt)) {
		t.Errorf("%s identity = day=%s nap=%t %s..%s, want day=%s nap=%t %s..%s",
			name, got.DayKey, got.IsNap, formatFixtureTime(got.StartedAt), formatFixtureTime(got.EndedAt),
			want.DayKey, want.IsNap, want.StartedAt, want.EndedAt)
	}
	if got.TotalMins != want.TotalMins || got.DeepMins != want.DeepMins || got.REMMins != want.REMMins ||
		got.LightMins != want.LightMins || got.WakeMins != want.WakeMins {
		t.Errorf("%s totals = %d/%d/%d/%d/%d, want %d/%d/%d/%d/%d", name,
			got.TotalMins, got.DeepMins, got.REMMins, got.LightMins, got.WakeMins,
			want.TotalMins, want.DeepMins, want.REMMins, want.LightMins, want.WakeMins)
	}
	if len(got.Stages) != want.Stages {
		t.Errorf("%s stages = %d, want %d", name, len(got.Stages), want.Stages)
	}
	if got.StagesTruncated != want.StagesTruncated {
		t.Errorf("%s stagesTruncated = %t, want %t", name, got.StagesTruncated, want.StagesTruncated)
	}
	switch {
	case got.Score == nil && want.Score == nil:
	case got.Score == nil || want.Score == nil:
		t.Errorf("%s score = %v, want %v", name, got.Score, want.Score)
	case *got.Score != *want.Score:
		t.Errorf("%s score = %d, want %d", name, *got.Score, *want.Score)
	}
}

// checkSleepStagesAgainstZepp compares the parsed timelines with the stage runs
// read from the Zepp sleep-minute export. The export labels the night one day
// after the sleep day used by SLEEP and SPORT; times and stages are unchanged.
func checkSleepStagesAgainstZepp(t *testing.T, main, nap SleepRow) {
	t.Helper()
	runs := zeppSleepRuns(t)
	if len(runs) < 22 {
		t.Fatalf("Zepp sleep runs = %d, want at least 22", len(runs))
	}
	compareStages(t, "main", main.Stages, runs[:17])
	compareStages(t, "nap", nap.Stages, runs[17:22])
}

func compareStages(t *testing.T, name string, got, want []SleepStage) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s stage count = %d, want %d", name, len(got), len(want))
	}
	for i := range want {
		if !got[i].Start.Equal(want[i].Start) || !got[i].End.Equal(want[i].End) || got[i].Type != want[i].Type {
			t.Errorf("%s stage %d = %s..%s type %d, want %s..%s type %d", name, i,
				formatFixtureTime(got[i].Start), formatFixtureTime(got[i].End), got[i].Type,
				formatFixtureTime(want[i].Start), formatFixtureTime(want[i].End), want[i].Type)
		}
	}
}

func zeppSleepRuns(t *testing.T) []SleepStage {
	t.Helper()
	rows := readZeppCSV(t, "sleep-minute.csv")
	var out []SleepStage
	var current SleepStage
	for _, row := range rows[1:] {
		at, err := time.ParseInLocation("2006-01-02 15:04", row[0]+" "+row[1], istLocation())
		if err != nil {
			t.Fatalf("sleep-minute row %v: %v", row, err)
		}
		stageType, ok := zeppStageType(row[2])
		if !ok {
			t.Fatalf("sleep-minute row %v has an unknown stage", row)
		}
		if len(out) > 0 && current.Type == stageType && current.End.Equal(at) {
			current.End = at.Add(time.Minute)
			out[len(out)-1] = current
			continue
		}
		current = SleepStage{Start: at.UTC(), End: at.Add(time.Minute).UTC(), Type: stageType}
		out = append(out, current)
	}
	return out
}

func zeppStageType(stage string) (int, bool) {
	switch stage {
	case "LIGHT":
		return 4, true
	case "DEEP":
		return 5, true
	case "WAKE":
		return 7, true
	case "REM":
		return 8, true
	}
	return 0, false
}

func checkRealWorkout(t *testing.T, batch ParsedBatch, expected expectedFixture) {
	t.Helper()
	want := expected.Workout
	if len(batch.Workouts) != 1 {
		t.Fatalf("workouts = %d, want 1", len(batch.Workouts))
	}
	got := batch.Workouts[0]
	if got.SportType != want.SportType || got.SportName != want.SportName {
		t.Errorf("workout sport = %d/%q, want %d/%q", got.SportType, got.SportName, want.SportType, want.SportName)
	}
	if !got.StartedAt.Equal(mustParseFixtureTime(t, want.StartedAt)) ||
		!got.EndedAt.Equal(mustParseFixtureTime(t, want.EndedAt)) {
		t.Errorf("workout window = %s..%s, want %s..%s",
			formatFixtureTime(got.StartedAt), formatFixtureTime(got.EndedAt), want.StartedAt, want.EndedAt)
	}
	if int(got.EndedAt.Sub(got.StartedAt)/time.Second) != want.DurationSec {
		t.Errorf("workout duration = %v, want %ds", got.EndedAt.Sub(got.StartedAt), want.DurationSec)
	}
	checkOptionalInt(t, "calories", got.Calories, want.Calories)
	checkOptionalInt(t, "avg_hr", got.AvgHR, want.AvgHR)
	checkOptionalInt(t, "max_hr", got.MaxHR, want.MaxHR)
	checkOptionalInt(t, "min_hr", got.MinHR, want.MinHR)
	if got.Steps != nil {
		t.Errorf("workout steps = %d, want absent", *got.Steps)
	}
	if result := batch.Types[typeWorkout]; result.Rows != 1 || result.Dropped != 0 {
		t.Errorf("0x05 result = rows=%d dropped=%d, want 1/0", result.Rows, result.Dropped)
	}

	blocks, err := parseWorkoutDetailBlocks(fixtureBytes(t, typeWorkoutDetail))
	if err != nil {
		t.Fatalf("parse workout detail blocks: %v", err)
	}
	if len(blocks) != want.DetailBlocks {
		t.Errorf("detail blocks = %d, want %d", len(blocks), want.DetailBlocks)
	}
	if len(blocks) == 0 {
		return
	}
	if !blocks[0].at.Equal(mustParseFixtureTime(t, want.FirstDetailAt)) ||
		!blocks[len(blocks)-1].at.Equal(mustParseFixtureTime(t, want.LastDetailAt)) {
		t.Errorf("detail block span = %s..%s, want %s..%s",
			formatFixtureTime(blocks[0].at), formatFixtureTime(blocks[len(blocks)-1].at),
			want.FirstDetailAt, want.LastDetailAt)
	}
	if got.HasDetail != want.HasDetail {
		t.Errorf("workout hasDetail = %t, want %t", got.HasDetail, want.HasDetail)
	}
}

func checkOptionalInt(t *testing.T, name string, got *int, want int) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s is absent, want %d", name, want)
	}
	if *got != want {
		t.Errorf("%s = %d, want %d", name, *got, want)
	}
}
