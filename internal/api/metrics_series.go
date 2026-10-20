package api

import (
	"net/http"

	"github.com/Anshuman115/heliolytics/internal/middleware"
	"github.com/Anshuman115/heliolytics/internal/store"
)

type seriesDaysResponse struct {
	Days []store.SeriesDay `json:"days"`
}

func (h *metricsHandler) series(w http.ResponseWriter, r *http.Request) {
	from, to, failure := h.rangeBounds(r, maxSampleRangeDays)
	if failure != nil {
		middleware.WriteError(w, failure)
		return
	}
	days, err := h.st.ListHealthSamplesCompact(r.Context(), from, to)
	if err != nil {
		writeStoreFailure(w, "series", err)
		return
	}
	for i := range days {
		normalizeCompact(&days[i].CompactDay)
	}
	writeJSON(w, http.StatusOK, seriesDaysResponse{Days: days})
}
