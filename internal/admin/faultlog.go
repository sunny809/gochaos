package admin

import "net/http"

// listFaultLog handles GET /__admin/fault-log.
// Returns all logged fault injection events.
func (h *Handler) listFaultLog(w http.ResponseWriter, r *http.Request) {
	entries := h.faultLog.List()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"entries": entries,
		"count":   h.faultLog.Len(),
	})
}

// clearFaultLog handles DELETE /__admin/fault-log.
// Clears all logged fault injection events.
func (h *Handler) clearFaultLog(w http.ResponseWriter, r *http.Request) {
	count := h.faultLog.Len()
	h.faultLog.Clear()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"cleared": true,
		"count":   count,
	})
}
