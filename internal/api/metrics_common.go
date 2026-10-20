package api

import (
	"encoding/json"
	"errors"
	"log"
	"math"
	"net/http"
	"time"

	"github.com/Anshuman115/heliolytics/internal/middleware"
	"github.com/Anshuman115/heliolytics/internal/store"
)

const dateLayout = "2006-01-02"

type metricsHandler struct {
	st  *store.Store
	now func() time.Time
}

func health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeStoreFailure(w http.ResponseWriter, route string, err error) {
	log.Printf("route=%s store error: %v", route, err)
	middleware.WriteCode(w, http.StatusInternalServerError, middleware.CodeInternal)
}

func (h *metricsHandler) rangeBounds(r *http.Request, maxDays int) (string, string, *middleware.Error) {
	query := r.URL.Query()
	hasFrom := query.Has("from")
	hasTo := query.Has("to")
	if !hasFrom && !hasTo {
		today := istToday(h.now())
		return today.AddDate(0, 0, -(defaultRangeDays - 1)).Format(dateLayout), today.Format(dateLayout), nil
	}
	if hasFrom != hasTo {
		return "", "", middleware.Fail(http.StatusBadRequest, middleware.CodeInvalidDate)
	}
	if len(query["from"]) != 1 || len(query["to"]) != 1 {
		return "", "", middleware.Fail(http.StatusBadRequest, middleware.CodeInvalidDate)
	}
	from, err := parseDay(query.Get("from"))
	if err != nil {
		return "", "", middleware.Fail(http.StatusBadRequest, middleware.CodeInvalidDate)
	}
	to, err := parseDay(query.Get("to"))
	if err != nil {
		return "", "", middleware.Fail(http.StatusBadRequest, middleware.CodeInvalidDate)
	}
	if from.After(to) {
		return "", "", middleware.Fail(http.StatusBadRequest, middleware.CodeInvalidRange)
	}
	if days := int(to.Sub(from)/(24*time.Hour)) + 1; days > maxDays {
		return "", "", middleware.Fail(http.StatusBadRequest, middleware.CodeRangeTooLarge)
	}
	return from.Format(dateLayout), to.Format(dateLayout), nil
}

func istToday(now time.Time) time.Time {
	local := now.In(store.IST)
	return store.Day(local.Year(), local.Month(), local.Day())
}

func parseDay(value string) (time.Time, error) {
	day, err := time.ParseInLocation(dateLayout, value, store.IST)
	if err != nil || day.Format(dateLayout) != value {
		return time.Time{}, errors.New("api: invalid date")
	}
	return day, nil
}

func formatInstant(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func formatOptional(t *time.Time) *string {
	if t == nil {
		return nil
	}
	formatted := formatInstant(*t)
	return &formatted
}

func round1(v *float64) *float64 {
	if v == nil {
		return nil
	}
	rounded := math.Round(*v*10) / 10
	return &rounded
}

func round2(v *float64) *float64 {
	if v == nil {
		return nil
	}
	rounded := math.Round(*v*100) / 100
	return &rounded
}

func normalizeCompact(day *store.CompactDay) {
	if day.Offsets == nil {
		day.Offsets = []int32{}
	}
	if day.Values == nil {
		day.Values = []float64{}
	}
}
