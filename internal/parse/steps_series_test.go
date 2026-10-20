package parse

import (
	"errors"
	"reflect"
	"testing"
)

func TestStepSeriesMatchesZeppMinutes(t *testing.T) {
	batch, err := ParseBlobs(loadFixtureCatalog(t),
		map[string][]byte{typeActivity: fixtureBytes(t, typeActivity)}, loadFixtureWindow(t))
	if err != nil {
		t.Fatalf("parse 0x01: %v", err)
	}
	zepp := zeppSeries(t, "activity-minute.csv")
	nonzero := 0
	for _, row := range batch.Steps {
		if row.Steps == 0 {
			continue
		}
		nonzero++
		want, ok := zepp[row.SampledAt.Unix()]
		if !ok || want != row.Steps {
			t.Errorf("step minute %s = %d, Zepp %d (present=%t)",
				formatFixtureTime(row.SampledAt), row.Steps, want, ok)
		}
	}
	if nonzero != len(zepp) {
		t.Fatalf("nonzero step minutes = %d, Zepp rows = %d", nonzero, len(zepp))
	}
}

func TestStepSeriesIsDeterministic(t *testing.T) {
	catalog := loadFixtureCatalog(t)
	window := loadFixtureWindow(t)
	raw := fixtureBytes(t, typeActivity)
	first, err := ParseBlobs(catalog, map[string][]byte{typeActivity: raw}, window)
	if err != nil {
		t.Fatalf("parse 0x01: %v", err)
	}
	second, err := ParseBlobs(catalog, map[string][]byte{typeActivity: raw}, window)
	if err != nil {
		t.Fatalf("parse 0x01 again: %v", err)
	}
	if !reflect.DeepEqual(first.Steps, second.Steps) || !reflect.DeepEqual(first.HeartRate, second.HeartRate) {
		t.Fatal("the same bytes and catalog produced different rows")
	}
}

func TestStepSeriesRetainsCompletePrefixOnTrailingFragment(t *testing.T) {
	catalog := loadFixtureCatalog(t)
	window := loadFixtureWindow(t)
	raw := fixtureBytes(t, typeActivity)[:16892]
	batch, err := ParseBlobs(catalog, map[string][]byte{typeActivity: raw}, window)
	if err != nil {
		t.Fatalf("parse truncated 0x01: %v", err)
	}
	result := batch.Types[typeActivity]
	if result.TrailingBytes != 4 || result.Status != "partial" {
		t.Fatalf("result = %+v, want a four byte trailing fragment", result)
	}
	if len(batch.Steps) != 2108 || len(batch.HeartRate) != 2108 {
		t.Fatalf("rows = %d steps %d hr, want 2108 each", len(batch.Steps), len(batch.HeartRate))
	}
	last := batch.Steps[len(batch.Steps)-1].SampledAt
	if got := formatFixtureTime(last); got != "2026-06-22T16:57:00Z" {
		t.Fatalf("last step minute = %s, want 2026-06-22T16:57:00Z", got)
	}
	sum := 0
	for _, row := range batch.Steps {
		sum += row.Steps
	}
	if sum != 4564 {
		t.Fatalf("step sum = %d, want 4564", sum)
	}
}

func TestStepSeriesRejectsInteriorBoundary(t *testing.T) {
	catalog := Catalog{Chunked: []CatalogEntry{{
		Code: typeActivity,
		RoundSegments: []RoundSegment{
			{ByteOffset: 0, RoundStart: "2026-06-21T11:16:00"},
			{ByteOffset: 633, RoundStart: "2026-06-21T12:35:00"},
		},
	}}}
	_, err := ParseBlobs(catalog,
		map[string][]byte{typeActivity: fixtureBytes(t, typeActivity)}, loadFixtureWindow(t))
	var parseErr *ParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("error = %v, want a structural *ParseError", err)
	}
}
