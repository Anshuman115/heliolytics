package parse

import (
	"fmt"
	"sort"
	"time"
)

type Window struct {
	StartedAt time.Time
	EndedAt   time.Time
}

const (
	ErrCodeInvalidCatalog      = "invalid_catalog"
	ErrCodeMissingAnchor       = "missing_anchor"
	ErrCodeInvalidWindow       = "invalid_window"
	ErrCodeInvalidFraming      = "invalid_framing"
	ErrCodeUnsupportedVersion  = "unsupported_version"
	ErrCodeUnsupportedTimezone = "unsupported_timezone"
	ErrCodeCountOverflow       = "count_overflow"
	ErrCodeMalformedProtobuf   = "malformed_protobuf"
)

const (
	typeActivity      = "0x01"
	typeWorkout       = "0x05"
	typeWorkoutDetail = "0x06"
	typePAI           = "0x0D"
	typeStress        = "0x13"
	typeSpO2          = "0x25"
	typeSpO2Sleep     = "0x26"
	typeTemperature   = "0x2E"
	typeRespiration   = "0x38"
	typeContinuousHR  = "0x46"
	typeSleep         = "0x48"
	typeHRV           = "0x49"
)

type ParseError struct {
	Code       string
	TypeCode   string
	ByteOffset int
	Reason     string
}

func (e *ParseError) Error() string {
	reason := e.Reason
	if reason == "" {
		reason = "parse error"
	}
	if e.TypeCode != "" {
		return fmt.Sprintf("%s [%s] at byte %d: %s", e.Code, e.TypeCode, e.ByteOffset, reason)
	}
	return fmt.Sprintf("%s at byte %d: %s", e.Code, e.ByteOffset, reason)
}

func newError(code, typeCode string, offset int, format string, args ...any) *ParseError {
	return &ParseError{
		Code: code, TypeCode: typeCode, ByteOffset: offset,
		Reason: fmt.Sprintf(format, args...),
	}
}

// Warnings must not carry raw bytes or device identity.
type Warning struct {
	Code       string
	ByteOffset int
	Count      int
}

func (w Warning) String() string {
	return fmt.Sprintf("%s offset=%d count=%d", w.Code, w.ByteOffset, w.Count)
}

type TypeResult struct {
	Status           string
	Rows             int
	Dropped          int
	TrailingBytes    int
	ProcessedThrough *time.Time
	Warnings         []Warning
}

type HeartRateRow struct {
	SampledAt time.Time
	Source    string
	BPM       int
}

type StepRow struct {
	SampledAt time.Time
	Steps     int
}

type TemperatureRow struct {
	SampledAt   time.Time
	HundredthsC int
}

type ScalarRow struct {
	SampledAt time.Time
	Value     float64
}

type SpO2Row struct {
	SampledAt time.Time
	Source    string
	Pct       int
	Mode      string
}

type SleepStage struct {
	Start time.Time
	End   time.Time
	Type  int
}

type SleepRow struct {
	StartedAt       time.Time
	EndedAt         time.Time
	DayKey          string
	IsNap           bool
	Score           *int
	TotalMins       int
	DeepMins        int
	REMMins         int
	LightMins       int
	WakeMins        int
	Stages          []SleepStage
	StagesTruncated bool
}

type WorkoutRow struct {
	StartedAt time.Time
	EndedAt   time.Time
	SportType int
	SportName string
	Calories  *int
	AvgHR     *int
	MaxHR     *int
	MinHR     *int
	Steps     *int
	HasDetail bool
}

type PAIRow struct {
	SampledAt time.Time
	DayKey    string
	Score     int
}

type ParsedBatch struct {
	Catalog     Catalog
	HeartRate   []HeartRateRow
	Steps       []StepRow
	Temperature []TemperatureRow
	HRV         []ScalarRow
	Stress      []ScalarRow
	Respiration []ScalarRow
	SpO2        []SpO2Row
	Sleep       []SleepRow
	Workouts    []WorkoutRow
	PAI         []PAIRow
	Types       map[string]TypeResult
}

func (w Window) validate() error {
	if w.StartedAt.IsZero() || w.EndedAt.IsZero() {
		return newError(ErrCodeInvalidWindow, "", 0, "session start and end are required")
	}
	if w.EndedAt.Before(w.StartedAt) {
		return newError(ErrCodeInvalidWindow, "", 0, "session end is before its start")
	}
	return nil
}

func (w Window) low() time.Time  { return w.StartedAt.Add(-measuredWindowSlack) }
func (w Window) high() time.Time { return w.EndedAt.Add(measuredWindowTail) }

func (w Window) contains(at time.Time) bool {
	return !at.Before(w.low()) && !at.After(w.high())
}

func (w Window) containsInterval(start, end time.Time) bool {
	return !start.Before(w.low()) && !end.After(w.high())
}

