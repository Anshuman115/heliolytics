package api

import (
	"net/http"
	"time"
)

func (h *metricsHandler) temperature(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	from, to := dayRange(r)
	rows, err := h.st.ListTemperature(r.Context(), from, to)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	type pt struct {
		DayKey    string  `json:"dayKey"`
		SampledAt string  `json:"sampledAt"`
		Celsius   float64 `json:"celsius"`
	}
	out := make([]pt, len(rows))
	for i, p := range rows {
		out[i] = pt{
			DayKey: p.DayKey, SampledAt: p.SampledAt.Format(time.RFC3339),
			Celsius: p.Celsius,
		}
	}
	writeJSON(w, map[string]any{"samples": out})
}
