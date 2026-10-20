package parse

import (
	"encoding/binary"
	"sort"
	"time"
)

const (
	sleepNapTableOff = 0x18
	sleepNapTableEnd = 0x54
	sleepNapStageOff = 0x155
)

// knownSleepStage reports the four stage types the protocol defines.
func knownSleepStage(stageType int) bool {
	return stageType == 4 || stageType == 5 || stageType == 7 || stageType == 8
}

func stageMinutes(stages []SleepStage, stageType int) int {
	total := 0
	for _, stage := range stages {
		if stage.Type == stageType {
			total += int(stage.End.Sub(stage.Start) / time.Minute)
		}
	}
	return total
}

type sleepNapWindow struct {
	start time.Time
	end   time.Time
}

// napSleepRows builds one row per nap table entry. The table end minute is
// inclusive, so the half-open window ends one minute later.
func napSleepRows(record []byte, base int64, window Window) ([]SleepRow, int) {
	windows := napWindows(record, base)
	if len(windows) == 0 {
		return nil, 0
	}
	pool, poolOmitted := napStages(record, base)
	claimed := make([]bool, len(pool))
	var out []SleepRow
	dropped := 0
	for _, nap := range windows {
		if !nap.end.After(nap.start) || !window.containsInterval(nap.start, nap.end) {
			dropped++
			continue
		}
		var stages []SleepStage
		omitted := poolOmitted
		for i, stage := range pool {
			if claimed[i] || stage.Start.Before(nap.start) || !stage.Start.Before(nap.end) {
				continue
			}
			claimed[i] = true
			if stage.End.After(nap.end) {
				omitted = true
				continue
			}
			stages = append(stages, stage)
		}
		if len(stages) == 0 {
			dropped++
			continue
		}
		sort.Slice(stages, func(i, j int) bool { return stages[i].Start.Before(stages[j].Start) })
		ordered := stages[:0]
		var lastEnd time.Time
		for _, stage := range stages {
			if !lastEnd.IsZero() && stage.Start.Before(lastEnd) {
				omitted = true
				continue
			}
			ordered = append(ordered, stage)
			lastEnd = stage.End
		}
		stages = ordered
		light := stageMinutes(stages, 4)
		deep := stageMinutes(stages, 5)
		rem := stageMinutes(stages, 8)
		wake := stageMinutes(stages, 7)
		out = append(out, SleepRow{
			StartedAt: nap.start, EndedAt: nap.end,
			DayKey: IstDayKey(nap.start.Unix()), IsNap: true,
			TotalMins: light + deep + rem,
			DeepMins:  deep, REMMins: rem, LightMins: light, WakeMins: wake,
			Stages: stages,
			StagesTruncated: omitted ||
				stages[0].Start.After(nap.start) || stages[len(stages)-1].End.Before(nap.end),
		})
	}
	return out, dropped
}

func napWindows(record []byte, base int64) []sleepNapWindow {
	var out []sleepNapWindow
	for offset := sleepNapTableOff; offset+6 <= sleepNapTableEnd; offset += 6 {
		start := int(binary.LittleEndian.Uint16(record[offset:]))
		end := int(binary.LittleEndian.Uint16(record[offset+2:]))
		minutes := int(binary.LittleEndian.Uint16(record[offset+4:]))
		if start == 0 && minutes == 0 {
			break
		}
		if minutes == 0 || end < start {
			continue
		}
		out = append(out, sleepNapWindow{
			start: EpochUTC(base + int64(start)*60),
			end:   EpochUTC(base + int64(end+1)*60),
		})
	}
	return out
}

func napStages(record []byte, base int64) ([]SleepStage, bool) {
	// The nap timeline ends where the device summary totals begin; reading
	// further turns those totals bytes into a phantom stage.
	limit := (sleepTotalsOff - sleepNapStageOff) / 5
	var out []SleepStage
	omitted := false
	for i := range limit {
		offset := sleepNapStageOff + 5*i
		start := int(binary.LittleEndian.Uint16(record[offset:]))
		end := int(binary.LittleEndian.Uint16(record[offset+2:]))
		stageType := int(record[offset+4])
		if start == 0 && end == 0 {
			continue
		}
		if !knownSleepStage(stageType) || end < start {
			omitted = true
			continue
		}
		out = append(out, SleepStage{
			Start: EpochUTC(base + int64(start)*60),
			End:   EpochUTC(base + int64(end+1)*60),
			Type:  stageType,
		})
	}
	return out, omitted
}
