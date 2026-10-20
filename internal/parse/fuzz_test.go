package parse

import (
	"testing"
	"time"
)

var fuzzCodes = []string{
	typeActivity, typeWorkout, typeWorkoutDetail, typePAI, typeStress, typeSpO2,
	typeSpO2Sleep, typeTemperature, typeRespiration, typeContinuousHR, typeSleep, typeHRV,
}

func fuzzWindow() Window {
	return Window{
		StartedAt: time.Date(2026, 6, 22, 17, 7, 40, 0, time.UTC),
		EndedAt:   time.Date(2026, 6, 22, 17, 8, 11, 0, time.UTC),
	}
}

func fuzzBlobCatalog() Catalog {
	return Catalog{Chunked: []CatalogEntry{
		{Code: typeActivity, RoundStart: "2026-06-21T11:16:00"},
		{Code: typeStress, RoundStart: "2026-06-21T11:16:00"},
		{Code: typeTemperature, RoundStart: "2026-06-21T11:17:00"},
	}}
}

func FuzzParseBlobs(f *testing.F) {
	spo2Cases := loadSpo2Boundaries(f)
	for index, code := range fuzzCodes {
		if code == typeSpO2 {
			f.Add(uint8(index), spo2Cases.file(f, "normal-two-rounds.bin"))
			continue
		}
		f.Add(uint8(index), fixtureBytes(f, code))
	}
	f.Add(uint8(0), []byte{})
	f.Add(uint8(5), []byte{0x02, 0x01})
	f.Fuzz(func(t *testing.T, selector uint8, payload []byte) {
		code := fuzzCodes[int(selector)%len(fuzzCodes)]
		batch, err := ParseBlobs(fuzzBlobCatalog(), map[string][]byte{code: payload}, fuzzWindow())
		if err != nil {
			if batch.HeartRate != nil || batch.Steps != nil || len(batch.Types) != 0 {
				t.Fatalf("a structural error returned a partial batch for %s", code)
			}
			return
		}
		window := fuzzWindow()
		for _, row := range batch.HeartRate {
			if !window.contains(row.SampledAt) {
				t.Fatalf("%s HR row outside the window: %s", code, formatFixtureTime(row.SampledAt))
			}
			if row.BPM < hrMinBPM || row.BPM > hrMaxBPM {
				t.Fatalf("%s HR row outside the accepted range: %d", code, row.BPM)
			}
		}
		for _, row := range batch.Steps {
			if !window.contains(row.SampledAt) {
				t.Fatalf("%s step row outside the window", code)
			}
			if row.Steps < 0 {
				t.Fatalf("%s negative step count", code)
			}
		}
		for _, row := range batch.Temperature {
			if row.HundredthsC < 0 || row.HundredthsC > temperatureMaxHundredths {
				t.Fatalf("%s temperature outside the domain: %d", code, row.HundredthsC)
			}
		}
		for _, row := range batch.SpO2 {
			if row.Pct < spo2MinPct || row.Pct > spo2MaxPct {
				t.Fatalf("%s SpO2 outside the domain: %d", code, row.Pct)
			}
			if row.Mode != "auto" && row.Mode != "manual" {
				t.Fatalf("%s SpO2 mode = %q", code, row.Mode)
			}
		}
		for _, row := range batch.Sleep {
			if !row.EndedAt.After(row.StartedAt) {
				t.Fatalf("%s sleep session is not positive: %+v", code, row)
			}
			for _, stage := range row.Stages {
				if !stage.End.After(stage.Start) {
					t.Fatalf("%s sleep stage is not positive: %+v", code, stage)
				}
				if stage.Start.Before(row.StartedAt) || stage.End.After(row.EndedAt) {
					t.Fatalf("%s sleep stage escapes its session: %+v", code, stage)
				}
			}
		}
		for _, row := range batch.Workouts {
			if !row.EndedAt.After(row.StartedAt) {
				t.Fatalf("%s workout is not positive: %+v", code, row)
			}
		}
		if _, ok := batch.Types[typeSpO2Sleep]; ok && len(batch.SpO2) != 0 {
			t.Fatalf("%s produced measured rows from 0x26", code)
		}
	})
}

func FuzzParseWorkouts(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0x00, 0x80, 0x0a, 0x03, 0x32, 0x2e})
	f.Add(fixtureBytes(f, typeWorkout))
	f.Fuzz(func(t *testing.T, payload []byte) {
		rows, result, err := parseWorkoutSummaries(payload, fuzzWindow())
		if err != nil {
			return
		}
		if result.Rows != len(rows) {
			t.Fatalf("rows=%d but the result reports %d", len(rows), result.Rows)
		}
		for _, row := range rows {
			if !row.EndedAt.After(row.StartedAt) {
				t.Fatalf("workout is not positive: %+v", row)
			}
			if row.SportName == "" {
				t.Fatalf("workout has no display name: %+v", row)
			}
		}
	})
}

func FuzzParseWorkoutDetails(f *testing.F) {
	f.Add([]byte{})
	f.Add(workoutDetailHeader)
	f.Add(fixtureBytes(f, typeWorkoutDetail))
	f.Fuzz(func(t *testing.T, payload []byte) {
		blocks, err := parseWorkoutDetailBlocks(payload)
		if err != nil {
			return
		}
		for _, block := range blocks {
			if block.off < 0 || block.off >= len(payload) {
				t.Fatalf("block offset %d is outside the payload", block.off)
			}
		}
	})
}
