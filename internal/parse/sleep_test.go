package parse

import (
	"encoding/binary"
	"testing"
	"time"
)

const (
	sleepFixtureWrite    = int64(1782144544) // 2026-06-22T16:09:04Z
	sleepFixtureMidnight = int64(1782066600) // 2026-06-22T00:00:00 IST
	sleepFixtureBase     = sleepFixtureMidnight - 86400
)

func sleepWindow() Window {
	return Window{
		StartedAt: time.Date(2026, 6, 22, 17, 7, 40, 0, time.UTC),
		EndedAt:   time.Date(2026, 6, 22, 17, 8, 11, 0, time.UTC),
	}
}

func sleepRecord(write, midnight int64, startMinute, endMinute int) []byte {
	record := make([]byte, sleepRecordSize)
	binary.LittleEndian.PutUint32(record[0:], uint32(write))
	binary.LittleEndian.PutUint32(record[4:], uint32(midnight))
	record[sleepFlagsOff], record[sleepFlagsOff+1] = 1, 1
	binary.LittleEndian.PutUint16(record[sleepHeaderStartOff:], uint16(startMinute))
	binary.LittleEndian.PutUint16(record[sleepHeaderEndOff:], uint16(endMinute))
	return record
}

func putSleepStage(record []byte, index, start, end, stageType int) {
	offset := sleepMainStageOff + 5*index
	binary.LittleEndian.PutUint16(record[offset:], uint16(start))
	binary.LittleEndian.PutUint16(record[offset+2:], uint16(end))
	record[offset+4] = byte(stageType)
}

func putSleepTotals(record []byte, remin, light, deep, wake int) {
	binary.LittleEndian.PutUint16(record[sleepTotalsOff:], uint16(remin))
	binary.LittleEndian.PutUint16(record[sleepTotalsOff+2:], uint16(light))
	binary.LittleEndian.PutUint16(record[sleepTotalsOff+4:], uint16(deep))
	binary.LittleEndian.PutUint16(record[sleepTotalsOff+6:], uint16(wake))
}

func putNap(record []byte, index, start, end, minutes int) {
	offset := sleepNapTableOff + 6*index
	binary.LittleEndian.PutUint16(record[offset:], uint16(start))
	binary.LittleEndian.PutUint16(record[offset+2:], uint16(end))
	binary.LittleEndian.PutUint16(record[offset+4:], uint16(minutes))
}

func putNapStage(record []byte, index, start, end, stageType int) {
	offset := sleepNapStageOff + 5*index
	binary.LittleEndian.PutUint16(record[offset:], uint16(start))
	binary.LittleEndian.PutUint16(record[offset+2:], uint16(end))
	record[offset+4] = byte(stageType)
}

func parseOneSleep(t *testing.T, record []byte) ([]SleepRow, TypeResult) {
	t.Helper()
	rows, result, err := parseSleep(record, sleepWindow())
	if err != nil {
		t.Fatalf("parse sleep: %v", err)
	}
	return rows, result
}

func TestSleepDayKeyComesFromMidnightField(t *testing.T) {
	record := sleepRecord(sleepFixtureWrite, sleepFixtureMidnight, 1520, 1530)
	record[sleepMainCountOff] = 1
	putSleepStage(record, 0, 1520, 1529, 4)
	putSleepTotals(record, 0, 10, 0, 0)

	rows, _ := parseOneSleep(t, record)
	if len(rows) != 1 {
		t.Fatalf("sleep rows = %d, want 1", len(rows))
	}
	want := time.Date(2026, 6, 21, 19, 50, 0, 0, time.UTC)
	if rows[0].DayKey != "2026-06-22" || !rows[0].StartedAt.Equal(want) {
		t.Fatalf("row = day=%s start=%s, want day=2026-06-22 start=%s",
			rows[0].DayKey, formatFixtureTime(rows[0].StartedAt), formatFixtureTime(want))
	}

	changed := sleepRecord(sleepFixtureWrite+3*86400, sleepFixtureMidnight, 1520, 1530)
	changed[sleepMainCountOff] = 1
	putSleepStage(changed, 0, 1520, 1529, 4)
	putSleepTotals(changed, 0, 10, 0, 0)
	again, _ := parseOneSleep(t, changed)
	if len(again) != 1 || again[0].DayKey != rows[0].DayKey ||
		!again[0].StartedAt.Equal(rows[0].StartedAt) || !again[0].EndedAt.Equal(rows[0].EndedAt) {
		t.Fatalf("changing the write epoch changed the row identity: before=%+v after=%+v", rows[0], again)
	}
}

