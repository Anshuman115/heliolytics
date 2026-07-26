package parse

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseWorkoutsProtobuf(t *testing.T) {
	p := filepath.Join("..", "..", "..", "Heliolytics_App", "helio_dump_v5", "0x05_raw.bin")
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Skip("dump missing")
	}
	ws := ParseWorkouts(raw)
	if len(ws) != 1 {
		t.Fatalf("want 1 workout got %d", len(ws))
	}
	w := ws[0]
	if w.SportType != 92 {
		t.Fatalf("sport %d name %q", w.SportType, w.SportName)
	}
	if w.SportName != "Badminton" {
		t.Fatalf("name %q", w.SportName)
	}
	if w.Calories == nil || *w.Calories != 872 {
		t.Fatalf("calories %v", w.Calories)
	}
	if w.DurationSec < 180 {
		t.Fatalf("duration %d", w.DurationSec)
	}
}

func TestMergeWorkoutsPreservesSourceProvenance(t *testing.T) {
	started := time.Date(2026, 7, 25, 8, 30, 0, 0, time.UTC)
	merged := MergeWorkouts(
		[]WorkoutRecord{{StartedAt: started, DurationSec: 300, HasSummary: true}},
		[]WorkoutRecord{{StartedAt: started, DurationSec: 600, HasDetail: true}},
	)
	if len(merged) != 1 {
		t.Fatalf("len=%d want 1", len(merged))
	}
	if !merged[0].HasSummary || !merged[0].HasDetail {
		t.Fatalf("provenance summary=%t detail=%t", merged[0].HasSummary, merged[0].HasDetail)
	}
	if merged[0].DurationSec != 600 {
		t.Fatalf("duration=%d want richer detail duration", merged[0].DurationSec)
	}
}

func TestParseWorkoutDetailsUsesTimestampedSessionEnvelopes(t *testing.T) {
	first := time.Date(2026, 7, 26, 9, 14, 18, 0, time.UTC)
	second := first.Add(10 * time.Minute)
	raw := appendDetailBlock(nil, first.UnixMilli(), detailSessionStartMarker)
	raw = appendDetailBlock(raw, first.Add(33*time.Second).UnixMilli(), []byte{0x00, 0x00, 0x08, 0x03})
	raw = appendDetailBlock(raw, second.UnixMilli(), detailSessionStartMarker)

	details := ParseWorkoutDetails(raw)
	if len(details) != 2 {
		t.Fatalf("len=%d want 2", len(details))
	}
	if !details[0].StartedAt.Equal(first) || details[0].DurationSec != 33 {
		t.Fatalf("first=%+v", details[0])
	}
	if !details[1].StartedAt.Equal(second) || details[1].DurationSec != 1 {
		t.Fatalf("second=%+v", details[1])
	}

	summary := WorkoutRecord{StartedAt: first, DurationSec: 252, HasSummary: true}
	details[0].HasDetail = true
	merged := MergeWorkouts([]WorkoutRecord{summary}, details[:1])
	if len(merged) != 1 || !merged[0].HasDetail || merged[0].DurationSec != 252 {
		t.Fatalf("merged=%+v", merged)
	}
}

func TestParseWorkoutDetailsBySegmentsIgnoresTrailerStreams(t *testing.T) {
	first := time.Date(2026, 7, 7, 2, 21, 41, 0, time.UTC)
	trailer := first.Add(73*time.Minute + 12*time.Second)
	second := time.Date(2026, 7, 8, 2, 32, 3, 0, time.UTC)
	raw := appendDetailBlock(nil, first.UnixMilli(), detailSessionStartMarker)
	raw = appendDetailBlock(raw, trailer.UnixMilli(), detailSessionStartMarker)
	secondOffset := len(raw)
	raw = appendDetailBlock(raw, second.UnixMilli(), detailSessionStartMarker)
	entry := &CatalogEntry{RoundSegments: []RoundSegment{
		{ByteOffset: 0},
		{ByteOffset: secondOffset},
	}}

	details := ParseWorkoutDetailsBySegments(raw, entry)
	if len(details) != 2 {
		t.Fatalf("len=%d want 2", len(details))
	}
	if !details[0].StartedAt.Equal(first) || !details[1].StartedAt.Equal(second) {
		t.Fatalf("details=%+v", details)
	}
}

func appendDetailBlock(raw []byte, millis int64, marker []byte) []byte {
	raw = append(raw, psmHeader...)
	for index := 0; index < detailTimestampBytes; index++ {
		raw = append(raw, byte(millis>>(8*index)))
	}
	return append(raw, marker...)
}
