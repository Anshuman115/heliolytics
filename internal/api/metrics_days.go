package api

import (
	"log"
	"net/http"
	"strconv"
)

func (h *metricsHandler) days(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		logMetrics("days", r, "reject=method_not_allowed")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	from, to := dayRange(r)
	logMetrics("days", r, "from="+from+" to="+to)
	rows, err := h.st.ListDays(r.Context(), from, to)
	if err != nil {
		log.Printf("metrics days db error from=%s to=%s err=%v", from, to, err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	logMetrics("days", r, "ok rows="+strconv.Itoa(len(rows)))
	writeJSON(w, map[string]any{"days": rows})
}
