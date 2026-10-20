package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Anshuman115/heliolytics/internal/store/db"
)

type DayMetric struct {
	DayKey              string          `json:"dayKey"`
	Steps               *int            `json:"steps"`
	AvgHr               *float64        `json:"avgHr"`
	RestingHr           *float64        `json:"restingHr"`
	HrvRmssd            *float64        `json:"hrvRmssd"`
	RespRate            *float64        `json:"respRate"`
	Spo2Avg             *float64        `json:"spo2Avg"`
	Spo2Min             *int            `json:"spo2Min"`
	Spo2SleepAvg        *float64        `json:"spo2SleepAvg"`
	Spo2SleepMin        *int            `json:"spo2SleepMin"`
	Spo2Samples         int             `json:"spo2Samples"`
	StressAvg           *float64        `json:"stressAvg"`
	SkinTempNightC      *float64        `json:"skinTempNightC"`
	SkinTempDeviationC  *float64        `json:"skinTempDeviationC"`
	SleepScore          *int            `json:"sleepScore"`
	SleepMins           *int            `json:"sleepMins"`
	SleepDeepMins       *int            `json:"sleepDeepMins"`
	SleepRemMins        *int            `json:"sleepRemMins"`
	SleepLightMins      *int            `json:"sleepLightMins"`
	SleepWakeMins       *int            `json:"sleepWakeMins"`
	TimeInBedMins       *int            `json:"timeInBedMins"`
	SleepEfficiencyPct  *int            `json:"sleepEfficiencyPct"`
	NapCount            int             `json:"napCount"`
	WorkoutCount        int             `json:"workoutCount"`
	AutoActivityCount   int             `json:"autoActivityCount"`
	WorkoutCalories     *int            `json:"workoutCalories"`
	TrainingLoad        *float64        `json:"trainingLoad"`
	PaiScore            *int            `json:"paiScore"`
	Readiness           *int            `json:"readiness"`
	ReadinessStatus     *string         `json:"readinessStatus"`
	ReadinessBand       *string         `json:"readinessBand"`
	ReadinessComponents json.RawMessage `json:"readinessComponents"`
	UpdatedAt           time.Time       `json:"updatedAt"`
}

type SleepMetric struct {
	DayKey string
	Row    SleepRow
}

type ActivityMetric struct {
	DayKey      string
	DurationSec int
	Row         ActivityRow
}

type CompactDay struct {
	DayKey    string    `json:"dayKey"`
	StartTime time.Time `json:"startTime"`
	Offsets   []int32   `json:"offsets"`
	Values    []float64 `json:"values"`
}

func (s *Store) ListDays(ctx context.Context, from, to string) ([]DayMetric, error) {
	fromDay, toDay, err := dayBounds(from, to)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListDailyMetrics(ctx, db.ListDailyMetricsParams{
		FromDay: pgDate(fromDay),
		ToDay:   pgDate(toDay),
	})
	if err != nil {
		return nil, err
	}
	out := make([]DayMetric, 0, len(rows))
	for _, r := range rows {
		out = append(out, dayMetricFromRow(r))
	}
	return out, nil
}

