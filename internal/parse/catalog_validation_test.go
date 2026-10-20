package parse

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func catalogWindow() Window {
	return Window{
		StartedAt: time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC),
		EndedAt:   time.Date(2026, 7, 26, 1, 0, 0, 0, time.UTC),
	}
}

func TestParseBlobsClassifiesRoundMetadataErrors(t *testing.T) {
	const validTime = "2026-07-25T23:59:00.000"
	tests := []struct {
		name     string
		code     string
		segments string
		wantText string
		rawLen   int
		wantCode string
	}{
		{name: "malformed timestamp", code: typeActivity, rawLen: 16,
			segments: `[{"byteOffset":0,"roundStart":"not-a-time"}]`, wantText: "offsetless IST"},
		{name: "Z suffix", code: typeActivity, rawLen: 16,
			segments: `[{"byteOffset":0,"roundStart":"2026-07-25T23:59:00Z"}]`, wantText: "offsetless IST"},
		{name: "explicit offset", code: typeActivity, rawLen: 16,
			segments: `[{"byteOffset":0,"roundStart":"2026-07-25T23:59:00+05:30"}]`, wantText: "offsetless IST"},
		{name: "second bearing anchor", code: typeActivity, rawLen: 16,
			segments: `[{"byteOffset":0,"roundStart":"2026-07-25T23:59:30"}]`, wantText: "whole minute"},
		{name: "fraction bearing anchor", code: typeActivity, rawLen: 16,
			segments: `[{"byteOffset":0,"roundStart":"2026-07-25T23:59:00.500"}]`, wantText: "whole minute"},
		{name: "anchor outside accepted range", code: typeActivity, rawLen: 16,
			segments: `[{"byteOffset":0,"roundStart":"1970-01-01T00:00:00"}]`, wantText: "whole minute"},
		{name: "negative offset", code: typeActivity, rawLen: 16,
			segments: `[{"byteOffset":-8,"roundStart":"` + validTime + `"}]`, wantText: "round offset is negative"},
		{name: "offset at payload end", code: typeActivity, rawLen: 16,
			segments: `[{"byteOffset":16,"roundStart":"` + validTime + `"}]`, wantText: "outside the 16-byte payload",
			wantCode: ErrCodeInvalidFraming},
		{name: "SpO2 offset outside payload", code: typeSpO2, rawLen: 132,
			segments: `[{"byteOffset":0,"roundStart":"` + validTime + `"},{"byteOffset":132,"roundStart":"` + validTime + `"}]`,
			wantText: "outside the 132-byte payload", wantCode: ErrCodeInvalidFraming},
		{name: "first offset not zero", code: typeActivity, rawLen: 16,
			segments: `[{"byteOffset":8,"roundStart":"` + validTime + `"}]`, wantText: "first round offset must be 0"},
		{name: "activity offset not aligned", code: typeActivity, rawLen: 16,
			segments: `[{"byteOffset":4,"roundStart":"` + validTime + `"}]`, wantText: "aligned to stride 8",
			wantCode: ErrCodeInvalidFraming},
		{name: "temperature offset not aligned", code: typeTemperature, rawLen: 16,
			segments: `[{"byteOffset":4,"roundStart":"` + validTime + `"}]`, wantText: "aligned to stride 8",
			wantCode: ErrCodeInvalidFraming},
		{name: "descending offsets", code: typeActivity, rawLen: 24,
			segments: `[{"byteOffset":0,"roundStart":"` + validTime + `"},{"byteOffset":16,"roundStart":"` + validTime + `"},{"byteOffset":8,"roundStart":"` + validTime + `"}]`,
			wantText: "not strictly after"},
		{name: "duplicate offsets", code: typeActivity, rawLen: 16,
			segments: `[{"byteOffset":0,"roundStart":"` + validTime + `"},{"byteOffset":0,"roundStart":"` + validTime + `"}]`,
			wantText: "not strictly after"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw := `{"chunked":[{"code":"` + test.code + `","roundSegments":` + test.segments + `}]}`
			catalog, err := ParseCatalog([]byte(raw))
			if err != nil {
				t.Fatalf("parse catalog: %v", err)
			}
			_, err = ParseBlobs(catalog, map[string][]byte{test.code: make([]byte, test.rawLen)}, catalogWindow())
			wantCode := test.wantCode
			if wantCode == "" {
				wantCode = ErrCodeInvalidCatalog
			}
			var parseErr *ParseError
			if !errors.As(err, &parseErr) || parseErr.Code != wantCode {
				t.Fatalf("error = %v, want %s", err, wantCode)
			}
			if !strings.Contains(parseErr.Reason, test.wantText) {
				t.Fatalf("reason = %q, want it to mention %q", parseErr.Reason, test.wantText)
			}
		})
	}
}

func TestParseBlobsRejectsMissingRequiredRoundEntry(t *testing.T) {
	for _, code := range []string{typeActivity, typeStress, typeTemperature} {
		t.Run(code, func(t *testing.T) {
			_, err := ParseBlobs(Catalog{}, map[string][]byte{code: make([]byte, 8)}, catalogWindow())
			var parseErr *ParseError
			if !errors.As(err, &parseErr) || parseErr.Code != ErrCodeMissingAnchor {
				t.Fatalf("error = %v, want missing_anchor", err)
			}
		})
	}
}

