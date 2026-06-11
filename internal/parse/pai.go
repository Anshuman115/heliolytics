package parse

import "encoding/binary"

const paiRecordSize = 61
const paiMarkerByte = 0x05

type PaiScore struct {
	DayKey string
	Score  int
}

// ParsePai scans for 0x05-marked records (~102 B apart on Helio firmware).
func ParsePai(raw []byte) []PaiScore {
	type slot struct {
		sec   int64
		score int
	}
	byDay := map[string]slot{}
	for i := 0; i+21 <= len(raw); i++ {
		if raw[i] != paiMarkerByte {
			continue
		}
		sec := int64(binary.LittleEndian.Uint32(raw[i+1:]))
		if !IsPlausibleUnixSec(sec) {
			continue
		}
		end := i + paiRecordSize
		if end > len(raw) {
			end = len(raw)
		}
		score := findPaiScore(raw[i:end])
		if score <= 0 {
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

func findPaiScore(rec []byte) int {
	if len(rec) < 21 {
		return 0
	}
	if rec[20] > 0 && rec[20] <= 100 {
		return int(rec[20])
	}
	for j := 10; j < 40 && j < len(rec); j++ {
		if rec[j] > 0 && rec[j] <= 100 {
			return int(rec[j])
		}
	}
	return 0
}
