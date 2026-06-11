package middleware

import (
	"log"
	"net/http"
	"time"
)

func RequestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		q := r.URL.RawQuery
		if q != "" {
			q = "?" + q
		}
		line := log.Printf
		if rw.status >= 400 {
			line = func(format string, v ...any) {
				log.Printf("[error] "+format, v...)
			}
		}
		line(
			"%s %s%s %d %s remote=%s ua=%q",
			r.Method, r.URL.Path, q, rw.status, time.Since(start).Round(time.Microsecond), r.RemoteAddr, r.UserAgent(),
		)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}
