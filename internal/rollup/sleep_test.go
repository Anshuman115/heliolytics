package rollup

import (
	"testing"
	"time"

	"github.com/Anshuman115/heliolytics/internal/store"
)

// Shared fixtures for the pure rollup tests.
var testDay = time.Date(2026, 10, 20, 0, 0, 0, 0, store.IST)

func at(hour, minute int) time.Time {
	return testDay.Add(time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute)
}

func testSleep(start, end time.Time, total, deep, rem, light, wake int, score int, nap bool) store.SleepRow {
	return store.SleepRow{
		StartedAt: start,
		EndedAt:   end,
		DayKey:    dayStart(testDay),
		IsNap:     nap,
		Score:     new(score),
		TotalMins: total,
		DeepMins:  deep,
		RemMins:   rem,
		LightMins: light,
		WakeMins:  wake,
	}
}

func TestSleepUnionExcludesInterSessionGap(t *testing.T) {
	rows := []store.SleepRow{
		testSleep(at(22, 0), at(25, 0), 120, 40, 30, 50, 10, 70, false),
		testSleep(at(27, 0), at(30, 0), 100, 30, 30, 40, 5, 90, false),
	}
	got := summarizeSleep(rows, testDay)
	if got.minutes != 360 {
		t.Fatalf("union minutes = %d, want 360", got.minutes)
	}
	if len(got.union) != 2 {
		t.Fatalf("union spans = %d, want 2", len(got.union))
	}
	if !got.quality {
		t.Fatal("a 360 minute union must clear the night floor")
	}
}

func TestSleepScoreComesFromLongestSession(t *testing.T) {
	rows := []store.SleepRow{
		testSleep(at(22, 0), at(25, 0), 120, 40, 30, 50, 10, 70, false),
		testSleep(at(27, 0), at(30, 0), 100, 30, 30, 40, 5, 90, false),
	}
	got := summarizeSleep(rows, testDay)
	if got.score == nil || *got.score != 70 {
		t.Fatalf("score = %v, want the longer session's 70", got.score)
	}
	if got.asleep == nil || *got.asleep != 220 {
		t.Fatalf("sleep mins = %v, want 220", got.asleep)
	}
	if got.wake == nil || *got.wake != 15 {
		t.Fatalf("wake mins = %v, want 15", got.wake)
	}
	if got.inBed == nil || *got.inBed != 235 {
		t.Fatalf("time in bed = %v, want 235", got.inBed)
	}
	if got.efficiency == nil || *got.efficiency != 94 {
		t.Fatalf("efficiency = %v, want 94", got.efficiency)
	}
	if got.deep == nil || *got.deep != 70 || got.rem == nil || *got.rem != 60 || got.light == nil || *got.light != 90 {
		t.Fatalf("stage sums = %v/%v/%v, want 70/60/90", got.deep, got.rem, got.light)
	}
}

func TestNapOnlyDayKeepsMainValuesNull(t *testing.T) {
	rows := []store.SleepRow{
		testSleep(at(14, 0), at(15, 0), 60, 20, 20, 20, 0, 60, true),
	}
	got := summarizeSleep(rows, testDay)
	if got.naps != 1 {
		t.Fatalf("nap count = %d, want 1", got.naps)
	}
	if got.score != nil || got.asleep != nil || got.inBed != nil || got.efficiency != nil {
		t.Fatal("a nap-only day has no main-sleep values")
	}
	if got.quality {
		t.Fatal("a nap must not open the night window")
	}
}

func TestNightFloorBoundary(t *testing.T) {
	short := []store.SleepRow{testSleep(at(1, 0), at(3, 59), 179, 60, 60, 59, 0, 60, false)}
	if summarizeSleep(short, testDay).quality {
		t.Fatal("179 minute union must stay below the night floor")
	}
	full := []store.SleepRow{testSleep(at(1, 0), at(4, 0), 180, 60, 60, 60, 0, 60, false)}
	if !summarizeSleep(full, testDay).quality {
		t.Fatal("180 minute union must clear the night floor")
	}
}

func TestSleepSummaryIgnoresOtherDays(t *testing.T) {
	rows := []store.SleepRow{
		testSleep(at(1, 0), at(4, 0), 180, 60, 60, 60, 0, 60, false),
		{
			StartedAt: at(1, 0), EndedAt: at(4, 0),
			DayKey: dayStart(testDay.AddDate(0, 0, -1)),
			Score:  new(10), TotalMins: 180, DeepMins: 60, RemMins: 60, LightMins: 60,
		},
	}
	got := summarizeSleep(rows, testDay)
	if got.asleep == nil || *got.asleep != 180 {
		t.Fatalf("sleep mins = %v, want 180", got.asleep)
	}
}
