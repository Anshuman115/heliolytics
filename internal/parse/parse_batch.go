package parse

import "time"

// ParsedBatch holds parse-only outputs from raw sync blobs.
type ParsedBatch struct {
	Catalog          Catalog
	StepsByDay       map[string]int
	Sleep            []SleepRecord
	Pai              []PaiScore
	Readiness        []ReadinessScore
	Temperature      []TempSample
	StressSeries     []HealthSample
	HrvSeries        []HealthSample
	Spo2Spot         []HealthSample
	Spo2Sleep        []HealthSample
	RhrSeries        []HealthSample
	RespRateSeries   []HealthSample
	MaxHrSeries      []HealthSample
	Workouts         []WorkoutRecord
	ActivitySessions []WorkoutRecord
	TempSeries       []TempSamplePoint
}

func ParseBlobs(catalogJSON []byte, blobs map[string][]byte, fetchEnd time.Time) ParsedBatch {
	cat := ParseCatalog(catalogJSON)
	out := ParsedBatch{Catalog: cat, StepsByDay: map[string]int{}}

	if raw, ok := blobs["0x01"]; ok && len(raw) > 0 {
		for day, steps := range SumStepsByDay(raw, catalogJSON, fetchEnd) {
			out.StepsByDay[day] = steps
		}
	}
	out.Sleep = ParseSleep(blobs["0x48"])
	out.Pai = ParsePai(blobs["0x0D"])
	out.Readiness = ParseReadiness(blobs["0x39"])
	out.Temperature = ParseTemperature(blobs["0x2E"], FindEntry(cat, "0x2E"))
	out.StressSeries = ParseStressSeries(blobs["0x13"], FindEntry(cat, "0x13"))
	out.HrvSeries = ParseHrvSeries(blobs["0x49"])
	out.Spo2Spot = ParseSpo2Series(blobs["0x25"])
	out.Spo2Sleep = ParseSpo2SleepSeries(blobs["0x26"])
	out.RhrSeries = ParseRhrSeries(blobs["0x3A"])
	out.RespRateSeries = ParseRespRateSeries(blobs["0x38"])
	out.MaxHrSeries = ParseMaxHrSeries(blobs["0x3D"])
	out.Workouts = MergeWorkouts(
		ParseWorkouts(blobs["0x05"]),
		ParseWorkoutDetails(blobs["0x06"]),
	)
	out.ActivitySessions = ParseActivitySessions(blobs["0x3B"])
	out.TempSeries = ParseTempSeries(blobs["0x2E"], FindEntry(cat, "0x2E"))
	return out
}