func TestSleepStagesAreHalfOpen(t *testing.T) {
	record := sleepRecord(sleepFixtureWrite, sleepFixtureMidnight, 1672, 1673)
	record[sleepMainCountOff] = 1
	putSleepStage(record, 0, 1672, 1672, 7)
	putSleepTotals(record, 0, 0, 0, 1)

	rows, _ := parseOneSleep(t, record)
	if len(rows) != 1 || len(rows[0].Stages) != 1 {
		t.Fatalf("rows = %+v, want one row with one stage", rows)
	}
	stage := rows[0].Stages[0]
	if width := stage.End.Sub(stage.Start); width != time.Minute {
		t.Fatalf("single minute wake width = %v, want 1m", width)
	}
	wantStart := time.Date(2026, 6, 21, 22, 22, 0, 0, time.UTC)
	if !stage.Start.Equal(wantStart) || !stage.End.Equal(wantStart.Add(time.Minute)) {
		t.Fatalf("stage = %s..%s, want %s..%s",
			formatFixtureTime(stage.Start), formatFixtureTime(stage.End),
			formatFixtureTime(wantStart), formatFixtureTime(wantStart.Add(time.Minute)))
	}
}

func TestSleepMainWindowComesFromHeader(t *testing.T) {
	record := sleepRecord(sleepFixtureWrite, sleepFixtureMidnight, 1520, 1830)
	record[sleepMainCountOff] = 1
	putSleepStage(record, 0, 1520, 1536, 4)
	putSleepTotals(record, 0, 17, 0, 0)

	rows, _ := parseOneSleep(t, record)
	if len(rows) != 1 {
		t.Fatalf("sleep rows = %d, want 1", len(rows))
	}
	wantEnd := time.Date(2026, 6, 22, 1, 0, 0, 0, time.UTC)
	if !rows[0].EndedAt.Equal(wantEnd) {
		t.Fatalf("main end = %s, want the exclusive header end %s",
			formatFixtureTime(rows[0].EndedAt), formatFixtureTime(wantEnd))
	}
	if !rows[0].StagesTruncated {
		t.Fatal("a timeline shorter than the header must be flagged as truncated")
	}
}

func TestSleepPhantomMainRecordKeepsIndependentNap(t *testing.T) {
	record := sleepRecord(sleepFixtureWrite, sleepFixtureMidnight, 0, 0)
	putNap(record, 0, 2646, 2719, 74)
	putNapStage(record, 0, 2646, 2658, 4)
	putNapStage(record, 1, 2659, 2668, 5)
	putNapStage(record, 2, 2669, 2705, 4)
	putNapStage(record, 3, 2706, 2710, 8)
	putNapStage(record, 4, 2711, 2719, 4)

	rows, result := parseOneSleep(t, record)
	if len(rows) != 1 || !rows[0].IsNap {
		t.Fatalf("rows = %+v, want only the nap", rows)
	}
	if result.Dropped != 1 {
		t.Errorf("dropped = %d, want the phantom main record", result.Dropped)
	}
	if rows[0].TotalMins != 74 || rows[0].LightMins != 59 || rows[0].DeepMins != 10 || rows[0].REMMins != 5 {
		t.Errorf("nap totals = %+v, want 74/59/10/5", rows[0])
	}
}

func TestNapTimelineStopsBeforeDeviceTotals(t *testing.T) {
	record := sleepRecord(sleepFixtureWrite, sleepFixtureMidnight, 0, 0)
	record[sleepFlagsOff+1] = 0
	putNap(record, 0, 2646, 2719, 74)
	putNapStage(record, 0, 2646, 2658, 4)
	putNapStage(record, 1, 2659, 2668, 5)
	putNapStage(record, 2, 2669, 2705, 4)
	putNapStage(record, 3, 2706, 2710, 8)
	putNapStage(record, 4, 2711, 2719, 4)
	putSleepTotals(record, 84, 164, 59, 3)

	rows, _ := parseOneSleep(t, record)
	if len(rows) != 1 || !rows[0].IsNap {
		t.Fatalf("rows = %+v, want only the nap", rows)
	}
	if len(rows[0].Stages) != 5 || rows[0].StagesTruncated {
		t.Fatalf("nap stages = %d truncated = %t, want five complete stages",
			len(rows[0].Stages), rows[0].StagesTruncated)
	}
}

func TestSleepStageCapNeverReadsNapSlots(t *testing.T) {
	record := sleepRecord(sleepFixtureWrite, sleepFixtureMidnight, 1520, 1830)
	record[sleepMainCountOff] = 0xFF
	for i := range sleepMainStageMax {
		putSleepStage(record, i, 1520+i, 1520+i, 4)
	}
	putNapStage(record, 0, 2600, 2610, 5)
	putSleepTotals(record, 0, 51, 0, 0)

	rows, _ := parseOneSleep(t, record)
	if len(rows) != 1 {
		t.Fatalf("sleep rows = %d, want the main row only", len(rows))
	}
	if len(rows[0].Stages) != sleepMainStageMax {
		t.Fatalf("main stages = %d, want the %d slot cap", len(rows[0].Stages), sleepMainStageMax)
	}
	for _, stage := range rows[0].Stages {
		if stage.Type == 5 {
			t.Fatal("the main timeline read a nap slot")
		}
	}
	if !rows[0].StagesTruncated {
		t.Fatal("a stage count above the cap must be flagged as truncated")
	}
}

