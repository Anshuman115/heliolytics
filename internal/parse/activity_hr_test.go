package parse

import (
	"testing"
	"time"
)

func TestParseActivityHrSeriesUsesRoundSegments(t *testing.T) {
	raw := []byte{
		0x50, 1, 3, 72, 0, 0, 0, 0,
		0x60, 0, 0, 0xff, 0, 0, 0, 0,
		0x70, 0, 0, 61, 0, 0, 0, 0,
	}
	catalog := []byte(`{"chunked":[{"code":"0x01","roundSegments":[
		{"byteOffset":0,"roundStart":"2026-07-25T23:59:00"},
		{"byteOffset":16,"roundStart":"2026-07-26T01:00:00"}
	]}]}`)

	points := ParseActivityHrSeries(raw, catalog, time.Time{})
	if len(points) != 2 {
		t.Fatalf("points=%d, want 2 valid BPM samples", len(points))
	}
	if points[0].Bpm != 72 || points[1].Bpm != 61 {
		t.Fatalf("BPM values=%d,%d want 72,61", points[0].Bpm, points[1].Bpm)
	}
	if points[0].DayKey != "2026-07-25" || points[1].DayKey != "2026-07-26" {
		t.Fatalf("day keys=%s,%s", points[0].DayKey, points[1].DayKey)
	}
}

func TestMergeHeartRateSeriesPrefersLaterSource(t *testing.T) {
	ts := time.Date(2026, 7, 26, 1, 0, 0, 0, time.UTC)
	merged := MergeHeartRateSeries(
		[]HrSamplePoint{{Ts: ts, DayKey: "2026-07-26", Bpm: 60, SourceType: "0x01"}},
		[]HrSamplePoint{{Ts: ts, DayKey: "2026-07-26", Bpm: 64, SourceType: "0x46"}},
	)
	if len(merged) != 1 || merged[0].Bpm != 64 || merged[0].SourceType != "0x46" {
		t.Fatalf("merged=%+v, want one later-source point at 64 BPM", merged)
	}
}
