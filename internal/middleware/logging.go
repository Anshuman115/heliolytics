package middleware

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"
)

// Outcome is the per-request state shared between the worker goroutine and the
// outer access logger. Every field is mutex-guarded because a timed-out
// request keeps working after the log line is written.
type Outcome struct {
	mu     sync.Mutex
	route  string
	kid    string
	reason string
	client string
}

func (o *Outcome) SetRoute(route string) {
	o.mu.Lock()
	o.route = route
	o.mu.Unlock()
}

func (o *Outcome) SetKid(kid string) {
	o.mu.Lock()
	o.kid = kid
	o.mu.Unlock()
}

func (o *Outcome) SetReason(reason string) {
	o.mu.Lock()
	o.reason = reason
	o.mu.Unlock()
}

func (o *Outcome) SetClient(client string) {
	o.mu.Lock()
	o.client = client
	o.mu.Unlock()
}

func (o *Outcome) snapshot() (route, kid, reason, client string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.route, o.kid, o.reason, o.client
}

type outcomeKey struct{}

func withOutcome(ctx context.Context, o *Outcome) context.Context {
	return context.WithValue(ctx, outcomeKey{}, o)
}

// OutcomeFrom returns the per-request outcome. It never returns nil so a
// handler used outside RequestLog still works.
func OutcomeFrom(r *http.Request) *Outcome {
	if o, ok := r.Context().Value(outcomeKey{}).(*Outcome); ok {
		return o
	}
	return &Outcome{}
}

// RequestLog writes exactly one access line per request. It logs the resolved
// route, the final status, the duration and safe identity fields, never the
// query string, headers, body or user agent.
func RequestLog(logger *log.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = log.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			outcome := &Outcome{}
			start := time.Now()
			recorder := &statusWriter{ResponseWriter: w}
			next.ServeHTTP(recorder, r.WithContext(withOutcome(r.Context(), outcome)))

			status, bytes := recorder.State()
			if status == 0 {
				status = http.StatusOK
			}
			route, kid, reason, client := outcome.snapshot()
			if route == "" {
				route = "-"
			}
			if kid == "" {
				kid = "-"
			}
			if reason == "" {
				reason = "-"
			}
			if client == "" {
				client = "-"
			}
			logger.Printf(
				"access method=%s route=%s status=%d bytes=%d duration=%s client=%s kid=%s reason=%s",
				r.Method, route, status, bytes, time.Since(start).Round(time.Microsecond), client, kid, reason,
			)
		})
	}
}

// statusWriter records the first status, the byte count and whether a
// response was committed.
type statusWriter struct {
	http.ResponseWriter
	mu      sync.Mutex
	status  int
	bytes   int64
	written bool
}

// HeaderWritten reports whether the response has been committed.
func (w *statusWriter) HeaderWritten() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.written
}

// Unwrap exposes the underlying writer to http.ResponseController.
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *statusWriter) WriteHeader(code int) {
	w.mu.Lock()
	if w.written {
		w.mu.Unlock()
		return
	}
	w.written = true
	w.status = code
	w.mu.Unlock()
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	if !w.written {
		w.written = true
		w.status = http.StatusOK
	}
	w.mu.Unlock()
	n, err := w.ResponseWriter.Write(p)
	w.mu.Lock()
	w.bytes += int64(n)
	w.mu.Unlock()
	return n, err
}

// State returns the recorded status and byte count.
func (w *statusWriter) State() (status int, bytes int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.status, w.bytes
}
