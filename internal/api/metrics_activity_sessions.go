package api

import (
	"net/http"
	"time"

	"github.com/heliolytics/api/internal/parse"
)

func (h *metricsHandler) activitySessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	from, to := workoutDayRange(r)
	rows, err := h.st.ListActivitySessions(r.Context(), from, to)
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
	for i, s := range rows {
		name := s.SportName
		if name == "" {
			name = parse.SportName(s.SportType)
		}
		out[i] = row{
			DayKey: s.DayKey, StartedAt: s.StartedAt.Format(time.RFC3339),
			SportType: s.SportType, SportName: name, DurationSec: s.DurationSec,
			Calories: s.Calories, AvgHr: s.AvgHr, MaxHr: s.MaxHr,
		}
	}
	writeJSON(w, map[string]any{"activitySessions": out})
}
