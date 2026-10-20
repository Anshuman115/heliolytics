package api

import (
	"net/http"

	"github.com/Anshuman115/heliolytics/internal/middleware"
)

func (h *metricsHandler) temperature(w http.ResponseWriter, r *http.Request) {
	from, to, failure := h.rangeBounds(r, maxSampleRangeDays)
	if failure != nil {
		middleware.WriteError(w, failure)
		return
	}
	days, err := h.st.ListTemperatureCompact(r.Context(), from, to)
	if err != nil {
		writeStoreFailure(w, "temperature", err)
		return
	}
	for i := range days {
		normalizeCompact(&days[i])
	}
	writeJSON(w, http.StatusOK, compactDaysResponse{Days: days})
}
