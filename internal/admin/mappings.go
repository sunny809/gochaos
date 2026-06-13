package admin

import (
	"encoding/json"
	"net/http"

	"github.com/sunny809/gochaos/internal/spec"
)

// createMapping handles POST /__admin/mappings.
// Accepts a single StubDefinition in JSON and registers it.
func (h *Handler) createMapping(w http.ResponseWriter, r *http.Request) {
	var def spec.StubDefinition
	if err := json.NewDecoder(r.Body).Decode(&def); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	id, err := h.registry.Add(def)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Return the created stub with its ID
	stored := h.registry.Get(id)
	writeJSON(w, http.StatusCreated, stored)
}

// listMappings handles GET /__admin/mappings.
// Returns all registered stubs in priority order.
func (h *Handler) listMappings(w http.ResponseWriter, r *http.Request) {
	stubs := h.registry.List()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"mappings": stubs,
		"meta":     map[string]int{"total": len(stubs)},
	})
}

// getMapping handles GET /__admin/mappings/{id}.
func (h *Handler) getMapping(w http.ResponseWriter, r *http.Request, id string) {
	def := h.registry.Get(id)
	if def == nil {
		writeError(w, http.StatusNotFound, "stub not found: "+id)
		return
	}
	writeJSON(w, http.StatusOK, def)
}

// deleteMapping handles DELETE /__admin/mappings/{id}.
func (h *Handler) deleteMapping(w http.ResponseWriter, r *http.Request, id string) {
	if !h.registry.Delete(id) {
		writeError(w, http.StatusNotFound, "stub not found: "+id)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deleteAllMappings handles DELETE /__admin/mappings.
func (h *Handler) deleteAllMappings(w http.ResponseWriter, r *http.Request) {
	h.registry.DeleteAll()
	w.WriteHeader(http.StatusNoContent)
}