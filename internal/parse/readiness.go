package parse

import "encoding/binary"

const (
	readinessRecordStride = 569
	readinessScoreOffset  = 4
	readinessLegacySize   = 6
	readinessLegacyScore  = 5
)

type ReadinessScore struct {
	DayKey    string
	Readiness int
}

func ParseReadiness(raw []byte) []ReadinessScore {
	if len(raw) >= readinessRecordStride && len(raw)%readinessRecordStride == 0 {
		if out := scanReadinessRecords(raw, readinessRecordStride, readinessScoreOffset); len(out) > 0 {
			return out
		}
	}
	for off := 0; off < 4; off++ {
		if (len(raw)-off)%readinessLegacySize != 0 {
			continue
		}
		if out := scanReadinessRecords(raw[off:], readinessLegacySize, readinessLegacyScore); len(out) > 0 {
			return out
		}
	}
	return scanReadinessRecords(raw, readinessLegacySize, readinessLegacyScore)
}

func scanReadinessRecords(raw []byte, stride, scoreOff int) []ReadinessScore {
	var out []ReadinessScore
	for i := 0; i+stride <= len(raw); i += stride {
		sec := int64(binary.LittleEndian.Uint32(raw[i:]))
		if !IsPlausibleUnixSec(sec) {
			continue
		}
		if scoreOff >= stride {
			continue
		}
		score := int(raw[i+scoreOff])
		if score <= 0 || score > 100 {
			continue
		}
		out = append(out, ReadinessScore{DayKey: IstDayKey(sec), Readiness: score})
	}
	return out
}
