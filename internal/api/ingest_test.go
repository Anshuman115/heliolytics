package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"github.com/Anshuman115/heliolytics/internal/auth"
	"github.com/Anshuman115/heliolytics/internal/parse"
	"github.com/Anshuman115/heliolytics/internal/store"
)

const testBoundary = "helio-test"

// part describes one multipart part.
type part struct {
	name     string
	filename string
	body     string
	headers  map[string]string
}

// buildUpload serializes parts with a fixed boundary and returns the exact
// bytes plus the Content-Type that must be signed with them.
func buildUpload(t *testing.T, parts []part) ([]byte, string) {
	t.Helper()
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	if err := writer.SetBoundary(testBoundary); err != nil {
		t.Fatalf("SetBoundary: %v", err)
	}
	for _, item := range parts {
		header := textproto.MIMEHeader{}
		disposition := fmt.Sprintf(`form-data; name=%q`, item.name)
		if item.filename != "" {
			disposition += fmt.Sprintf(`; filename=%q`, item.filename)
		}
		header.Set("Content-Disposition", disposition)
		for key, value := range item.headers {
			header.Set(key, value)
		}
		partWriter, err := writer.CreatePart(header)
		if err != nil {
			t.Fatalf("CreatePart: %v", err)
		}
		if _, err := partWriter.Write([]byte(item.body)); err != nil {
			t.Fatalf("part write: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return buffer.Bytes(), "multipart/form-data; boundary=" + testBoundary
}

const validSessionJSON = `{"sessionId":"session-1","startedAt":"2026-10-01T00:00:00Z",` +
	`"endedAt":"2026-10-01T01:00:00Z","deviceMac":"02:00:00:00:00:01","batteryPercent":80}`

const validCatalogJSON = `{"schemaVersion":2,"sessionId":"session-1","chunked":[]}`

// validUpload is the smallest accepted envelope: session, catalog and one
// non-empty raw part.
func validUpload(t *testing.T) ([]byte, string) {
	t.Helper()
	return buildUpload(t, []part{
		{name: "session", filename: "session.json", body: validSessionJSON},
		{name: "catalog", filename: "catalog.json", body: validCatalogJSON},
		{name: "0x01_raw.bin", filename: "0x01_raw.bin", body: "raw-bytes"},
	})
}

// ingestMux builds a mux with offline storage and a recording service.
func ingestMux(t *testing.T, keys auth.KeySet, nonces *fakeNonces, ingest *stubIngest) http.Handler {
	t.Helper()
	return newMux(offlineStore(t), testConfig(keys), ingest.run, (&stubProfile{}).update, nonces)
}

// postIngest sends one signed multipart upload.
func postIngest(t *testing.T, handler http.Handler, keys auth.KeySet, body []byte, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ingest", bytes.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	signRequest(t, keys, "phone", req, body)
	return serve(handler, req)
}

func TestIngestAcceptsValidUpload(t *testing.T) {
	keys := testKeys()
	nonces := newFakeNonces()
	ingest := &stubIngest{result: store.IngestResult{
		SessionID: "session-1",
		Complete:  true,
		Types: map[string]store.TypeResult{
			"0x01": {Status: "accepted", Rows: 3},
		},
	}}
	handler := ingestMux(t, keys, nonces, ingest)
	body, contentType := validUpload(t)

	recorder := postIngest(t, handler, keys, body, contentType)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	if ingest.calls != 1 {
		t.Fatalf("service calls = %d, want 1", ingest.calls)
	}
	if ingest.meta.ID != "session-1" || ingest.meta.DeviceMAC != "02:00:00:00:00:01" {
		t.Fatalf("meta = %+v", ingest.meta)
	}
	if ingest.meta.BatteryPct == nil || *ingest.meta.BatteryPct != 80 {
		t.Fatalf("battery = %v", ingest.meta.BatteryPct)
	}
	if ingest.meta.StartedAt.IsZero() || ingest.meta.EndedAt.IsZero() {
		t.Fatalf("timestamps not parsed: %+v", ingest.meta)
	}
	if string(ingest.blobs["0x01"]) != "raw-bytes" {
		t.Fatalf("blob = %q", ingest.blobs["0x01"])
	}
	if string(ingest.catalog) != validCatalogJSON {
		t.Fatalf("catalog = %q", ingest.catalog)
	}

	var body2 struct {
		SessionID string                     `json:"sessionId"`
		Complete  bool                       `json:"complete"`
		Types     map[string]json.RawMessage `json:"types"`
		Warnings  []string                   `json:"warnings"`
	}
	decodeJSON(t, recorder, &body2)
	if body2.SessionID != "session-1" || !body2.Complete {
		t.Fatalf("response = %+v", body2)
	}
	if body2.Warnings == nil {
		t.Fatal("warnings serialized as null")
	}
}

func TestIngestNormalizesEmptyWarnings(t *testing.T) {
	keys := testKeys()
	ingest := &stubIngest{result: store.IngestResult{
		SessionID: "session-1",
		Complete:  true,
		Types: map[string]store.TypeResult{
			"0x26": {Status: "raw_only", Rows: 0, Dropped: 13, Warnings: nil},
		},
		Warnings: nil,
	}}
	handler := ingestMux(t, keys, newFakeNonces(), ingest)
	body, contentType := validUpload(t)
	recorder := postIngest(t, handler, keys, body, contentType)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	var response struct {
		Warnings []string `json:"warnings"`
		Types    map[string]struct {
			Warnings    []string `json:"warnings"`
			DataThrough *string  `json:"dataThrough"`
		} `json:"types"`
	}
	decodeJSON(t, recorder, &response)
	if response.Warnings == nil {
		t.Fatalf("warnings serialized as null: %s", recorder.Body.String())
	}
	entry, ok := response.Types["0x26"]
	if !ok {
		t.Fatalf("0x26 missing from the response: %s", recorder.Body.String())
	}
	if entry.Warnings == nil {
		t.Fatalf("type warnings serialized as null: %s", recorder.Body.String())
	}
	if entry.DataThrough != nil {
		t.Fatalf("dataThrough = %v, want null for raw_only", *entry.DataThrough)
	}
}

func TestIngestStructuralFailures(t *testing.T) {
	keys := testKeys()
	session := part{name: "session", filename: "session.json", body: validSessionJSON}
	catalog := part{name: "catalog", filename: "catalog.json", body: validCatalogJSON}
	raw := part{name: "0x01_raw.bin", filename: "0x01_raw.bin", body: "raw-bytes"}

	manyParts := make([]part, 0, 41)
	manyParts = append(manyParts, session, catalog)
	accepted := []string{"0x01", "0x05", "0x06", "0x0D", "0x13", "0x25", "0x26", "0x2E", "0x38", "0x46", "0x48", "0x49"}
	for i := range 39 {
		manyParts = append(manyParts, part{name: accepted[i%len(accepted)] + "_raw.bin", body: "x"})
	}

	cases := []struct {
		name   string
		parts  []part
		codes  []string
		status int
	}{
		{"missing catalog", []part{session, raw}, []string{"invalid_body"}, http.StatusBadRequest},
		{"missing session", []part{catalog, raw}, []string{"invalid_body"}, http.StatusBadRequest},
		{"duplicate session", []part{session, session, catalog, raw}, []string{"invalid_body"}, http.StatusBadRequest},
		{"duplicate code", []part{session, catalog, raw, raw}, []string{"invalid_body"}, http.StatusBadRequest},
		{"unknown part", []part{session, catalog, raw, {name: "probe", body: "x"}},
			[]string{"unknown_part"}, http.StatusBadRequest},
		{"retired type", []part{session, catalog, raw, {name: "0x39_raw.bin", body: "x"}},
			[]string{"unknown_part"}, http.StatusBadRequest},
		{"lowercase type", []part{session, catalog, {name: "0x0d_raw.bin", body: "x"}},
			[]string{"unknown_part"}, http.StatusBadRequest},
		{"no raw files", []part{session, catalog}, []string{"empty_upload"}, http.StatusBadRequest},
		{"empty raw file", []part{session, catalog, {name: "0x01_raw.bin", body: ""}},
			[]string{"empty_upload"}, http.StatusBadRequest},
		{"nested multipart", []part{session, catalog, raw, {
			name: "0x46_raw.bin", body: "x",
			headers: map[string]string{"Content-Type": "multipart/mixed; boundary=nested"},
		}}, []string{"invalid_body"}, http.StatusBadRequest},
		{"encoded part", []part{session, catalog, raw, {
			name: "0x46_raw.bin", body: "x",
			headers: map[string]string{"Content-Transfer-Encoding": "base64"},
		}}, []string{"invalid_body"}, http.StatusBadRequest},
		{"forty one parts", manyParts, []string{"invalid_body"}, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			nonces := newFakeNonces()
			ingest := &stubIngest{}
			handler := ingestMux(t, keys, nonces, ingest)
			body, contentType := buildUpload(t, tc.parts)
			recorder := postIngest(t, handler, keys, body, contentType)
			if recorder.Code != tc.status {
				t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
			}
			if code := errorCode(t, recorder); code != tc.codes[0] {
				t.Fatalf("code = %q, want %q", code, tc.codes[0])
			}
			if ingest.calls != 0 {
				t.Fatalf("service called %d times", ingest.calls)
			}
			if nonces.consumeCount() != 0 {
				t.Fatalf("structural failure consumed a nonce")
			}
		})
	}
}

func TestIngestSessionSemantics(t *testing.T) {
	keys := testKeys()
	cases := map[string]string{
		"garbage endedAt": `{"sessionId":"s-1","startedAt":"2026-10-01T00:00:00Z",` +
			`"endedAt":"tomorrow","deviceMac":"02:00:00:00:00:01"}`,
		"ended before started": `{"sessionId":"s-1","startedAt":"2026-10-01T02:00:00Z",` +
			`"endedAt":"2026-10-01T01:00:00Z","deviceMac":"02:00:00:00:00:01"}`,
		"naive timestamp": `{"sessionId":"s-1","startedAt":"2026-10-01T00:00:00",` +
			`"endedAt":"2026-10-01T01:00:00Z","deviceMac":"02:00:00:00:00:01"}`,
		"lowercase mac": `{"sessionId":"s-1","startedAt":"2026-10-01T00:00:00Z",` +
			`"endedAt":"2026-10-01T01:00:00Z","deviceMac":"02:00:00:00:00:0a"}`,
		"battery high": `{"sessionId":"s-1","startedAt":"2026-10-01T00:00:00Z",` +
			`"endedAt":"2026-10-01T01:00:00Z","deviceMac":"02:00:00:00:00:01","batteryPercent":101}`,
		"battery fraction": `{"sessionId":"s-1","startedAt":"2026-10-01T00:00:00Z",` +
			`"endedAt":"2026-10-01T01:00:00Z","deviceMac":"02:00:00:00:00:01","batteryPercent":50.5}`,
		"empty session id": `{"sessionId":"","startedAt":"2026-10-01T00:00:00Z",` +
			`"endedAt":"2026-10-01T01:00:00Z","deviceMac":"02:00:00:00:00:01"}`,
		"long session id": `{"sessionId":"` + strings.Repeat("s", 65) + `","startedAt":"2026-10-01T00:00:00Z",` +
			`"endedAt":"2026-10-01T01:00:00Z","deviceMac":"02:00:00:00:00:01"}`,
	}
	for name, sessionJSON := range cases {
		t.Run(name, func(t *testing.T) {
			nonces := newFakeNonces()
			ingest := &stubIngest{}
			handler := ingestMux(t, keys, nonces, ingest)
			body, contentType := buildUpload(t, []part{
				{name: "session", filename: "session.json", body: sessionJSON},
				{name: "catalog", filename: "catalog.json", body: validCatalogJSON},
				{name: "0x01_raw.bin", filename: "0x01_raw.bin", body: "raw-bytes"},
			})
			recorder := postIngest(t, handler, keys, body, contentType)
			if recorder.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
			}
			if code := errorCode(t, recorder); code != "invalid_session" {
				t.Fatalf("code = %q", code)
			}
			if ingest.calls != 0 {
				t.Fatalf("service called %d times", ingest.calls)
			}
			if nonces.consumeCount() != 1 {
				t.Fatalf("semantic failure consumed %d nonces, want 1", nonces.consumeCount())
			}
		})
	}
}

func TestIngestMalformedSessionJSONIs400(t *testing.T) {
	keys := testKeys()
	cases := map[string]string{
		"not json":       `{`,
		"missing ended":  `{"sessionId":"s-1","startedAt":"2026-10-01T00:00:00Z","deviceMac":"02:00:00:00:00:01"}`,
		"unknown field":  `{"sessionId":"s-1","startedAt":"2026-10-01T00:00:00Z","endedAt":"2026-10-01T01:00:00Z","deviceMac":"02:00:00:00:00:01","extra":1}`,
		"wrong id type":  `{"sessionId":7,"startedAt":"2026-10-01T00:00:00Z","endedAt":"2026-10-01T01:00:00Z","deviceMac":"02:00:00:00:00:01"}`,
		"battery string": `{"sessionId":"s-1","startedAt":"2026-10-01T00:00:00Z","endedAt":"2026-10-01T01:00:00Z","deviceMac":"02:00:00:00:00:01","batteryPercent":"80"}`,
	}
	for name, sessionJSON := range cases {
		t.Run(name, func(t *testing.T) {
			nonces := newFakeNonces()
			ingest := &stubIngest{}
			handler := ingestMux(t, keys, nonces, ingest)
			body, contentType := buildUpload(t, []part{
				{name: "session", body: sessionJSON},
				{name: "catalog", body: validCatalogJSON},
				{name: "0x01_raw.bin", body: "raw-bytes"},
			})
			recorder := postIngest(t, handler, keys, body, contentType)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
			}
			if code := errorCode(t, recorder); code != "invalid_body" {
				t.Fatalf("code = %q", code)
			}
			if ingest.calls != 0 || nonces.consumeCount() != 0 {
				t.Fatalf("malformed session reached the service or the nonce")
			}
		})
	}
}

