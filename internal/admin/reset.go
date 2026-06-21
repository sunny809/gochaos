package admin

import (
	"net/http"
	"time"
)

// reset handles POST /__admin/reset.
// Clears all stubs, request log, fault injection log, and any registered reset hooks (e.g., scenarios).
func (h *Handler) reset(w http.ResponseWriter, r *http.Request) {
	h.registry.DeleteAll()
	h.requestLog.Clear()
	h.faultLog.Clear()

	for _, fn := range h.resetFns {
		fn()
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status": "reset",
	})
}

// health handles GET /__admin/health.
// Returns a simple health check response with the current time.
func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "ok",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"stubCount": h.registry.Len(),
	})
}
