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
