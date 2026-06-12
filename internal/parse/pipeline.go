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
	batch := ParseBlobs(catalogJSON, blobs, fetchEnd)
	days := map[string]*DayAcc{}

	for day, steps := range batch.StepsByDay {
		acc(days, day).Steps = steps
	}
	mergeSleep(days, batch.Sleep)
	for _, s := range batch.Sleep {
		if s.IsNap {
			acc(days, s.DayKey).NapCount++
		}
	}
	for _, s := range batch.Pai {
		v := s.Score
		acc(days, s.DayKey).Pai = &v
	}
	for _, s := range batch.Readiness {
		v := s.Readiness
		acc(days, s.DayKey).Readiness = &v
	}
	for _, s := range batch.Temperature {
		a := acc(days, s.DayKey)
		a.TempSum += s.Celsius
		a.TempCount++
	}
	ApplyCanonicalVitals(days, batch.Sleep, batch.StressSeries, batch.HrvSeries,
		batch.Spo2Spot, batch.Spo2Sleep, batch.RhrSeries, batch.RespRateSeries, batch.MaxHrSeries)
	for _, w := range batch.Workouts {
		acc(days, w.DayKey).WorkoutCount++
	}
	for _, s := range batch.ActivitySessions {
		acc(days, s.DayKey).ActivitySessionCount++
	}
	if err := st.ReplaceSleepSessions(ctx, syncSessionID, toSleepRows(syncSessionID, batch.Sleep)); err != nil {
		return err
	}
	if err := st.ReplaceWorkouts(ctx, syncSessionID, toWorkoutRows(syncSessionID, batch.Workouts)); err != nil {
		return err
	}
	if err := st.ReplaceActivitySessions(ctx, syncSessionID, toActivitySessionRows(syncSessionID, batch.ActivitySessions)); err != nil {
		return err
	}
	if err := st.ReplaceTemperature(ctx, syncSessionID, toTempRows(batch.TempSeries)); err != nil {
		return err
	}
	series := appendHealthSeries(batch.StressSeries, batch.HrvSeries, batch.Spo2Spot, batch.Spo2Sleep,
		batch.RhrSeries, batch.RespRateSeries, batch.MaxHrSeries)
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
