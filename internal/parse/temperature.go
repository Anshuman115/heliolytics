package parse

import (
	"encoding/binary"
	"math"
)

type TempSample struct {
	DayKey  string
	Celsius float64
}

func ParseTemperature(raw []byte, entry *CatalogEntry) []TempSample {
	if entry == nil || len(raw) == 0 {
		return nil
	}
	var out []TempSample
	if len(entry.RoundSegments) > 0 {
		for si, seg := range entry.RoundSegments {
			start := seg.ByteOffset
			end := len(raw)
			if si+1 < len(entry.RoundSegments) {
				end = entry.RoundSegments[si+1].ByteOffset
			}
			if start >= len(raw) {
				continue
			}
			if end > len(raw) {
				end = len(raw)
			}
			rs := ParseRoundStartIst(seg.RoundStart)
			out = append(out, parseTempChunk(raw[start:end], rs)...)
		}
		return out
	}
	return parseTempChunk(raw, ParseRoundStartIst(entry.RoundStart))
}

func parseTempChunk(raw []byte, rs int64) []TempSample {
	var out []TempSample
	ts := rs
	for i := 0; i+8 <= len(raw); i += 8 {
		v := int16(binary.LittleEndian.Uint16(raw[i+2:]))
		if v != 0x7FFF && v != -1 {
			out = append(out, TempSample{
				DayKey: IstDayKey(ts), Celsius: float64(v) / 100.0,
			})
		}
		ts += 60
	}
	return out
}

func avgTemp(samples []TempSample, day string) *float64 {
	var sum float64
	var n int
	for _, s := range samples {
		if s.DayKey != day {
			continue
		}
		sum += s.Celsius
		n++
	}
	if n == 0 {
		return nil
	}
	v := math.Round(sum/float64(n)*10) / 10
	return &v
}
