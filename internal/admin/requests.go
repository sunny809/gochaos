package admin

import "net/http"

// listRequests handles GET /__admin/requests.
// Returns all logged requests. Supports ?unmatched=true filter.
func (h *Handler) listRequests(w http.ResponseWriter, r *http.Request) {
	var requests interface{}

	switch r.URL.Query().Get("filter") {
	case "unmatched":
		requests = h.requestLog.Unmatched()
	case "matched":
		requests = h.requestLog.Matched()
	default:
		requests = h.requestLog.List()
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"requests": requests,
		"meta":     map[string]int{"total": h.requestLog.Len()},
	})
}

// clearRequests handles DELETE /__admin/requests.
func (h *Handler) clearRequests(w http.ResponseWriter, r *http.Request) {
	h.requestLog.Clear()
	w.WriteHeader(http.StatusNoContent)
}
