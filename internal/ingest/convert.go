package ingest

import (
	"time"

	"github.com/Anshuman115/heliolytics/internal/parse"
	"github.com/Anshuman115/heliolytics/internal/store"
)

// batchRows is one upload converted to the exact store row shapes. Every
// source, mode, precision and nullable field survives the conversion.
type batchRows struct {
	heartRate   []store.HeartRateSample
	steps       []store.StepSample
	temperature []store.TemperatureSample
	hrv         []store.HRVSample
	stress      []store.StressSample
	resp        []store.RespSample
	spo2        []store.SpO2Sample
	sleep       []store.SleepRow
	workouts    []store.ActivityRow
	pai         []store.PAIScore
}

func convertBatch(batch parse.ParsedBatch) batchRows {
	out := batchRows{
		heartRate:   make([]store.HeartRateSample, 0, len(batch.HeartRate)),
		steps:       make([]store.StepSample, 0, len(batch.Steps)),
		temperature: make([]store.TemperatureSample, 0, len(batch.Temperature)),
		hrv:         make([]store.HRVSample, 0, len(batch.HRV)),
		stress:      make([]store.StressSample, 0, len(batch.Stress)),
		resp:        make([]store.RespSample, 0, len(batch.Respiration)),
		spo2:        make([]store.SpO2Sample, 0, len(batch.SpO2)),
		sleep:       make([]store.SleepRow, 0, len(batch.Sleep)),
		workouts:    make([]store.ActivityRow, 0, len(batch.Workouts)),
		pai:         make([]store.PAIScore, 0, len(batch.PAI)),
	}
	for _, r := range batch.HeartRate {
		out.heartRate = append(out.heartRate, store.HeartRateSample{At: r.SampledAt, Source: r.Source, BPM: r.BPM})
	}
	for _, r := range batch.Steps {
		out.steps = append(out.steps, store.StepSample{At: r.SampledAt, Steps: r.Steps})
	}
	for _, r := range batch.Temperature {
		out.temperature = append(out.temperature, store.TemperatureSample{At: r.SampledAt, CentiC: r.HundredthsC})
	}
	for _, r := range batch.HRV {
		out.hrv = append(out.hrv, store.HRVSample{At: r.SampledAt, RMSSDMS: r.Value})
	}
	for _, r := range batch.Stress {
		out.stress = append(out.stress, store.StressSample{At: r.SampledAt, Stress: int(r.Value)})
	}
	for _, r := range batch.Respiration {
		out.resp = append(out.resp, store.RespSample{At: r.SampledAt, BRMin: r.Value})
	}
	for _, r := range batch.SpO2 {
		out.spo2 = append(out.spo2, store.SpO2Sample{At: r.SampledAt, Source: r.Source, Mode: r.Mode, Pct: r.Pct})
	}
	for _, r := range batch.Sleep {
		out.sleep = append(out.sleep, store.SleepRow{
			StartedAt:       r.StartedAt,
			EndedAt:         r.EndedAt,
			DayKey:          parseDayKey(r.DayKey),
			IsNap:           r.IsNap,
			Score:           r.Score,
			TotalMins:       r.TotalMins,
			DeepMins:        r.DeepMins,
			RemMins:         r.REMMins,
			LightMins:       r.LightMins,
			WakeMins:        r.WakeMins,
			Stages:          convertStages(r.Stages),
			StagesTruncated: r.StagesTruncated,
		})
	}
	for _, r := range batch.Workouts {
		out.workouts = append(out.workouts, store.ActivityRow{
			StartedAt: r.StartedAt,
			EndedAt:   r.EndedAt,
			Kind:      "workout",
			SportType: r.SportType,
			SportName: r.SportName,
			Calories:  r.Calories,
			AvgHR:     r.AvgHR,
			MaxHR:     r.MaxHR,
			MinHR:     r.MinHR,
			Steps:     r.Steps,
			HasDetail: r.HasDetail,
		})
	}
	for _, r := range batch.PAI {
		out.pai = append(out.pai, store.PAIScore{
			DayKey:    parseDayKey(r.DayKey),
			SampledAt: r.SampledAt,
			Score:     r.Score,
		})
	}
	return out
}

func convertStages(stages []parse.SleepStage) []store.SleepStagePoint {
	if len(stages) == 0 {
		return nil
	}
	out := make([]store.SleepStagePoint, 0, len(stages))
	for _, s := range stages {
		out = append(out, store.SleepStagePoint{Start: s.Start, End: s.End, Type: s.Type})
	}
	return out
}

// parseDayKey reads the parser's IST calendar date into a normalized day key.
func parseDayKey(value string) time.Time {
	parsed, err := time.ParseInLocation("2006-01-02", value, store.IST)
	if err != nil {
		return time.Time{}
	}
	return store.Day(parsed.Year(), parsed.Month(), parsed.Day())
}
