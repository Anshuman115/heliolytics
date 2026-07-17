package api

import "net/http"

func (h *metricsHandler) rhr(w http.ResponseWriter, r *http.Request) {
	from, to := dayRange(r)
	samples, err := h.st.ListRhrSamplesRange(r.Context(), from, to)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	logMetrics("rhr", r, "")
	writeJSON(w, map[string]any{"points": toTrendPoints(samples)})
}
