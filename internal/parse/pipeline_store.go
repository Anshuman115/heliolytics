package parse

import "github.com/heliolytics/api/internal/store"

func toSleepRows(recs []SleepRecord) []store.SleepRow {
	out := make([]store.SleepRow, len(recs))
	for i, s := range recs {
		st := make([]store.SleepStagePoint, len(s.Stages))
		for j, g := range s.Stages {
			st[j] = store.SleepStagePoint{Start: g.Start, End: g.End, Type: g.Type}
		}
		out[i] = store.SleepRow{
			DayKey: s.DayKey, StartedAt: s.StartedAt,
			Score: s.Score, TotalMins: s.TotalMin, DeepMins: s.DeepMin,
			RemMins: s.RemMin, LightMins: s.LightMin, WakeMins: s.WakeMin,
			IsNap: s.IsNap, Stages: st,
		}
	}
	return out
}

func toTempRows(pts []TempSamplePoint) []store.TempPoint {
	out := make([]store.TempPoint, len(pts))
	for i, p := range pts {
		out[i] = store.TempPoint{DayKey: p.DayKey, SampledAt: p.Ts, Celsius: p.Celsius}
	}
	return out
}

func appendHealthSeries(parts ...[]HealthSample) []HealthSample {
	var out []HealthSample
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// toSampleValues converts a per-metric parse.HealthSample slice into
// store.SampleValue rows. Metric is dropped here — it's implied by which
// UpsertXSamplesTx method the caller passes the result to.
func toSampleValues(hs []HealthSample) []store.SampleValue {
	out := make([]store.SampleValue, 0, len(hs))
	for _, h := range hs {
		out = append(out, store.SampleValue{DayKey: h.DayKey, SampledAt: h.SampledAt, Value: h.Value})
	}
	return out
}

func toSpo2Values(hs []HealthSample) []store.SampleValue {
	out := make([]store.SampleValue, 0, len(hs))
	for _, h := range hs {
		sourceType := "0x26"
		if h.Metric == "spo2" {
			sourceType = "0x25"
		}
		out = append(out, store.SampleValue{
			DayKey: h.DayKey, SampledAt: h.SampledAt,
			Value: h.Value, SourceType: sourceType,
		})
	}
	return out
}

func toHrRows(pts []HrSamplePoint) []store.HeartRateSample {
	out := make([]store.HeartRateSample, len(pts))
	for i, p := range pts {
		out[i] = store.HeartRateSample{
			DayKey: p.DayKey, SampledAt: p.Ts, Bpm: p.Bpm, SourceType: p.SourceType,
		}
	}
	return out
}

func toStepRows(pts []StepSample) []store.StepSample {
	out := make([]store.StepSample, len(pts))
	for i, p := range pts {
		out[i] = store.StepSample{DayKey: p.DayKey, SampledAt: p.SampledAt, Steps: p.Steps}
	}
	return out
}

func toWorkoutRows(recs []WorkoutRecord) []store.WorkoutRow {
	out := make([]store.WorkoutRow, len(recs))
	for i, w := range recs {
		out[i] = store.WorkoutRow{
			DayKey: w.DayKey, StartedAt: w.StartedAt,
			SportType: w.SportType, SportName: w.SportName,
			DurationSec: w.DurationSec, Calories: w.Calories,
			AvgHr: w.AvgHr, MaxHr: w.MaxHr,
			HasSummary: w.HasSummary, HasDetail: w.HasDetail,
		}
	}
	return out
}

func toActivitySessionRows(recs []WorkoutRecord) []store.ActivitySessionRow {
	out := make([]store.ActivitySessionRow, len(recs))
	for i, s := range recs {
		out[i] = store.ActivitySessionRow{
			DayKey: s.DayKey, StartedAt: s.StartedAt,
			SportType: s.SportType, SportName: s.SportName,
			DurationSec: s.DurationSec, Calories: s.Calories,
			AvgHr: s.AvgHr, MaxHr: s.MaxHr,
		}
	}
	return out
}
