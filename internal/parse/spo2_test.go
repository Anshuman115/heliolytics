package parse

import (
	"encoding/binary"
	"errors"
	"testing"
	"time"
)

func spo2Window() Window {
	return Window{
		StartedAt: time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC),
		EndedAt:   time.Date(2026, 6, 22, 11, 0, 0, 0, time.UTC),
	}
}

func parseSpO2Fixture(t *testing.T, payload []byte, offsets []int) ([]SpO2Row, TypeResult, error) {
	t.Helper()
	batch, err := ParseBlobs(segmentCatalog(typeSpO2, offsets),
		map[string][]byte{typeSpO2: payload}, spo2Window())
	if err != nil {
		return nil, TypeResult{}, err
	}
	return batch.SpO2, batch.Types[typeSpO2], nil
}

func TestSpO2TwoRoundsUseCatalogOffsets(t *testing.T) {
	boundaries := loadSpo2Boundaries(t)
	payload := boundaries.file(t, "normal-two-rounds.bin")
	rows, result, err := parseSpO2Fixture(t, payload, boundaries.offsets(t, "normal-two-rounds.bin"))
	if err != nil {
		t.Fatalf("parse 0x25: %v", err)
	}
	if len(rows) != 2 || result.Rows != 2 || result.Dropped != 0 || result.TrailingBytes != 0 {
		t.Fatalf("rows=%v result=%+v, want two accepted rounds", rows, result)
	}
	wantTimes := []time.Time{
		time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC),
		time.Date(2026, 6, 22, 10, 1, 0, 0, time.UTC),
	}
	for i, want := range wantTimes {
		if !rows[i].SampledAt.Equal(want) || rows[i].Pct != 98 || rows[i].Source != typeSpO2 {
			t.Errorf("row %d = %+v, want 98 percent at %s", i, rows[i], formatFixtureTime(want))
		}
	}
	if rows[0].Mode != "manual" || rows[1].Mode != "auto" {
		t.Errorf("modes = %q/%q, want manual/auto", rows[0].Mode, rows[1].Mode)
	}
}

func TestSpO2SleepTwoRoundsStayRawOnly(t *testing.T) {
	boundaries := loadSpo2Boundaries(t)
	payload := boundaries.file(t, "sleep-two-rounds.bin")
	batch, err := ParseBlobs(segmentCatalog(typeSpO2Sleep, boundaries.offsets(t, "sleep-two-rounds.bin")),
		map[string][]byte{typeSpO2Sleep: payload}, spo2Window())
	if err != nil {
		t.Fatalf("parse 0x26: %v", err)
	}
	result := batch.Types[typeSpO2Sleep]
	if result.Status != "raw_only" || result.Rows != 0 || result.Dropped != 2 || result.ProcessedThrough != nil {
		t.Fatalf("result = %+v, want raw_only with two placeholders", result)
	}
	if len(result.Warnings) != 1 || result.Warnings[0].String() != "spo2_sleep_placeholder offset=1 count=2" {
		t.Fatalf("warnings = %v", result.Warnings)
	}
	if len(batch.SpO2) != 0 {
		t.Fatalf("0x26 produced %d measured rows", len(batch.SpO2))
	}
}

func TestSpO2WrongVersionIsRejected(t *testing.T) {
	boundaries := loadSpo2Boundaries(t)
	_, _, err := parseSpO2Fixture(t, boundaries.file(t, "wrong-version.bin"),
		boundaries.offsets(t, "wrong-version.bin"))
	var parseErr *ParseError
	if !errors.As(err, &parseErr) || parseErr.Code != ErrCodeUnsupportedVersion {
		t.Fatalf("error = %v, want unsupported_version", err)
	}
}

func TestSpO2TrailingFragmentKeepsCompleteRounds(t *testing.T) {
	boundaries := loadSpo2Boundaries(t)
	payload := boundaries.file(t, "normal-trailing-fragment.bin")
	rows, result, err := parseSpO2Fixture(t, payload, boundaries.offsets(t, "normal-trailing-fragment.bin"))
	if err != nil {
		t.Fatalf("parse 0x25: %v", err)
	}
	if len(rows) != 2 || result.TrailingBytes != 3 || result.Status != "partial" {
		t.Fatalf("rows=%d result=%+v, want two rows and a three byte fragment", len(rows), result)
	}
	if len(result.Warnings) != 1 || result.Warnings[0].String() != "trailing_fragment offset=132 count=3" {
		t.Fatalf("warnings = %v", result.Warnings)
	}
}

func TestSpO2ValueAndModeBoundaries(t *testing.T) {
	boundaries := loadSpo2Boundaries(t)
	accepted := []struct {
		name string
		pct  int
		mode string
	}{
		{name: "manual-98", pct: 98, mode: "manual"},
		{name: "automatic-98", pct: 98, mode: "auto"},
		{name: "pct-70", pct: 70, mode: "manual"},
		{name: "pct-100", pct: 100, mode: "manual"},
	}
	for _, test := range accepted {
		t.Run(test.name, func(t *testing.T) {
			rows, result, err := parseSpO2Fixture(t, boundaries.hexCase(t, test.name), nil)
			if err != nil {
				t.Fatalf("parse case: %v", err)
			}
			if len(rows) != 1 || result.Dropped != 0 {
				t.Fatalf("rows=%v result=%+v, want one accepted sample", rows, result)
			}
			if rows[0].Pct != test.pct || rows[0].Mode != test.mode {
				t.Fatalf("row = %+v, want %d percent %s", rows[0], test.pct, test.mode)
			}
		})
	}
	for _, name := range []string{"pct-69", "pct-101", "pct-127-masked"} {
		t.Run(name, func(t *testing.T) {
			rows, result, err := parseSpO2Fixture(t, boundaries.hexCase(t, name), nil)
			if err != nil {
				t.Fatalf("parse case: %v", err)
			}
			if len(rows) != 0 || result.Dropped != 1 {
				t.Fatalf("rows=%v result=%+v, want the sample dropped", rows, result)
			}
		})
	}
}

