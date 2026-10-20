package middleware

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecoverAnswersJSON500BeforeHeaders(t *testing.T) {
	var buffer bytes.Buffer
	logger := log.New(&buffer, "", 0)
	handler := Recover(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	}))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/sleep", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", recorder.Code)
	}
	if code := recordedCode(t, recorder); code != CodeInternal {
		t.Fatalf("code = %q", code)
	}
	if strings.Contains(recorder.Body.String(), "boom") {
		t.Fatalf("panic value leaked to the client: %s", recorder.Body.String())
	}
	if strings.Count(buffer.String(), "panic method=") != 1 {
		t.Fatalf("panic logged %d times: %q", strings.Count(buffer.String(), "panic method="), buffer.String())
	}
	if !strings.Contains(buffer.String(), "goroutine") {
		t.Fatal("stack trace missing from the server log")
	}
}

func TestRecoverKeepsCommittedStatusAndAborts(t *testing.T) {
	handler := Recover(log.New(io.Discard, "", 0))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("partial"))
		panic("late failure")
	}))

	recorder := httptest.NewRecorder()
	func() {
		defer func() {
			if recovered := recover(); recovered != http.ErrAbortHandler {
				t.Errorf("recovered %v, want http.ErrAbortHandler", recovered)
			}
		}()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/sleep", nil))
	}()

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want the committed 200", recorder.Code)
	}
	if body := recorder.Body.String(); body != "partial" {
		t.Fatalf("body = %q, want the original partial response", body)
	}
}

func TestRecoverPreservesAbortHandler(t *testing.T) {
	handler := Recover(log.New(io.Discard, "", 0))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic(http.ErrAbortHandler)
	}))

	recorder := httptest.NewRecorder()
	func() {
		defer func() {
			if recovered := recover(); recovered != http.ErrAbortHandler {
				t.Errorf("recovered %v, want http.ErrAbortHandler", recovered)
			}
		}()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/sleep", nil))
	}()

	if recorder.Body.Len() != 0 {
		t.Fatalf("aborted handler wrote a body: %q", recorder.Body.String())
	}
}
