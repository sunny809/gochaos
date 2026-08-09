package gmock

import (
	"expvar"
	"fmt"
	"io"
	"log/slog"
)

type (
	// promMetric describes a single Prometheus metric in the registration table.
	promMetric struct {
		name  string // Prometheus metric name (e.g. "gochaos_requests_total")
		help  string // HELP string
		typ   string // "counter" or "gauge"
		field string // Snapshot map key
	}

	// Metrics holds expvar counters for the gmock server.
	// All counters are concurrent-safe (expvar.Int uses atomic operations internally).
	//
	// Counters:
	//   - requests_total: Total requests received by the server
	//   - requests_matched: Requests that matched a stub
	//   - requests_unmatched: Requests that returned 404
	//   - faults_injected: Total faults injected (all types)
	//   - faults_delayed: Responses with configured delay
	//   - nearmiss_queries: Near-miss diagnostic calls
	//   - stubs_registered: Active stub count (snapshot, updated on register/delete)
	//   - admin_operations: Admin API calls (CRUD, reset, log queries)
	Metrics struct {
		requestsTotal     expvar.Int
		requestsMatched   expvar.Int
		requestsUnmatched expvar.Int
		faultsInjected    expvar.Int
		faultsDelayed     expvar.Int
		nearMissQueries   expvar.Int
		stubsRegistered   expvar.Int
		adminOperations   expvar.Int
		promMetrics       []promMetric // registration table for Prometheus export
	}
)

// newMetrics creates and initializes all expvar counters.
// Each counter is registered as "gmock_<name>" so they are discoverable
// via the expvar HTTP handler (and /__admin/metrics endpoint).
func newMetrics() *Metrics {
	m := &Metrics{}
	m.promMetrics = []promMetric{
		{name: "gochaos_requests_total", help: "Total requests received", typ: "counter", field: "requests_total"},
		{name: "gochaos_requests_matched", help: "Requests that matched a stub", typ: "counter", field: "requests_matched"},
		{name: "gochaos_requests_unmatched", help: "Requests that did not match any stub", typ: "counter", field: "requests_unmatched"},
		{name: "gochaos_faults_injected", help: "Total faults injected", typ: "counter", field: "faults_injected"},
		{name: "gochaos_delays_applied", help: "Delays applied to responses", typ: "counter", field: "faults_delayed"},
		{name: "gochaos_nearmiss_queries", help: "Near-miss diagnostic queries", typ: "counter", field: "nearmiss_queries"},
		{name: "gochaos_stubs_registered", help: "Currently registered stub count", typ: "gauge", field: "stubs_registered"},
		{name: "gochaos_admin_operations", help: "Admin API operations", typ: "counter", field: "admin_operations"},
	}
	return m
}

// Add increments a named counter by delta. Unknown counter names are
// logged as a warning and otherwise ignored.
func (m *Metrics) Add(name string, delta int64) {
	switch name {
	case "admin_operations":
		m.adminOperations.Add(delta)
	case "requests_total":
		m.requestsTotal.Add(delta)
	case "requests_matched":
		m.requestsMatched.Add(delta)
	case "requests_unmatched":
		m.requestsUnmatched.Add(delta)
	case "faults_injected":
		m.faultsInjected.Add(delta)
	case "faults_delayed":
		m.faultsDelayed.Add(delta)
	case "nearmiss_queries":
		m.nearMissQueries.Add(delta)
	case "stubs_registered":
		m.stubsRegistered.Add(delta)
	default:
		slog.Warn("gmock: unknown metric name", "name", name)
	}
}

// Snapshot returns a map of all current counter values for JSON serialization.
func (m *Metrics) Snapshot() map[string]int64 {
	return map[string]int64{
		"requests_total":     m.requestsTotal.Value(),
		"requests_matched":   m.requestsMatched.Value(),
		"requests_unmatched": m.requestsUnmatched.Value(),
		"faults_injected":    m.faultsInjected.Value(),
		"faults_delayed":     m.faultsDelayed.Value(),
		"nearmiss_queries":   m.nearMissQueries.Value(),
		"stubs_registered":   m.stubsRegistered.Value(),
		"admin_operations":   m.adminOperations.Value(),
	}
}

// WritePrometheus writes all metrics in Prometheus text format (exposition v0.0.4).
// No external Prometheus client library is used; the format is self-contained.
// Note: each call allocates a snapshot map — this is acceptable for monitoring
// scrape intervals (typically 15s) but not for per-request hot paths.
// Returns an error if writing to w fails.
func (m *Metrics) WritePrometheus(w io.Writer) error {
	snap := m.Snapshot()
	for _, pm := range m.promMetrics {
		if _, err := fmt.Fprintf(w, "# HELP %s %s\n", pm.name, pm.help); err != nil {
			return fmt.Errorf("write HELP for %s: %w", pm.name, err)
		}
		if _, err := fmt.Fprintf(w, "# TYPE %s %s\n", pm.name, pm.typ); err != nil {
			return fmt.Errorf("write TYPE for %s: %w", pm.name, err)
		}
		if _, err := fmt.Fprintf(w, "%s %d\n", pm.name, snap[pm.field]); err != nil {
			return fmt.Errorf("write value for %s: %w", pm.name, err)
		}
	}
	return nil
}

// resetAll sets all counters to zero. Called during server Reset().
func (m *Metrics) resetAll() {
	m.requestsTotal.Set(0)
	m.requestsMatched.Set(0)
	m.requestsUnmatched.Set(0)
	m.faultsInjected.Set(0)
	m.faultsDelayed.Set(0)
	m.nearMissQueries.Set(0)
	m.stubsRegistered.Set(0)
	m.adminOperations.Set(0)
}
