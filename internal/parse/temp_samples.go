package parse

import (
	"encoding/binary"
	"sort"
)

const (
	temperatureStride        = 8
	temperatureAbsent        = int16(-1)
	temperatureSentinel      = int16(0x7FFF)
	temperatureMaxHundredths = 5000
)

// parseTemperature decodes the signed int16 hundredths field at record+2 and
// keeps the raw precision. The surrounding padding bytes are not a wear flag.
func parseTemperature(raw []byte, catalog Catalog, window Window) ([]TemperatureRow, TypeResult, error) {
	records, trailing, err := minuteRecords(typeTemperature, raw,
		FindEntry(catalog, typeTemperature), temperatureStride)
	if err != nil {
		return nil, TypeResult{}, err
	}
	byTime := make(map[int64]TemperatureRow, len(records))
	dropped := 0
	for _, record := range records {
		value := int16(binary.LittleEndian.Uint16(raw[record.off+2:]))
		if !window.contains(record.at) || value == temperatureAbsent ||
			value == temperatureSentinel || value < 0 || value > temperatureMaxHundredths {
			dropped++
			continue
		}
		byTime[record.at.Unix()] = TemperatureRow{
			SampledAt: record.at, HundredthsC: int(value),
		}
	}
	out := make([]TemperatureRow, 0, len(byTime))
	for _, row := range byTime {
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SampledAt.Before(out[j].SampledAt) })
	result := TypeResult{
		Status:        statusFor(trailing),
		Rows:          len(out),
		Dropped:       dropped,
		TrailingBytes: trailing,
	}
	if trailing > 0 {
		result.Warnings = append(result.Warnings, Warning{
			Code: "trailing_fragment", ByteOffset: len(raw) - trailing, Count: trailing,
		})
	}
	for i := range out {
		result.ProcessedThrough = maxTime(result.ProcessedThrough, out[i].SampledAt)
	}
	return out, result, nil
}
