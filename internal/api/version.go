package api

import "net/http"

type versionHandler struct {
	commit  *string
	builtAt *string
}

type versionResponse struct {
	Commit  *string `json:"commit"`
	BuiltAt *string `json:"builtAt"`
}

func (h *versionHandler) serve(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, versionResponse{Commit: h.commit, BuiltAt: h.builtAt})
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