func dayMetricFromRow(r db.DailyMetric) DayMetric {
	readiness := int2Val(r.Readiness)
	components := json.RawMessage(nil)
	if len(r.ReadinessComponents) > 0 {
		components = json.RawMessage(r.ReadinessComponents)
	}
	return DayMetric{
		DayKey:              r.DayKey.Time.Format(dateLayout),
		Steps:               int4Val(r.Steps),
		AvgHr:               float4Val(r.AvgHr),
		RestingHr:           float4Val(r.RestingHr),
		HrvRmssd:            float4Val(r.HrvRmssdMs),
		RespRate:            float4Val(r.RespRate),
		Spo2Avg:             float4Val(r.Spo2Avg),
		Spo2Min:             int2Val(r.Spo2Min),
		Spo2SleepAvg:        float4Val(r.Spo2SleepAvg),
		Spo2SleepMin:        int2Val(r.Spo2SleepMin),
		Spo2Samples:         int(r.Spo2N),
		StressAvg:           float4Val(r.StressAvg),
		SkinTempNightC:      float4Val(r.SkinTempNightC),
		SkinTempDeviationC:  float4Val(r.SkinTempDevC),
		SleepScore:          int2Val(r.SleepScore),
		SleepMins:           int2Val(r.SleepMins),
		SleepDeepMins:       int2Val(r.SleepDeepMins),
		SleepRemMins:        int2Val(r.SleepRemMins),
		SleepLightMins:      int2Val(r.SleepLightMins),
		SleepWakeMins:       int2Val(r.SleepWakeMins),
		TimeInBedMins:       int2Val(r.TimeInBedMins),
		SleepEfficiencyPct:  int2Val(r.SleepEfficiencyPct),
		NapCount:            int(r.NapCount),
		WorkoutCount:        int(r.WorkoutCount),
		AutoActivityCount:   int(r.AutoActivityCount),
		WorkoutCalories:     int4Val(r.WorkoutCalories),
		TrainingLoad:        float4Val(r.Trimp),
		PaiScore:            int2Val(r.PaiScore),
		Readiness:           readiness,
		ReadinessStatus:     readinessStatus(components),
		ReadinessBand:       readinessBand(readiness),
		ReadinessComponents: components,
		UpdatedAt:           r.UpdatedAt.Time,
	}
}

func readinessStatus(components json.RawMessage) *string {
	if len(components) == 0 {
		return nil
	}
	var wrapper struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(components, &wrapper); err != nil || wrapper.Status == "" {
		return nil
	}
	return new(wrapper.Status)
}

func readinessBand(score *int) *string {
	if score == nil {
		return nil
	}
	band := "low"
	switch {
	case *score >= 67:
		band = "good"
	case *score >= 34:
		band = "fair"
	}
	return &band
}

func (s *Store) ListSleep(ctx context.Context, from, to string) ([]SleepMetric, error) {
	fromDay, toDay, err := dayBounds(from, to)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListSleepSessions(ctx, db.ListSleepSessionsParams{
		FromDay: pgDate(fromDay),
		ToDay:   pgDate(toDay),
	})
	if err != nil {
		return nil, err
	}
	out := make([]SleepMetric, 0, len(rows))
	for _, r := range rows {
		row, err := sleepRowOf(r.StartedAt.Time, r.EndedAt.Time, r.DayKey, r.IsNap, r.Score,
			r.TotalMins, r.DeepMins, r.RemMins, r.LightMins, r.WakeMins, r.Stages, r.StagesTruncated)
		if err != nil {
			return nil, err
		}
		out = append(out, SleepMetric{DayKey: dateOf(r.DayKey).Format(dateLayout), Row: row})
	}
	return out, nil
}

func (s *Store) ListActivities(ctx context.Context, from, to string) ([]ActivityMetric, error) {
	fromDay, toDay, err := dayBounds(from, to)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListActivitiesRange(ctx, db.ListActivitiesRangeParams{
		FromDay: pgDate(fromDay),
		ToDay:   pgDate(toDay),
	})
	if err != nil {
		return nil, err
	}
	out := make([]ActivityMetric, 0, len(rows))
	for _, r := range rows {
		row := ActivityRow{
			StartedAt: r.StartedAt.Time,
			EndedAt:   r.EndedAt.Time,
			Kind:      r.Kind,
			SportType: int(r.SportType),
			SportName: r.SportName,
			Calories:  int2Val(r.Calories),
			AvgHR:     int2Val(r.AvgHr),
			MaxHR:     int2Val(r.MaxHr),
			MinHR:     int2Val(r.MinHr),
			Steps:     int4Val(r.Steps),
			HasDetail: r.HasDetail,
		}
		out = append(out, ActivityMetric{
			DayKey:      dayOf(row.StartedAt).Format(dateLayout),
			DurationSec: int(row.EndedAt.Sub(row.StartedAt).Seconds()),
			Row:         row,
		})
	}
	return out, nil
}
