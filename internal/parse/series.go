package parse

import (
	"encoding/binary"
	"time"
)

type HealthSample struct {
	Metric    string
	DayKey    string
	SampledAt time.Time
	Value     float64
}

func ParseStressSeries(raw []byte, entry *CatalogEntry) []HealthSample {
	if entry == nil || len(raw) == 0 {
		return nil
	}
	var out []HealthSample
	appendChunk := func(chunk []byte, rs int64) {
		ts := rs
		for _, b := range chunk {
			if b != 0xFF {
				out = append(out, HealthSample{
					Metric: "stress", DayKey: IstDayKey(ts),
					SampledAt: EpochUTC(ts), Value: float64(b),
				})
			}
			ts += 60
		}
	}
	if len(entry.RoundSegments) > 0 {
		for si, seg := range entry.RoundSegments {
			start, end := seg.ByteOffset, len(raw)
			if si+1 < len(entry.RoundSegments) {
				end = entry.RoundSegments[si+1].ByteOffset
			}
			if start >= len(raw) {
				continue
			}
			if end > len(raw) {
				end = len(raw)
			}
			appendChunk(raw[start:end], ParseRoundStartIst(seg.RoundStart))
		}
		return out
	}
	appendChunk(raw, ParseRoundStartIst(entry.RoundStart))
	return out
}

func ParseHrvSeries(raw []byte) []HealthSample {
	if len(raw)%6 != 0 {
		return nil
	}
	var out []HealthSample
	for i := 0; i < len(raw); i += 6 {
		sec := int64(binary.LittleEndian.Uint32(raw[i:]))
		if !IsPlausibleUnixSec(sec) {
			continue
		}
		rmssd := int(raw[i+5])
		if rmssd <= 0 {
			continue
		}
		out = append(out, HealthSample{
			Metric: "hrv", DayKey: IstDayKey(sec),
			SampledAt: EpochUTC(sec), Value: float64(rmssd),
		})
	}
	return out
}

func ParseSpo2Series(raw []byte) []HealthSample {
	return spo2ToSeries(ParseSpo2(raw), "spo2")
}

func ParseSpo2SleepSeries(raw []byte) []HealthSample {
	return spo2ToSeries(ParseSpo2Sleep(raw), "spo2_sleep")
}

func spo2ToSeries(rows []Spo2Sample, metric string) []HealthSample {
	out := make([]HealthSample, 0, len(rows))
	for _, s := range rows {
		out = append(out, HealthSample{
			Metric: metric, DayKey: s.DayKey,
			SampledAt: EpochUTC(s.SampledAt), Value: float64(s.Percent),
		})
	}
	return out
}

func ParseRhrSeries(raw []byte) []HealthSample {
	return parseHr6Series(raw, "rhr", 30, 220)
}

func ParseMaxHrSeries(raw []byte) []HealthSample {
	return parseHr6Series(raw, "max_hr", 30, 220)
}

func parseHr6Series(raw []byte, metric string, lo, hi int) []HealthSample {
	if len(raw)%6 != 0 {
		return nil
	}
	var out []HealthSample
	for i := 0; i < len(raw); i += 6 {
		sec := int64(binary.LittleEndian.Uint32(raw[i:]))
		if !IsPlausibleUnixSec(sec) {
			continue
		}
		bpm := int(raw[i+5])
		if bpm == 0xFF || bpm < lo || bpm > hi {
			continue
		}
		out = append(out, HealthSample{
			Metric: metric, DayKey: IstDayKey(sec),
			SampledAt: EpochUTC(sec), Value: float64(bpm),
		})
	}
	return out
}

func ParseRespRateSeries(raw []byte) []HealthSample {
	if len(raw)%8 != 0 {
		return nil
	}
	var out []HealthSample
	for i := 0; i < len(raw); i += 8 {
		sec := int64(binary.LittleEndian.Uint32(raw[i:]))
		if !IsPlausibleUnixSec(sec) {
			continue
		}
		rate := int(raw[i+5])
		if rate == 0xFF || rate <= 0 {
			continue
		}
		out = append(out, HealthSample{
			Metric: "resp_rate", DayKey: IstDayKey(sec),
			SampledAt: EpochUTC(sec), Value: float64(rate),
		})
	}
	return out
}
