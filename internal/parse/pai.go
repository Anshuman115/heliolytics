package parse

import (
	"encoding/binary"
	"math"
	"sort"
)

const (
	paiStride     = 102
	paiMarker     = 0x05
	paiEpochAt    = 1
	paiTimezoneAt = 5
	paiScoreAt    = 59
	paiMaxScore   = 1000.0
)

// parsePAI decodes 102-byte records that start with the 0x05 marker. The score
// is the float32 total at marker+59, never a byte read from the exponent.
func parsePAI(raw []byte, window Window) ([]PAIRow, TypeResult, error) {
	offsets, trailing, err := fixedRecords(typePAI, raw, paiStride)
	if err != nil {
		return nil, TypeResult{}, err
	}
	byDay := make(map[string]PAIRow, len(offsets))
	dropped := 0
	for _, off := range offsets {
		if raw[off] != paiMarker {
			return nil, TypeResult{}, newError(ErrCodeInvalidFraming, typePAI, off,
				"record does not start with the 0x05 marker")
		}
		if raw[off+paiTimezoneAt] != istTimezoneQuarter {
			return nil, TypeResult{}, timezoneError(typePAI, off+paiTimezoneAt, raw[off+paiTimezoneAt])
		}
		epoch := int64(binary.LittleEndian.Uint32(raw[off+paiEpochAt:]))
		score, ok := paiScore(raw[off : off+paiStride])
		at := EpochUTC(epoch)
		if !ok || !window.contains(at) {
			dropped++
			continue
		}
		row := PAIRow{SampledAt: at, DayKey: IstDayKey(epoch), Score: score}
		previous, exists := byDay[row.DayKey]
		if !exists || !row.SampledAt.Before(previous.SampledAt) {
			byDay[row.DayKey] = row
		}
	}
	out := make([]PAIRow, 0, len(byDay))
	for _, row := range byDay {
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].DayKey != out[j].DayKey {
			return out[i].DayKey < out[j].DayKey
		}
		return out[i].SampledAt.Before(out[j].SampledAt)
	})
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

func paiScore(record []byte) (int, bool) {
	value := math.Float32frombits(binary.LittleEndian.Uint32(record[paiScoreAt:]))
	total := float64(value)
	if math.IsNaN(total) || math.IsInf(total, 0) || total < 0 || total > paiMaxScore {
		return 0, false
	}
	return int(math.Round(total)), true
}
