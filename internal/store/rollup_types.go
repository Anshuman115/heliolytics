package store

import (
	"encoding/json"
	"time"
)

// RollupInputs is every exact canonical input the rollup needs for a date
// range. Nothing here is pre-averaged or preselected; B05 owns the math.
type RollupInputs struct {
	HR           []HeartRateSample
	Steps        []StepSample
	Temperatures []TemperatureReadRow
	HRV          []HRVSample
	Stress       []StressSample
	Resp         []RespSample
	SpO2         []SpO2Sample
	Sleep        []SleepRow
	Activities   []ActivityRow
	PAI          []PAIScore
}

// DailyAggregate owns the aggregate columns of one daily_metrics row.
type DailyAggregate struct {
	DayKey             time.Time
	Steps              *int
	AvgHR              *float64
	RestingHR          *float64
	HRVRMSSDMS         *float64
	RespRate           *float64
	SpO2Avg            *float64
	SpO2Min            *int
	SpO2SleepAvg       *float64
	SpO2SleepMin       *int
	SpO2N              int
	StressAvg          *float64
	SkinTempNightC     *float64
	SleepScore         *int
	SleepMins          *int
	SleepDeepMins      *int
	SleepRemMins       *int
	SleepLightMins     *int
	SleepWakeMins      *int
	TimeInBedMins      *int
	SleepEfficiencyPct *int
	NapCount           int
	WorkoutCount       int
	AutoActivityCount  int
	WorkoutCalories    *int
	PAIScore           *int
	TRIMP              *float64
}

// LoadUpdate owns the training-load column of one day.
type LoadUpdate struct {
	DayKey time.Time
	TRIMP  *float64
}

// ReadinessUpdate owns the readiness columns of one day.
type ReadinessUpdate struct {
	DayKey         time.Time
	Score          *int
	Components     json.RawMessage
	TempDeviationC *float64
}

// ActivityMinute is one observed step minute with optional 0x01 heart rate.
type ActivityMinute struct {
	At    time.Time
	Steps int
	HR    *int
}

// WorkoutInterval is one authoritative workout barrier.
type WorkoutInterval struct {
	Start time.Time
	End   time.Time
}
