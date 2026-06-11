package api

import (
	"net/http"
	"time"

	"github.com/heliolytics/api/internal/parse"
)

func (h *metricsHandler) workouts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	from, to := workoutDayRange(r)
	rows, err := h.st.ListWorkouts(r.Context(), from, to)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	type row struct {
		DayKey      string `json:"dayKey"`
		StartedAt   string `json:"startedAt"`
		SportType   int    `json:"sportType"`
		SportName   string `json:"sportName"`
		DurationSec int    `json:"durationSec"`
		Calories    *int   `json:"calories,omitempty"`
		AvgHr       *int   `json:"avgHr,omitempty"`
		MaxHr       *int   `json:"maxHr,omitempty"`
	}
	out := make([]row, len(rows))
	for i, w := range rows {
		name := w.SportName
		if name == "" {
			name = parse.SportName(w.SportType)
		}
		out[i] = row{
			DayKey: w.DayKey, StartedAt: w.StartedAt.Format(time.RFC3339),
			SportType: w.SportType, SportName: name, DurationSec: w.DurationSec,
			Calories: w.Calories, AvgHr: w.AvgHr, MaxHr: w.MaxHr,
		}
	}
	writeJSON(w, map[string]any{"workouts": out})
}
