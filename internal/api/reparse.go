package api

import (
	"log"
	"net/http"

	"github.com/heliolytics/api/internal/parse"
	"github.com/heliolytics/api/internal/store"
)

type reparseHandler struct {
	st      *store.Store
	enabled bool
	secret  string
}

func (h *reparseHandler) serve(w http.ResponseWriter, r *http.Request) {
	if !h.enabled {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.secret != "" && r.Header.Get("X-Reparse-Secret") != h.secret {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	rep, err := h.st.LatestReplay(r.Context())
	if err != nil {
		log.Printf("reparse error: %v", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := parse.RunIngest(r.Context(), h.st, rep.SessionID, rep.Catalog, rep.Blobs, rep.EndedAt); err != nil {
		log.Printf("reparse ingest error: %v", err)
		http.Error(w, "parse error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":true}`))
}
