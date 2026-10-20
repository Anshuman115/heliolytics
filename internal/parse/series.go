package parse

import (
	"encoding/binary"
	"sort"
)

type scalarSpec struct {
	code       string
	stride     int
	valueAt    int
	timezoneAt int
	accept     func(int) bool
}

var hrvSpec = scalarSpec{
	code: typeHRV, stride: 6, valueAt: 5, timezoneAt: 4,
	accept: func(value int) bool { return value > 0 && value <= 300 },
}

var respirationSpec = scalarSpec{
	code: typeRespiration, stride: 8, valueAt: 5, timezoneAt: 4,
	accept: func(value int) bool { return value >= 4 && value <= 40 },
}

func parseHRV(raw []byte, window Window) ([]ScalarRow, TypeResult, error) {
	return parseScalarRecords(hrvSpec, raw, window)
}

func parseRespiration(raw []byte, window Window) ([]ScalarRow, TypeResult, error) {
	return parseScalarRecords(respirationSpec, raw, window)
}

// parseStress decodes 0x13 as one byte per anchored minute. The wire value
// 0xff is a missing reading and values above 100 are out of domain.
func parseStress(raw []byte, catalog Catalog, window Window) ([]ScalarRow, TypeResult, error) {
	records, trailing, err := minuteRecords(typeStress, raw, FindEntry(catalog, typeStress), 1)
	if err != nil {
		return nil, TypeResult{}, err
	}
	byTime := make(map[int64]ScalarRow, len(records))
	dropped := 0
	for _, record := range records {
		value := raw[record.off]
		if !window.contains(record.at) || value == stepsMissing || value > 100 {
			dropped++
			continue
		}
		byTime[record.at.Unix()] = ScalarRow{SampledAt: record.at, Value: float64(value)}
	}
	out := make([]ScalarRow, 0, len(byTime))
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

// parseScalarRecords decodes records that carry their own UTC epoch, a device
// timezone byte that must be IST and one numeric value byte.
func parseScalarRecords(spec scalarSpec, raw []byte, window Window) ([]ScalarRow, TypeResult, error) {
	offsets, trailing, err := fixedRecords(spec.code, raw, spec.stride)
	if err != nil {
		return nil, TypeResult{}, err
	}
	byTime := make(map[int64]ScalarRow, len(offsets))
	dropped := 0
	for _, off := range offsets {
		if raw[off+spec.timezoneAt] != istTimezoneQuarter {
			return nil, TypeResult{}, timezoneError(spec.code, off+spec.timezoneAt, raw[off+spec.timezoneAt])
		}
		epoch := int64(binary.LittleEndian.Uint32(raw[off:]))
		value := raw[off+spec.valueAt]
		at := EpochUTC(epoch)
		if !window.contains(at) || !spec.accept(int(value)) {
			dropped++
			continue
		}
		byTime[at.Unix()] = ScalarRow{SampledAt: at, Value: float64(value)}
	}
	out := make([]ScalarRow, 0, len(byTime))
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
