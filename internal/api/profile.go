package api

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/heliolytics/api/internal/store"
)

type profileHandler struct {
	st *store.Store
}

func (h *profileHandler) serve(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.get(w, r)
	case http.MethodPut:
		h.put(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *profileHandler) get(w http.ResponseWriter, r *http.Request) {
	p, err := h.st.GetProfile(r.Context())
	if err != nil {
		log.Printf("profile get error: %v", err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	logMetrics("profile_get", r, "")
	writeJSON(w, p)
}

func (h *profileHandler) put(w http.ResponseWriter, r *http.Request) {
	var p store.Profile
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if err := h.st.UpsertProfile(r.Context(), p); err != nil {
		log.Printf("profile put error: %v", err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	logMetrics("profile_put", r, "")
	writeJSON(w, p)
}
