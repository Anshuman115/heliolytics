package parse

import (
	"encoding/binary"
	"time"
)

const hrSessionHeaderSize = 6

type HrSamplePoint struct {
	Ts     time.Time
	DayKey string
	Bpm    int
}

// ParseContinuousHr parses 0x46 PPG session blobs: 6-byte header + 1 BPM/sec.
func ParseContinuousHr(raw []byte) []HrSamplePoint {
	var out []HrSamplePoint
	i := 0
	for i+hrSessionHeaderSize <= len(raw) {
		startSec := int64(binary.LittleEndian.Uint32(raw[i:]))
		if !IsPlausibleUnixSec(startSec) {
			i++
			continue
		}
		i += hrSessionHeaderSize
		sec := startSec
		for i < len(raw) {
			if isHrSessionHeader(raw, i) {
				break
			}
			b := raw[i]
			i++
			if b == 0 || b == 0xFF || b < 30 || b > 220 {
				sec++
				continue
			}
			out = append(out, HrSamplePoint{
				Ts: EpochUTC(sec), DayKey: IstDayKey(sec), Bpm: int(b),
			})
			sec++
		}
	}
	return out
}

func isHrSessionHeader(raw []byte, off int) bool {
	if off+hrSessionHeaderSize > len(raw) {
		return false
	}
	sec := int64(binary.LittleEndian.Uint32(raw[off:]))
	return IsPlausibleUnixSec(sec)
}
