// Package admin implements the gmock admin REST API.
//
// The admin API provides HTTP endpoints for managing stubs, viewing request logs,
// fault injection logs, and resetting server state. It is mounted under the
// /__admin/ prefix by default.
//
// API endpoints:
//
//	POST   /__admin/mappings       Create a stub
//	GET    /__admin/mappings       List all stubs
//	DELETE /__admin/mappings       Delete all stubs
//	GET    /__admin/mappings/{id}  Get a stub by ID
//	DELETE /__admin/mappings/{id}  Delete a stub by ID
//	POST   /__admin/nearmiss       Near-miss diagnostics
//	POST   /__admin/reset          Reset all server state
//	GET    /__admin/requests       List logged requests
//	DELETE /__admin/requests       Clear request log
//	GET    /__admin/fault-log      List fault injection events
//	DELETE /__admin/fault-log      Clear fault injection log
//	GET    /__admin/health         Health check
package admin

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/sunny809/gochaos/internal/faultlog"
	"github.com/sunny809/gochaos/internal/log"
	"github.com/sunny809/gochaos/internal/nearmiss"
	"github.com/sunny809/gochaos/internal/stub"
)

// Prefix is the URL prefix for all admin endpoints.
const Prefix = "/__admin/"

// Handler implements the admin API HTTP handler.
type Handler struct {
	registry       *stub.Registry
	requestLog     *log.RequestLog
	faultLog       *faultlog.FaultInjectionLog
	nearMissEngine *nearmiss.Engine
	resetFns       []func() // additional reset hooks (scenarios, proxy, etc.)
}

// New creates an admin Handler bound to the given registry, request log, fault log, and near-miss engine.
func New(registry *stub.Registry, requestLog *log.RequestLog, faultLog *faultlog.FaultInjectionLog, nearMissEngine *nearmiss.Engine) *Handler {
	return &Handler{
		registry:       registry,
		requestLog:     requestLog,
		faultLog:       faultLog,
		nearMissEngine: nearMissEngine,
	}
}

// RegisterResetHook adds a function to be called when POST /__admin/reset is invoked.
// Use this to register cleanup for additional state (scenarios, proxy recordings, etc.).
func (h *Handler) RegisterResetHook(fn func()) {
	h.resetFns = append(h.resetFns, fn)
}

// IsAdminPath returns true if the given path targets the admin API.
func IsAdminPath(path string) bool {
	return strings.HasPrefix(path, Prefix)
}

// ServeHTTP implements http.Handler.
// Dispatches to the appropriate handler method based on path and method.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	switch {
	case path == Prefix+"mappings" || path == Prefix+"mappings/":
		switch r.Method {
		case http.MethodGet:
			h.listMappings(w, r)
		case http.MethodPost:
			h.createMapping(w, r)
		case http.MethodDelete:
			h.deleteAllMappings(w, r)
		default:
			methodNotAllowed(w)
		}

	case strings.HasPrefix(path, Prefix+"mappings/"):
		id := strings.TrimPrefix(path, Prefix+"mappings/")
		switch r.Method {
		case http.MethodGet:
			h.getMapping(w, r, id)
		case http.MethodDelete:
			h.deleteMapping(w, r, id)
		default:
			methodNotAllowed(w)
		}

	case path == Prefix+"reset":
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		h.reset(w, r)

	case path == Prefix+"nearmiss":
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		h.nearMiss(w, r)

	case path == Prefix+"requests":
		switch r.Method {
		case http.MethodGet:
			h.listRequests(w, r)
		case http.MethodDelete:
			h.clearRequests(w, r)
		default:
			methodNotAllowed(w)
		}

	case path == Prefix+"fault-log":
		switch r.Method {
		case http.MethodGet:
			h.listFaultLog(w, r)
		case http.MethodDelete:
			h.clearFaultLog(w, r)
		default:
			methodNotAllowed(w)
		}

	case path == Prefix+"health":
		h.health(w, r)

	default:
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "admin endpoint not found: " + path,
		})
	}
}

// --- Response Helpers ---

func writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func methodNotAllowed(w http.ResponseWriter) {
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}
