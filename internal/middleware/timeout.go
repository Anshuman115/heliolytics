package middleware

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"
)

// Timeout runs the handler under a real deadline. A response that has not
// started becomes one 503 request_timeout and every later write from the
// worker fails instead of reaching the client.
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

// serveWithDeadline runs next in its own goroutine and coordinates the
// response with the deadline carried by r. Tests call it with a context they
// cancel, so the commit, the deadline and the late write need no sleeps.
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
	// net/http forbids touching the response writer after ServeHTTP returns, so
	// the worker must be gone before the coordinator returns. The timeout
	// response was already written and flushed above.
	<-done
	writer.finish()
	if value := <-panicked; value != nil {
		if value == http.ErrAbortHandler {
			panic(value)
		}
		log.Printf("panic outside recovery: %v", value)
	}
}

// timeoutWriter coordinates one response between the worker and the deadline
// coordinator. Every method holds the mutex across the real writer so the
// timeout response and a worker write can never interleave.
type timeoutWriter struct {
	writer http.ResponseWriter
	ctx    context.Context
	header http.Header

	mu       sync.Mutex
	timedOut bool
	written  bool
}

// HeaderWritten reports whether a response has been committed.
func (t *timeoutWriter) HeaderWritten() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.written
}

// Unwrap exposes the underlying writer to http.ResponseController.
func (t *timeoutWriter) Unwrap() http.ResponseWriter { return t.writer }

// Header returns the response headers before they reach the writer, so a
// worker that outlives the deadline mutates only this copy.
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

// blocked reports whether the deadline passed or the timeout response was
// sent. The context records the deadline, so a write after Done fails even
// before the coordinator observes the expiry and takes the mutex.
func (t *timeoutWriter) blocked() bool {
	return t.timedOut || t.ctx.Err() != nil
}

// commitHeader copies the worker headers to the real writer once, before the
// status is committed.
func (t *timeoutWriter) commitHeader() {
	dst := t.writer.Header()
	for key, values := range t.header {
		dst[key] = append([]string(nil), values...)
	}
}

// finish makes the coordinator's single response decision. A response that
// started before the deadline keeps its status; a worker that sent nothing
// after the deadline gets one 503 request_timeout, and later writes fail.
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
