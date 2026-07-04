package admin

import "net/http"

// listCallbacks handles GET /__admin/callbacks.
// Returns all logged callback dispatch events.
func (h *Handler) listCallbacks(w http.ResponseWriter, r *http.Request) {
	entries := h.callbackLog.List()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"entries": entries,
		"count":   h.callbackLog.Len(),
	})
}

// clearCallbacks handles DELETE /__admin/callbacks.
// Clears all logged callback dispatch events.
func (h *Handler) clearCallbacks(w http.ResponseWriter, r *http.Request) {
	count := h.callbackLog.Len()
	h.callbackLog.Clear()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"cleared": true,
		"count":   count,
	})
}