func TestIngestServiceErrorMapping(t *testing.T) {
	keys := testKeys()
	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"session conflict", store.ErrSessionConflict, http.StatusConflict, "session_conflict"},
		{"invalid catalog", &parse.ParseError{Code: parse.ErrCodeInvalidCatalog},
			http.StatusUnprocessableEntity, "invalid_catalog"},
		{"missing anchor", &parse.ParseError{Code: parse.ErrCodeMissingAnchor},
			http.StatusUnprocessableEntity, "invalid_catalog"},
		{"invalid window", &parse.ParseError{Code: parse.ErrCodeInvalidWindow},
			http.StatusUnprocessableEntity, "invalid_session"},
		{"invalid framing", &parse.ParseError{Code: parse.ErrCodeInvalidFraming, TypeCode: "0x01"},
			http.StatusUnprocessableEntity, "invalid_type"},
		{"unsupported version", &parse.ParseError{Code: parse.ErrCodeUnsupportedVersion},
			http.StatusUnprocessableEntity, "invalid_type"},
		{"invalid row", &store.InvalidRowError{Table: "sync_sessions"},
			http.StatusUnprocessableEntity, "invalid_session"},
		{"deadline", context.DeadlineExceeded, http.StatusServiceUnavailable, "request_timeout"},
		{"other", errors.New("boom"), http.StatusInternalServerError, "internal"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ingest := &stubIngest{err: tc.err}
			handler := ingestMux(t, keys, newFakeNonces(), ingest)
			body, contentType := validUpload(t)
			recorder := postIngest(t, handler, keys, body, contentType)
			if recorder.Code != tc.status {
				t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
			}
			if code := errorCode(t, recorder); code != tc.code {
				t.Fatalf("code = %q, want %q", code, tc.code)
			}
		})
	}
}

