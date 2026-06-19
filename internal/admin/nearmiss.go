// Package admin — near-miss diagnostics endpoint.
//
// This file implements the HTTP surface for the N1 near-miss engine:
//
//	POST /__admin/nearmiss
//	Content-Type: application/json
//
// Request body:
//
//	{
//	  "method":  "GET",                            // optional, default GET
//	  "path":    "/api/users/123",                 // required
//	  "headers": {"Accept": "application/json"},   // optional
//	  "body":    ""                                // optional (raw string)
//	}
//
// Response 200:
//
//	{
//	  "nearMisses": [ { "stubId", "stubName", "score", "maxScore", "breakdown": [...] }, ... ],
//	  "meta":       { "total": <n>, "topN": <n> }
//	}
//
// Response 400 is returned with a JSON {"error": ...} body when the request
// body is not valid JSON, when "path" is missing, or when the supplied path
// cannot be parsed by net/http.
package admin

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/sunny809/gochaos/internal/spec"
)

// NearMissRequest is the JSON body accepted by POST /__admin/nearmiss.
//
// Only Path is required. Method defaults to GET when empty. Headers and Body
// are forwarded to the synthesized *http.Request so per-dimension matchers
// (header / body / jsonpath) can produce meaningful diagnostics.
type NearMissRequest struct {
	Method  string            `json:"method,omitempty"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"`
}

// nearMiss handles POST /__admin/nearmiss.
//
// It synthesizes an *http.Request from the JSON payload, snapshots the current
// stub registry, and asks the near-miss engine to compute per-stub diagnostic
// breakdowns. Stubs that fully match are omitted by Engine.Compute, so an
// exact-match request returns an empty nearMisses array (not 404).
func (h *Handler) nearMiss(w http.ResponseWriter, r *http.Request) {
	var req NearMissRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	if req.Method == "" {
		req.Method = http.MethodGet
	}

	// Use a synthetic host — the engine never inspects it; only the path,
	// query, headers, method, and body are consumed by matchers.
	httpReq, err := http.NewRequest(req.Method, "http://internal"+req.Path, strings.NewReader(req.Body))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid path: "+err.Error())
		return
	}
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}

	stubs := h.registry.List()
	results := h.nearMissEngine.Compute(httpReq, stubs)

	// Normalize a nil result to an empty slice so the JSON payload always
	// carries `"nearMisses": []` rather than `"nearMisses": null`.
	if results == nil {
		results = []spec.NearMissResult{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"nearMisses": results,
		"meta": map[string]interface{}{
			"total": len(results),
			"topN":  h.nearMissEngine.TopN(),
		},
	})
}
