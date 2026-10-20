package integration

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"testing"
	"time"

	"github.com/Anshuman115/heliolytics/internal/auth"
)

type tamperUpload struct {
	session []byte
	catalog []byte
	blobs   map[string][]byte
	body    []byte
	content string
}

func newTamperUpload(t *testing.T) tamperUpload {
	t.Helper()
	started := time.Date(2026, time.September, 23, 1, 0, 0, 0, time.UTC)
	session := sessionPart(t, "signing-2026-09-23", started, started.Add(10*time.Minute))
	catalog := catalogPart(t, catalogEntry{Code: "0x2E", RoundStart: istWallClock(started)})
	blobs := map[string][]byte{"0x2E": temperatureMinute(3200)}
	body, content := multipartBody(t, session, catalog, blobs)
	return tamperUpload{
		session: session, catalog: catalog, blobs: blobs, body: body, content: content,
	}
}

func (u tamperUpload) request(t *testing.T, s *stack, method, path, query string,
	body []byte, content string) *http.Request {
	t.Helper()
	target := "http://api.test" + path
	if query != "" {
		target += "?" + query
	}
	request, err := http.NewRequest(method, target, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if content != "" {
		request.Header.Set("Content-Type", content)
	}
	s.sign(t, phoneKid, request, body, freshNonce(t), time.Now().Unix())
	return request
}

func TestSignedRequestTamperIsRejected(t *testing.T) {
	s := newStack(t)
	upload := newTamperUpload(t)
	const path = "/api/ingest"

	cases := []struct {
		name   string
		build  func(t *testing.T, u tamperUpload) *http.Request
		status int
		code   string
	}{
		{
			name: "mutated body",
			build: func(t *testing.T, u tamperUpload) *http.Request {
				request := u.request(t, s, http.MethodPost, path, "", u.body, u.content)
				tampered := append([]byte(nil), u.body...)
				tampered[len(tampered)-1] ^= 0x01
				request.Body = io.NopCloser(bytes.NewReader(tampered))
				request.ContentLength = int64(len(tampered))
				return request
			},
			status: http.StatusUnauthorized,
			code:   "unauthorized",
		},
		{
			name: "appended query",
			build: func(t *testing.T, u tamperUpload) *http.Request {
				request := u.request(t, s, http.MethodPost, path, "", u.body, u.content)
				request.URL.RawQuery = "from=2026-09-23"
				return request
			},
			status: http.StatusUnauthorized,
			code:   "unauthorized",
		},
		{
			name: "reordered query",
			build: func(t *testing.T, u tamperUpload) *http.Request {
				request := u.request(t, s, http.MethodPost, path, "from=a&to=b", u.body, u.content)
				request.URL.RawQuery = "to=b&from=a"
				return request
			},
			status: http.StatusUnauthorized,
			code:   "unauthorized",
		},
		{
			name: "changed path",
			build: func(t *testing.T, u tamperUpload) *http.Request {
				request, err := http.NewRequest(http.MethodGet,
					"http://api.test/api/metrics/coverage", nil)
				if err != nil {
					t.Fatalf("new request: %v", err)
				}
				s.sign(t, phoneKid, request, nil, freshNonce(t), time.Now().Unix())
				request.URL.Path = "/api/metrics/series"
				request.URL.RawPath = ""
				return request
			},
			status: http.StatusUnauthorized,
			code:   "unauthorized",
		},
		{
			// A POST must stay multipart for the body-shape check to pass, so a
			// non-multipart value would be refused as 400 before the MAC.
			name: "changed content type",
			build: func(t *testing.T, u tamperUpload) *http.Request {
				request := u.request(t, s, http.MethodPost, path, "", u.body, u.content)
				request.Header.Set("Content-Type", "multipart/form-data; boundary=replaced-boundary")
				return request
			},
			status: http.StatusUnauthorized,
			code:   "unauthorized",
		},
		{
			name: "duplicate auth header",
			build: func(t *testing.T, u tamperUpload) *http.Request {
				request := u.request(t, s, http.MethodPost, path, "", u.body, u.content)
				request.Header.Add(auth.HeaderAuth, request.Header.Get(auth.HeaderAuth))
				return request
			},
			status: http.StatusUnauthorized,
			code:   "unauthorized",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := s.serve(tc.build(t, upload))
			if recorder.Code != tc.status {
				t.Fatalf("status %d, want %d: %s", recorder.Code, tc.status, recorder.Body.String())
			}
			if code := errorCode(t, recorder); code != tc.code {
				t.Fatalf("error code = %q, want %q", code, tc.code)
			}
			if nonces := s.count(t, "auth_nonces"); nonces != 0 {
				t.Fatalf("auth_nonces = %d after a rejected signature", nonces)
			}
			if sessions := s.count(t, "sync_sessions"); sessions != 0 {
				t.Fatalf("sync_sessions = %d after a rejected signature", sessions)
			}
		})
	}

	recorder := s.serve(upload.request(t, s, http.MethodPost, path, "", upload.body, upload.content))
	if recorder.Code != http.StatusOK {
		t.Fatalf("signed request status %d: %s", recorder.Code, recorder.Body.String())
	}
	if sessions := s.count(t, "sync_sessions"); sessions != 1 {
		t.Fatalf("sync_sessions = %d, want 1", sessions)
	}
}

func TestNonIdentityTransferEncodingRejected(t *testing.T) {
	s := newStack(t)
	upload := newTamperUpload(t)

	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="session"`)
	header.Set("Content-Transfer-Encoding", "base64")
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatalf("create part: %v", err)
	}
	if _, err := part.Write(upload.session); err != nil {
		t.Fatalf("write part: %v", err)
	}
	writePart(t, writer, "catalog", upload.catalog)
	writePart(t, writer, "0x2E_raw.bin", upload.blobs["0x2E"])
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}

	request := upload.request(t, s, http.MethodPost, "/api/ingest", "",
		buffer.Bytes(), writer.FormDataContentType())
	recorder := s.serve(request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400: %s", recorder.Code, recorder.Body.String())
	}
	if code := errorCode(t, recorder); code != "invalid_body" {
		t.Fatalf("error code = %q", code)
	}
	if nonces := s.count(t, "auth_nonces"); nonces != 0 {
		t.Fatalf("auth_nonces = %d after a structural failure", nonces)
	}
	if sessions := s.count(t, "sync_sessions"); sessions != 0 {
		t.Fatalf("sync_sessions = %d after a structural failure", sessions)
	}
}
