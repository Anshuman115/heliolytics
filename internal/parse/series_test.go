package parse

import (
	"encoding/binary"
	"errors"
	"testing"
	"time"
)

func scalarRecord(epoch int64, timezone, value byte, stride int) []byte {
	record := make([]byte, stride)
	binary.LittleEndian.PutUint32(record[0:], uint32(epoch))
	record[4] = timezone
	record[5] = value
	return record
}

func TestHRVAndRespirationMatchRealFixtures(t *testing.T) {
	window := loadFixtureWindow(t)
	hrv, hrvResult, err := parseHRV(fixtureBytes(t, typeHRV), window)
	if err != nil {
		t.Fatalf("parse 0x49: %v", err)
	}
	if len(hrv) != 306 || hrvResult.Rows != 306 || hrvResult.Dropped != 0 {
		t.Fatalf("HRV rows=%d result=%+v, want 306 accepted", len(hrv), hrvResult)
	}
	checkScalarRange(t, "hrv", hrv, 306, 39, 131)

	respiration, respirationResult, err := parseRespiration(fixtureBytes(t, typeRespiration), window)
	if err != nil {
		t.Fatalf("parse 0x38: %v", err)
	}
	if len(respiration) != 413 || respirationResult.Rows != 413 || respirationResult.Dropped != 0 {
		t.Fatalf("respiration rows=%d result=%+v, want 413 accepted", len(respiration), respirationResult)
	}
	checkScalarRange(t, "respiration", respiration, 413, 14, 19)
}

func TestScalarSeriesRejectsNonIstTimezone(t *testing.T) {
	at := time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		code  string
		value byte
	}{
		{name: "hrv", code: typeHRV, value: 50},
		{name: "respiration", code: typeRespiration, value: 14},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stride := 6
			if test.code == typeRespiration {
				stride = 8
			}
			raw := scalarRecord(at.Unix(), 0, test.value, stride)
			_, _, err := parseScalarsForTest(test.code, raw, catalogWindow())
			var parseErr *ParseError
			if !errors.As(err, &parseErr) || parseErr.Code != ErrCodeUnsupportedTimezone {
				t.Fatalf("error = %v, want unsupported_timezone", err)
			}
		})
	}
}

func parseScalarsForTest(code string, raw []byte, window Window) ([]ScalarRow, TypeResult, error) {
	switch code {
	case typeHRV:
		return parseHRV(raw, window)
	default:
		return parseRespiration(raw, window)
	}
}

func TestScalarSeriesRetainsCompletePrefix(t *testing.T) {
	base := time.Date(2026, 7, 25, 23, 0, 0, 0, time.UTC)
	hrvRaw := append(scalarRecord(base.Unix(), istTimezoneQuarter, 50, 6),
		scalarRecord(base.Add(time.Minute).Unix(), istTimezoneQuarter, 51, 6)...)
	hrvRaw = append(hrvRaw, 0x01, 0x02, 0x03)
	hrv, hrvResult, err := parseHRV(hrvRaw, catalogWindow())
	if err != nil {
		t.Fatalf("parse HRV: %v", err)
	}
	if len(hrv) != 2 || hrvResult.TrailingBytes != 3 || hrvResult.Status != "partial" {
		t.Fatalf("HRV rows=%d result=%+v, want 2 rows and a three byte fragment", len(hrv), hrvResult)
	}
	if len(hrvResult.Warnings) != 1 || hrvResult.Warnings[0].ByteOffset != 12 || hrvResult.Warnings[0].Count != 3 {
		t.Fatalf("warnings = %v, want trailing_fragment offset=12 count=3", hrvResult.Warnings)
	}

	respirationRaw := append(scalarRecord(base.Unix(), istTimezoneQuarter, 15, 8),
		scalarRecord(base.Add(time.Minute).Unix(), istTimezoneQuarter, 16, 8)...)
	respirationRaw = append(respirationRaw, 0x01, 0x02, 0x03, 0x04, 0x05)
	respiration, respirationResult, err := parseRespiration(respirationRaw, catalogWindow())
	if err != nil {
		t.Fatalf("parse respiration: %v", err)
	}
	if len(respiration) != 2 || respirationResult.TrailingBytes != 5 {
		t.Fatalf("respiration rows=%d result=%+v, want 2 rows and a five byte fragment",
			len(respiration), respirationResult)
	}
}

func TestScalarSeriesDropsValuesOutsideTheirDomain(t *testing.T) {
	base := time.Date(2026, 7, 25, 23, 0, 0, 0, time.UTC)
	hrvRaw := append(scalarRecord(base.Unix(), istTimezoneQuarter, 0, 6),
		scalarRecord(base.Add(time.Minute).Unix(), istTimezoneQuarter, 255, 6)...)
	hrv, hrvResult, err := parseHRV(hrvRaw, catalogWindow())
	if err != nil {
		t.Fatalf("parse HRV: %v", err)
	}
	if len(hrv) != 1 || hrvResult.Dropped != 1 || hrv[0].Value != 255 {
		t.Fatalf("HRV rows=%v result=%+v, want only the 255 ms value", hrv, hrvResult)
	}

	respirationRaw := append(scalarRecord(base.Unix(), istTimezoneQuarter, 3, 8),
		scalarRecord(base.Add(time.Minute).Unix(), istTimezoneQuarter, 41, 8)...)
	respirationRaw = append(respirationRaw, scalarRecord(base.Add(2*time.Minute).Unix(), istTimezoneQuarter, 4, 8)...)
	respirationRaw = append(respirationRaw, scalarRecord(base.Add(3*time.Minute).Unix(), istTimezoneQuarter, 40, 8)...)
	respiration, respirationResult, err := parseRespiration(respirationRaw, catalogWindow())
	if err != nil {
		t.Fatalf("parse respiration: %v", err)
	}
	if len(respiration) != 2 || respirationResult.Dropped != 2 {
		t.Fatalf("respiration rows=%v result=%+v, want the 4 and 40 bounds only", respiration, respirationResult)
	}
}
