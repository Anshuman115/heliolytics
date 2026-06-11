package api

import (
	"net/http"
	"time"
)

func (h *metricsHandler) coverage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cov, err := h.st.GetCoverage(r.Context())
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	out := map[string]any{"hasData": cov.HasData}
	if cov.DataThrough != nil {
		out["dataThrough"] = cov.DataThrough.UTC().Format(time.RFC3339)
	}
	if cov.LastIngestAt != nil {
		out["lastIngestAt"] = cov.LastIngestAt.UTC().Format(time.RFC3339)
	}
	writeJSON(w, out)
}
