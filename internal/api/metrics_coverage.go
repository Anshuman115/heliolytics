package api

import (
	"net/http"

	"github.com/Anshuman115/heliolytics/internal/store"
)

type coverageResponse struct {
	HasData      bool               `json:"hasData"`
	LastIngestAt *string            `json:"lastIngestAt"`
	DataThrough  *string            `json:"dataThrough"`
	Types        map[string]*string `json:"types"`
}

func (h *metricsHandler) coverage(w http.ResponseWriter, r *http.Request) {
	cov, err := h.st.GetCoverage(r.Context())
	if err != nil {
		writeStoreFailure(w, "coverage", err)
		return
	}
	out := coverageResponse{
		HasData:      cov.HasData,
		LastIngestAt: formatOptional(cov.LastIngestAt),
		DataThrough:  formatOptional(cov.DataThrough),
		Types:        make(map[string]*string, len(store.MeasuredTypeCodes)),
	}
	for _, code := range store.MeasuredTypeCodes {
		out.Types[code] = formatOptional(cov.Types[code])
	}
	writeJSON(w, http.StatusOK, out)
}
