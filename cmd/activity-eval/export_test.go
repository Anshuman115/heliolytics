package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Anshuman115/heliolytics/internal/activity"
)

const fixtureDir = "../../internal/activity/testdata"

func fixture(name string) string {
	return filepath.Join(fixtureDir, name)
}

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func TestLoadStepsTrimmedFixture(t *testing.T) {
	rows, hash, err := LoadSteps(fixture("zepp_steps_trimmed.csv"))
	if err != nil {
		t.Fatalf("load steps: %v", err)
	}
	if len(hash) != 64 {
		t.Fatalf("hash=%q is not a sha256 hex digest", hash)
	}
	if len(rows) != 10 {
		t.Fatalf("rows=%d want 10", len(rows))
	}
	first := rows[0]
	if want := time.Date(2026, 6, 22, 2, 28, 0, 0, time.UTC); !first.At.Equal(want) {
		t.Fatalf("first minute=%s want %s (IST 07:58)", first.At.Format(time.RFC3339), want.Format(time.RFC3339))
	}
	if first.Steps != 34 {
		t.Fatalf("first steps=%d want 34", first.Steps)
	}
	for i := 1; i < len(rows); i++ {
		if rows[i].At.Before(rows[i-1].At) {
			t.Fatalf("rows are not sorted at %d", i)
		}
	}
}

func TestLoadStagesTrimmedFixture(t *testing.T) {
	rows, _, err := LoadStages(fixture("zepp_stages_trimmed.csv"))
	if err != nil {
		t.Fatalf("load stages: %v", err)
	}
	if len(rows) != 4 {
		t.Fatalf("rows=%d want 4", len(rows))
	}
	var lowStep *StageRow
	for i := range rows {
		if rows[i].Steps == 158 {
			lowStep = &rows[i]
		}
	}
	if lowStep == nil {
		t.Fatal("low-step 18:00 stage was not loaded")
	}
	if want := time.Date(2026, 6, 22, 12, 30, 0, 0, time.UTC); !lowStep.Start.Equal(want) {
		t.Fatalf("stage start=%s want %s", lowStep.Start.Format(time.RFC3339), want.Format(time.RFC3339))
	}
	if want := time.Date(2026, 6, 22, 12, 44, 0, 0, time.UTC); !lowStep.End.Equal(want) {
		t.Fatalf("stage end=%s want %s", lowStep.End.Format(time.RFC3339), want.Format(time.RFC3339))
	}
}

func TestLoadStagesRollover(t *testing.T) {
	path := writeTemp(t, "stages.csv", "date,start,stop,steps\n2026-06-21,23:50,00:10,120\n")
	rows, _, err := LoadStages(path)
	if err != nil {
		t.Fatalf("load rollover stage: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows=%d want 1", len(rows))
	}
	duration := rows[0].End.Sub(rows[0].Start)
	if duration != 20*time.Minute {
		t.Fatalf("duration=%s want 20m0s", duration)
	}
}

func TestLoadStagesRejectsEqualBounds(t *testing.T) {
	path := writeTemp(t, "stages.csv", "date,start,stop,steps\n2026-06-21,10:00,10:00,10\n")
	if _, _, err := LoadStages(path); err == nil {
		t.Fatal("equal stage bounds were accepted")
	}
}

func TestLoadStepsRejectsConflictingDuplicate(t *testing.T) {
	path := writeTemp(t, "steps.csv", "date,time,steps\n2026-06-21,10:00,40\n2026-06-21,10:00,41\n")
	_, _, err := LoadSteps(path)
	if err == nil {
		t.Fatal("conflicting duplicate minute was accepted")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("error %q does not report the duplicate", err)
	}
}

func TestLoadStepsRejectsInvalidTime(t *testing.T) {
	path := writeTemp(t, "steps.csv", "date,time,steps\n2026-06-21,24:00,40\n")
	if _, _, err := LoadSteps(path); err == nil {
		t.Fatal("invalid time was accepted")
	}
}

func TestLoadStepsRejectsMissingColumn(t *testing.T) {
	path := writeTemp(t, "steps.csv", "date,time\n2026-06-21,10:00\n")
	_, _, err := LoadSteps(path)
	if err == nil || !strings.Contains(err.Error(), "missing column") {
		t.Fatalf("error=%v, want a missing-column error", err)
	}
}

