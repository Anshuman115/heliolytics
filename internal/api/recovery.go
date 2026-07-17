package api

import (
	"log"
	"net/http"

	"github.com/heliolytics/api/internal/readiness"
)

func (h *metricsHandler) recovery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		logMetrics("recovery", r, "reject=method_not_allowed")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	day := r.URL.Query().Get("day")
	if day == "" {
		http.Error(w, "day query param required", http.StatusBadRequest)
		return
	}
	hist, err := h.st.ReadinessHistoryPublic(r.Context(), day)
	if err != nil {
		log.Printf("recovery db error day=%s err=%v", day, err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	score, components, ok := readiness.Compute(hist)
	logMetrics("recovery", r, "day="+day)
	if !ok {
		writeJSON(w, map[string]any{"dayKey": day, "score": nil, "components": nil, "buildingBaseline": true})
		return
	}
	writeJSON(w, map[string]any{"dayKey": day, "score": score, "components": components})
}