func TestSpO2RepeatedVersionBytesInsideTailAreNotFraming(t *testing.T) {
	boundaries := loadSpo2Boundaries(t)
	rows, result, err := parseSpO2Fixture(t, boundaries.hexCase(t, "repeated-02-tail"), nil)
	if err != nil {
		t.Fatalf("parse case: %v", err)
	}
	if len(rows) != 1 || rows[0].Pct != 98 || result.TrailingBytes != 0 {
		t.Fatalf("rows=%v result=%+v, want one 98 percent sample", rows, result)
	}
}

func TestSpO2RejectsHeaderlessAndIncompleteRounds(t *testing.T) {
	boundaries := loadSpo2Boundaries(t)
	tests := []struct {
		name string
		code string
	}{
		{name: "headerless-65", code: ErrCodeUnsupportedVersion},
		{name: "incomplete-first-record", code: ErrCodeInvalidFraming},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := parseSpO2Fixture(t, boundaries.hexCase(t, test.name), nil)
			var parseErr *ParseError
			if !errors.As(err, &parseErr) || parseErr.Code != test.code {
				t.Fatalf("error = %v, want %s", err, test.code)
			}
		})
	}
}

func TestSpO2TruncatedFinalRoundKeepsEarlierRecords(t *testing.T) {
	boundaries := loadSpo2Boundaries(t)
	payload := boundaries.hexCase(t, "truncated-final-round")
	rows, result, err := parseSpO2Fixture(t, payload, boundaries.offsets(t, "truncated-final-round"))
	if err != nil {
		t.Fatalf("parse case: %v", err)
	}
	if len(rows) != 1 || result.TrailingBytes != 30 || result.Status != "partial" {
		t.Fatalf("rows=%d result=%+v, want one record and a 30 byte fragment", len(rows), result)
	}
	if len(result.Warnings) != 1 || result.Warnings[0].String() != "trailing_fragment offset=67 count=30" {
		t.Fatalf("warnings = %v", result.Warnings)
	}
}

func TestSpO2SleepRejectsWrongVersion(t *testing.T) {
	boundaries := loadSpo2Boundaries(t)
	batch, err := ParseBlobs(Catalog{}, map[string][]byte{
		typeSpO2Sleep: boundaries.hexCase(t, "sleep-wrong-version"),
	}, spo2Window())
	var parseErr *ParseError
	if !errors.As(err, &parseErr) || parseErr.Code != ErrCodeUnsupportedVersion {
		t.Fatalf("error = %v, want unsupported_version", err)
	}
	if len(batch.Types) != 0 {
		t.Fatalf("failed type returned results: %v", batch.Types)
	}
}

func TestSpO2SleepNonPlaceholderStaysRaw(t *testing.T) {
	boundaries := loadSpo2Boundaries(t)
	batch, err := ParseBlobs(Catalog{}, map[string][]byte{
		typeSpO2Sleep: boundaries.hexCase(t, "sleep-non-placeholder"),
	}, spo2Window())
	if err != nil {
		t.Fatalf("parse 0x26: %v", err)
	}
	result := batch.Types[typeSpO2Sleep]
	if result.Rows != 0 || result.Dropped != 1 || result.Status != "raw_only" {
		t.Fatalf("result = %+v, want an unvalidated raw record", result)
	}
	if len(result.Warnings) != 1 || result.Warnings[0].String() != "spo2_sleep_unvalidated offset=1 count=1" {
		t.Fatalf("warnings = %v", result.Warnings)
	}
}

func TestSpO2RejectsSamplesBeyondTheNextRoundCursor(t *testing.T) {
	record := make([]byte, spo2Stride)
	binary.LittleEndian.PutUint32(record[0:], uint32(time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC).Unix()))
	record[4] = 0x62
	payload := append([]byte{headeredRoundVersion}, record...)
	payload = append(payload, headeredRoundVersion)
	payload = append(payload, record...)

	catalog := Catalog{Chunked: []CatalogEntry{{
		Code: typeSpO2,
		RoundSegments: []RoundSegment{
			{ByteOffset: 0, RoundStart: "2026-06-22T15:00:00"},
			{ByteOffset: 66, RoundStart: "2026-06-22T14:00:00"},
		},
	}}}
	batch, err := ParseBlobs(catalog, map[string][]byte{typeSpO2: payload}, spo2Window())
	if err != nil {
		t.Fatalf("parse 0x25: %v", err)
	}
	result := batch.Types[typeSpO2]
	if len(batch.SpO2) != 1 || result.Dropped != 1 {
		t.Fatalf("rows=%d result=%+v, want the first round dropped by the cursor bound",
			len(batch.SpO2), result)
	}
}

func TestSpO2SleepRealFixtureIsRawOnly(t *testing.T) {
	batch, err := ParseBlobs(Catalog{}, map[string][]byte{
		typeSpO2Sleep: fixtureBytes(t, typeSpO2Sleep),
	}, loadFixtureWindow(t))
	if err != nil {
		t.Fatalf("parse 0x26: %v", err)
	}
	result := batch.Types[typeSpO2Sleep]
	if result.Rows != 0 || result.Dropped != 13 || result.ProcessedThrough != nil {
		t.Fatalf("result = %+v, want 13 raw placeholders", result)
	}
	if len(result.Warnings) != 1 || result.Warnings[0].String() != "spo2_sleep_placeholder offset=1 count=13" {
		t.Fatalf("warnings = %v", result.Warnings)
	}
}
