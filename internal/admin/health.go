// Package admin implements the gmock admin REST API.
//
// This file contains health check endpoints for Kubernetes liveness and readiness probes.

package admin

import (
	"net/http"
)

// HealthLive handles GET /__admin/health/live.
// Returns 200 OK if the HTTP server is running and accepting connections.
func (h *Handler) HealthLive(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "alive",
	})
}

// HealthReady handles GET /__admin/health/ready.
// Returns 200 OK when the server can serve requests (stub registry initialized),
// and 503 Service Unavailable when the server is shutting down.
func (h *Handler) HealthReady(w http.ResponseWriter, r *http.Request) {
	if h.shuttingDown.Load() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "not ready — shutting down",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "ready",
		"stubCount": h.registry.Len(),
	})
}
