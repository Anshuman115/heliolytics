package parse

import (
	"bytes"
	"encoding/binary"
	"sort"
)

const (
	spo2Stride        = 65
	spo2SleepStride   = 30
	spo2ModeAutomatic = 0x80
	spo2MinPct        = 70
	spo2MaxPct        = 100
)

// spO2SleepPlaceholder is the payload every 0x26 record carries on this
// firmware: 100, duration 64, high 100s, low 99s and a fixed signal tail. It is
// a firmware constant, not an optical measurement.
var spO2SleepPlaceholder = []byte{
	0x64, 0x40,
	0x64, 0x64, 0x64, 0x64, 0x64, 0x64,
	0x63, 0x63, 0x63, 0x63, 0x63, 0x63,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40, 0x00,
	0x00, 0x00, 0x00, 0x00,
}

type spo2Key struct {
	at     int64
	source string
}

// parseSpO2 decodes 0x25 optical samples. The low seven bits are the value and
// bit seven records an automatic measurement.
func parseSpO2(raw []byte, catalog Catalog, window Window) ([]SpO2Row, TypeResult, error) {
	records, trailing, err := headeredRecords(typeSpO2, raw, FindEntry(catalog, typeSpO2), spo2Stride)
	if err != nil {
		return nil, TypeResult{}, err
	}
	byKey := make(map[spo2Key]SpO2Row, len(records))
	dropped := 0
	for _, record := range records {
		epoch := int64(binary.LittleEndian.Uint32(raw[record.off:]))
		at := EpochUTC(epoch)
		rawValue := raw[record.off+4]
		pct := int(rawValue & 0x7f)
		mode := "manual"
		if rawValue&spo2ModeAutomatic != 0 {
			mode = "auto"
		}
		beyondRound := record.nextRound != nil && at.After(*record.nextRound)
		if !window.contains(at) || beyondRound || pct < spo2MinPct || pct > spo2MaxPct {
			dropped++
			continue
		}
		byKey[spo2Key{at.Unix(), typeSpO2}] = SpO2Row{
			SampledAt: at, Source: typeSpO2, Pct: pct, Mode: mode,
		}
	}
	out := make([]SpO2Row, 0, len(byKey))
	for _, row := range byKey {
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

// parseSpO2Sleep classifies 0x26 records. Every record stays raw: placeholders
// are firmware constants and unvalidated payloads have no confirmed meaning.
func parseSpO2Sleep(raw []byte, catalog Catalog) (TypeResult, error) {
	records, trailing, err := headeredRecords(typeSpO2Sleep, raw, FindEntry(catalog, typeSpO2Sleep), spo2SleepStride)
	if err != nil {
		return TypeResult{}, err
	}
	placeholders, unvalidated := 0, 0
	firstPlaceholder, firstUnvalidated := -1, -1
	for _, record := range records {
		payload := raw[record.off+4 : record.off+4+len(spO2SleepPlaceholder)]
		if bytes.Equal(payload, spO2SleepPlaceholder) {
			placeholders++
			if firstPlaceholder < 0 {
				firstPlaceholder = record.off
			}
			continue
		}
		unvalidated++
		if firstUnvalidated < 0 {
			firstUnvalidated = record.off
		}
	}
	result := TypeResult{
		Status:        "raw_only",
		Dropped:       placeholders + unvalidated,
		TrailingBytes: trailing,
	}
	if placeholders > 0 {
		result.Warnings = append(result.Warnings, Warning{
			Code: "spo2_sleep_placeholder", ByteOffset: firstPlaceholder, Count: placeholders,
		})
	}
	if unvalidated > 0 {
		result.Warnings = append(result.Warnings, Warning{
			Code: "spo2_sleep_unvalidated", ByteOffset: firstUnvalidated, Count: unvalidated,
		})
	}
	return result, nil
}
