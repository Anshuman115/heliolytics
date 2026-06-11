package parse

import (
	"encoding/binary"
	"time"
)

const sleepRecordSize = 594

type SleepStage struct {
	Start time.Time
	End   time.Time
	Type  int
}

type SleepRecord struct {
	DayKey     string
	StartedAt  time.Time
	Score      int
	TotalMin   int
	DeepMin    int
	RemMin     int
	LightMin   int
	WakeMin    int
	IsNap      bool
	Stages     []SleepStage
	WindowStart time.Time
	WindowEnd   time.Time
}

func (s SleepRecord) sleepWindow() (time.Time, time.Time) {
	if !s.WindowStart.IsZero() && !s.WindowEnd.IsZero() {
		return s.WindowStart, s.WindowEnd
	}
	if len(s.Stages) == 0 {
		return s.StartedAt, s.StartedAt
	}
	return s.Stages[0].Start, s.Stages[len(s.Stages)-1].End
}

func ParseSleep(raw []byte) []SleepRecord {
	var out []SleepRecord
	for off := 0; off+sleepRecordSize <= len(raw); {
		rec := raw[off : off+sleepRecordSize]
		tsSession := int64(binary.LittleEndian.Uint32(rec[0:]))
		tsMidnight := int64(binary.LittleEndian.Uint32(rec[4:]))
		if !IsPlausibleUnixSec(tsSession) || !IsPlausibleUnixSec(tsMidnight) {
			off += sleepRecordSize
			continue
		}
		if rec[0x08] != 1 || rec[0x09] != 1 {
			off += sleepRecordSize
			continue
		}
		base := tsMidnight - 24*3600
		score := int(rec[0x16])
		numStages := int(rec[0x54])
		night := parseNightStages(rec, 0x56, numStages, base)
		rem := int(binary.LittleEndian.Uint16(rec[0x24a:]))
		light := int(binary.LittleEndian.Uint16(rec[0x24c:]))
		deep := int(binary.LittleEndian.Uint16(rec[0x24e:]))
		wake := int(binary.LittleEndian.Uint16(rec[0x250:]))
		main := SleepRecord{
			DayKey: IstDayKey(tsSession), StartedAt: EpochUTC(tsSession),
			Score: score, Stages: night, RemMin: rem, LightMin: light,
			DeepMin: deep, WakeMin: wake, TotalMin: rem + light + deep,
		}
		if len(night) > 0 {
			main.WindowStart = night[0].Start
			main.WindowEnd = night[len(night)-1].End
		}
		out = append(out, main)
		out = append(out, parseNaps(rec, base)...)
		off += sleepRecordSize
	}
	return out
}
