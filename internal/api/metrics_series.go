package api

import (
	"net/http"
	"time"
)

func (h *metricsHandler) series(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	from, to := dayRange(r)
	rows, err := h.st.ListHealthSamples(r.Context(), from, to)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	type pt struct {
		Metric    string  `json:"metric"`
		DayKey    string  `json:"dayKey"`
		SampledAt string  `json:"sampledAt"`
		Value     float64 `json:"value"`
	}
	out := make([]pt, len(rows))
	for i, p := range rows {
		out[i] = pt{
			Metric: p.Metric, DayKey: p.DayKey,
			SampledAt: p.SampledAt.Format(time.RFC3339), Value: p.Value,
		}
	}
	writeJSON(w, map[string]any{"samples": out})
}
