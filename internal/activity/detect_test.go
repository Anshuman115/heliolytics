package activity

import (
	"bytes"
	"encoding/json"
	"os"
	"strconv"
	"testing"
	"time"
)

type caseParams struct {
	CadenceMin  int `json:"cadenceMin"`
	GapMax      int `json:"gapMax"`
	DurationMin int `json:"durationMin"`
	RunCadence  int `json:"runCadence"`
}

type caseFile struct {
	Start  string      `json:"start"`
	Params caseParams  `json:"params"`
	Cases  []caseEntry `json:"cases"`
}

type caseEntry struct {
	Name     string      `json:"name"`
	Start    string      `json:"start"`
	Params   *caseParams `json:"params"`
	Steps    []*int      `json:"steps"`
	HRs      []*int      `json:"hrs"`
	Workouts []caseSpan  `json:"workouts"`
	Expect   []caseWant  `json:"expect"`
}

type caseSpan struct {
	StartSec int `json:"startSec"`
	EndSec   int `json:"endSec"`
}

type caseWant struct {
	StartSec int    `json:"startSec"`
	EndSec   int    `json:"endSec"`
	Steps    int    `json:"steps"`
	AvgHR    *int   `json:"avgHr"`
	MaxHR    *int   `json:"maxHr"`
	MinHR    *int   `json:"minHr"`
	Label    string `json:"label"`
}

func (p caseParams) toParams() Params {
	return Params{CadenceMin: p.CadenceMin, GapMax: p.GapMax, DurationMin: p.DurationMin, RunCadence: p.RunCadence}
}

func loadCases(t *testing.T) caseFile {
	t.Helper()
	data, err := os.ReadFile("testdata/cases.json")
	if err != nil {
		t.Fatalf("read cases: %v", err)
	}
	var file caseFile
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&file); err != nil {
		t.Fatalf("decode cases: %v", err)
	}
	if len(file.Cases) == 0 {
		t.Fatal("case file has no cases")
	}
	return file
}

func TestDetectorCases(t *testing.T) {
	file := loadCases(t)
	base, err := time.Parse(time.RFC3339, file.Start)
	if err != nil {
		t.Fatalf("case file start: %v", err)
	}
	for _, entry := range file.Cases {
		entry := entry
		t.Run(entry.Name, func(t *testing.T) {
			start := base
			if entry.Start != "" {
				parsed, err := time.Parse(time.RFC3339, entry.Start)
				if err != nil {
					t.Fatalf("case start: %v", err)
				}
				start = parsed
			}
			params := file.Params.toParams()
			if entry.Params != nil {
				params = entry.Params.toParams()
			}
			minutes := buildMinutes(start, entry.Steps, entry.HRs)
			workouts := buildWorkouts(start, entry.Workouts)
			got := DetectWithParams(minutes, workouts, params)
			if len(got) != len(entry.Expect) {
				t.Fatalf("sessions=%d want %d (%+v)", len(got), len(entry.Expect), got)
			}
			for i, want := range entry.Expect {
				session := got[i]
				if wantStart := start.Add(time.Duration(want.StartSec) * time.Second); !session.Start.Equal(wantStart) {
					t.Errorf("session %d start=%s want %s", i, session.Start.Format(time.RFC3339), wantStart.Format(time.RFC3339))
				}
				if wantEnd := start.Add(time.Duration(want.EndSec) * time.Second); !session.End.Equal(wantEnd) {
					t.Errorf("session %d end=%s want %s", i, session.End.Format(time.RFC3339), wantEnd.Format(time.RFC3339))
				}
				if session.Steps != want.Steps {
					t.Errorf("session %d steps=%d want %d", i, session.Steps, want.Steps)
				}
				if session.Label != want.Label {
					t.Errorf("session %d label=%q want %q", i, session.Label, want.Label)
				}
				if !pointerEqual(session.AvgHR, want.AvgHR) {
					t.Errorf("session %d avgHr=%s want %s", i, describePointer(session.AvgHR), describePointer(want.AvgHR))
				}
				if !pointerEqual(session.MaxHR, want.MaxHR) {
					t.Errorf("session %d maxHr=%s want %s", i, describePointer(session.MaxHR), describePointer(want.MaxHR))
				}
				if !pointerEqual(session.MinHR, want.MinHR) {
					t.Errorf("session %d minHr=%s want %s", i, describePointer(session.MinHR), describePointer(want.MinHR))
				}
			}
		})
	}
}

