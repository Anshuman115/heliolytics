package parse

import (
	"encoding/binary"
	"sort"
	"time"
)

const (
	sleepRecordSize     = 594
	sleepFlagsOff       = 8
	sleepHeaderStartOff = 0x0a
	sleepHeaderEndOff   = 0x0c
	sleepScoreOff       = 0x16
	sleepMainCountOff   = 0x54
	sleepMainStageOff   = 0x56
	sleepMainStageMax   = 51
	sleepTotalsOff      = 0x24a
)

// parseSleep decodes 594-byte sleep records. A record whose flags report no
// main sleep still contributes its independent naps.
func parseSleep(raw []byte, window Window) ([]SleepRow, TypeResult, error) {
	offsets, trailing, err := fixedRecords(typeSleep, raw, sleepRecordSize)
	if err != nil {
		return nil, TypeResult{}, err
	}
	var out []SleepRow
	dropped := 0
	for _, off := range offsets {
		rows, recordDropped := parseSleepRecord(raw[off:off+sleepRecordSize], window)
		out = append(out, rows...)
		dropped += recordDropped
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].StartedAt.Equal(out[j].StartedAt) {
			return out[i].StartedAt.Before(out[j].StartedAt)
		}
		if out[i].IsNap != out[j].IsNap {
			return !out[i].IsNap
		}
		return out[i].EndedAt.Before(out[j].EndedAt)
	})
	result := TypeResult{
		Status:        statusFor(trailing),
		Rows:          len(out),
		Dropped:       dropped,
		TrailingBytes: trailing,
	}
	if trailing > 0 {
		result.Warnings = append(result.Warnings, Warning{
			Code: "trailing_fragment", ByteOffset: len(raw) - trailing, Count: trailing,
		})
	}
	for i := range out {
		result.ProcessedThrough = maxTime(result.ProcessedThrough, out[i].EndedAt)
	}
	return out, result, nil
}

func parseSleepRecord(record []byte, window Window) ([]SleepRow, int) {
	midnight := int64(binary.LittleEndian.Uint32(record[4:]))
	if midnight == 0 || !anchorPlausible(midnight) {
		return nil, 1
	}
	base := midnight - 86400
	dayKey := IstDayKey(midnight)
	var out []SleepRow
	dropped := 0
	if record[sleepFlagsOff] == 1 && record[sleepFlagsOff+1] == 1 {
		if row, ok := mainSleepRow(record, base, dayKey, window); ok {
			out = append(out, row)
		} else {
			dropped++
		}
	}
	naps, napDropped := napSleepRows(record, base, window)
	out = append(out, naps...)
	return out, dropped + napDropped
}

// mainSleepRow uses the sleep-day midnight field for identity and the header
// minutes for the half-open window. The device summary stays authoritative.
func mainSleepRow(record []byte, base int64, dayKey string, window Window) (SleepRow, bool) {
	startMinute := int(binary.LittleEndian.Uint16(record[sleepHeaderStartOff:]))
	endMinute := int(binary.LittleEndian.Uint16(record[sleepHeaderEndOff:]))
	if endMinute <= startMinute {
		return SleepRow{}, false
	}
	count := int(record[sleepMainCountOff])
	remin := int(binary.LittleEndian.Uint16(record[sleepTotalsOff:]))
	light := int(binary.LittleEndian.Uint16(record[sleepTotalsOff+2:]))
	deep := int(binary.LittleEndian.Uint16(record[sleepTotalsOff+4:]))
	wake := int(binary.LittleEndian.Uint16(record[sleepTotalsOff+6:]))
	if count == 0 && remin == 0 && light == 0 && deep == 0 && wake == 0 {
		return SleepRow{}, false
	}
	start := EpochUTC(base + int64(startMinute)*60)
	end := EpochUTC(base + int64(endMinute)*60)
	if !window.containsInterval(start, end) {
		return SleepRow{}, false
	}
	stages, omitted := mainStages(record, base, count, start, end)
	return SleepRow{
		StartedAt: start, EndedAt: end, DayKey: dayKey,
		Score:     sleepScore(record),
		TotalMins: remin + light + deep,
		DeepMins:  deep, REMMins: remin, LightMins: light, WakeMins: wake,
		Stages:          stages,
		StagesTruncated: sleepTruncated(count, stages, start, end, remin, light, deep, wake, omitted),
	}, true
}

// mainStages decodes the bounded 51-slot main timeline as half-open stages. A
// stage outside the header window is omitted and flagged.
func mainStages(record []byte, base int64, count int, sessionStart, sessionEnd time.Time) ([]SleepStage, bool) {
	limit := count
	if limit > sleepMainStageMax {
		limit = sleepMainStageMax
	}
	omitted := count > sleepMainStageMax
	var stages []SleepStage
	var lastEnd time.Time
	for i := range limit {
		off := sleepMainStageOff + 5*i
		start := int(binary.LittleEndian.Uint16(record[off:]))
		end := int(binary.LittleEndian.Uint16(record[off+2:]))
		stageType := int(record[off+4])
		if start == 0 && end == 0 {
			break
		}
		if !knownSleepStage(stageType) || end < start {
			omitted = true
			continue
		}
		stageStart := EpochUTC(base + int64(start)*60)
		stageEnd := EpochUTC(base + int64(end+1)*60)
		if stageStart.Before(sessionStart) || stageEnd.After(sessionEnd) ||
			(!lastEnd.IsZero() && stageStart.Before(lastEnd)) {
			omitted = true
			continue
		}
		stages = append(stages, SleepStage{Start: stageStart, End: stageEnd, Type: stageType})
		lastEnd = stageEnd
	}
	if len(stages) != count {
		omitted = true
	}
	return stages, omitted
}

// sleepTruncated flags a timeline that may have lost stages. Totals and the
// header window stay valid either way.
func sleepTruncated(count int, stages []SleepStage, start, end time.Time, remin, light, deep, wake int, omitted bool) bool {
	if count >= sleepMainStageMax-1 || omitted {
		return true
	}
	if len(stages) == 0 {
		return false
	}
	if !stages[0].Start.Equal(start) || !stages[len(stages)-1].End.Equal(end) {
		return true
	}
	return stageMinutes(stages, 4) != light || stageMinutes(stages, 5) != deep ||
		stageMinutes(stages, 8) != remin || stageMinutes(stages, 7) != wake
}

func sleepScore(record []byte) *int {
	value := int(record[sleepScoreOff])
	if value == 0 || value > 100 {
		return nil
	}
	return &value
}
