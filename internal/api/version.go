package api

import "net/http"

// versionHandler answers /version from build metadata only. No database, no
// secrets.
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

// optionalString renders an uninjected build field as JSON null.
func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
