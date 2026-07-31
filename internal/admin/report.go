package admin

import (
	"log/slog"
	"net/http"

	"github.com/sunny809/gochaos/internal/report"
)

// reportHandler serves GET /__admin/report — chaos evidence export.
// Formats: junit (JUnit XML), json (structured envelope). Default: json.
func (h *Handler) reportHandler(w http.ResponseWriter, r *http.Request) {
	if h.metrics != nil {
		h.metrics.Add("admin_operations", 1)
	}

	format := r.URL.Query().Get("format")
	if format == "" {
		format = "json"
	}

	entries := h.faultLog.List()
	switch format {
	case "junit":
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(report.JUnit(entries, "gmock-chaos")); err != nil {
			slog.Warn("failed to write junit report", "error", err)
		}
	case "json":
		body, err := report.JSON(entries, "gmock-chaos")
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to encode report")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(body); err != nil {
			slog.Warn("failed to write json report", "error", err)
		}
	default:
		writeError(w, http.StatusBadRequest, "invalid format: must be junit or json")
	}
}