func TestSleepTruncatedTimelineKeepsDeviceTotals(t *testing.T) {
	record := sleepRecord(sleepFixtureWrite, sleepFixtureMidnight, 1520, 1540)
	record[sleepMainCountOff] = 2
	putSleepStage(record, 0, 1520, 1529, 4)
	putSleepStage(record, 1, 1530, 1539, 5)
	putSleepTotals(record, 0, 100, 0, 0)

	rows, _ := parseOneSleep(t, record)
	if len(rows) != 1 {
		t.Fatalf("sleep rows = %d, want 1", len(rows))
	}
	row := rows[0]
	if row.LightMins != 100 || row.TotalMins != 100 {
		t.Fatalf("totals = light %d total %d, want the device summary 100/100", row.LightMins, row.TotalMins)
	}
	if !row.StagesTruncated {
		t.Fatal("disagreeing stage and device totals must be flagged")
	}
}

func TestNapInclusiveEndBecomesExclusive(t *testing.T) {
	record := sleepRecord(sleepFixtureWrite, sleepFixtureMidnight, 0, 0)
	record[sleepFlagsOff+1] = 0
	putNap(record, 0, 2646, 2719, 74)
	putNapStage(record, 0, 2646, 2658, 4)
	putNapStage(record, 1, 2659, 2668, 5)
	putNapStage(record, 2, 2669, 2705, 4)
	putNapStage(record, 3, 2706, 2710, 8)
	putNapStage(record, 4, 2711, 2719, 4)

	rows, _ := parseOneSleep(t, record)
	if len(rows) != 1 {
		t.Fatalf("sleep rows = %d, want 1 nap", len(rows))
	}
	wantStart := time.Date(2026, 6, 22, 14, 36, 0, 0, time.UTC)
	wantEnd := time.Date(2026, 6, 22, 15, 50, 0, 0, time.UTC)
	if !rows[0].StartedAt.Equal(wantStart) || !rows[0].EndedAt.Equal(wantEnd) {
		t.Fatalf("nap window = %s..%s, want %s..%s",
			formatFixtureTime(rows[0].StartedAt), formatFixtureTime(rows[0].EndedAt),
			formatFixtureTime(wantStart), formatFixtureTime(wantEnd))
	}
}

func TestSleepWriteDayRegression(t *testing.T) {
	midnight := time.Date(2026, 9, 24, 18, 30, 0, 0, time.UTC) // September 25 IST midnight
	write := time.Date(2026, 9, 25, 21, 39, 0, 0, time.UTC)    // September 26 03:09 IST
	window := Window{
		StartedAt: time.Date(2026, 9, 25, 0, 32, 0, 0, time.UTC),
		EndedAt:   time.Date(2026, 9, 25, 0, 33, 0, 0, time.UTC),
	}
	// Header minutes count from 24 hours before the midnight field, so a 01:29
	// IST start is 1440+89.
	record := sleepRecord(write.Unix(), midnight.Unix(), 1440+89, 1440+361)
	record[sleepMainCountOff] = 1
	putSleepStage(record, 0, 1440+89, 1440+360, 4)
	putSleepTotals(record, 0, 0, 272, 0)

	rows, _, err := parseSleep(record, window)
	if err != nil {
		t.Fatalf("parse sleep: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("sleep rows = %d, want 1", len(rows))
	}
	if rows[0].DayKey != "2026-09-25" {
		t.Fatalf("day key = %s, want 2026-09-25", rows[0].DayKey)
	}
	wantStart := time.Date(2026, 9, 24, 19, 59, 0, 0, time.UTC)
	if !rows[0].StartedAt.Equal(wantStart) {
		t.Fatalf("start = %s, want %s", formatFixtureTime(rows[0].StartedAt), formatFixtureTime(wantStart))
	}
}

func TestSleepRecordWithoutMidnightIsDropped(t *testing.T) {
	record := sleepRecord(sleepFixtureWrite, 0, 1520, 1530)
	record[sleepMainCountOff] = 1
	putSleepStage(record, 0, 1520, 1529, 4)
	putSleepTotals(record, 0, 10, 0, 0)

	rows, result := parseOneSleep(t, record)
	if len(rows) != 0 || result.Dropped != 1 {
		t.Fatalf("rows=%d dropped=%d, want the record dropped", len(rows), result.Dropped)
	}
}
