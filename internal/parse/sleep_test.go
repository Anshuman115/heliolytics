package parse

import (
	"encoding/binary"
	"testing"
	"time"
)

func TestParseMainSleepStartsAtFirstStage(t *testing.T) {
	raw := make([]byte, sleepRecordSize)
	sessionTime := time.Date(2026, 7, 26, 4, 15, 0, 0, time.UTC)
	deviceMidnight := time.Date(2026, 7, 26, 18, 30, 0, 0, time.UTC)
	binary.LittleEndian.PutUint32(raw[0:], uint32(sessionTime.Unix()))
	binary.LittleEndian.PutUint32(raw[4:], uint32(deviceMidnight.Unix()))
	raw[0x08], raw[0x09] = 1, 1
	raw[0x54] = 1
	binary.LittleEndian.PutUint16(raw[0x56:], 480)
	binary.LittleEndian.PutUint16(raw[0x58:], 540)
	raw[0x5a] = 4
	binary.LittleEndian.PutUint16(raw[0x24c:], 60)

	out := ParseSleep(raw)
	if len(out) != 1 {
		t.Fatalf("records=%d want 1", len(out))
	}
	want := deviceMidnight.Add(-16 * time.Hour)
	if !out[0].StartedAt.Equal(want) {
		t.Fatalf("startedAt=%s want first stage=%s", out[0].StartedAt, want)
	}
	if !out[0].StartedAt.Equal(out[0].Stages[0].Start) {
		t.Fatal("startedAt must match the first stage")
	}
}

func TestSleepStageMinutesIncludeBothEndpointMinutes(t *testing.T) {
	base := time.Date(2026, 7, 25, 6, 0, 0, 0, time.UTC)
	stages := []SleepStage{
		{Start: base, End: base.Add(9 * time.Minute), Type: 4},
		{Start: base.Add(10 * time.Minute), End: base.Add(10 * time.Minute), Type: 7},
	}
	if got := sumStageMins(stages, 4); got != 10 {
		t.Fatalf("light minutes=%d want 10", got)
	}
	if got := sumStageMins(stages, 7); got != 1 {
		t.Fatalf("wake minutes=%d want 1", got)
	}
}

func TestParseNapExcludesAwakeMinutesFromTotalSleep(t *testing.T) {
	raw := make([]byte, sleepRecordSize)
	binary.LittleEndian.PutUint16(raw[0x18:], 100)
	binary.LittleEndian.PutUint16(raw[0x1a:], 109)
	binary.LittleEndian.PutUint16(raw[0x1c:], 10)
	binary.LittleEndian.PutUint16(raw[0x155:], 100)
	binary.LittleEndian.PutUint16(raw[0x157:], 108)
	raw[0x159] = 4
	binary.LittleEndian.PutUint16(raw[0x15a:], 109)
	binary.LittleEndian.PutUint16(raw[0x15c:], 109)
	raw[0x15e] = 7

	naps := parseNaps(raw, time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC).Unix())
	if len(naps) != 1 {
		t.Fatalf("naps=%d want 1", len(naps))
	}
	if naps[0].TotalMin != 9 || naps[0].WakeMin != 1 {
		t.Fatalf("total=%d wake=%d want total=9 wake=1", naps[0].TotalMin, naps[0].WakeMin)
	}
}
