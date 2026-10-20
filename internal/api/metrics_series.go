package api

import (
	"net/http"

	"github.com/Anshuman115/heliolytics/internal/middleware"
	"github.com/Anshuman115/heliolytics/internal/store"
)

type seriesDaysResponse struct {
	Days []store.SeriesDay `json:"days"`
}

// series serves the four declared metrics hrv, stress, resp and spo2. There is
// no metric query parameter and no rhr or vo2max stub.
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
