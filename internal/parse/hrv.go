package parse

import "encoding/binary"

type HrvSample struct {
	DayKey string
	Rmssd  int
}

func ParseHrv(raw []byte) []HrvSample {
	if len(raw)%6 != 0 {
		return nil
	}
	var out []HrvSample
	for i := 0; i < len(raw); i += 6 {
		sec := int64(binary.LittleEndian.Uint32(raw[i:]))
		if !IsPlausibleUnixSec(sec) {
			continue
		}
		rmssd := int(raw[i+5])
		if rmssd <= 0 {
			continue
		}
		out = append(out, HrvSample{DayKey: IstDayKey(sec), Rmssd: rmssd})
	}
	return out
}
