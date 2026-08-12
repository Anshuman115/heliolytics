package parse

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestParseBlobsRejectsInvalidRoundMetadata(t *testing.T) {
	validTime := "2026-07-25T23:59:00.000"
	tests := []struct {
		name, code, segments, wantText string
		rawLen                         int
	}{
		{name: "malformed timestamp", code: "0x01", rawLen: 16, segments: `[{
			"byteOffset":0,"roundStart":"not-a-time"}]`, wantText: "plausible timestamp"},
		{name: "trailing timestamp junk", code: "0x01", rawLen: 16, segments: `[{
			"byteOffset":0,"roundStart":"2026-07-25T23:59:00junk"}]`, wantText: "plausible timestamp"},
		{name: "implausible timestamp", code: "0x01", rawLen: 16, segments: `[{
			"byteOffset":0,"roundStart":"1970-01-01T00:00:00"}]`, wantText: "plausible timestamp"},
		{name: "negative offset", code: "0x01", rawLen: 16, segments: `[{
			"byteOffset":-8,"roundStart":"` + validTime + `"}]`, wantText: "outside payload"},
		{name: "offset at payload end", code: "0x01", rawLen: 16, segments: `[{
			"byteOffset":16,"roundStart":"` + validTime + `"}]`, wantText: "outside payload"},
		{name: "activity offset not aligned", code: "0x01", rawLen: 16, segments: `[{
			"byteOffset":4,"roundStart":"` + validTime + `"}]`, wantText: "stride 8"},
		{name: "temperature offset not aligned", code: "0x2E", rawLen: 16, segments: `[{
			"byteOffset":4,"roundStart":"` + validTime + `"}]`, wantText: "stride 8"},
		{name: "descending offsets", code: "0x01", rawLen: 16, segments: `[{
			"byteOffset":8,"roundStart":"` + validTime + `"},{
			"byteOffset":0,"roundStart":"` + validTime + `"}]`, wantText: "not strictly after"},
		{name: "duplicate offsets", code: "0x01", rawLen: 16, segments: `[{
			"byteOffset":0,"roundStart":"` + validTime + `"},{
			"byteOffset":0,"roundStart":"` + validTime + `"}]`, wantText: "not strictly after"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			catalog := []byte(`{"chunked":[{"code":"` + tt.code + `","roundSegments":` + tt.segments + `}]}`)
			_, err := ParseBlobs(catalog, map[string][]byte{tt.code: make([]byte, tt.rawLen)}, time.Time{})
			if !errors.Is(err, ErrInvalidCatalog) || !strings.Contains(err.Error(), tt.wantText) {
				t.Fatalf("error=%v, want ErrInvalidCatalog containing %q", err, tt.wantText)
			}
		})
	}
}

func TestParseBlobsRejectsMissingRequiredRoundEntry(t *testing.T) {
	for _, code := range []string{"0x13", "0x2E"} {
		t.Run(code, func(t *testing.T) {
			_, err := ParseBlobs([]byte(`{"chunked":[]}`), map[string][]byte{code: make([]byte, 8)}, time.Now().UTC())
			if !errors.Is(err, ErrInvalidCatalog) || !strings.Contains(err.Error(), "missing catalog entry") {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestParseBlobsUsesValidMultiSegmentAnchors(t *testing.T) {
	raw := make([]byte, 24)
	raw[2], raw[10], raw[18] = 2, 3, 4
	raw[3], raw[11], raw[19] = 70, 71, 72
	catalog := []byte(`{"chunked":[{"code":"0x01","roundSegments":[
		{"byteOffset":0,"roundStart":"2026-07-25T23:59:00.000"},
		{"byteOffset":16,"roundStart":"2026-07-26T01:00:00.000"}
	]}]}`)
	batch, err := ParseBlobs(catalog, map[string][]byte{"0x01": raw}, time.Time{})
	if err != nil {
		t.Fatalf("parse blobs: %v", err)
	}
	if batch.StepsByDay["2026-07-25"] != 2 || batch.StepsByDay["2026-07-26"] != 7 {
		t.Fatalf("steps=%v", batch.StepsByDay)
	}
	if len(batch.StepSeries) != 3 || len(batch.HrSeries) != 3 {
		t.Fatalf("step points=%d hr points=%d", len(batch.StepSeries), len(batch.HrSeries))
	}
}

func TestInvalidRoundAnchorNeverProducesEpochRows(t *testing.T) {
	raw := make([]byte, 8)
	raw[2], raw[3] = 5, 70
	catalog := []byte(`{"chunked":[{"code":"0x01","roundStart":"bad"}]}`)
	batch, err := ParseBlobs(catalog, map[string][]byte{"0x01": raw}, time.Now().UTC())
	if !errors.Is(err, ErrInvalidCatalog) {
		t.Fatalf("error=%v, want ErrInvalidCatalog", err)
	}
	if len(batch.StepSeries) != 0 || len(batch.HrSeries) != 0 || len(batch.StepsByDay) != 0 {
		t.Fatalf("invalid anchor produced rows: %+v", batch)
	}
	if rows := ParseStepSeries(raw, catalog, time.Now().UTC()); len(rows) != 0 {
		t.Fatalf("invalid anchor produced direct step rows: %+v", rows)
	}
}

func TestParseBlobsUsesOnlyPlausibleFetchEndFallback(t *testing.T) {
	raw := make([]byte, 8)
	raw[2] = 5
	fetchEnd := time.Date(2024, 7, 25, 12, 0, 0, 0, time.UTC)
	batch, err := ParseBlobs(nil, map[string][]byte{"0x01": raw}, fetchEnd)
	if err != nil {
		t.Fatalf("safe fallback: %v", err)
	}
	if len(batch.StepSeries) != 1 || !batch.StepSeries[0].SampledAt.Equal(fetchEnd) {
		t.Fatalf("step series=%+v", batch.StepSeries)
	}
	_, err = ParseBlobs(nil, map[string][]byte{"0x01": raw}, time.Time{})
	if !errors.Is(err, ErrInvalidCatalog) {
		t.Fatalf("unsafe fallback error=%v, want ErrInvalidCatalog", err)
	}
}
