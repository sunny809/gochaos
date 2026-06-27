package admin

import (
	"io"
	"net/http"
)

// MetricsProvider is the interface the admin handler uses to retrieve
// current metric values for the /__admin/metrics endpoint and to
// increment counters from admin handler methods.
type MetricsProvider interface {
	Snapshot() map[string]int64
	Add(name string, delta int64)
	WritePrometheus(w io.Writer)
}

// metricsHandler handles GET /__admin/metrics.
func (h *Handler) metricsHandler(w http.ResponseWriter, r *http.Request) {
	if h.metrics == nil {
		writeJSON(w, http.StatusOK, map[string]string{
			"message": "metrics not initialized",
		})
		return
	}
	writeJSON(w, http.StatusOK, h.metrics.Snapshot())
}

// prometheusHandler handles GET /__admin/metrics/prometheus.
func (h *Handler) prometheusHandler(w http.ResponseWriter, r *http.Request) {
	if h.metrics == nil {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	h.metrics.WritePrometheus(w)
}
