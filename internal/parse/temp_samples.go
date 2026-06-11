package parse

import (
	"encoding/binary"
	"time"
)

type TempSamplePoint struct {
	Ts      time.Time
	DayKey  string
	Celsius float64
}

func ParseTempSeries(raw []byte, entry *CatalogEntry) []TempSamplePoint {
	if entry == nil || len(raw) == 0 {
		return nil
	}
	var out []TempSamplePoint
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
			out = append(out, parseTempSeriesChunk(raw[start:end], rs)...)
		}
		return out
	}
	return parseTempSeriesChunk(raw, ParseRoundStartIst(entry.RoundStart))
}

func parseTempSeriesChunk(raw []byte, rs int64) []TempSamplePoint {
	var out []TempSamplePoint
	ts := rs
	for i := 0; i+8 <= len(raw); i += 8 {
		v := int16(binary.LittleEndian.Uint16(raw[i+2:]))
		if v != 0x7FFF && v != -1 {
			c := float64(v) / 100.0
			sec := ts
			out = append(out, TempSamplePoint{
				Ts: EpochUTC(sec), DayKey: IstDayKey(sec), Celsius: c,
			})
		}
		ts += 60
	}
	return out
}