func TestIngestCatalogCrossCheck(t *testing.T) {
	keys := testKeys()
	cases := map[string]string{
		"session mismatch": `{"sessionId":"other-session","chunked":[]}`,
		"duplicate code":   `{"sessionId":"session-1","chunked":[{"code":"0x01"},{"code":"0x01"}]}`,
	}
	for name, catalogJSON := range cases {
		t.Run(name, func(t *testing.T) {
			nonces := newFakeNonces()
			ingest := &stubIngest{}
			handler := ingestMux(t, keys, nonces, ingest)
			body, contentType := buildUpload(t, []part{
				{name: "session", body: validSessionJSON},
				{name: "catalog", body: catalogJSON},
				{name: "0x01_raw.bin", body: "raw-bytes"},
			})
			recorder := postIngest(t, handler, keys, body, contentType)
			if recorder.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
			}
			if code := errorCode(t, recorder); code != "invalid_catalog" {
				t.Fatalf("code = %q", code)
			}
			if ingest.calls != 0 {
				t.Fatalf("service called %d times", ingest.calls)
			}
		})
	}
}

func TestIngestOversizeIs413(t *testing.T) {
	keys := testKeys()
	nonces := newFakeNonces()
	ingest := &stubIngest{}
	handler := ingestMux(t, keys, nonces, ingest)
	body := bytes.Repeat([]byte("a"), int(maxIngestBody)+1)

	recorder := postIngest(t, handler, keys, body, "multipart/form-data; boundary="+testBoundary)
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d", recorder.Code)
	}
	if code := errorCode(t, recorder); code != "payload_too_large" {
		t.Fatalf("code = %q", code)
	}
	if ingest.calls != 0 || nonces.consumeCount() != 0 {
		t.Fatal("oversize request reached the service or the nonce")
	}
}
