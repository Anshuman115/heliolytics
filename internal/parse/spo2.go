package parse

import "encoding/binary"

const spo2RecordSize = 65
const spo2SleepRecordSize = 30
const spo2HeaderByte = 0x02

type Spo2Sample struct {
	DayKey    string
	SampledAt int64
	Percent   int
}

func decodeSpo2Pct(raw byte) int {
	v := int(raw)
	if v >= 128 {
		v -= 128
	}
	return v
}

func ParseSpo2(raw []byte) []Spo2Sample {
	payload := spo2Payload(raw)
	if payload == nil {
		return nil
	}
	var out []Spo2Sample
	for i := 0; i+spo2RecordSize <= len(payload); i += spo2RecordSize {
		sec := int64(binary.LittleEndian.Uint32(payload[i:]))
		pct := decodeSpo2Pct(payload[i+4])
		if pct < 70 || pct > 100 || !IsPlausibleUnixSec(sec) {
			continue
		}
		out = append(out, Spo2Sample{DayKey: IstDayKey(sec), SampledAt: sec, Percent: pct})
	}
	return out
}

func spo2Payload(raw []byte) []byte {
	if len(raw) >= spo2RecordSize && len(raw)%spo2RecordSize == 0 {
		return raw
	}
	if len(raw) > 1 && (len(raw)-1)%spo2RecordSize == 0 {
		return raw[1:]
	}
	return nil
}

// ParseSpo2Sleep — 1 header byte (0x02) + 30-byte epoch records.
func ParseSpo2Sleep(raw []byte) []Spo2Sample {
	if len(raw) == 0 || raw[0] != spo2HeaderByte {
		return nil
	}
	var out []Spo2Sample
	for i := 1; i+spo2SleepRecordSize <= len(raw); i += spo2SleepRecordSize {
		sec := int64(binary.LittleEndian.Uint32(raw[i:]))
		pct := int(raw[i+4])
		if pct <= 0 || pct > 100 || !IsPlausibleUnixSec(sec) {
			continue
		}
		out = append(out, Spo2Sample{DayKey: IstDayKey(sec), SampledAt: sec, Percent: pct})
	}
	return out
}
