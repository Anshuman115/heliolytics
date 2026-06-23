package store

import "time"

type DayMetric struct {
	DayKey         string    `json:"dayKey"`
	Steps          int       `json:"steps"`
	PaiScore       *int      `json:"paiScore,omitempty"`
	Readiness      *int      `json:"readiness,omitempty"`
	Spo2Avg        *int      `json:"spo2Avg,omitempty"`
	HrvRmssd       *int      `json:"hrvRmssd,omitempty"`
	RestingHr      *int      `json:"restingHr,omitempty"`
	RespRateAvg    *int      `json:"respRateAvg,omitempty"`
	StressAvg      *int      `json:"stressAvg,omitempty"`
	SleepScore     *int      `json:"sleepScore,omitempty"`
	SleepMins      *int      `json:"sleepMins,omitempty"`
	SleepDeepMins  *int      `json:"sleepDeepMins,omitempty"`
	SleepRemMins   *int      `json:"sleepRemMins,omitempty"`
	SleepLightMins *int      `json:"sleepLightMins,omitempty"`
	TempAvgC       *float64  `json:"tempAvgC,omitempty"`
	NapCount             int       `json:"napCount"`
	WorkoutCount         int       `json:"workoutCount"`
	ActivitySessionCount int       `json:"activitySessionCount"`
	Updated              time.Time `json:"updatedAt"`
}

type SleepMetric struct {
	DayKey    string            `json:"dayKey"`
	StartedAt time.Time         `json:"startedAt"`
	Score     int               `json:"score"`
	TotalMins int               `json:"totalMins"`
	DeepMins  int               `json:"deepMins"`
	RemMins   int               `json:"remMins"`
	LightMins int               `json:"lightMins"`
	WakeMins  int               `json:"wakeMins"`
	IsNap     bool              `json:"isNap"`
	Stages    []SleepStagePoint `json:"stages,omitempty"`
}

type SleepRow struct {
	DayKey    string
	StartedAt time.Time
	Score     int
	TotalMins int
	DeepMins  int
	RemMins   int
	LightMins int
	WakeMins  int
	IsNap     bool
	Stages    []SleepStagePoint
}

type WorkoutRow struct {
	DayKey      string
	StartedAt   time.Time
	SportType   int
	SportName   string
	DurationSec int
	Calories    *int
	AvgHr       *int
	MaxHr       *int
}

type ActivitySessionRow struct {
	DayKey      string
	StartedAt   time.Time
	SportType   int
	SportName   string
	DurationSec int
	Calories    *int
	AvgHr       *int
	MaxHr       *int
}
