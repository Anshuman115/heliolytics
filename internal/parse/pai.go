package parse

import (
	"encoding/binary"
	"math"
)

const (
	paiRecordSize = 102 // 0x05 marker → next marker, on Helio firmware
	paiMarkerByte = 0x05
	paiScoreOff   = 59 // float32 LE PAI total, offset from the 0x05 marker
	paiScoreMax   = 1000.0
)

type PaiScore struct {
	DayKey string
	Score  int
}

// ParsePai scans for 0x05-marked records. Each record holds an absolute epoch
// (u32 LE at marker+1) and the PAI total as a float32 LE at marker+59. The
// score is NOT an integer byte — earlier firmware-agnostic byte scans landed on
// a float's exponent byte and returned bogus values (e.g. 61 instead of 81).
func ParsePai(raw []byte) []PaiScore {
	type slot struct {
		sec   int64
		score int
	}
	byDay := map[string]slot{}
	for i := 0; i+paiScoreOff+4 <= len(raw); i++ {
		if raw[i] != paiMarkerByte {
			continue
		}
		sec := int64(binary.LittleEndian.Uint32(raw[i+1:]))
		if !IsPlausibleUnixSec(sec) {
			continue
		}
		score, ok := paiTotal(raw[i:])
		if !ok {
			continue
		}
		dk := IstDayKey(sec)
		if prev, ok := byDay[dk]; !ok || sec >= prev.sec {
			byDay[dk] = slot{sec: sec, score: score}
		}
	}
	out := make([]PaiScore, 0, len(byDay))
	for dk, s := range byDay {
		out = append(out, PaiScore{DayKey: dk, Score: s.score})
	}
	return out
}

// paiTotal reads the float32 PAI total at the fixed in-record offset.
func paiTotal(rec []byte) (int, bool) {
	if len(rec) < paiScoreOff+4 {
		return 0, false
	}
	v := math.Float32frombits(binary.LittleEndian.Uint32(rec[paiScoreOff:]))
	f := float64(v)
	if math.IsNaN(f) || math.IsInf(f, 0) || f <= 0 || f > paiScoreMax {
		return 0, false
	}
	return int(math.Round(f)), true
}
