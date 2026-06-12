package api

import (
	"net/http"
	"time"

	"github.com/heliolytics/api/internal/store"
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
	writeJSON(w, buildCoverageResponse(cov))
}

func buildCoverageResponse(cov store.DataCoverage) map[string]any {
	out := map[string]any{"hasData": cov.HasData}
	if cov.DataThrough != nil {
		out["dataThrough"] = cov.DataThrough.UTC().Format(time.RFC3339)
	}
	if cov.LastIngestAt != nil {
		out["lastIngestAt"] = cov.LastIngestAt.UTC().Format(time.RFC3339)
	}
	if len(cov.Types) > 0 {
		types := map[string]any{}
		for key, ts := range cov.Types {
			if ts != nil {
				types[key] = ts.UTC().Format(time.RFC3339)
			} else {
				types[key] = nil
			}
		}
		out["types"] = types
	}
	return out
}
