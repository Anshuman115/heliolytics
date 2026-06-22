package parse

import "github.com/heliolytics/api/internal/store"

// AggregatedBatch is store-ready ingest output before session IDs are applied.
type AggregatedBatch struct {
	Days             []store.DayMetric
	Sleep            []SleepRecord
	Workouts         []WorkoutRecord
	ActivitySessions []WorkoutRecord
	TempSeries       []TempSamplePoint
	HrSeries         []HrSamplePoint
	HealthSeries     []HealthSample
	StepSeries       []StepSample
}

// Aggregate rolls parsed blobs into daily metrics and canonical vitals.
func Aggregate(parsed ParsedBatch) AggregatedBatch {
	days := map[string]*DayAcc{}
	for day, steps := range parsed.StepsByDay {
		acc(days, day).Steps = steps
	}
	mergeSleep(days, parsed.Sleep)
	for _, s := range parsed.Sleep {
		if s.IsNap {
			acc(days, s.DayKey).NapCount++
		}
	}
	for _, s := range parsed.Pai {
		v := s.Score
		acc(days, s.DayKey).Pai = &v
	}
	for _, s := range parsed.Readiness {
		v := s.Readiness
		acc(days, s.DayKey).Readiness = &v
	}
	for _, s := range parsed.Temperature {
		a := acc(days, s.DayKey)
		a.TempSum += s.Celsius
		a.TempCount++
	}
	ApplyCanonicalVitals(days, parsed.Sleep, parsed.StressSeries, parsed.HrvSeries,
		parsed.Spo2Spot, parsed.Spo2Sleep, parsed.RhrSeries,
		parsed.RespRateSeries, parsed.MaxHrSeries)
	for _, w := range parsed.Workouts {
		acc(days, w.DayKey).WorkoutCount++
	}
	for _, s := range parsed.ActivitySessions {
		acc(days, s.DayKey).ActivitySessionCount++
	}
	return AggregatedBatch{
		Days:             toStoreDays(days),
		Sleep:            parsed.Sleep,
		Workouts:         parsed.Workouts,
		ActivitySessions: parsed.ActivitySessions,
		TempSeries:       parsed.TempSeries,
		HrSeries:         parsed.HrSeries,
		HealthSeries:     parsed.healthSeries(),
		StepSeries:       parsed.StepSeries,
	}
}
