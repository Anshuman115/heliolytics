package parse

import "github.com/heliolytics/api/internal/store"

func toStoreDays(m map[string]*DayAcc) []store.DayMetric {
	out := make([]store.DayMetric, 0, len(m))
	for day, d := range m {
		out = append(out, store.DayMetric{
			DayKey: day, Steps: d.Steps, PaiScore: d.Pai, Readiness: d.Readiness,
			Spo2Avg: d.Spo2Avg(), HrvRmssd: d.HrvAvg(), RestingHr: d.RestingHr,
			MaxHr: d.MaxHr, RespRateAvg: d.RespRateAvg(), StressAvg: d.StressAvg(),
			SleepScore: d.SleepScore, SleepMins: ptrInt(d.SleepMins),
			SleepDeepMins: ptrInt(d.SleepDeep), SleepRemMins: ptrInt(d.SleepRem),
			SleepLightMins: ptrInt(d.SleepLight), TempAvgC: d.TempAvg(),
			NapCount: d.NapCount, WorkoutCount: d.WorkoutCount,
			ActivitySessionCount: d.ActivitySessionCount,
		})
	}
	return out
}

func ptrInt(v int) *int {
	if v == 0 {
		return nil
	}
	return &v
}

func toSleepRows(sid string, recs []SleepRecord) []store.SleepRow {
	out := make([]store.SleepRow, len(recs))
	for i, s := range recs {
		st := make([]store.SleepStagePoint, len(s.Stages))
		for j, g := range s.Stages {
			st[j] = store.SleepStagePoint{Start: g.Start, End: g.End, Type: g.Type}
		}
		out[i] = store.SleepRow{
			SyncSessionID: sid, DayKey: s.DayKey, StartedAt: s.StartedAt,
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

func toHealthRows(pts []HealthSample) []store.HealthSample {
	out := make([]store.HealthSample, len(pts))
	for i, p := range pts {
		out[i] = store.HealthSample{
			Metric: p.Metric, DayKey: p.DayKey,
			SampledAt: p.SampledAt, Value: p.Value,
		}
	}
	return out
}

func toWorkoutRows(sid string, recs []WorkoutRecord) []store.WorkoutRow {
	out := make([]store.WorkoutRow, len(recs))
	for i, w := range recs {
		out[i] = store.WorkoutRow{
			SyncSessionID: sid, DayKey: w.DayKey, StartedAt: w.StartedAt,
			SportType: w.SportType, SportName: w.SportName,
			DurationSec: w.DurationSec, Calories: w.Calories,
			AvgHr: w.AvgHr, MaxHr: w.MaxHr,
		}
	}
	return out
}

func toActivitySessionRows(sid string, recs []WorkoutRecord) []store.ActivitySessionRow {
	out := make([]store.ActivitySessionRow, len(recs))
	for i, s := range recs {
		out[i] = store.ActivitySessionRow{
			SyncSessionID: sid, DayKey: s.DayKey, StartedAt: s.StartedAt,
			SportType: s.SportType, SportName: s.SportName,
			DurationSec: s.DurationSec, Calories: s.Calories,
			AvgHr: s.AvgHr, MaxHr: s.MaxHr,
		}
	}
	return out
}
