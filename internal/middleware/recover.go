package middleware

import (
	"log"
	"net/http"
	"runtime/debug"
	"sync"
)

// After headers are committed, abort rather than write a second response.
func Recover(logger *log.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = log.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			outcome := OutcomeFrom(r)
			tracked := &recoverWriter{ResponseWriter: w}
			defer func() {
				recovered := recover()
				if recovered == nil {
					return
				}
				if recovered == http.ErrAbortHandler {
					panic(recovered)
				}
				route, kid, _, client := outcome.snapshot()
				if route == "" {
					route = "-"
				}
				if kid == "" {
					kid = "-"
				}
				if client == "" {
					client = "-"
				}
				logger.Printf(
					"panic method=%s route=%s client=%s kid=%s value=%v\n%s",
					r.Method, route, client, kid, recovered, debug.Stack(),
				)
				outcome.SetReason("panic")
				if tracked.HeaderWritten() || HeaderWritten(w) {
					panic(http.ErrAbortHandler)
				}
				WriteCode(tracked, http.StatusInternalServerError, CodeInternal)
			}()
			next.ServeHTTP(tracked, r)
		})
	}
}

type recoverWriter struct {
	http.ResponseWriter
	mu      sync.Mutex
	written bool
}

func (w *recoverWriter) HeaderWritten() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.written
}

func (w *recoverWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *recoverWriter) WriteHeader(code int) {
	w.mu.Lock()
	w.written = true
	w.mu.Unlock()
	w.ResponseWriter.WriteHeader(code)
}

func (w *recoverWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	w.written = true
	w.mu.Unlock()
	return w.ResponseWriter.Write(p)
}
