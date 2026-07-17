package parse

// AggregatedBatch holds one sync's parsed rows, grouped by destination table.
// No daily_metrics fields here — those are filled by internal/rollup after
// commit, reading back from the tables these rows land in.
type AggregatedBatch struct {
	Sleep            []SleepRecord
	Workouts         []WorkoutRecord
	ActivitySessions []WorkoutRecord
	TempSeries       []TempSamplePoint
	HrSeries         []HrSamplePoint
	StepSeries       []StepSample
	HrvSeries        []HealthSample // parse.HealthSample, converted to store.SampleValue at the write site
	Spo2Series       []HealthSample // Spo2Spot + Spo2Sleep merged via appendHealthSeries
	StressSeries     []HealthSample
	RespRateSeries   []HealthSample
	RhrSeries        []HealthSample
	PaiScores        map[string]int // day_key -> score, from parsed.Pai
	ReadinessScores  map[string]int // day_key -> score, from parsed.Readiness
	TouchedDays      []string
}

// Aggregate regroups a ParsedBatch into per-table row slices ready for the
// store layer. It performs no daily rollup itself — that happens after the
// write transaction commits, by reading the committed rows back from the DB.
func Aggregate(parsed ParsedBatch) AggregatedBatch {
	pai := map[string]int{}
	for _, p := range parsed.Pai {
		pai[p.DayKey] = p.Score
	}
	readiness := map[string]int{}
	for _, r := range parsed.Readiness {
		readiness[r.DayKey] = r.Readiness
	}
	return AggregatedBatch{
		Sleep:            parsed.Sleep,
		Workouts:         parsed.Workouts,
		ActivitySessions: parsed.ActivitySessions,
		TempSeries:       parsed.TempSeries,
		HrSeries:         parsed.HrSeries,
		StepSeries:       parsed.StepSeries,
		HrvSeries:        parsed.HrvSeries,
		Spo2Series:       appendHealthSeries(parsed.Spo2Spot, parsed.Spo2Sleep),
		StressSeries:     parsed.StressSeries,
		RespRateSeries:   parsed.RespRateSeries,
		RhrSeries:        parsed.RhrSeries,
		PaiScores:        pai,
		ReadinessScores:  readiness,
		TouchedDays:      TouchedDayKeys(parsed),
	}
}
