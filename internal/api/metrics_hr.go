package api

import (
	"net/http"

	"github.com/Anshuman115/heliolytics/internal/middleware"
	"github.com/Anshuman115/heliolytics/internal/store"
)

// compactDaysResponse is the shared {"days": CompactDay[]} envelope.
type compactDaysResponse struct {
	Days []store.CompactDay `json:"days"`
}

// hr serves the compact 0x01 heart rate series.
func (h *metricsHandler) hr(w http.ResponseWriter, r *http.Request) {
	from, to, failure := h.rangeBounds(r, maxSampleRangeDays)
	if failure != nil {
		middleware.WriteError(w, failure)
		return
	}
	days, err := h.st.ListHeartRateSamplesCompact(r.Context(), from, to)
	if err != nil {
		writeStoreFailure(w, "hr", err)
		return
	}
	for i := range days {
		normalizeCompact(&days[i])
	}
	writeJSON(w, http.StatusOK, compactDaysResponse{Days: days})
}
