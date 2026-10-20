package parse

import (
	"encoding/binary"
	"testing"
	"time"
)

func activityCatalog(roundStart string) Catalog {
	return Catalog{Chunked: []CatalogEntry{{Code: typeActivity, RoundStart: roundStart}}}
}

func activityMinute(steps, bpm byte) []byte {
	return []byte{0x50, 0, steps, bpm, 0, 0, 0, 0}
}

// TestActivityHeartRateMatchesZepp pins the 0x01 minute HR stream against the
// independently exported Zepp auto HR rows.
func TestActivityHeartRateMatchesZepp(t *testing.T) {
	catalog := loadFixtureCatalog(t)
	window := loadFixtureWindow(t)
	batch, err := ParseBlobs(catalog, map[string][]byte{typeActivity: fixtureBytes(t, typeActivity)}, window)
	if err != nil {
		t.Fatalf("parse 0x01: %v", err)
	}
	zepp := zeppSeries(t, "heartrate-auto.csv")
	if len(batch.HeartRate) != len(zepp) {
		t.Fatalf("0x01 HR rows = %d, Zepp rows = %d", len(batch.HeartRate), len(zepp))
	}
	for _, row := range batch.HeartRate {
		if row.Source != typeActivity {
			t.Fatalf("unexpected HR source %q", row.Source)
		}
		want, ok := zepp[row.SampledAt.Unix()]
		if !ok {
			t.Errorf("HR minute %s is absent from the Zepp export", formatFixtureTime(row.SampledAt))
			continue
		}
		if row.BPM != want {
			t.Errorf("HR minute %s = %d, Zepp %d", formatFixtureTime(row.SampledAt), row.BPM, want)
		}
	}
}

func TestHeartRateSourceCollisionKeepsBothRows(t *testing.T) {
	at := time.Date(2026, 6, 22, 2, 20, 0, 0, time.UTC)
	catalog := activityCatalog("2026-06-22T07:50:00")

	frame := make([]byte, 9)
	binary.LittleEndian.PutUint32(frame[0:], uint32(at.Unix()))
	binary.LittleEndian.PutUint32(frame[4:], 1)
	frame[8] = 120

	batch, err := ParseBlobs(catalog, map[string][]byte{
		typeActivity:     activityMinute(5, 60),
		typeContinuousHR: frame,
	}, Window{StartedAt: at.Add(-30 * time.Minute), EndedAt: at})
	if err != nil {
		t.Fatalf("parse blobs: %v", err)
	}
	if len(batch.HeartRate) != 2 {
		t.Fatalf("HR rows = %d, want both sources at one instant", len(batch.HeartRate))
	}
	bySource := map[string]int{}
	for _, row := range batch.HeartRate {
		if !row.SampledAt.Equal(at) {
			t.Errorf("HR row at %s, want %s", formatFixtureTime(row.SampledAt), formatFixtureTime(at))
		}
		bySource[row.Source] = row.BPM
	}
	if bySource[typeActivity] != 60 || bySource[typeContinuousHR] != 120 {
		t.Fatalf("HR by source = %v, want 0x01=60 and 0x46=120", bySource)
	}
}

func TestZeroStepVersusAbsentReading(t *testing.T) {
	base := time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC)
	raw := append(activityMinute(0, 70), activityMinute(0xFF, 70)...)
	raw = append(raw, activityMinute(7, 0xFF)...)
	catalog := activityCatalog("2026-07-26T05:30:00")

	batch, err := ParseBlobs(catalog, map[string][]byte{typeActivity: raw}, catalogWindow())
	if err != nil {
		t.Fatalf("parse blobs: %v", err)
	}
	if len(batch.Steps) != 2 {
		t.Fatalf("step rows = %d, want the known zero and the step of 7", len(batch.Steps))
	}
	if batch.Steps[0].Steps != 0 || !batch.Steps[0].SampledAt.Equal(base) {
		t.Errorf("first step row = %+v, want a known zero at %s", batch.Steps[0], formatFixtureTime(base))
	}
	if batch.Steps[1].Steps != 7 || !batch.Steps[1].SampledAt.Equal(base.Add(2*time.Minute)) {
		t.Errorf("second step row = %+v, want 7 at +2m", batch.Steps[1])
	}
	if len(batch.HeartRate) != 2 {
		t.Fatalf("HR rows = %d, want the two 70 readings", len(batch.HeartRate))
	}
	if !batch.HeartRate[1].SampledAt.Equal(base.Add(time.Minute)) {
		t.Errorf("second HR row = %s, want %s",
			formatFixtureTime(batch.HeartRate[1].SampledAt), formatFixtureTime(base.Add(time.Minute)))
	}
	if result := batch.Types[typeActivity]; result.Dropped != 2 {
		t.Errorf("dropped = %d, want one step and one HR sentinel", result.Dropped)
	}
}
