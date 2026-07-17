package api

import "net/http"

// vo2max has no data source yet — no BLE type code parses it, no formula
// exists. This stub keeps the route live for client integration without
// blocking on reverse-engineering a new type code. Always returns an empty
// series; swap the body for a real query once a source is identified.
func (h *metricsHandler) vo2max(w http.ResponseWriter, r *http.Request) {
	logMetrics("vo2max", r, "stub_empty")
	writeJSON(w, map[string]any{"points": []trendPoint{}})
}
