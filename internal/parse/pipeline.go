package parse

import (
	"context"
	"time"

	"github.com/heliolytics/api/internal/store"
)

func RunIngest(
	ctx context.Context,
	st *store.Store,
	syncSessionID string,
	catalogJSON []byte,
	blobs map[string][]byte,
	fetchEnd time.Time,
) error {
	cat := ParseCatalog(catalogJSON)
	days := map[string]*DayAcc{}

	if raw, ok := blobs["0x01"]; ok && len(raw) > 0 {
		for day, steps := range SumStepsByDay(raw, catalogJSON, fetchEnd) {
			acc(days, day).Steps = steps
		}
	}
	sleepRecs := ParseSleep(blobs["0x48"])
	mergeSleep(days, sleepRecs)
	for _, s := range sleepRecs {
		if s.IsNap {
			acc(days, s.DayKey).NapCount++
		}
	}
	for _, s := range ParsePai(blobs["0x0D"]) {
		v := s.Score
		acc(days, s.DayKey).Pai = &v
	}
	for _, s := range ParseReadiness(blobs["0x39"]) {
		v := s.Readiness
		acc(days, s.DayKey).Readiness = &v
	}
	for _, s := range ParseTemperature(blobs["0x2E"], FindEntry(cat, "0x2E")) {
		a := acc(days, s.DayKey)
		a.TempSum += s.Celsius
		a.TempCount++
	}
	stressSeries := ParseStressSeries(blobs["0x13"], FindEntry(cat, "0x13"))
	hrvSeries := ParseHrvSeries(blobs["0x49"])
	spo2Spot := ParseSpo2Series(blobs["0x25"])
	spo2Sleep := ParseSpo2SleepSeries(blobs["0x26"])
	rhrSeries := ParseRhrSeries(blobs["0x3A"])
	respRateSeries := ParseRespRateSeries(blobs["0x38"])
	maxHrSeries := ParseMaxHrSeries(blobs["0x3D"])
	ApplyCanonicalVitals(days, sleepRecs, stressSeries, hrvSeries,
		spo2Spot, spo2Sleep, rhrSeries, respRateSeries, maxHrSeries)
	workouts := MergeWorkouts(
		ParseWorkouts(blobs["0x05"]),
		ParseWorkoutDetails(blobs["0x06"]),
	)
	for _, w := range workouts {
		acc(days, w.DayKey).WorkoutCount++
	}
	activitySessions := ParseActivitySessions(blobs["0x3B"])
	for _, s := range activitySessions {
		acc(days, s.DayKey).ActivitySessionCount++
	}
	if err := st.ReplaceSleepSessions(ctx, syncSessionID, toSleepRows(syncSessionID, sleepRecs)); err != nil {
		return err
	}
	if err := st.ReplaceWorkouts(ctx, syncSessionID, toWorkoutRows(syncSessionID, workouts)); err != nil {
		return err
	}
	if err := st.ReplaceActivitySessions(ctx, syncSessionID, toActivitySessionRows(syncSessionID, activitySessions)); err != nil {
		return err
	}
	temps := ParseTempSeries(blobs["0x2E"], FindEntry(cat, "0x2E"))
	if err := st.ReplaceTemperature(ctx, syncSessionID, toTempRows(temps)); err != nil {
		return err
	}
	series := appendHealthSeries(stressSeries, hrvSeries, spo2Spot, spo2Sleep,
		rhrSeries, respRateSeries, maxHrSeries)
	if err := st.ReplaceHealthSamples(ctx, syncSessionID, toHealthRows(series)); err != nil {
		return err
	}
	return st.UpsertDayMetrics(ctx, toStoreDays(days))
}

func acc(m map[string]*DayAcc, day string) *DayAcc {
	if d, ok := m[day]; ok {
		return d
	}
	d := &DayAcc{}
	m[day] = d
	return d
}

func mergeSleep(days map[string]*DayAcc, recs []SleepRecord) {
	best := map[string]SleepRecord{}
	for _, s := range recs {
		if s.IsNap {
			continue
		}
		if prev, ok := best[s.DayKey]; !ok || s.Score > prev.Score {
			best[s.DayKey] = s
		}
	}
	for day, s := range best {
		a := acc(days, day)
		a.SleepScore = &s.Score
		a.SleepMins = s.TotalMin
		a.SleepDeep = s.DeepMin
		a.SleepRem = s.RemMin
		a.SleepLight = s.LightMin
	}
}

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