func TestDetectLeavesInputsUntouched(t *testing.T) {
	start := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	minutes := make([]Minute, 24)
	for i := range minutes {
		bpm := 90 + i
		minutes[i] = Minute{At: start.Add(time.Duration(i) * time.Minute), Steps: 100, HR: &bpm}
	}
	workouts := []Interval{
		{Start: start.Add(10 * time.Minute), End: start.Add(14 * time.Minute)},
		{Start: start.Add(12 * time.Minute), End: start.Add(13 * time.Minute)},
		{Start: start.Add(20 * time.Minute), End: start.Add(21 * time.Minute)},
	}
	minutesBefore := append([]Minute(nil), minutes...)
	workoutsBefore := append([]Interval(nil), workouts...)

	first := Detect(minutes, workouts)
	second := Detect(minutes, workouts)
	if len(first) != len(second) {
		t.Fatalf("repeat sessions=%d want %d", len(second), len(first))
	}
	for i := range first {
		if !first[i].Start.Equal(second[i].Start) || !first[i].End.Equal(second[i].End) ||
			first[i].Steps != second[i].Steps || first[i].Label != second[i].Label ||
			!pointerEqual(first[i].AvgHR, second[i].AvgHR) {
			t.Errorf("session %d differs between identical calls", i)
		}
	}
	for i := range minutes {
		if !minutes[i].At.Equal(minutesBefore[i].At) || minutes[i].Steps != minutesBefore[i].Steps {
			t.Fatalf("minute %d mutated", i)
		}
		if !pointerEqual(minutes[i].HR, minutesBefore[i].HR) {
			t.Fatalf("minute %d HR mutated", i)
		}
	}
	for i := range workouts {
		if !workouts[i].Start.Equal(workoutsBefore[i].Start) || !workouts[i].End.Equal(workoutsBefore[i].End) {
			t.Fatalf("workout %d mutated or reordered", i)
		}
	}
	if len(first) != 1 {
		t.Fatalf("sessions=%d want 1", len(first))
	}
	if !first[0].Start.Equal(start) || !first[0].End.Equal(start.Add(10*time.Minute)) {
		t.Fatalf("session bounds=%s..%s want the first workout-free run", first[0].Start, first[0].End)
	}
}

func TestDefaultParamsAreUsable(t *testing.T) {
	params := DefaultParams()
	if params.CadenceMin <= 0 || params.GapMax < 0 || params.DurationMin <= 0 || params.RunCadence <= 0 {
		t.Fatalf("default params not usable: %+v", params)
	}
	if params.RunCadence <= params.CadenceMin {
		t.Fatalf("run cadence %d is not above movement cadence %d", params.RunCadence, params.CadenceMin)
	}
}

func buildMinutes(start time.Time, steps, hrs []*int) []Minute {
	minutes := make([]Minute, 0, len(steps))
	for i, value := range steps {
		if value == nil {
			continue
		}
		minute := Minute{At: start.Add(time.Duration(i) * time.Minute), Steps: *value}
		if i < len(hrs) && hrs[i] != nil {
			bpm := *hrs[i]
			minute.HR = &bpm
		}
		minutes = append(minutes, minute)
	}
	return minutes
}

func buildWorkouts(start time.Time, spans []caseSpan) []Interval {
	workouts := make([]Interval, 0, len(spans))
	for _, span := range spans {
		workouts = append(workouts, Interval{
			Start: start.Add(time.Duration(span.StartSec) * time.Second),
			End:   start.Add(time.Duration(span.EndSec) * time.Second),
		})
	}
	return workouts
}

func pointerEqual(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func describePointer(value *int) string {
	if value == nil {
		return "nil"
	}
	return strconv.Itoa(*value)
}
