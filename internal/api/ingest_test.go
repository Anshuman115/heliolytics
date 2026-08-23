package api

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/heliolytics/api/internal/store"
)

func TestIngestRejectsOversizedRequest(t *testing.T) {
	called := false
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("session", "session.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(strings.Repeat("x", 512))); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ingest", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()

	(&ingestHandler{
		maxBytes: 256,
		runIngest: func(context.Context, *store.Store, store.SessionMeta, map[string][]byte, time.Time) error {
			called = true
			return nil
		},
	}).serve(recorder, req)

	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusRequestEntityTooLarge)
	}
	if called {
		t.Fatal("ingest runner called after oversized request")
	}
}

func TestIngestAcceptsCurrentMultipartUpload(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	writeUploadPart(t, writer, "session", "session.json", `{"sessionId":"session-1","startedAt":"2026-08-23T20:00:00Z"}`)
	writeUploadPart(t, writer, "catalog", "catalog.json", `{"types":[]}`)
	writeUploadPart(t, writer, "0x01_raw.bin", "0x01_raw.bin", "raw-bytes")
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	called := false
	handler := &ingestHandler{
		runIngest: func(_ context.Context, _ *store.Store, meta store.SessionMeta, blobs map[string][]byte, _ time.Time) error {
			called = true
			if meta.ID != "session-1" {
				t.Fatalf("session ID = %q", meta.ID)
			}
			if string(blobs["0x01"]) != "raw-bytes" {
				t.Fatalf("raw blob = %q", blobs["0x01"])
			}
			return nil
		},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ingest", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()

	handler.serve(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if !called {
		t.Fatal("ingest runner was not called for a valid upload")
	}
}

func TestIngestRejectsMalformedMultipart(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ingest", strings.NewReader("not multipart"))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=missing")
	recorder := httptest.NewRecorder()

	(&ingestHandler{}).serve(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestIngestRejectsInterruptedPartRead(t *testing.T) {
	boundary := "test-boundary"
	prefix := "--" + boundary + "\r\n" +
		"Content-Disposition: form-data; name=\"session\"; filename=\"session.json\"\r\n" +
		"Content-Type: application/json\r\n\r\n" +
		`{"sessionId":"session-1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ingest", &failingReader{
		data: []byte(prefix),
		err:  errors.New("upload interrupted"),
	})
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)
	recorder := httptest.NewRecorder()

	(&ingestHandler{}).serve(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

type failingReader struct {
	data []byte
	err  error
}

func (r *failingReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, r.err
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}

var _ io.Reader = (*failingReader)(nil)

func writeUploadPart(t *testing.T, writer *multipart.Writer, field, filename, value string) {
	t.Helper()
	part, err := writer.CreateFormFile(field, filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(value)); err != nil {
		t.Fatal(err)
	}
}
