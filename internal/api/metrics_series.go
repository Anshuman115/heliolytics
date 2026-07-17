package api

import (
	"log"
	"net/http"
	"strconv"
)

// series restores the v6 compact multi-metric series endpoint, sourced from
// the 5 per-metric tables introduced by the health_samples split instead of
// the old tall table. Response shape is unchanged from v6 so existing app
// clients (Heliolytics_App/lib/services/metrics_api_client.dart) keep working.
func (h *metricsHandler) series(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		logMetrics("series", r, "reject=method_not_allowed")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	from, to := dayRange(r)
	days, err := h.st.ListHealthSamplesCompact(r.Context(), from, to)
	if err != nil {
		log.Printf("metrics series db error from=%s to=%s err=%v", from, to, err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	logMetrics("series", r, "ok rows="+strconv.Itoa(len(days)))
	writeJSON(w, map[string]any{"days": days})
}
