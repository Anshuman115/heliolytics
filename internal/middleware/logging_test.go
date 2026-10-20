package middleware

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestLogWritesOneAccessLine(t *testing.T) {
	var buffer bytes.Buffer
	logger := log.New(&buffer, "", 0)
	handler := RequestLog(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		outcome := OutcomeFrom(r)
		outcome.SetRoute("/api/v1/sleep")
		outcome.SetKid("web")
		w.WriteHeader(http.StatusNoContent)
	}))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/sleep?from=2026-10-01", nil))

	line := buffer.String()
	if strings.Count(line, "access ") != 1 {
		t.Fatalf("access lines = %d, want 1: %q", strings.Count(line, "access "), line)
	}
	for _, want := range []string{"method=GET", "route=/api/v1/sleep", "status=204", "kid=web"} {
		if !strings.Contains(line, want) {
			t.Fatalf("line %q missing %q", line, want)
		}
	}
	if strings.Contains(line, "from=2026-10-01") {
		t.Fatalf("query string leaked into the log: %q", line)
	}
}

func TestRequestLogRecordsReasonAndUnknownRoute(t *testing.T) {
	var buffer bytes.Buffer
	logger := log.New(&buffer, "", 0)
	handler := RequestLog(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		OutcomeFrom(r).SetReason("not_found")
		WriteCode(w, http.StatusNotFound, CodeNotFound)
	}))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/nope", nil))

	line := buffer.String()
	for _, want := range []string{"status=404", "reason=not_found", "route=-", "kid=-"} {
		if !strings.Contains(line, want) {
			t.Fatalf("line %q missing %q", line, want)
		}
	}
}

func TestStatusWriterKeepsFirstStatusAndCountsBytes(t *testing.T) {
	recorder := httptest.NewRecorder()
	writer := &statusWriter{ResponseWriter: recorder}
	writer.WriteHeader(http.StatusCreated)
	writer.WriteHeader(http.StatusTeapot)
	if _, err := writer.Write([]byte("hello")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	status, bytes := writer.State()
	if status != http.StatusCreated {
		t.Fatalf("status = %d, want 201", status)
	}
	if bytes != 5 {
		t.Fatalf("bytes = %d, want 5", bytes)
	}
	if !writer.HeaderWritten() {
		t.Fatal("HeaderWritten false after a commit")
	}
	if writer.Unwrap() != recorder {
		t.Fatal("Unwrap did not return the wrapped writer")
	}
}

func TestStatusWriterImplicitWriteRecordsOK(t *testing.T) {
	writer := &statusWriter{ResponseWriter: httptest.NewRecorder()}
	if _, err := writer.Write([]byte("x")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if status, _ := writer.State(); status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
}
