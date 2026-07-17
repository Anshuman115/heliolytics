package api

import (
	"log"
	"net/http"
)

func (h *metricsHandler) strain(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		logMetrics("strain", r, "reject=method_not_allowed")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	day := r.URL.Query().Get("day")
	if day == "" {
		http.Error(w, "day query param required", http.StatusBadRequest)
		return
	}
	pai, err := h.st.GetPaiScoreForDay(r.Context(), day)
	if err != nil {
		log.Printf("strain db error day=%s err=%v", day, err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	logMetrics("strain", r, "day="+day)
	writeJSON(w, map[string]any{"dayKey": day, "paiScore": pai})
}
