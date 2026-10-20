package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Anshuman115/heliolytics/internal/activity"
)

var ist = time.FixedZone("IST", 5*60*60+30*60)

const (
	sourceSteps    = "steps"
	sourceStages   = "stages"
	sourceWorkouts = "workouts"
	sourceHR       = "hr"
)

const (
	minHeartRate = 25
	maxHeartRate = 250
)

type Sources struct {
	Steps     string
	Stages    string
	Workouts  string
	HeartRate string
}

type StepRow struct {
	At    time.Time
	Steps int
}

// StageRow is one ACTIVITY_STAGE interval in UTC; End is exclusive.
type StageRow struct {
	Start time.Time
	End   time.Time
	Steps int
}

type WorkoutRow struct {
	Start     time.Time
	End       time.Time
	SportType int
}

type HeartRateRow struct {
	At  time.Time
	BPM int
}

type Export struct {
	Steps            []StepRow
	Stages           []StageRow
	Workouts         []WorkoutRow
	HeartRate        []HeartRateRow
	HeartRateIgnored int
	Hashes           map[string]string
}

func LoadExport(sources Sources) (Export, error) {
	required := []struct {
		flag string
		path string
	}{
		{"--steps", sources.Steps},
		{"--stages", sources.Stages},
		{"--workouts", sources.Workouts},
	}
	for _, item := range required {
		if item.path == "" {
			return Export{}, fmt.Errorf("missing %s", item.flag)
		}
	}

	export := Export{Hashes: map[string]string{}}
	steps, hash, err := LoadSteps(sources.Steps)
	if err != nil {
		return Export{}, err
	}
	export.Steps, export.Hashes[sourceSteps] = steps, hash

	stages, hash, err := LoadStages(sources.Stages)
	if err != nil {
		return Export{}, err
	}
	export.Stages, export.Hashes[sourceStages] = stages, hash

	workouts, hash, err := LoadWorkouts(sources.Workouts)
	if err != nil {
		return Export{}, err
	}
	export.Workouts, export.Hashes[sourceWorkouts] = workouts, hash

	if sources.HeartRate != "" {
		hrRows, ignored, hash, err := LoadHeartRate(sources.HeartRate)
		if err != nil {
			return Export{}, err
		}
		export.HeartRate, export.HeartRateIgnored = hrRows, ignored
		export.Hashes[sourceHR] = hash
	}
	return export, nil
}

func (e Export) Minutes() []activity.Minute {
	heartRate := make(map[int64]int, len(e.HeartRate))
	for _, row := range e.HeartRate {
		heartRate[row.At.Unix()] = row.BPM
	}
	minutes := make([]activity.Minute, 0, len(e.Steps))
	for _, row := range e.Steps {
		minute := activity.Minute{At: row.At, Steps: row.Steps}
		if bpm, ok := heartRate[row.At.Unix()]; ok {
			value := bpm
			minute.HR = &value
		}
		minutes = append(minutes, minute)
	}
	return minutes
}

func (e Export) WorkoutIntervals() []activity.Interval {
	out := make([]activity.Interval, 0, len(e.Workouts))
	for _, row := range e.Workouts {
		out = append(out, activity.Interval{Start: row.Start, End: row.End})
	}
	return out
}

func LoadSteps(path string) ([]StepRow, string, error) {
	records, hash, err := readCSV(path)
	if err != nil {
		return nil, "", err
	}
	columns, err := headerIndex(path, records[0], "date", "time", "steps")
	if err != nil {
		return nil, "", err
	}
	seen := make(map[int64]int, len(records))
	rows := make([]StepRow, 0, len(records)-1)
	for i, record := range records[1:] {
		at, err := istMinute(cell(record, columns["date"]), cell(record, columns["time"]))
		if err != nil {
			return nil, "", rowError(path, i+2, err)
		}
		steps, err := strconv.Atoi(strings.TrimSpace(cell(record, columns["steps"])))
		if err != nil || steps < 0 {
			return nil, "", rowError(path, i+2, fmt.Errorf("invalid steps value"))
		}
		if previous, duplicate := seen[at.Unix()]; duplicate {
			return nil, "", rowError(path, i+2, fmt.Errorf("duplicate minute %s with steps %d and %d",
				at.Format(time.RFC3339), previous, steps))
		}
		seen[at.Unix()] = steps
		rows = append(rows, StepRow{At: at, Steps: steps})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].At.Before(rows[j].At) })
	return rows, hash, nil
}

