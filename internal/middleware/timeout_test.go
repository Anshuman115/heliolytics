package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTimeoutWritesRequestTimeout(t *testing.T) {
	handler := Timeout(10*time.Millisecond, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/daily-metrics", nil))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
	if code := recordedCode(t, recorder); code != CodeRequestTimeout {
		t.Fatalf("code = %q", code)
	}
}

func TestTimeoutKeepsFastResponse(t *testing.T) {
	handler := Timeout(time.Second, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/api/profile", nil))

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d", recorder.Code)
	}
	if recorder.Body.Len() != 0 {
		t.Fatalf("204 carried a body: %q", recorder.Body.String())
	}
}

func TestTimeoutBlocksLateWrites(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	lateErr := make(chan error, 1)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		_, err := w.Write([]byte("late"))
		lateErr <- err
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/daily-metrics", nil).WithContext(ctx)

	served := make(chan struct{})
	go func() {
		defer close(served)
		serveWithDeadline(recorder, request, handler)
	}()

	cancel()
	<-served

	if err := <-lateErr; !errors.Is(err, http.ErrHandlerTimeout) {
		t.Fatalf("late write error = %v, want http.ErrHandlerTimeout", err)
	}
	if strings.Contains(recorder.Body.String(), "late") {
		t.Fatalf("late bytes reached the client: %q", recorder.Body.String())
	}
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want the timeout response", recorder.Code)
	}
	if code := recordedCode(t, recorder); code != CodeRequestTimeout {
		t.Fatalf("code = %q", code)
	}
}

func TestTimeoutAnswersWhileWorkerRuns(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	started := make(chan struct{})
	release := make(chan struct{}, 1)
	defer func() { release <- struct{}{} }()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		_, _ = w.Write([]byte("late"))
	})

	recorder := &signallingRecorder{ResponseRecorder: httptest.NewRecorder(), status: make(chan int, 1)}
	request := httptest.NewRequest(http.MethodGet, "/api/daily-metrics", nil).WithContext(ctx)

	served := make(chan struct{})
	go func() {
		defer close(served)
		serveWithDeadline(recorder, request, handler)
	}()

	<-started
	cancel()
	select {
	case code := <-recorder.status:
		if code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the coordinator waited for the worker before answering")
	}
	release <- struct{}{}
	<-served
}

type signallingRecorder struct {
	*httptest.ResponseRecorder
	status chan int
}

func (s *signallingRecorder) WriteHeader(code int) {
	select {
	case s.status <- code:
	default:
	}
	s.ResponseRecorder.WriteHeader(code)
}

func TestTimeoutKeepsCommittedStatus(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	committed := make(chan struct{})
	returned := make(chan struct{})
	lateErr := make(chan error, 1)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(returned)
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("early")); err != nil {
			t.Errorf("early write failed: %v", err)
		}
		close(committed)
		<-r.Context().Done()
		_, err := w.Write([]byte("late"))
		lateErr <- err
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/daily-metrics", nil).WithContext(ctx)

	served := make(chan struct{})
	go func() {
		defer close(served)
		serveWithDeadline(recorder, request, handler)
	}()

	<-committed
	cancel()
	<-served

	select {
	case <-returned:
	default:
		t.Fatal("the coordinator returned before the handler stopped")
	}
	if err := <-lateErr; !errors.Is(err, http.ErrHandlerTimeout) {
		t.Fatalf("late write error = %v, want http.ErrHandlerTimeout", err)
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want the committed 200", recorder.Code)
	}
	if body := recorder.Body.String(); body != "early" {
		t.Fatalf("body = %q, want the committed bytes only", body)
	}
}
