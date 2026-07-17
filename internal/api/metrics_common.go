package api

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/heliolytics/api/internal/store"
)

var ist = time.FixedZone("IST", 5*3600+30*60)

type metricsHandler struct {
	st *store.Store
}

func (h *metricsHandler) health(w http.ResponseWriter, r *http.Request) {
	if err := h.st.Ping(r.Context()); err != nil {
		http.Error(w, "unhealthy", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func dayRange(r *http.Request) (string, string) {
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	if from != "" && to != "" {
		return from, to
	}
	now := time.Now().In(ist)
	return now.AddDate(0, 0, -30).Format("2006-01-02"), now.Format("2006-01-02")
}

func workoutDayRange(r *http.Request) (string, string) {
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	if from != "" && to != "" {
		return from, to
	}
	now := time.Now().In(ist)
	return now.AddDate(0, 0, -90).Format("2006-01-02"), now.Format("2006-01-02")
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func logMetrics(op string, r *http.Request, detail string) {
	q := r.URL.RawQuery
	if q != "" {
		q = "?" + q
	}
	log.Printf("metrics %s path=%s%s remote=%s %s", op, r.URL.Path, q, r.RemoteAddr, detail)
}

// strOrDash renders an optional string for logs. Logging the pointer itself
// prints an address, which looks like data and is not.
func strOrDash(s *string) string {
	if s == nil || *s == "" {
		return "-"
	}
	return *s
}

// trendPoint is the shared response shape for single-value trend endpoints
// (hrv, rhr, vo2max stub).
type trendPoint struct {
	SampledAt string  `json:"sampledAt"`
	Value     float64 `json:"value"`
}

func toTrendPoints(samples []store.SampleValue) []trendPoint {
	out := make([]trendPoint, 0, len(samples))
	for _, s := range samples {
		out = append(out, trendPoint{SampledAt: s.SampledAt.Format(time.RFC3339), Value: s.Value})
	}
	return out
}
