package parse

import (
	"encoding/binary"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestParseBlobsIsAtomicOnTypeError(t *testing.T) {
	window := loadFixtureWindow(t)
	valid := fixtureBytes(t, typeActivity)
	malformedWorkout := []byte{0x00, 0x80, 0x08, 0x01}
	batch, err := ParseBlobs(loadFixtureCatalog(t), map[string][]byte{
		typeActivity: valid,
		typeWorkout:  malformedWorkout,
	}, window)
	var parseErr *ParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("error = %v, want a structural *ParseError", err)
	}
	if batch.HeartRate != nil || batch.Steps != nil || len(batch.Types) != 0 {
		t.Fatalf("failed upload returned a partial batch: %+v", batch)
	}
}

func TestParseBlobsAcceptsRawOnlySleepBreathing(t *testing.T) {
	batch, err := ParseBlobs(Catalog{}, map[string][]byte{
		typeSpO2Sleep: fixtureBytes(t, typeSpO2Sleep),
	}, loadFixtureWindow(t))
	if err != nil {
		t.Fatalf("parse 0x26: %v", err)
	}
	result, ok := batch.Types[typeSpO2Sleep]
	if !ok {
		t.Fatal("0x26 has no type result")
	}
	if result.Status != "raw_only" || result.Rows != 0 || result.ProcessedThrough != nil {
		t.Fatalf("0x26 result = %+v, want raw_only with no watermark", result)
	}
	if len(batch.SpO2) != 0 {
		t.Fatalf("0x26 produced %d SpO2 rows", len(batch.SpO2))
	}
	if len(batch.Types) != 1 {
		t.Fatalf("types = %v, want only 0x26", batch.Types)
	}
}

func TestParseBlobsIgnoresEmptyAndUnknownParts(t *testing.T) {
	batch, err := ParseBlobs(loadFixtureCatalog(t), map[string][]byte{
		typeActivity: nil,
		"0x77":       []byte{1, 2, 3},
	}, loadFixtureWindow(t))
	if err != nil {
		t.Fatalf("parse blobs: %v", err)
	}
	if len(batch.Types) != 0 {
		t.Fatalf("types = %v, want no results for empty or unknown parts", batch.Types)
	}
	if batch.Types == nil {
		t.Fatal("Types must be non-nil for consumers")
	}
}

func TestParseBlobsWindowBoundsAreInclusive(t *testing.T) {
	at := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	window := Window{StartedAt: at, EndedAt: at}
	low := window.low()
	high := window.high()

	raw := append(hrFrame(low.Add(-time.Second).Unix(), 80), hrFrame(low.Unix(), 81)...)
	raw = append(raw, hrFrame(high.Unix(), 82)...)
	raw = append(raw, hrFrame(high.Add(time.Second).Unix(), 83)...)

	batch, err := ParseBlobs(Catalog{}, map[string][]byte{typeContinuousHR: raw}, window)
	if err != nil {
		t.Fatalf("parse blobs: %v", err)
	}
	if len(batch.HeartRate) != 2 {
		t.Fatalf("HR rows = %d, want the two boundary samples only", len(batch.HeartRate))
	}
	if batch.HeartRate[0].BPM != 81 || batch.HeartRate[1].BPM != 82 {
		t.Fatalf("boundary rows = %+v, want 81 at the lower bound and 82 at the upper bound", batch.HeartRate)
	}
	if result := batch.Types[typeContinuousHR]; result.Dropped != 2 {
		t.Fatalf("dropped = %d, want the two out-of-window samples", result.Dropped)
	}
}

func TestParseErrorReportsStructuralContext(t *testing.T) {
	raw := make([]byte, 8)
	binary.LittleEndian.PutUint32(raw[0:], uint32(time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC).Unix()))
	raw[4] = 200
	raw[5] = 200
	_, err := ParseBlobs(Catalog{}, map[string][]byte{typeContinuousHR: raw}, catalogWindow())
	var parseErr *ParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("error = %v, want a *ParseError", err)
	}
	message := parseErr.Error()
	for _, part := range []string{parseErr.Code, typeContinuousHR, "byte"} {
		if !strings.Contains(message, part) {
			t.Errorf("message %q does not mention %q", message, part)
		}
	}
}
