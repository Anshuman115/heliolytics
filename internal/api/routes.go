package api

import (
	"net/http"

	"github.com/heliolytics/api/internal/config"
	"github.com/heliolytics/api/internal/middleware"
	"github.com/heliolytics/api/internal/store"
)

func NewMux(st *store.Store, cfg config.Config) http.Handler {
	root := http.NewServeMux()
	apiMux := http.NewServeMux()
	mh := &metricsHandler{st: st}
	ih := &ingestHandler{st: st}
	rh := &reparseHandler{
		st:      st,
		enabled: cfg.ReparseEnabled,
		secret:  cfg.ReparseSecret,
	}

	root.HandleFunc("/health", mh.health)
	apiMux.HandleFunc("/api/v1/metrics/days", mh.days)
	apiMux.HandleFunc("/api/v1/metrics/sleep", mh.sleep)
	apiMux.HandleFunc("/api/v1/metrics/temperature", mh.temperature)
	apiMux.HandleFunc("/api/v1/metrics/series", mh.series)
	apiMux.HandleFunc("/api/v1/metrics/workouts", mh.workouts)
	apiMux.HandleFunc("/api/v1/metrics/activity-sessions", mh.activitySessions)
	apiMux.HandleFunc("/api/v1/metrics/coverage", mh.coverage)
	apiMux.HandleFunc("/api/v1/ingest", ih.serve)
	apiMux.HandleFunc("/api/v1/reparse", rh.serve)

	rl := middleware.NewRateLimiter(cfg.RateLimitPerMin, cfg.TrustProxy)
	chain := middleware.RequestLog(
		middleware.RateLimit(rl)(
			middleware.HMACToken(cfg.SigningSecret)(apiMux),
		),
	)
	root.Handle("/", chain)
	return root
}
