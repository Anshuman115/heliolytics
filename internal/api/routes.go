package api

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/Anshuman115/heliolytics/internal/config"
	"github.com/Anshuman115/heliolytics/internal/middleware"
	"github.com/Anshuman115/heliolytics/internal/store"
)

// IngestFunc runs one validated upload. B07 closes over ingest.Run.
type IngestFunc func(ctx context.Context, meta store.SessionMeta, catalog []byte, blobs map[string][]byte) (store.IngestResult, error)

// ProfileFunc replaces the profile and its dependent derived state. B07 closes
// over ingest.UpdateProfile.
type ProfileFunc func(ctx context.Context, profile *store.Profile) error

// Route body caps from C5.
const (
	maxIngestBody  int64 = 16 << 20
	maxProfileBody int64 = 64 << 10
)

// Inclusive day caps from C3.1.
const (
	maxDailyRangeDays  = 366
	maxSampleRangeDays = 31
	defaultRangeDays   = 30
)

// routeEntry is one exact path and method.
type routeEntry struct {
	protected bool
	maxBody   int64
	prepare   middleware.PrepareFunc
	handler   http.Handler
}

// router resolves the exact escaped path and method before any auth or body
// work. It never redirects and never canonicalizes a path.
type router struct {
	routes map[string]map[string]routeEntry
	allow  map[string]string
}

func (rt *router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	outcome := middleware.OutcomeFrom(r)
	path := r.URL.EscapedPath()
	byMethod, known := rt.routes[path]
	if !known {
		outcome.SetReason("not_found")
		middleware.WriteCode(w, http.StatusNotFound, middleware.CodeNotFound)
		return
	}
	entry, ok := byMethod[r.Method]
	if !ok {
		outcome.SetReason("method_not_allowed")
		w.Header().Set("Allow", rt.allow[path])
		middleware.WriteCode(w, http.StatusMethodNotAllowed, middleware.CodeMethodNotAllowed)
		return
	}
	outcome.SetRoute(path)
	entry.handler.ServeHTTP(w, r)
}

// NewMux builds the API handler. It panics when a required dependency is nil:
// an unwired service is a startup failure, never a fake success.
func NewMux(st *store.Store, cfg config.Config, ingestFn IngestFunc, profileFn ProfileFunc) http.Handler {
	if st == nil || ingestFn == nil || profileFn == nil {
		panic("api.NewMux: nil dependency")
	}
	return newMux(st, cfg, ingestFn, profileFn, st)
}

// newMux wires the mux with an explicit replay store so tests can substitute
// one without a database.
func newMux(st *store.Store, cfg config.Config, ingestFn IngestFunc, profileFn ProfileFunc, nonces middleware.NonceStore) http.Handler {
	cfg = withDefaults(cfg)
	now := time.Now

	metrics := &metricsHandler{st: st, now: now}
	ingest := &ingestHandler{run: ingestFn}
	profile := &profileHandler{update: profileFn, now: now}
	version := &versionHandler{commit: optionalString(cfg.Commit), builtAt: optionalString(cfg.BuiltAt)}

	routes := map[string]map[string]routeEntry{
		"/health":  {http.MethodGet: {handler: http.HandlerFunc(health)}},
		"/version": {http.MethodGet: {handler: http.HandlerFunc(version.serve)}},

		"/api/daily-metrics": {http.MethodGet: {protected: true, handler: http.HandlerFunc(metrics.days)}},
		"/api/sleep":         {http.MethodGet: {protected: true, handler: http.HandlerFunc(metrics.sleep)}},
		"/api/activities":    {http.MethodGet: {protected: true, handler: http.HandlerFunc(metrics.activities)}},

		"/api/metrics/hr":          {http.MethodGet: {protected: true, handler: http.HandlerFunc(metrics.hr)}},
		"/api/metrics/temperature": {http.MethodGet: {protected: true, handler: http.HandlerFunc(metrics.temperature)}},
		"/api/metrics/series":      {http.MethodGet: {protected: true, handler: http.HandlerFunc(metrics.series)}},
		"/api/metrics/coverage":    {http.MethodGet: {protected: true, handler: http.HandlerFunc(metrics.coverage)}},

		"/api/profile": {
			http.MethodPut: {protected: true, maxBody: maxProfileBody, prepare: profile.prepare, handler: http.HandlerFunc(profile.put)},
		},
		"/api/ingest": {
			http.MethodPost: {protected: true, maxBody: maxIngestBody, prepare: ingest.prepare, handler: http.HandlerFunc(ingest.serve)},
		},
	}

	guard := middleware.NewGuard(middleware.GuardConfig{
		Keys:     cfg.Keys,
		Policy:   middleware.IPPolicy{TrustProxy: cfg.TrustProxy, Trusted: cfg.TrustedProxyCIDRs},
		IPLimit:  middleware.NewLimiter(cfg.RateLimitPerMin, middleware.MaxIPKeys),
		KeyLimit: middleware.NewLimiter(cfg.RateLimitPerKeyPerMin, len(cfg.Keys.Kids())),
		Slots:    middleware.NewSemaphore(cfg.MaxConcurrentBodyReads),
		Nonces:   nonces,
		Clock:    now,
	})

	allow := make(map[string]string, len(routes))
	for path, byMethod := range routes {
		methods := make([]string, 0, len(byMethod))
		for method, entry := range byMethod {
			methods = append(methods, method)
			if entry.protected {
				entry.handler = guard.Protect(entry.maxBody, entry.prepare, entry.handler)
				byMethod[method] = entry
			}
		}
		sort.Strings(methods)
		allow[path] = strings.Join(methods, ", ")
	}

	var handler http.Handler = &router{routes: routes, allow: allow}
	handler = middleware.Recover(nil)(handler)
	handler = middleware.Timeout(cfg.RequestTimeout, handler)
	return middleware.RequestLog(nil)(handler)
}

// withDefaults fills option values a partially constructed Config leaves zero.
// Production configuration always sets them explicitly.
func withDefaults(cfg config.Config) config.Config {
	if cfg.RateLimitPerMin <= 0 {
		cfg.RateLimitPerMin = config.DefaultRateLimitPerMin
	}
	if cfg.RateLimitPerKeyPerMin <= 0 {
		cfg.RateLimitPerKeyPerMin = config.DefaultRateLimitPerKeyPerMin
	}
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = config.DefaultRequestTimeoutSeconds * time.Second
	}
	if cfg.MaxConcurrentBodyReads <= 0 {
		cfg.MaxConcurrentBodyReads = config.DefaultMaxConcurrentBodyReads
	}
	return cfg
}