func TestLoadWorkoutsTrimmedFixture(t *testing.T) {
	rows, _, err := LoadWorkouts(fixture("zepp_sport_trimmed.csv"))
	if err != nil {
		t.Fatalf("load workouts: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows=%d want 2", len(rows))
	}
	if want := time.Date(2026, 6, 20, 14, 39, 0, 0, time.UTC); !rows[0].Start.Equal(want) {
		t.Fatalf("first start=%s want %s", rows[0].Start.Format(time.RFC3339), want.Format(time.RFC3339))
	}
	var long WorkoutRow
	for _, row := range rows {
		if row.SportType == 92 && row.Start.Equal(time.Date(2026, 6, 22, 2, 20, 16, 0, time.UTC)) {
			long = row
		}
	}
	if long.End.Sub(long.Start) != 5383*time.Second {
		t.Fatalf("duration=%s want 5383s", long.End.Sub(long.Start))
	}
	if want := time.Date(2026, 6, 22, 3, 49, 59, 0, time.UTC); !long.End.Equal(want) {
		t.Fatalf("end=%s want %s", long.End.Format(time.RFC3339), want.Format(time.RFC3339))
	}
}

func TestLoadHeartRateTrimmedFixture(t *testing.T) {
	rows, ignored, _, err := LoadHeartRate(fixture("zepp_hr_trimmed.csv"))
	if err != nil {
		t.Fatalf("load heart rate: %v", err)
	}
	if ignored != 0 {
		t.Fatalf("ignored=%d want 0", ignored)
	}
	if len(rows) != 9 {
		t.Fatalf("rows=%d want 9", len(rows))
	}
	if rows[0].BPM != 84 {
		t.Fatalf("first bpm=%d want 84", rows[0].BPM)
	}
}

func TestLoadHeartRateIgnoresOutOfRange(t *testing.T) {
	path := writeTemp(t, "hr.csv", "date,time,heartRate\n2026-06-21,10:00,0\n2026-06-21,10:01,251\n2026-06-21,10:02,88\n")
	rows, ignored, _, err := LoadHeartRate(path)
	if err != nil {
		t.Fatalf("load heart rate: %v", err)
	}
	if ignored != 2 || len(rows) != 1 {
		t.Fatalf("rows=%d ignored=%d want 1 and 2", len(rows), ignored)
	}
}

func TestLoadExportRequiresSources(t *testing.T) {
	if _, err := LoadExport(Sources{}); err == nil {
		t.Fatal("empty sources were accepted")
	}
	sources := Sources{
		Steps:     fixture("zepp_steps_trimmed.csv"),
		Stages:    fixture("zepp_stages_trimmed.csv"),
		Workouts:  fixture("zepp_sport_trimmed.csv"),
		HeartRate: fixture("zepp_hr_trimmed.csv"),
	}
	export, err := LoadExport(sources)
	if err != nil {
		t.Fatalf("load export: %v", err)
	}
	if len(export.Hashes) != 4 {
		t.Fatalf("hashes=%d want 4", len(export.Hashes))
	}
	sources.HeartRate = ""
	export, err = LoadExport(sources)
	if err != nil {
		t.Fatalf("load export without hr: %v", err)
	}
	if len(export.Hashes) != 3 {
		t.Fatalf("hashes=%d want 3", len(export.Hashes))
	}
}

func TestReferencesKeepLowStepStage(t *testing.T) {
	stages, _, err := LoadStages(fixture("zepp_stages_trimmed.csv"))
	if err != nil {
		t.Fatalf("load stages: %v", err)
	}
	refs, stats := References(stages, nil, 10*time.Minute)
	if stats.RawCount != 2 {
		t.Fatalf("raw stages=%d want 2 (5m and 9m stages are below the minimum)", stats.RawCount)
	}
	if stats.EligibleCount != 2 || len(refs) != 2 {
		t.Fatalf("eligible=%d refs=%d want 2", stats.EligibleCount, len(refs))
	}
	found := false
	for _, ref := range refs {
		if ref.End.Sub(ref.Start) == 14*time.Minute {
			found = true
		}
	}
	if !found {
		t.Fatal("the 14-minute 158-step stage was dropped")
	}
}

