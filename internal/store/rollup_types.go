package store

import (
	"encoding/json"
	"time"
)

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

type LoadUpdate struct {
	DayKey time.Time
	TRIMP  *float64
}

type ReadinessUpdate struct {
	DayKey         time.Time
	Score          *int
	Components     json.RawMessage
	TempDeviationC *float64
}

type ActivityMinute struct {
	At    time.Time
	Steps int
	HR    *int
}

type WorkoutInterval struct {
	Start time.Time
	End   time.Time
}
