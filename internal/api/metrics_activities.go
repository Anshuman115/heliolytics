package api

import (
	"net/http"
	"sort"

	"github.com/Anshuman115/heliolytics/internal/middleware"
	"github.com/Anshuman115/heliolytics/internal/store"
)

// activity is one C3.3 unified activity row. A workout and an automatic
// activity may share a timestamp; kind keeps them distinct.
type activity struct {
	Kind        string `json:"kind"`
	DayKey      string `json:"dayKey"`
	StartedAt   string `json:"startedAt"`
	EndedAt     string `json:"endedAt"`
	DurationSec int    `json:"durationSec"`
	SportType   int    `json:"sportType"`
	SportName   string `json:"sportName"`
	Calories    *int   `json:"calories"`
	AvgHr       *int   `json:"avgHr"`
	MaxHr       *int   `json:"maxHr"`
	MinHr       *int   `json:"minHr"`
	Steps       *int   `json:"steps"`
	HasDetail   bool   `json:"hasDetail"`
}

type activitiesResponse struct {
	Activities []activity `json:"activities"`
}

// activities serves the stored unified activity rows in ascending start order.
// Labels and durations come from storage; nothing is derived here.
func (h *metricsHandler) activities(w http.ResponseWriter, r *http.Request) {
	from, to, failure := h.rangeBounds(r, maxDailyRangeDays)
	if failure != nil {
		middleware.WriteError(w, failure)
		return
	}
	rows, err := h.st.ListActivities(r.Context(), from, to)
	if err != nil {
		writeStoreFailure(w, "activities", err)
		return
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Row.StartedAt.Equal(rows[j].Row.StartedAt) {
			return rows[i].Row.Kind < rows[j].Row.Kind
		}
		return rows[i].Row.StartedAt.Before(rows[j].Row.StartedAt)
	})
	out := make([]activity, 0, len(rows))
	for _, row := range rows {
		out = append(out, activityOf(row))
	}
	writeJSON(w, http.StatusOK, activitiesResponse{Activities: out})
}

func activityOf(metric store.ActivityMetric) activity {
	return activity{
		Kind:        metric.Row.Kind,
		DayKey:      metric.DayKey,
		StartedAt:   formatInstant(metric.Row.StartedAt),
		EndedAt:     formatInstant(metric.Row.EndedAt),
		DurationSec: metric.DurationSec,
		SportType:   metric.Row.SportType,
		SportName:   metric.Row.SportName,
		Calories:    metric.Row.Calories,
		AvgHr:       metric.Row.AvgHR,
		MaxHr:       metric.Row.MaxHR,
		MinHr:       metric.Row.MinHR,
		Steps:       metric.Row.Steps,
		HasDetail:   metric.Row.HasDetail,
	}
}
