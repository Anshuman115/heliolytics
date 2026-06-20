package api

import (
	"net/http"
	"time"
)

func (h *metricsHandler) hr(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	from, to := dayRange(r)
	rows, err := h.st.ListHeartRateSamples(r.Context(), from, to)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	type pt struct {
		DayKey    string `json:"dayKey"`
		SampledAt string `json:"sampledAt"`
		Bpm       int    `json:"bpm"`
	}
	out := make([]pt, len(rows))
	for i, p := range rows {
		out[i] = pt{
			DayKey: p.DayKey, SampledAt: p.SampledAt.Format(time.RFC3339), Bpm: p.Bpm,
		}
	}
	writeJSON(w, map[string]any{"samples": out})
}
