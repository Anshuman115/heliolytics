package middleware

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"
)

func Timeout(d time.Duration, next http.Handler) http.Handler {
	if d <= 0 {
		d = 90 * time.Second
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), d)
		defer cancel()
		serveWithDeadline(w, r.WithContext(ctx), next)
	})
}

func serveWithDeadline(w http.ResponseWriter, r *http.Request, next http.Handler) {
	writer := &timeoutWriter{writer: w, ctx: r.Context(), header: make(http.Header)}
	done := make(chan struct{})
	panicked := make(chan any, 1)
	go func() {
		defer close(done)
		defer func() { panicked <- recover() }()
		next.ServeHTTP(writer, r)
	}()

	select {
	case <-done:
	case <-r.Context().Done():
		writer.finish()
	}
	// net/http forbids using the response writer after ServeHTTP returns; wait for the worker.
	<-done
	writer.finish()
	if value := <-panicked; value != nil {
		if value == http.ErrAbortHandler {
			panic(value)
		}
		log.Printf("panic outside recovery: %v", value)
	}
}

type timeoutWriter struct {
	writer http.ResponseWriter
	ctx    context.Context
	header http.Header

	mu       sync.Mutex
	timedOut bool
	written  bool
}

func (t *timeoutWriter) HeaderWritten() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.written
}

func (t *timeoutWriter) Unwrap() http.ResponseWriter { return t.writer }

func (t *timeoutWriter) Header() http.Header { return t.header }

func (t *timeoutWriter) WriteHeader(code int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.written || t.blocked() {
		return
	}
	t.written = true
	t.commitHeader()
	t.writer.WriteHeader(code)
}

func (t *timeoutWriter) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.blocked() {
		return 0, http.ErrHandlerTimeout
	}
	if !t.written {
		t.written = true
		t.commitHeader()
	}
	return t.writer.Write(p)
}

func (t *timeoutWriter) blocked() bool {
	return t.timedOut || t.ctx.Err() != nil
}

func (t *timeoutWriter) commitHeader() {
	dst := t.writer.Header()
	for key, values := range t.header {
		dst[key] = append([]string(nil), values...)
	}
}

func (t *timeoutWriter) finish() {
	t.mu.Lock()
	defer t.mu.Unlock()
	switch {
	case t.written:
		t.timedOut = true
	case t.ctx.Err() != nil:
		t.timedOut = true
		t.written = true
		WriteCode(t.writer, http.StatusServiceUnavailable, CodeRequestTimeout)
		_ = http.NewResponseController(t.writer).Flush()
	}
}
