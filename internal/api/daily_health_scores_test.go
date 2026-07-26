package api

import (
	"testing"

	"github.com/heliolytics/api/internal/store"
)

func TestToHealthScoreTilesUsesActualMainSleepWindow(t *testing.T) {
	rows := []store.DayMetric{{DayKey: "2026-07-26"}}
	sleep := []store.SleepMetric{
		{DayKey: "2026-07-26", Score: 70, TotalMins: 90, WakeMins: 10, IsNap: true},
		{DayKey: "2026-07-26", Score: 88, TotalMins: 406, WakeMins: 34},
	}

	tiles := toHealthScoreTiles(rows, sleep)
	if len(tiles) != 1 || tiles[0].TimeInBedMins == nil || *tiles[0].TimeInBedMins != 440 {
		t.Fatalf("timeInBedMins = %#v, want 440", tiles[0].TimeInBedMins)
	}
	if tiles[0].SleepEfficiencyPct == nil || *tiles[0].SleepEfficiencyPct != 92 {
		t.Fatalf("sleepEfficiencyPct = %#v, want 92", tiles[0].SleepEfficiencyPct)
	}
}
