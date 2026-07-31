// Package admin implements the gmock admin REST API.
//
// The admin API provides HTTP endpoints for managing stubs, viewing request logs,
// fault injection logs, and resetting server state. It is mounted under the
// /__admin/ prefix by default.
//
// API endpoints:
//
//	POST   /__admin/mappings              Create a stub
//	GET    /__admin/mappings              List all stubs
//	DELETE /__admin/mappings              Delete all stubs
//	GET    /__admin/mappings/{id}         Get a stub by ID
//	DELETE /__admin/mappings/{id}         Delete a stub by ID
//	POST   /__admin/nearmiss              Near-miss diagnostics
//	POST   /__admin/reset                 Reset all server state
//	GET    /__admin/requests              List logged requests
//	DELETE /__admin/requests              Clear request log
//	GET    /__admin/fault-log             List fault injection events
//	DELETE /__admin/fault-log             Clear fault injection log
//	GET    /__admin/callbacks             List callback dispatch events
//	DELETE /__admin/callbacks             Clear callback dispatch log
//	GET    /__admin/health                Health check (legacy)
//	GET    /__admin/health/live           Liveness probe (K8s)
//	GET    /__admin/health/ready          Readiness probe (K8s)
//	GET    /__admin/metrics               Server metrics (expvar counters)
//	GET    /__admin/report                Export chaos evidence (JUnit XML or JSON)
package admin

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/sunny809/gochaos/internal/callbacklog"
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
	callbackLog    *callbacklog.Log
	nearMissEngine *nearmiss.Engine
	metrics        MetricsProvider
	resetFns       []func()    // additional reset hooks (scenarios, proxy, etc.)
	shuttingDown   atomic.Bool // set to true when the server is shutting down (for readiness probe)
}

// SetShuttingDown marks the server as shutting down, causing the readiness
// probe to return 503. Call this before Server.Shutdown().
func (h *Handler) SetShuttingDown(v bool) {
	h.shuttingDown.Store(v)
}

// New creates an admin Handler bound to the given dependencies.
func New(registry *stub.Registry, requestLog *log.RequestLog, faultLog *faultlog.FaultInjectionLog, callbackLog *callbacklog.Log, nearMissEngine *nearmiss.Engine, metrics MetricsProvider) *Handler {
	return &Handler{
		registry:       registry,
		requestLog:     requestLog,
		faultLog:       faultLog,
		callbackLog:    callbackLog,
		nearMissEngine: nearMissEngine,
		metrics:        metrics,
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
			if h.metrics != nil {
				h.metrics.Add("admin_operations", 1)
			}
			h.listMappings(w, r)
		case http.MethodPost:
			if h.metrics != nil {
				h.metrics.Add("admin_operations", 1)
			}
			h.createMapping(w, r)
		case http.MethodDelete:
			if h.metrics != nil {
				h.metrics.Add("admin_operations", 1)
			}
			h.deleteAllMappings(w, r)
		default:
			methodNotAllowed(w)
		}

	case strings.HasPrefix(path, Prefix+"mappings/"):
		id := strings.TrimPrefix(path, Prefix+"mappings/")
		switch r.Method {
		case http.MethodGet:
			if h.metrics != nil {
				h.metrics.Add("admin_operations", 1)
			}
			h.getMapping(w, r, id)
		case http.MethodDelete:
			if h.metrics != nil {
				h.metrics.Add("admin_operations", 1)
			}
			h.deleteMapping(w, r, id)
		default:
			methodNotAllowed(w)
		}

	case path == Prefix+"reset":
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		if h.metrics != nil {
			h.metrics.Add("admin_operations", 1)
		}
		h.reset(w, r)

	case path == Prefix+"nearmiss":
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		if h.metrics != nil {
			h.metrics.Add("admin_operations", 1)
		}
		h.nearMiss(w, r)

	case path == Prefix+"requests":
		switch r.Method {
		case http.MethodGet:
			if h.metrics != nil {
				h.metrics.Add("admin_operations", 1)
			}
			h.listRequests(w, r)
		case http.MethodDelete:
			if h.metrics != nil {
				h.metrics.Add("admin_operations", 1)
			}
			h.clearRequests(w, r)
		default:
			methodNotAllowed(w)
		}

	case path == Prefix+"fault-log":
		switch r.Method {
		case http.MethodGet:
			if h.metrics != nil {
				h.metrics.Add("admin_operations", 1)
			}
			h.listFaultLog(w, r)
		case http.MethodDelete:
			if h.metrics != nil {
				h.metrics.Add("admin_operations", 1)
			}
			h.clearFaultLog(w, r)
		default:
			methodNotAllowed(w)
		}

	case path == Prefix+"callbacks":
		switch r.Method {
		case http.MethodGet:
			if h.metrics != nil {
				h.metrics.Add("admin_operations", 1)
			}
			h.listCallbacks(w, r)
		case http.MethodDelete:
			if h.metrics != nil {
				h.metrics.Add("admin_operations", 1)
			}
			h.clearCallbacks(w, r)
		default:
			methodNotAllowed(w)
		}

	case path == Prefix+"health" || path == Prefix+"health/":
		h.health(w, r)

	case path == Prefix+"health/live" || path == Prefix+"health/live/":
		h.HealthLive(w, r)

	case path == Prefix+"health/ready" || path == Prefix+"health/ready/":
		h.HealthReady(w, r)

	case path == Prefix+"metrics/prometheus":
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		if h.metrics != nil {
			h.metrics.Add("admin_operations", 1)
		}
		h.prometheusHandler(w, r)

	case path == Prefix+"metrics" || path == Prefix+"metrics/":
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		if h.metrics != nil {
			h.metrics.Add("admin_operations", 1)
		}
		h.metricsHandler(w, r)

	case path == Prefix+"report" || path == Prefix+"report/":
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		h.reportHandler(w, r)

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
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Warn("failed to encode JSON response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func methodNotAllowed(w http.ResponseWriter) {
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}