func LoadStages(path string) ([]StageRow, string, error) {
	records, hash, err := readCSV(path)
	if err != nil {
		return nil, "", err
	}
	columns, err := headerIndex(path, records[0], "date", "start", "stop")
	if err != nil {
		return nil, "", err
	}
	stepColumn, hasSteps := columns["steps"]
	rows := make([]StageRow, 0, len(records)-1)
	for i, record := range records[1:] {
		start, end, err := stageBounds(
			cell(record, columns["date"]),
			cell(record, columns["start"]),
			cell(record, columns["stop"]),
		)
		if err != nil {
			return nil, "", rowError(path, i+2, err)
		}
		row := StageRow{Start: start, End: end}
		if hasSteps {
			steps, err := strconv.Atoi(strings.TrimSpace(cell(record, stepColumn)))
			if err != nil || steps < 0 {
				return nil, "", rowError(path, i+2, fmt.Errorf("invalid steps value"))
			}
			row.Steps = steps
		}
		rows = append(rows, row)
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Start.Before(rows[j].Start) })
	return rows, hash, nil
}

func LoadWorkouts(path string) ([]WorkoutRow, string, error) {
	records, hash, err := readCSV(path)
	if err != nil {
		return nil, "", err
	}
	columns, err := headerIndex(path, records[0], "type", "startTime", "sportTime(s)")
	if err != nil {
		return nil, "", err
	}
	rows := make([]WorkoutRow, 0, len(records)-1)
	for i, record := range records[1:] {
		sportType, err := strconv.Atoi(strings.TrimSpace(cell(record, columns["type"])))
		if err != nil {
			return nil, "", rowError(path, i+2, fmt.Errorf("invalid sport type"))
		}
		start, err := time.Parse("2006-01-02 15:04:05-0700", strings.TrimSpace(cell(record, columns["startTime"])))
		if err != nil {
			return nil, "", rowError(path, i+2, fmt.Errorf("invalid startTime: %w", err))
		}
		seconds, err := strconv.Atoi(strings.TrimSpace(cell(record, columns["sportTime(s)"])))
		if err != nil || seconds <= 0 {
			return nil, "", rowError(path, i+2, fmt.Errorf("invalid sportTime(s)"))
		}
		start = start.UTC()
		rows = append(rows, WorkoutRow{
			Start:     start,
			End:       start.Add(time.Duration(seconds) * time.Second),
			SportType: sportType,
		})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Start.Before(rows[j].Start) })
	return rows, hash, nil
}

func LoadHeartRate(path string) ([]HeartRateRow, int, string, error) {
	records, hash, err := readCSV(path)
	if err != nil {
		return nil, 0, "", err
	}
	columns, err := headerIndex(path, records[0], "date", "time", "heartRate")
	if err != nil {
		return nil, 0, "", err
	}
	seen := make(map[int64]int, len(records))
	rows := make([]HeartRateRow, 0, len(records)-1)
	ignored := 0
	for i, record := range records[1:] {
		at, err := istMinute(cell(record, columns["date"]), cell(record, columns["time"]))
		if err != nil {
			return nil, 0, "", rowError(path, i+2, err)
		}
		bpm, err := strconv.Atoi(strings.TrimSpace(cell(record, columns["heartRate"])))
		if err != nil {
			return nil, 0, "", rowError(path, i+2, fmt.Errorf("invalid heartRate value"))
		}
		if previous, duplicate := seen[at.Unix()]; duplicate {
			return nil, 0, "", rowError(path, i+2, fmt.Errorf("duplicate minute %s with heartRate %d and %d",
				at.Format(time.RFC3339), previous, bpm))
		}
		seen[at.Unix()] = bpm
		if bpm < minHeartRate || bpm > maxHeartRate {
			ignored++
			continue
		}
		rows = append(rows, HeartRateRow{At: at, BPM: bpm})
	}
	return rows, ignored, hash, nil
}

type RefStats struct {
	RawCount               int
	ExcludedCount          int
	ShortenedCount         int
	ShortResidualCount     int
	EligibleCount          int
	WorkoutExcludedMinutes int
}

// References subtracts the workout union from every stage of at least minDuration
// and keeps the residuals that still reach minDuration.
func References(stages []StageRow, workouts []activity.Interval, minDuration time.Duration) ([]activity.Interval, RefStats) {
	cuts := append([]activity.Interval(nil), workouts...)
	sort.SliceStable(cuts, func(i, j int) bool { return cuts[i].Start.Before(cuts[j].Start) })

	var (
		refs  []activity.Interval
		stats RefStats
	)
	for _, stage := range stages {
		base := activity.Interval{Start: stage.Start, End: stage.End}
		if base.End.Sub(base.Start) < minDuration {
			continue
		}
		stats.RawCount++
		residuals := subtractIntervals(base, cuts)
		removed := base.End.Sub(base.Start)
		for _, residual := range residuals {
			removed -= residual.End.Sub(residual.Start)
		}
		stats.WorkoutExcludedMinutes += int(removed / time.Minute)
		if len(residuals) == 0 {
			stats.ExcludedCount++
			continue
		}
		if len(residuals) > 1 || !residuals[0].Start.Equal(base.Start) || !residuals[0].End.Equal(base.End) {
			stats.ShortenedCount++
		}
		for _, residual := range residuals {
			if residual.End.Sub(residual.Start) < minDuration {
				stats.ShortResidualCount++
				continue
			}
			refs = append(refs, residual)
		}
	}
	sort.SliceStable(refs, func(i, j int) bool { return refs[i].Start.Before(refs[j].Start) })
	stats.EligibleCount = len(refs)
	return refs, stats
}

