package api

import (
	"log"
	"net/http"
	"strconv"

	"github.com/heliolytics/api/internal/store"
)

func (h *metricsHandler) dailyHealthScores(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		logMetrics("daily_health_scores", r, "reject=method_not_allowed")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	from, to := dayRange(r)
	rows, err := h.st.ListDays(r.Context(), from, to)
	if err != nil {
		log.Printf("daily-health-scores db error from=%s to=%s err=%v", from, to, err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	sleep, err := h.st.ListSleep(r.Context(), from, to)
	if err != nil {
		log.Printf("daily-health-scores sleep db error from=%s to=%s err=%v", from, to, err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	logMetrics("daily_health_scores", r, "rows="+strconv.Itoa(len(rows)))
	writeJSON(w, map[string]any{"days": toHealthScoreTiles(rows, sleep)})
}

// healthScoreTile is one daily_metrics row reshaped for the dashboard's
// health-scores tile grid. Vo2max has no data source yet — always nil.
type healthScoreTile struct {
	DayKey             string `json:"dayKey"`
	Hrv                *int   `json:"hrv"`
	Rhr                *int   `json:"rhr"`
	Vo2max             *int   `json:"vo2max"`
	CaloriesTotal      *int   `json:"caloriesTotal"`
	SleepDeepMins      *int   `json:"sleepDeepMins"`
	SleepRemMins       *int   `json:"sleepRemMins"`
	SleepLightMins     *int   `json:"sleepLightMins"`
	AvgHr              *int   `json:"avgHr"`
	TimeInBedMins      *int   `json:"timeInBedMins"`
	SleepEfficiencyPct *int   `json:"sleepEfficiencyPct"`
}

func toHealthScoreTiles(rows []store.DayMetric, sleep []store.SleepMetric) []healthScoreTile {
	mainSleep := make(map[string]store.SleepMetric)
	for _, session := range sleep {
		best, exists := mainSleep[session.DayKey]
		if session.IsNap || exists && session.Score <= best.Score {
			continue
		}
		mainSleep[session.DayKey] = session
	}
	out := make([]healthScoreTile, 0, len(rows))
	for _, r := range rows {
		tile := healthScoreTile{
			DayKey: r.DayKey, Hrv: r.HrvRmssd, Rhr: r.RestingHr,
			CaloriesTotal: r.CaloriesTotal, SleepDeepMins: r.SleepDeepMins,
			SleepRemMins: r.SleepRemMins, SleepLightMins: r.SleepLightMins,
			AvgHr: r.AvgHr,
		}
		if session, ok := mainSleep[r.DayKey]; ok {
			timeInBed := session.TotalMins + session.WakeMins
			tile.TimeInBedMins = &timeInBed
			if timeInBed > 0 {
				efficiency := (session.TotalMins*100 + timeInBed/2) / timeInBed
				tile.SleepEfficiencyPct = &efficiency
			}
		}
		out = append(out, tile)
	}
	return out
}
