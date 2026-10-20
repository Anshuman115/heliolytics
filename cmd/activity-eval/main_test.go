package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type exportFile struct {
	steps  []string
	stages []string
	sports []string
	hr     []string
}

func writeExport(t *testing.T, files exportFile) Sources {
	t.Helper()
	dir := t.TempDir()
	sources := Sources{
		Steps:     filepath.Join(dir, "steps.csv"),
		Stages:    filepath.Join(dir, "stages.csv"),
		Workouts:  filepath.Join(dir, "sport.csv"),
		HeartRate: filepath.Join(dir, "hr.csv"),
	}
	write := func(path, header string, rows []string) {
		content := header + "\n"
		if len(rows) > 0 {
			content += strings.Join(rows, "\n") + "\n"
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	write(sources.Steps, "date,time,steps", files.steps)
	write(sources.Stages, "date,start,stop,distance,calories,steps", files.stages)
	write(sources.Workouts, "type,startTime,sportTime(s)", files.sports)
	write(sources.HeartRate, "date,time,heartRate", files.hr)
	return sources
}

// minuteRows returns contiguous observed minutes starting at start.
func minuteRows(t *testing.T, date, start string, count, steps int) []string {
	t.Helper()
	begin, err := time.ParseInLocation("2006-01-02 15:04", date+" "+start, ist)
	if err != nil {
		t.Fatalf("parse minute start: %v", err)
	}
	rows := make([]string, 0, count)
	for i := range count {
		at := begin.Add(time.Duration(i) * time.Minute)
		rows = append(rows, fmt.Sprintf("%s,%s,%d", at.Format("2006-01-02"), at.Format("15:04"), steps))
	}
	return rows
}

func sourcesArgs(sources Sources) []string {
	return []string{
		"--steps", sources.Steps,
		"--stages", sources.Stages,
		"--workouts", sources.Workouts,
		"--hr", sources.HeartRate,
	}
}

func selectToFile(t *testing.T, sources Sources, out string) int {
	t.Helper()
	args := append([]string{"select"}, sourcesArgs(sources)...)
	args = append(args, "--out", out)
	var stdout, stderr bytes.Buffer
	return run(args, &stdout, &stderr)
}

func evaluateToFile(t *testing.T, sources Sources, selection, out, holdoutFrom, holdoutTo string) int {
	t.Helper()
	args := append([]string{"evaluate", "--selection", selection}, sourcesArgs(sources)...)
	args = append(args, "--holdout-from", holdoutFrom, "--holdout-to", holdoutTo, "--out", out)
	var stdout, stderr bytes.Buffer
	return run(args, &stdout, &stderr)
}

func runCommand(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestRunDispatch(t *testing.T) {
	if code, _, _ := runCommand(t); code != exitInvalid {
		t.Fatalf("no command exit=%d want %d", code, exitInvalid)
	}
	if code, stdout, _ := runCommand(t, "help"); code != exitOK || !strings.Contains(stdout, "activity-eval select") {
		t.Fatalf("help exit=%d stdout=%q", code, stdout)
	}
	if code, _, _ := runCommand(t, "bogus"); code != exitInvalid {
		t.Fatalf("unknown command exit=%d want %d", code, exitInvalid)
	}
}

func TestSelectRequiresArguments(t *testing.T) {
	sources := writeExport(t, exportFile{
		steps:  minuteRows(t, "2026-02-10", "10:00", 20, 120),
		stages: []string{"2026-02-10,10:00,10:20,0,0,2400"},
	})
	out := filepath.Join(t.TempDir(), "selection.json")
	if code, _, _ := runCommand(t, append([]string{"select"}, sourcesArgs(sources)...)...); code != exitInvalid {
		t.Fatalf("missing --out exit=%d want %d", code, exitInvalid)
	}
	args := append([]string{"select"}, sourcesArgs(sources)...)
	args = append(args, "--out", out, "--bogus-flag")
	if code, _, _ := runCommand(t, args...); code != exitInvalid {
		t.Fatalf("unknown flag exit=%d want %d", code, exitInvalid)
	}
	if code, _, _ := runCommand(t, "select", "--out", out); code != exitInvalid {
		t.Fatalf("missing sources exit=%d want %d", code, exitInvalid)
	}
}

func TestEvaluateRejectsGridFlags(t *testing.T) {
	sources := writeExport(t, exportFile{
		steps:  minuteRows(t, "2026-02-10", "10:00", 20, 120),
		stages: []string{"2026-02-10,10:00,10:20,0,0,2400"},
	})
	out := filepath.Join(t.TempDir(), "selection.json")
	if code := selectToFile(t, sources, out); code != exitOK {
		t.Fatalf("select exit=%d want %d", code, exitOK)
	}
	args := append([]string{"evaluate", "--selection", out}, sourcesArgs(sources)...)
	args = append(args, "--holdout-from", "2026-05-01", "--holdout-to", "2026-06-23",
		"--out", filepath.Join(t.TempDir(), "holdout.json"), "--cadence", "70")
	if code, _, _ := runCommand(t, args...); code != exitInvalid {
		t.Fatalf("grid flag exit=%d want %d", code, exitInvalid)
	}
}

func TestSelectAndEvaluateMeetTarget(t *testing.T) {
	files := exportFile{
		steps: append(
			minuteRows(t, "2026-02-10", "10:00", 20, 120),
			minuteRows(t, "2026-06-01", "10:00", 20, 120)...,
		),
		stages: []string{
			"2026-02-10,10:00,10:20,0,0,2400",
			"2026-06-01,10:00,10:20,0,0,2400",
		},
	}
	sources := writeExport(t, files)
	dir := t.TempDir()
	selectionPath := filepath.Join(dir, "selection.json")
	if code := selectToFile(t, sources, selectionPath); code != exitOK {
		t.Fatalf("select exit=%d want %d", code, exitOK)
	}
	selection, _, err := loadSelection(selectionPath)
	if err != nil {
		t.Fatalf("load selection: %v", err)
	}
	if len(selection.Candidates) != 36 {
		t.Fatalf("candidates=%d want 36", len(selection.Candidates))
	}
	if selection.DevelopmentScore.TP != 1 || selection.DevelopmentScore.FP != 0 || selection.DevelopmentScore.FN != 0 {
		t.Fatalf("development tp=%d fp=%d fn=%d want 1/0/0",
			selection.DevelopmentScore.TP, selection.DevelopmentScore.FP, selection.DevelopmentScore.FN)
	}
	if selection.DevelopmentScore.PredictionCount != 1 {
		t.Fatalf("development predictions=%d want 1 (holdout run must be excluded)",
			selection.DevelopmentScore.PredictionCount)
	}

	holdoutPath := filepath.Join(dir, "holdout.json")
	if code := evaluateToFile(t, sources, selectionPath, holdoutPath, "2026-05-01", "2026-06-23"); code != exitOK {
		t.Fatalf("evaluate exit=%d want %d", code, exitOK)
	}
	report := readEvaluation(t, holdoutPath)
	if !report.Gate.Passed {
		t.Fatalf("gate did not pass: %s", report.Gate.Reason)
	}
	holdout := report.Partitions[1]
	if holdout.Name != "holdout" || holdout.Score.TP != 1 || holdout.Score.FP != 0 || holdout.Score.FN != 0 {
		t.Fatalf("holdout=%s tp=%d fp=%d fn=%d want holdout 1/0/0",
			holdout.Name, holdout.Score.TP, holdout.Score.FP, holdout.Score.FN)
	}
	if report.Partitions[0].Score.TP != 1 {
		t.Fatalf("development tp=%d want 1", report.Partitions[0].Score.TP)
	}
}

func TestEvaluateMissesTargetWithoutHoldoutEvidence(t *testing.T) {
	sources := writeExport(t, exportFile{
		steps:  minuteRows(t, "2026-02-10", "10:00", 20, 120),
		stages: []string{"2026-02-10,10:00,10:20,0,0,2400"},
	})
	dir := t.TempDir()
	selectionPath := filepath.Join(dir, "selection.json")
	if code := selectToFile(t, sources, selectionPath); code != exitOK {
		t.Fatalf("select exit=%d want %d", code, exitOK)
	}
	holdoutPath := filepath.Join(dir, "holdout.json")
	if code := evaluateToFile(t, sources, selectionPath, holdoutPath, "2026-05-01", "2026-06-23"); code != exitTarget {
		t.Fatalf("evaluate exit=%d want %d", code, exitTarget)
	}
	report := readEvaluation(t, holdoutPath)
	if report.Gate.Passed {
		t.Fatal("gate passed without holdout evidence")
	}
	if !strings.Contains(report.Gate.Reason, "insufficient evidence") {
		t.Fatalf("reason=%q want an insufficient-evidence reason", report.Gate.Reason)
	}
}

func TestEvaluateRejectsOverlappingBounds(t *testing.T) {
	sources := writeExport(t, exportFile{
		steps:  minuteRows(t, "2026-02-10", "10:00", 20, 120),
		stages: []string{"2026-02-10,10:00,10:20,0,0,2400"},
	})
	dir := t.TempDir()
	selectionPath := filepath.Join(dir, "selection.json")
	if code := selectToFile(t, sources, selectionPath); code != exitOK {
		t.Fatalf("select exit=%d want %d", code, exitOK)
	}
	out := filepath.Join(dir, "overlap.json")
	if code := evaluateToFile(t, sources, selectionPath, out, "2026-04-01", "2026-06-23"); code != exitInvalid {
		t.Fatalf("overlapping bounds exit=%d want %d", code, exitInvalid)
	}
	if _, err := os.Stat(out); err == nil {
		t.Fatal("overlapping bounds still wrote a report")
	}
}

func TestEvaluateAcceptsAdjacentBounds(t *testing.T) {
	sources := writeExport(t, exportFile{
		steps:  minuteRows(t, "2026-02-10", "10:00", 20, 120),
		stages: []string{"2026-02-10,10:00,10:20,0,0,2400"},
	})
	dir := t.TempDir()
	selectionPath := filepath.Join(dir, "selection.json")
	if code := selectToFile(t, sources, selectionPath); code != exitOK {
		t.Fatalf("select exit=%d want %d", code, exitOK)
	}
	out := filepath.Join(dir, "adjacent.json")
	code := evaluateToFile(t, sources, selectionPath, out, "2026-05-01", "2026-06-23")
	if code == exitInvalid {
		t.Fatalf("adjacent bounds exit=%d want a measured result", code)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("adjacent bounds did not write a report: %v", err)
	}
}

func TestEvaluateRejectsSourceDrift(t *testing.T) {
	sources := writeExport(t, exportFile{
		steps:  minuteRows(t, "2026-02-10", "10:00", 20, 120),
		stages: []string{"2026-02-10,10:00,10:20,0,0,2400"},
	})
	dir := t.TempDir()
	selectionPath := filepath.Join(dir, "selection.json")
	if code := selectToFile(t, sources, selectionPath); code != exitOK {
		t.Fatalf("select exit=%d want %d", code, exitOK)
	}
	handle, err := os.OpenFile(sources.Steps, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open steps: %v", err)
	}
	if _, err := handle.WriteString("2026-04-30,23:59,5\n"); err != nil {
		t.Fatalf("append steps: %v", err)
	}
	if err := handle.Close(); err != nil {
		t.Fatalf("close steps: %v", err)
	}
	out := filepath.Join(dir, "drift.json")
	if code := evaluateToFile(t, sources, selectionPath, out, "2026-05-01", "2026-06-23"); code != exitInvalid {
		t.Fatalf("source drift exit=%d want %d", code, exitInvalid)
	}
}

func TestEvaluateRejectsModifiedParameters(t *testing.T) {
	sources := writeExport(t, exportFile{
		steps:  minuteRows(t, "2026-02-10", "10:00", 20, 120),
		stages: []string{"2026-02-10,10:00,10:20,0,0,2400"},
	})
	dir := t.TempDir()
	selectionPath := filepath.Join(dir, "selection.json")
	if code := selectToFile(t, sources, selectionPath); code != exitOK {
		t.Fatalf("select exit=%d want %d", code, exitOK)
	}
	original, err := os.ReadFile(selectionPath)
	if err != nil {
		t.Fatalf("read selection: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(original, &document); err != nil {
		t.Fatalf("decode selection: %v", err)
	}
	frozen, ok := document["frozen"].(map[string]any)
	if !ok {
		t.Fatal("selection artifact has no frozen object")
	}
	frozen["cadenceMin"] = 61.0
	modified, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		t.Fatalf("encode selection: %v", err)
	}
	if err := os.WriteFile(selectionPath, modified, 0o644); err != nil {
		t.Fatalf("rewrite selection: %v", err)
	}
	out := filepath.Join(dir, "modified.json")
	if code := evaluateToFile(t, sources, selectionPath, out, "2026-05-01", "2026-06-23"); code != exitInvalid {
		t.Fatalf("modified parameters exit=%d want %d", code, exitInvalid)
	}
}

func TestSelectIgnoresHoldoutLabels(t *testing.T) {
	files := exportFile{
		steps: append(
			minuteRows(t, "2026-02-10", "10:00", 20, 120),
			minuteRows(t, "2026-06-01", "10:00", 20, 120)...,
		),
		stages: []string{
			"2026-02-10,10:00,10:20,0,0,2400",
			"2026-06-01,10:00,10:20,0,0,2400",
		},
	}
	sources := writeExport(t, files)
	dir := t.TempDir()
	firstPath := filepath.Join(dir, "first.json")
	if code := selectToFile(t, sources, firstPath); code != exitOK {
		t.Fatalf("first select exit=%d", code)
	}
	first, _, err := loadSelection(firstPath)
	if err != nil {
		t.Fatalf("load first selection: %v", err)
	}

	// Radically different holdout labels: one very long stage and no dev change.
	files.stages = []string{
		"2026-02-10,10:00,10:20,0,0,2400",
		"2026-06-01,00:00,23:59,9999,9999,99999",
	}
	changedSources := writeExport(t, files)
	secondPath := filepath.Join(dir, "second.json")
	if code := selectToFile(t, changedSources, secondPath); code != exitOK {
		t.Fatalf("second select exit=%d", code)
	}
	second, _, err := loadSelection(secondPath)
	if err != nil {
		t.Fatalf("load second selection: %v", err)
	}
	if !reflect.DeepEqual(first.Frozen, second.Frozen) {
		t.Fatalf("frozen parameters changed: %+v vs %+v", first.Frozen, second.Frozen)
	}
	if !reflect.DeepEqual(first.Candidates, second.Candidates) {
		t.Fatal("candidate scores changed after rewriting holdout labels")
	}
	if !reflect.DeepEqual(first.DevelopmentScore, second.DevelopmentScore) {
		t.Fatal("development score changed after rewriting holdout labels")
	}
}

func TestSelectIsDeterministic(t *testing.T) {
	files := exportFile{
		steps: append(
			minuteRows(t, "2026-02-10", "10:00", 20, 120),
			minuteRows(t, "2026-03-03", "07:00", 15, 90)...,
		),
		stages: []string{
			"2026-02-10,10:00,10:20,0,0,2400",
			"2026-03-03,07:00,07:15,0,0,1350",
		},
	}
	sources := writeExport(t, files)
	dir := t.TempDir()
	firstPath := filepath.Join(dir, "first.json")
	secondPath := filepath.Join(dir, "second.json")
	if code := selectToFile(t, sources, firstPath); code != exitOK {
		t.Fatalf("first select exit=%d", code)
	}
	if code := selectToFile(t, sources, secondPath); code != exitOK {
		t.Fatalf("second select exit=%d", code)
	}
	first, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatalf("read first artifact: %v", err)
	}
	second, err := os.ReadFile(secondPath)
	if err != nil {
		t.Fatalf("read second artifact: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("selection artifact is not byte-identical across runs")
	}
}

func TestBoundaryCrossingIntervalsAreExcluded(t *testing.T) {
	files := exportFile{
		steps: append(
			minuteRows(t, "2026-02-10", "10:00", 20, 120),
			minuteRows(t, "2026-04-30", "23:55", 10, 120)...,
		),
		stages: []string{
			"2026-02-10,10:00,10:20,0,0,2400",
			"2026-04-30,23:50,00:10,0,0,2400",
		},
	}
	sources := writeExport(t, files)
	dir := t.TempDir()
	selectionPath := filepath.Join(dir, "selection.json")
	if code := selectToFile(t, sources, selectionPath); code != exitOK {
		t.Fatalf("select exit=%d", code)
	}
	selection, _, err := loadSelection(selectionPath)
	if err != nil {
		t.Fatalf("load selection: %v", err)
	}
	development := selection.DevelopmentScore
	if development.CrossedPredictions != 1 || development.CrossedReferences != 1 {
		t.Fatalf("crossed predictions=%d references=%d want 1 and 1",
			development.CrossedPredictions, development.CrossedReferences)
	}
	if development.PredictionCount != 1 || development.ReferenceCount != 1 {
		t.Fatalf("scored predictions=%d references=%d want the contained pair only",
			development.PredictionCount, development.ReferenceCount)
	}
	if development.TP != 1 || development.FP != 0 || development.FN != 0 {
		t.Fatalf("tp=%d fp=%d fn=%d want 1/0/0", development.TP, development.FP, development.FN)
	}

	holdoutPath := filepath.Join(dir, "holdout.json")
	if code := evaluateToFile(t, sources, selectionPath, holdoutPath, "2026-05-01", "2026-06-23"); code != exitTarget {
		t.Fatalf("evaluate exit=%d want %d for a holdout with only crossing intervals", code, exitTarget)
	}
	report := readEvaluation(t, holdoutPath)
	holdout := report.Partitions[1].Score
	if holdout.CrossedPredictions != 1 || holdout.CrossedReferences != 1 {
		t.Fatalf("holdout crossed predictions=%d references=%d want 1 and 1",
			holdout.CrossedPredictions, holdout.CrossedReferences)
	}
	if holdout.PredictionCount != 0 || holdout.ReferenceCount != 0 {
		t.Fatalf("holdout scored predictions=%d references=%d want none",
			holdout.PredictionCount, holdout.ReferenceCount)
	}
}

func readEvaluation(t *testing.T, path string) evaluationArtifact {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read evaluation: %v", err)
	}
	var report evaluationArtifact
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("decode evaluation: %v", err)
	}
	if report.Kind != "activity-evaluation" {
		t.Fatalf("kind=%q want activity-evaluation", report.Kind)
	}
	if len(report.Partitions) != 2 {
		t.Fatalf("partitions=%d want 2", len(report.Partitions))
	}
	return report
}