func TestParseBlobsUsesValidMultiSegmentAnchors(t *testing.T) {
	raw := make([]byte, 24)
	for i := range 3 {
		raw[i*8+2] = byte(2 + i)
		raw[i*8+3] = byte(70 + i)
	}
	catalog, err := ParseCatalog([]byte(`{"chunked":[{"code":"0x01","roundSegments":[
		{"byteOffset":0,"roundStart":"2026-07-25T23:59:00.000"},
		{"byteOffset":16,"roundStart":"2026-07-26T01:00:00.000"}]}]}`))
	if err != nil {
		t.Fatalf("parse catalog: %v", err)
	}
	batch, err := ParseBlobs(catalog, map[string][]byte{typeActivity: raw}, catalogWindow())
	if err != nil {
		t.Fatalf("parse blobs: %v", err)
	}
	if batch.Types[typeActivity].Rows != 6 {
		t.Fatalf("rows = %d, want 6", batch.Types[typeActivity].Rows)
	}
	wantTimes := []time.Time{
		time.Date(2026, 7, 25, 18, 29, 0, 0, time.UTC),
		time.Date(2026, 7, 25, 18, 30, 0, 0, time.UTC),
		time.Date(2026, 7, 25, 19, 30, 0, 0, time.UTC),
	}
	for i, want := range wantTimes {
		if !batch.Steps[i].SampledAt.Equal(want) || batch.Steps[i].Steps != i+2 {
			t.Errorf("step %d = %+v, want %s steps %d", i, batch.Steps[i], want, i+2)
		}
		if !batch.HeartRate[i].SampledAt.Equal(want) || batch.HeartRate[i].BPM != 70+i {
			t.Errorf("hr %d = %+v, want %s bpm %d", i, batch.HeartRate[i], want, 70+i)
		}
	}
}

func TestParseBlobsRejectsInvalidAnchorWithoutRows(t *testing.T) {
	raw := make([]byte, 8)
	raw[2], raw[3] = 5, 70
	catalog, err := ParseCatalog([]byte(`{"chunked":[{"code":"0x01","roundStart":"bad"}]}`))
	if err != nil {
		t.Fatalf("parse catalog: %v", err)
	}
	batch, err := ParseBlobs(catalog, map[string][]byte{typeActivity: raw}, catalogWindow())
	var parseErr *ParseError
	if !errors.As(err, &parseErr) || parseErr.Code != ErrCodeInvalidCatalog {
		t.Fatalf("error = %v, want invalid_catalog", err)
	}
	if batch.Steps != nil || batch.HeartRate != nil || len(batch.Types) != 0 {
		t.Fatalf("invalid anchor produced a batch: %+v", batch)
	}
}

func TestParseBlobsRejectsInvalidWindow(t *testing.T) {
	moment := time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		window Window
	}{
		{name: "zero start", window: Window{EndedAt: moment}},
		{name: "zero end", window: Window{StartedAt: moment}},
		{name: "end before start", window: Window{StartedAt: moment.Add(time.Hour), EndedAt: moment}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseBlobs(Catalog{}, map[string][]byte{}, test.window)
			var parseErr *ParseError
			if !errors.As(err, &parseErr) || parseErr.Code != ErrCodeInvalidWindow {
				t.Fatalf("error = %v, want invalid_window", err)
			}
		})
	}
}

func TestSameBytesWithAnotherCatalogDecodeToOtherTimes(t *testing.T) {
	raw := make([]byte, 8)
	raw[2], raw[3] = 4, 70
	first := Catalog{Chunked: []CatalogEntry{{
		Code: typeActivity, RoundStart: "2026-07-25T23:59:00",
	}}}
	second := Catalog{Chunked: []CatalogEntry{{
		Code: typeActivity, RoundStart: "2026-07-25T23:00:00",
	}}}
	earlier, err := ParseBlobs(second, map[string][]byte{typeActivity: raw}, catalogWindow())
	if err != nil {
		t.Fatalf("parse with the early anchor: %v", err)
	}
	later, err := ParseBlobs(first, map[string][]byte{typeActivity: raw}, catalogWindow())
	if err != nil {
		t.Fatalf("parse with the late anchor: %v", err)
	}
	if len(earlier.Steps) != 1 || len(later.Steps) != 1 {
		t.Fatalf("step rows = %d/%d, want one each", len(earlier.Steps), len(later.Steps))
	}
	if !earlier.Steps[0].SampledAt.Before(later.Steps[0].SampledAt) {
		t.Fatalf("anchors produced %s and %s, want different minutes",
			formatFixtureTime(earlier.Steps[0].SampledAt), formatFixtureTime(later.Steps[0].SampledAt))
	}
}

func TestMinuteTypeRejectsInteriorTrailingBytes(t *testing.T) {
	raw := make([]byte, 28)
	catalog := Catalog{Chunked: []CatalogEntry{{
		Code: typeActivity,
		RoundSegments: []RoundSegment{
			{ByteOffset: 0, RoundStart: "2026-07-25T23:59:00"},
			{ByteOffset: 8, RoundStart: "2026-07-26T00:30:00"},
			{ByteOffset: 20, RoundStart: "2026-07-26T01:00:00"},
		},
	}}}
	_, err := ParseBlobs(catalog, map[string][]byte{typeActivity: raw}, catalogWindow())
	var parseErr *ParseError
	if !errors.As(err, &parseErr) || parseErr.Code != ErrCodeInvalidFraming {
		t.Fatalf("error = %v, want invalid_framing", err)
	}
}

func TestMinuteTypeRetainsCompleteFinalPrefix(t *testing.T) {
	raw := []byte{10, 20, 30, 40, 50}
	catalog := Catalog{Chunked: []CatalogEntry{{
		Code:       typeStress,
		RoundStart: "2026-07-25T23:59:00",
	}}}
	batch, err := ParseBlobs(catalog, map[string][]byte{typeStress: raw}, catalogWindow())
	if err != nil {
		t.Fatalf("parse stress: %v", err)
	}
	result := batch.Types[typeStress]
	if len(batch.Stress) != 5 || result.TrailingBytes != 0 || result.Status != "accepted" {
		t.Fatalf("stress rows=%d result=%+v", len(batch.Stress), result)
	}
}
