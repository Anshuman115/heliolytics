package parse

import (
	"encoding/binary"
	"time"
)

const napBlobStride = 9
const napMinDurSec = 45 * 60
const napMinHourIst = 11

type NapRecord struct {
	DayKey      string
	StartedSec  int64
	DurationMin int
}

func ParseNaps(raw []byte) []NapRecord {
	var out []NapRecord
	for i := 0; i+napBlobStride <= len(raw); i += napBlobStride {
		sec := int64(binary.LittleEndian.Uint32(raw[i:]))
		if !IsPlausibleUnixSec(sec) {
			continue
		}
		dur := int(binary.LittleEndian.Uint32(raw[i+5:]))
		if dur < napMinDurSec {
			continue
		}
		t := EpochUTC(sec).Add(5*time.Hour + 30*time.Minute)
		if t.Hour() < napMinHourIst {
			continue
		}
		out = append(out, NapRecord{
			DayKey: IstDayKey(sec), StartedSec: sec, DurationMin: dur / 60,
		})
	}
	return out
}

func CountNapsByDay(naps []NapRecord) map[string]int {
	out := map[string]int{}
	for _, n := range naps {
		out[n.DayKey]++
	}
	return out
}