func subtractIntervals(base activity.Interval, cuts []activity.Interval) []activity.Interval {
	var out []activity.Interval
	cursor := base.Start
	for _, cut := range cuts {
		if !cut.End.After(base.Start) || !cut.Start.Before(base.End) {
			continue
		}
		if cut.Start.After(cursor) {
			out = append(out, activity.Interval{Start: cursor, End: cut.Start})
		}
		if cut.End.After(cursor) {
			cursor = cut.End
		}
	}
	if cursor.Before(base.End) {
		out = append(out, activity.Interval{Start: cursor, End: base.End})
	}
	return out
}

type Coverage struct {
	From             time.Time
	To               time.Time
	ExpectedMinutes  int
	ObservedMinutes  int
	FirstObservation *time.Time
	LastObservation  *time.Time
}

func (e Export) CoverageOf(from, to time.Time) Coverage {
	coverage := Coverage{From: from, To: to, ExpectedMinutes: int(to.Sub(from) / time.Minute)}
	for _, row := range e.Steps {
		if row.At.Before(from) || !row.At.Before(to) {
			continue
		}
		coverage.ObservedMinutes++
		at := row.At
		if coverage.FirstObservation == nil || at.Before(*coverage.FirstObservation) {
			coverage.FirstObservation = &at
		}
		if coverage.LastObservation == nil || !at.Before(*coverage.LastObservation) {
			coverage.LastObservation = &at
		}
	}
	return coverage
}

func ParseDay(value string) (time.Time, error) {
	day, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(value), ist)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date %q: %w", value, err)
	}
	return day, nil
}

func readCSV(path string) ([][]string, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", path, err)
	}
	sum := sha256.Sum256(data)
	reader := csv.NewReader(bytes.NewReader(data))
	records, err := reader.ReadAll()
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", path, err)
	}
	if len(records) == 0 {
		return nil, "", fmt.Errorf("%s: empty file", path)
	}
	return records, hex.EncodeToString(sum[:]), nil
}

func headerIndex(path string, header []string, required ...string) (map[string]int, error) {
	columns := make(map[string]int, len(header))
	for i, name := range header {
		name = strings.TrimSpace(strings.TrimPrefix(name, "\ufeff"))
		if _, duplicate := columns[name]; duplicate {
			return nil, fmt.Errorf("%s: duplicate column %q", path, name)
		}
		columns[name] = i
	}
	for _, name := range required {
		if _, ok := columns[name]; !ok {
			return nil, fmt.Errorf("%s: missing column %q", path, name)
		}
	}
	return columns, nil
}

func cell(record []string, column int) string {
	if column >= len(record) {
		return ""
	}
	return record[column]
}

func rowError(path string, row int, err error) error {
	return fmt.Errorf("%s: row %d: %w", path, row, err)
}

func istMinute(date, clock string) (time.Time, error) {
	day, err := ParseDay(date)
	if err != nil {
		return time.Time{}, err
	}
	hourMinute, err := time.ParseInLocation("15:04", strings.TrimSpace(clock), ist)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid time %q: %w", clock, err)
	}
	return time.Date(day.Year(), day.Month(), day.Day(), hourMinute.Hour(), hourMinute.Minute(), 0, 0, ist).UTC(), nil
}

func stageClock(value string) (time.Duration, error) {
	parsed, err := time.ParseInLocation("15:04", strings.TrimSpace(value), ist)
	if err != nil {
		return 0, fmt.Errorf("invalid time %q: %w", value, err)
	}
	return time.Duration(parsed.Hour())*time.Hour + time.Duration(parsed.Minute())*time.Minute, nil
}

func stageBounds(date, startClock, stopClock string) (time.Time, time.Time, error) {
	day, err := ParseDay(date)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	startOffset, err := stageClock(startClock)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	stopOffset, err := stageClock(stopClock)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if stopOffset == startOffset {
		return time.Time{}, time.Time{}, fmt.Errorf("stage start and stop are equal")
	}
	midnight := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, ist)
	start, stop := midnight.Add(startOffset), midnight.Add(stopOffset)
	if !stop.After(start) {
		stop = stop.Add(24 * time.Hour)
	}
	return start.UTC(), stop.UTC(), nil
}