func TestReferencesSubtractWorkouts(t *testing.T) {
	start := time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC)
	stages := []StageRow{
		{Start: start, End: start.Add(30 * time.Minute), Steps: 2400},
		{Start: start.Add(time.Hour), End: start.Add(70 * time.Minute), Steps: 800},
		{Start: start.Add(2 * time.Hour), End: start.Add(150 * time.Minute), Steps: 4000},
	}
	workouts := []activity.Interval{
		{Start: start.Add(10 * time.Minute), End: start.Add(20 * time.Minute)},
		{Start: start.Add(time.Hour), End: start.Add(70 * time.Minute)},
	}
	refs, stats := References(stages, workouts, 10*time.Minute)
	if stats.RawCount != 3 {
		t.Fatalf("raw=%d want 3", stats.RawCount)
	}
	if stats.ShortenedCount != 1 {
		t.Fatalf("shortened=%d want 1", stats.ShortenedCount)
	}
	if stats.ExcludedCount != 1 {
		t.Fatalf("excluded=%d want 1", stats.ExcludedCount)
	}
	if stats.EligibleCount != 3 || len(refs) != 3 {
		t.Fatalf("eligible=%d refs=%d want 2 residual halves plus the untouched stage", stats.EligibleCount, len(refs))
	}
	if stats.WorkoutExcludedMinutes != 20 {
		t.Fatalf("removed minutes=%d want 20", stats.WorkoutExcludedMinutes)
	}
	halves := 0
	for _, ref := range refs {
		switch ref.End.Sub(ref.Start) {
		case 10 * time.Minute:
			halves++
		case 30 * time.Minute:
		default:
			t.Fatalf("unexpected residual %s", ref.End.Sub(ref.Start))
		}
	}
	if halves != 2 {
		t.Fatalf("10-minute residuals=%d want 2", halves)
	}
}

func TestReferencesDropShortResidual(t *testing.T) {
	start := time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC)
	stages := []StageRow{{Start: start, End: start.Add(20 * time.Minute), Steps: 1600}}
	workouts := []activity.Interval{
		{Start: start.Add(4 * time.Minute), End: start.Add(16 * time.Minute)},
	}
	refs, stats := References(stages, workouts, 10*time.Minute)
	if len(refs) != 0 || stats.ShortResidualCount != 2 {
		t.Fatalf("refs=%d shortResiduals=%d want 0 and 2", len(refs), stats.ShortResidualCount)
	}
}

func TestSparseFixtureYieldsNoFabricatedSessions(t *testing.T) {
	sources := Sources{
		Steps:     fixture("zepp_steps_trimmed.csv"),
		Stages:    fixture("zepp_stages_trimmed.csv"),
		Workouts:  fixture("zepp_sport_trimmed.csv"),
		HeartRate: fixture("zepp_hr_trimmed.csv"),
	}
	export, err := LoadExport(sources)
	if err != nil {
		t.Fatalf("load export: %v", err)
	}
	sessions := activity.Detect(export.Minutes(), export.WorkoutIntervals())
	if len(sessions) != 0 {
		t.Fatalf("sessions=%d want 0: sparse minutes must not fabricate movement", len(sessions))
	}
	references, stats := References(export.Stages, export.WorkoutIntervals(), 10*time.Minute)
	if stats.RawCount != 2 || stats.ExcludedCount != 1 || stats.EligibleCount != 1 {
		t.Fatalf("raw=%d excluded=%d eligible=%d want 2/1/1: the 08:12 stage lies inside the workout",
			stats.RawCount, stats.ExcludedCount, stats.EligibleCount)
	}
	if got := references[0].End.Sub(references[0].Start); got != 14*time.Minute {
		t.Fatalf("reference=%s want the 14-minute 158-step stage", got)
	}
	score := ScoreIntervals(nil, references)
	if score.TP != 0 || score.FP != 0 || score.FN != 1 {
		t.Fatalf("tp=%d fp=%d fn=%d want the sparse reference as one false negative",
			score.TP, score.FP, score.FN)
	}
	if score.Precision != nil || score.Recall == nil || *score.Recall != 0 {
		t.Fatalf("precision=%s recall=%s want undefined precision and zero recall",
			describeMetric(score.Precision), describeMetric(score.Recall))
	}
}

func TestCoverageCounts(t *testing.T) {
	sources := Sources{
		Steps:    fixture("zepp_steps_trimmed.csv"),
		Stages:   fixture("zepp_stages_trimmed.csv"),
		Workouts: fixture("zepp_sport_trimmed.csv"),
	}
	export, err := LoadExport(sources)
	if err != nil {
		t.Fatalf("load export: %v", err)
	}
	from := time.Date(2026, 6, 22, 12, 30, 0, 0, time.UTC)
	coverage := export.CoverageOf(from, from.Add(15*time.Minute))
	if coverage.ExpectedMinutes != 15 {
		t.Fatalf("expected minutes=%d want 15", coverage.ExpectedMinutes)
	}
	if coverage.ObservedMinutes != 7 {
		t.Fatalf("observed minutes=%d want 7", coverage.ObservedMinutes)
	}
}
