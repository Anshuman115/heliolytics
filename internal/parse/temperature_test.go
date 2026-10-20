package parse

import (
	"encoding/binary"
	"testing"
)

func temperatureRecord(hundredths int16) []byte {
	record := make([]byte, temperatureStride)
	record[0], record[1] = 0xFF, 0x7F
	binary.LittleEndian.PutUint16(record[2:], uint16(hundredths))
	record[4], record[5], record[6], record[7] = 0x5A, 0x5A, 0x5A, 0x5A
	return record
}

func TestTemperatureRealFixtureKeepsHundredths(t *testing.T) {
	rows, result, err := parseTemperature(fixtureBytes(t, typeTemperature),
		loadFixtureCatalog(t), loadFixtureWindow(t))
	if err != nil {
		t.Fatalf("parse 0x2E: %v", err)
	}
	if len(rows) != 2120 || result.Rows != 2120 || result.Dropped != 0 {
		t.Fatalf("rows=%d result=%+v, want 2120 raw samples", len(rows), result)
	}
	if rows[0].HundredthsC != 3212 {
		t.Fatalf("first temperature = %d hundredths, want 3212", rows[0].HundredthsC)
	}
	minimum, maximum := rows[0].HundredthsC, rows[0].HundredthsC
	for _, row := range rows {
		minimum = min(minimum, row.HundredthsC)
		maximum = max(maximum, row.HundredthsC)
	}
	if minimum != 2868 || maximum != 3500 {
		t.Fatalf("temperature range = %d..%d, want 2868..3500", minimum, maximum)
	}
}

func TestTemperatureSentinelsAndDomain(t *testing.T) {
	accepted := []int16{3212, 0, 5000}
	dropped := []int16{-1, 0x7FFF, 5001, -100}
	var raw []byte
	for _, value := range accepted {
		raw = append(raw, temperatureRecord(value)...)
	}
	for _, value := range dropped {
		raw = append(raw, temperatureRecord(value)...)
	}
	catalog := Catalog{Chunked: []CatalogEntry{{
		Code: typeTemperature, RoundStart: "2026-07-25T23:59:00",
	}}}
	batch, err := ParseBlobs(catalog, map[string][]byte{typeTemperature: raw}, catalogWindow())
	if err != nil {
		t.Fatalf("parse 0x2E: %v", err)
	}
	if len(batch.Temperature) != len(accepted) {
		t.Fatalf("rows = %d, want %d accepted samples", len(batch.Temperature), len(accepted))
	}
	for i, want := range accepted {
		if batch.Temperature[i].HundredthsC != int(want) {
			t.Errorf("row %d = %d hundredths, want %d", i, batch.Temperature[i].HundredthsC, want)
		}
	}
	result := batch.Types[typeTemperature]
	if result.Dropped != len(dropped) {
		t.Errorf("dropped = %d, want %d", result.Dropped, len(dropped))
	}
}
