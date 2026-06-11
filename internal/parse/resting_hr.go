package parse

import "encoding/binary"

type HrSample struct {
	DayKey string
	Bpm    int
}

func ParseHr6(raw []byte) []HrSample {
	if len(raw)%6 != 0 {
		return nil
	}
	var out []HrSample
	for i := 0; i < len(raw); i += 6 {
		sec := int64(binary.LittleEndian.Uint32(raw[i:]))
		if !IsPlausibleUnixSec(sec) {
			continue
		}
		bpm := int(raw[i+5])
		if bpm < 30 || bpm > 220 {
			continue
		}
		out = append(out, HrSample{DayKey: IstDayKey(sec), Bpm: bpm})
	}
	return out
}
