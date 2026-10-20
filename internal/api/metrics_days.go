package api

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/Anshuman115/heliolytics/internal/middleware"
	"github.com/Anshuman115/heliolytics/internal/store"
)

// dayMetric is the C3.3 DayMetric. Every key is present; a null value means
// the measurement is absent from storage.
type dayMetric struct {
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
	UpdatedAt           string          `json:"updatedAt"`
}

type daysResponse struct {
	Days []dayMetric `json:"days"`
}

// days serves the stored daily rows. Readiness status, band and components come
// from the row; nothing is scored or relabelled here.
func (h *metricsHandler) days(w http.ResponseWriter, r *http.Request) {
	from, to, failure := h.rangeBounds(r, maxDailyRangeDays)
	if failure != nil {
		middleware.WriteError(w, failure)
		return
	}
	rows, err := h.st.ListDays(r.Context(), from, to)
	if err != nil {
		writeStoreFailure(w, "days", err)
		return
	}
	out := make([]dayMetric, 0, len(rows))
	for _, row := range rows {
		out = append(out, dayMetricOf(row))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DayKey < out[j].DayKey })
	writeJSON(w, http.StatusOK, daysResponse{Days: out})
}

// dayMetricOf maps one stored row to the contract shape, rounding only
// presentation floats.
func dayMetricOf(row store.DayMetric) dayMetric {
	components := row.ReadinessComponents
	if len(components) == 0 {
		components = nil
	}
	return dayMetric{
		DayKey:              row.DayKey,
		Steps:               row.Steps,
		AvgHr:               round1(row.AvgHr),
		RestingHr:           round1(row.RestingHr),
		HrvRmssd:            round1(row.HrvRmssd),
		RespRate:            round1(row.RespRate),
		Spo2Avg:             round1(row.Spo2Avg),
		Spo2Min:             row.Spo2Min,
		Spo2SleepAvg:        round1(row.Spo2SleepAvg),
		Spo2SleepMin:        row.Spo2SleepMin,
		Spo2Samples:         row.Spo2Samples,
		StressAvg:           round1(row.StressAvg),
		SkinTempNightC:      round2(row.SkinTempNightC),
		SkinTempDeviationC:  round2(row.SkinTempDeviationC),
		SleepScore:          row.SleepScore,
		SleepMins:           row.SleepMins,
		SleepDeepMins:       row.SleepDeepMins,
		SleepRemMins:        row.SleepRemMins,
		SleepLightMins:      row.SleepLightMins,
		SleepWakeMins:       row.SleepWakeMins,
		TimeInBedMins:       row.TimeInBedMins,
		SleepEfficiencyPct:  row.SleepEfficiencyPct,
		NapCount:            row.NapCount,
		WorkoutCount:        row.WorkoutCount,
		AutoActivityCount:   row.AutoActivityCount,
		WorkoutCalories:     row.WorkoutCalories,
		TrainingLoad:        round1(row.TrainingLoad),
		PaiScore:            row.PaiScore,
		Readiness:           row.Readiness,
		ReadinessStatus:     row.ReadinessStatus,
		ReadinessBand:       row.ReadinessBand,
		ReadinessComponents: components,
		UpdatedAt:           formatInstant(row.UpdatedAt),
	}
}
