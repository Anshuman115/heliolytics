package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/heliolytics/api/internal/parse"
	"github.com/heliolytics/api/internal/store"
)

type ingestHandler struct {
	st        *store.Store
	maxBytes  int64
	runIngest func(context.Context, *store.Store, store.SessionMeta, map[string][]byte, time.Time) error
}

const (
	maxIngestBytes     int64 = 128 << 20
	maxMultipartMemory int64 = 32 << 20
)

func (h *ingestHandler) serve(w http.ResponseWriter, r *http.Request) {
	log.Printf("ingest start remote=%s content_length=%d", r.RemoteAddr, r.ContentLength)
	if r.Method != http.MethodPost {
		log.Printf("ingest reject reason=method_not_allowed method=%s", r.Method)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit := h.maxBytes
	if limit == 0 {
		limit = maxIngestBytes
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	if err := r.ParseMultipartForm(maxMultipartMemory); err != nil {
		log.Printf("ingest reject reason=bad_multipart err=%v", err)
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "bad multipart", http.StatusBadRequest)
		return
	}
	defer r.MultipartForm.RemoveAll()
	log.Printf("ingest multipart ok parts=%d", len(r.MultipartForm.File))
	sessionJSON, err := readPart(r, "session")
	if err != nil {
		log.Printf("ingest reject reason=missing_session err=%v", err)
		http.Error(w, "missing session", http.StatusBadRequest)
		return
	}
	catalogJSON, err := readPart(r, "catalog")
	if err != nil {
		log.Printf("ingest reject reason=missing_catalog err=%v", err)
		http.Error(w, "missing catalog", http.StatusBadRequest)
		return
	}
	var sess struct {
		SessionID      string  `json:"sessionId"`
		StartedAt      string  `json:"startedAt"`
		EndedAt        *string `json:"endedAt"`
		DeviceMAC      *string `json:"deviceMac"`
		BatteryPercent *int    `json:"batteryPercent"`
	}
	if err := json.Unmarshal(sessionJSON, &sess); err != nil || sess.SessionID == "" {
		log.Printf("ingest reject reason=invalid_session err=%v bytes=%d", err, len(sessionJSON))
		http.Error(w, "invalid session json", http.StatusBadRequest)
		return
	}
	// deviceMac is nullable, so print the value rather than the pointer.
	log.Printf("ingest session id=%s started=%s mac=%s",
		sess.SessionID, sess.StartedAt, strOrDash(sess.DeviceMAC))
	started, err := time.Parse(time.RFC3339, sess.StartedAt)
	if err != nil {
		log.Printf("ingest reject reason=invalid_started_at session=%s val=%q err=%v", sess.SessionID, sess.StartedAt, err)
		http.Error(w, "invalid startedAt timestamp", http.StatusBadRequest)
		return
	}
	var ended *time.Time
	if sess.EndedAt != nil {
		t, err := time.Parse(time.RFC3339, *sess.EndedAt)
		if err == nil {
			ended = &t
		}
	}
	meta := store.SessionMeta{
		ID: sess.SessionID, StartedAt: started, EndedAt: ended,
		CatalogJSON: catalogJSON,
	}
	if sess.DeviceMAC != nil {
		meta.DeviceMAC = *sess.DeviceMAC
	}
	meta.BatteryPct = sess.BatteryPercent

	ctx := r.Context()
	fetchEnd := started
	if ended != nil {
		fetchEnd = *ended
	}
	// Session row and raw blobs are no longer upserted here — they're written
	// inside RunIngest's single transaction (internal/parse/write_batch.go),
	// alongside every other row for this sync, so a failure partway through
	// rolls back the whole sync instead of leaving an orphaned session row.
	blobs := map[string][]byte{}
	for name, headers := range r.MultipartForm.File {
		if name == "session" || name == "catalog" {
			continue
		}
		if len(headers) == 0 {
			log.Printf("ingest reject reason=missing_blob_header name=%s", name)
			http.Error(w, "invalid raw part", http.StatusBadRequest)
			return
		}
		fh := headers[0]
		f, err := fh.Open()
		if err != nil {
			log.Printf("ingest reject reason=blob_open_failed name=%s err=%v", name, err)
			http.Error(w, "invalid raw part", http.StatusBadRequest)
			return
		}
		raw, readErr := io.ReadAll(f)
		closeErr := f.Close()
		if readErr != nil || closeErr != nil {
			log.Printf("ingest reject reason=blob_read_failed name=%s read_err=%v close_err=%v", name, readErr, closeErr)
			http.Error(w, "invalid raw part", http.StatusBadRequest)
			return
		}
		typeCode := strings.TrimSuffix(name, "_raw.bin")
		if typeCode == name {
			log.Printf("ingest blob skipped name=%s (unexpected filename)", name)
			continue
		}
		blobs[typeCode] = raw
		log.Printf("ingest blob read type=%s bytes=%d", typeCode, len(raw))
	}
	log.Printf("ingest parsing session=%s blob_types=%d", sess.SessionID, len(blobs))
	runIngest := h.runIngest
	if runIngest == nil {
		runIngest = parse.RunIngest
	}
	if err := runIngest(ctx, h.st, meta, blobs, fetchEnd); err != nil {
		log.Printf("ingest error session=%s: %v", sess.SessionID, err)
		http.Error(w, "ingest error", http.StatusInternalServerError)
		return
	}
	log.Printf("ingest ok session=%s types=%d mac=%s", sess.SessionID, len(blobs), meta.DeviceMAC)
	writeJSON(w, map[string]any{"ok": true, "sessionId": sess.SessionID})
}

func readPart(r *http.Request, field string) ([]byte, error) {
	f, hdr, err := r.FormFile(field)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if hdr.Size == 0 {
		return nil, io.EOF
	}
	return io.ReadAll(f)
}