func ParseBlobs(catalog Catalog, blobs map[string][]byte, window Window) (ParsedBatch, error) {
	if err := window.validate(); err != nil {
		return ParsedBatch{}, err
	}
	out := ParsedBatch{Catalog: catalog, Types: make(map[string]TypeResult, len(blobs))}

	if raw := blobs[typeActivity]; len(raw) > 0 {
		hr, steps, result, err := parseActivity(raw, catalog, window)
		if err != nil {
			return ParsedBatch{}, err
		}
		out.HeartRate = hr
		out.Steps = steps
		out.Types[typeActivity] = result
	}
	if raw := blobs[typeWorkout]; len(raw) > 0 {
		workouts, result, err := parseWorkoutSummaries(raw, window)
		if err != nil {
			return ParsedBatch{}, err
		}
		out.Workouts = workouts
		out.Types[typeWorkout] = result
	}
	if raw := blobs[typeWorkoutDetail]; len(raw) > 0 {
		blocks, err := parseWorkoutDetailBlocks(raw)
		if err != nil {
			return ParsedBatch{}, err
		}
		out.Types[typeWorkoutDetail] = attachWorkoutDetails(out.Workouts, blocks)
	}
	if raw := blobs[typePAI]; len(raw) > 0 {
		pai, result, err := parsePAI(raw, window)
		if err != nil {
			return ParsedBatch{}, err
		}
		out.PAI = pai
		out.Types[typePAI] = result
	}
	if raw := blobs[typeStress]; len(raw) > 0 {
		stress, result, err := parseStress(raw, catalog, window)
		if err != nil {
			return ParsedBatch{}, err
		}
		out.Stress = stress
		out.Types[typeStress] = result
	}
	if raw := blobs[typeSpO2]; len(raw) > 0 {
		spo2, result, err := parseSpO2(raw, catalog, window)
		if err != nil {
			return ParsedBatch{}, err
		}
		out.SpO2 = spo2
		out.Types[typeSpO2] = result
	}
	if raw := blobs[typeSpO2Sleep]; len(raw) > 0 {
		result, err := parseSpO2Sleep(raw, catalog)
		if err != nil {
			return ParsedBatch{}, err
		}
		out.Types[typeSpO2Sleep] = result
	}
	if raw := blobs[typeTemperature]; len(raw) > 0 {
		temperature, result, err := parseTemperature(raw, catalog, window)
		if err != nil {
			return ParsedBatch{}, err
		}
		out.Temperature = temperature
		out.Types[typeTemperature] = result
	}
	if raw := blobs[typeRespiration]; len(raw) > 0 {
		respiration, result, err := parseRespiration(raw, window)
		if err != nil {
			return ParsedBatch{}, err
		}
		out.Respiration = respiration
		out.Types[typeRespiration] = result
	}
	if raw := blobs[typeContinuousHR]; len(raw) > 0 {
		hr, result, err := parseContinuousHR(raw, window)
		if err != nil {
			return ParsedBatch{}, err
		}
		out.HeartRate = mergeHeartRate(out.HeartRate, hr)
		out.Types[typeContinuousHR] = result
	}
	if raw := blobs[typeSleep]; len(raw) > 0 {
		sleep, result, err := parseSleep(raw, window)
		if err != nil {
			return ParsedBatch{}, err
		}
		out.Sleep = sleep
		out.Types[typeSleep] = result
	}
	if raw := blobs[typeHRV]; len(raw) > 0 {
		hrv, result, err := parseHRV(raw, window)
		if err != nil {
			return ParsedBatch{}, err
		}
		out.HRV = hrv
		out.Types[typeHRV] = result
	}
	for code, result := range out.Types {
		result.Warnings = sortWarnings(result.Warnings)
		out.Types[code] = result
	}
	return out, nil
}

func sortWarnings(w []Warning) []Warning {
	if len(w) == 0 {
		return nil
	}
	sort.SliceStable(w, func(i, j int) bool {
		if w[i].Code != w[j].Code {
			return w[i].Code < w[j].Code
		}
		return w[i].ByteOffset < w[j].ByteOffset
	})
	return w
}

func statusFor(trailing int) string {
	if trailing > 0 {
		return "partial"
	}
	return "accepted"
}

func maxTime(current *time.Time, candidate time.Time) *time.Time {
	if current == nil || candidate.After(*current) {
		value := candidate
		return &value
	}
	return current
}

func mergeHeartRate(existing, extra []HeartRateRow) []HeartRateRow {
	type key struct {
		at     int64
		source string
	}
	merged := make(map[key]HeartRateRow, len(existing)+len(extra))
	for _, row := range existing {
		merged[key{row.SampledAt.Unix(), row.Source}] = row
	}
	for _, row := range extra {
		merged[key{row.SampledAt.Unix(), row.Source}] = row
	}
	out := make([]HeartRateRow, 0, len(merged))
	for _, row := range merged {
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].SampledAt.Equal(out[j].SampledAt) {
			return out[i].SampledAt.Before(out[j].SampledAt)
		}
		return out[i].Source < out[j].Source
	})
	return out
}
