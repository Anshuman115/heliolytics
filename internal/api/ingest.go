package api

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/heliolytics/api/internal/parse"
	"github.com/heliolytics/api/internal/store"
)

type ingestHandler struct {
	st *store.Store
}

func (h *ingestHandler) serve(w http.ResponseWriter, r *http.Request) {
	log.Printf("ingest start remote=%s content_length=%d", r.RemoteAddr, r.ContentLength)
	if r.Method != http.MethodPost {
		log.Printf("ingest reject reason=method_not_allowed method=%s", r.Method)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseMultipartForm(128 << 20); err != nil {
		log.Printf("ingest reject reason=bad_multipart err=%v", err)
		http.Error(w, "bad multipart", http.StatusBadRequest)
		return
	}
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
	log.Printf("ingest session id=%s started=%s mac=%v", sess.SessionID, sess.StartedAt, sess.DeviceMAC)
	started, _ := time.Parse(time.RFC3339, sess.StartedAt)
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
	if err := h.st.UpsertSession(ctx, meta); err != nil {
		log.Printf("ingest reject reason=db_session session=%s err=%v", sess.SessionID, err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	log.Printf("ingest session upserted id=%s", sess.SessionID)
	fetchEnd := started
	if ended != nil {
		fetchEnd = *ended
	}
	blobs := map[string][]byte{}
	for name, headers := range r.MultipartForm.File {
		if name == "session" || name == "catalog" {
			continue
		}
		fh := headers[0]
		f, err := fh.Open()
		if err != nil {
			log.Printf("ingest blob open failed name=%s err=%v", name, err)
			continue
		}
		raw, _ := io.ReadAll(f)
		f.Close()
		typeCode := strings.TrimSuffix(name, "_raw.bin")
		if typeCode == name {
			log.Printf("ingest blob skipped name=%s (unexpected filename)", name)
			continue
		}
		_ = h.st.UpsertRaw(ctx, sess.SessionID, typeCode, raw)
		blobs[typeCode] = raw
		log.Printf("ingest blob stored type=%s bytes=%d", typeCode, len(raw))
	}
	log.Printf("ingest parsing session=%s blob_types=%d", sess.SessionID, len(blobs))
	if err := parse.RunIngest(ctx, h.st, sess.SessionID, catalogJSON, blobs, fetchEnd); err != nil {
		log.Printf("ingest parse error session=%s: %v", sess.SessionID, err)
		http.Error(w, "parse error", http.StatusInternalServerError)
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
