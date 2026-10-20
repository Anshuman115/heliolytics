package store

import (
	"sort"
	"time"
)

const (
	sampleBatchSize = 4096
	pointInterval   = time.Microsecond
)

type SessionMeta struct {
	ID         string
	DeviceMAC  string
	StartedAt  time.Time
	EndedAt    time.Time
	BatteryPct *int
}

// Normalized returns envelope instants at the precision used for observation
// ordering and PostgreSQL persistence. Device measurement times are unchanged.
func (m SessionMeta) Normalized() SessionMeta {
	m.StartedAt = normInstant(m.StartedAt)
	m.EndedAt = normInstant(m.EndedAt)
	return m
}

type RawBlob struct {
	TypeCode string
	SHA256   [32]byte
	Payload  []byte
}

type TypeResult struct {
	Status        string     `json:"status"`
	Rows          int        `json:"rows"`
	Dropped       int        `json:"dropped"`
	TrailingBytes int        `json:"trailingBytes"`
	Warnings      []string   `json:"warnings"`
	DataThrough   *time.Time `json:"dataThrough"`
}

type IngestResult struct {
	SessionID string                `json:"sessionId"`
	Complete  bool                  `json:"complete"`
	Types     map[string]TypeResult `json:"types"`
	Warnings  []string              `json:"warnings"`
}

type HeartRateSample struct {
	At     time.Time
	Source string
	BPM    int
}

type StepSample struct {
	At    time.Time
	Steps int
}

type TemperatureSample struct {
	At     time.Time
	CentiC int
}

type TemperatureReadRow struct {
	At      time.Time
	CentiC  int
	OnWrist bool
}

type HRVSample struct {
	At      time.Time
	RMSSDMS float64
}

type StressSample struct {
	At     time.Time
	Stress int
}

type RespSample struct {
	At    time.Time
	BRMin float64
}

type SpO2Sample struct {
	At     time.Time
	Source string
	Mode   string
	Pct    int
}

type SleepRow struct {
	StartedAt       time.Time
	EndedAt         time.Time
	DayKey          time.Time
	IsNap           bool
	Score           *int
	TotalMins       int
	DeepMins        int
	RemMins         int
	LightMins       int
	WakeMins        int
	Stages          []SleepStagePoint
	StagesTruncated bool
}

type ActivityRow struct {
	StartedAt time.Time
	EndedAt   time.Time
	Kind      string
	SportType int
	SportName string
	Calories  *int
	AvgHR     *int
	MaxHR     *int
	MinHR     *int
	Steps     *int
	HasDetail bool
}

type PAIScore struct {
	DayKey    time.Time
	SampledAt time.Time
	Score     int
}

type InstantInterval struct {
	From time.Time
	To   time.Time
}

type Mutation struct {
	Days      []time.Time
	Intervals []InstantInterval
	Changed   bool
}

type DataCoverage struct {
	HasData      bool                  `json:"hasData"`
	LastIngestAt *time.Time            `json:"lastIngestAt"`
	DataThrough  *time.Time            `json:"dataThrough"`
	Types        map[string]*time.Time `json:"types"`
}

type mutationBuilder struct {
	days  map[time.Time]struct{}
	spans []InstantInterval
}

func newMutation() *mutationBuilder {
	return &mutationBuilder{days: map[time.Time]struct{}{}}
}

func (m *mutationBuilder) addDay(d time.Time) {
	m.days[dayOf(d)] = struct{}{}
}

func (m *mutationBuilder) addSpan(from, to time.Time) {
	if !to.After(from) {
		return
	}
	m.spans = append(m.spans, InstantInterval{From: from, To: to})
}

func (m *mutationBuilder) addPoint(at time.Time) {
	m.addDay(at)
	m.addSpan(at, at.Add(pointInterval))
}

func (m *mutationBuilder) result() Mutation {
	out := Mutation{Changed: len(m.days) > 0 || len(m.spans) > 0}
	out.Days = make([]time.Time, 0, len(m.days))
	for d := range m.days {
		out.Days = append(out.Days, d)
	}
	sort.Slice(out.Days, func(i, j int) bool { return out.Days[i].Before(out.Days[j]) })
	out.Intervals = mergeIntervals(m.spans)
	return out
}

func mergeIntervals(spans []InstantInterval) []InstantInterval {
	if len(spans) == 0 {
		return nil
	}
	sorted := make([]InstantInterval, len(spans))
	copy(sorted, spans)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].From.Equal(sorted[j].From) {
			return sorted[i].To.Before(sorted[j].To)
		}
		return sorted[i].From.Before(sorted[j].From)
	})
	out := make([]InstantInterval, 0, len(sorted))
	out = append(out, sorted[0])
	for _, s := range sorted[1:] {
		last := &out[len(out)-1]
		if !s.From.After(last.To) {
			if s.To.After(last.To) {
				last.To = s.To
			}
			continue
		}
		out = append(out, s)
	}
	return out
}

func normInstant(t time.Time) time.Time { return t.UTC().Truncate(time.Microsecond) }

type sampleKey struct {
	at     int64
	source string
}

func keyOf(at time.Time, source string) sampleKey {
	return sampleKey{at: at.UTC().UnixNano(), source: source}
}

// tupleGE compares observation tuples; equal tuples stay writable so an exact
// retry is idempotent and deterministic.
func tupleGE(observedAt time.Time, sessionID string, storedAt time.Time, storedSessionID string) bool {
	if observedAt.After(storedAt) {
		return true
	}
	if observedAt.Before(storedAt) {
		return false
	}
	return sessionID >= storedSessionID
}

func tupleAfter(observedAt time.Time, sessionID string, storedAt time.Time, storedSessionID string) bool {
	if observedAt.After(storedAt) {
		return true
	}
	if observedAt.Before(storedAt) {
		return false
	}
	return sessionID > storedSessionID
}

func dedupeSamples[T any](table string, rows []T, key func(T) sampleKey, value func(T) any) ([]T, error) {
	seen := make(map[sampleKey]int, len(rows))
	out := make([]T, 0, len(rows))
	for _, r := range rows {
		k := key(r)
		if i, ok := seen[k]; ok {
			if value(out[i]) != value(r) {
				return nil, invalidRow(table, "conflicting duplicate row for one key")
			}
			continue
		}
		seen[k] = len(out)
		out = append(out, r)
	}
	return out, nil
}
