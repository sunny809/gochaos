package admin

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sunny809/gochaos/internal/callbacklog"
	"github.com/sunny809/gochaos/internal/faultlog"
	"github.com/sunny809/gochaos/internal/log"
	"github.com/sunny809/gochaos/internal/nearmiss"
	"github.com/sunny809/gochaos/internal/spec"
	"github.com/sunny809/gochaos/internal/stub"
)

// testMetrics is a simple MetricsProvider for tests.
type testMetrics struct{}

func (m *testMetrics) Snapshot() map[string]int64 {
	return map[string]int64{
		"requests_total":     10,
		"requests_matched":   8,
		"requests_unmatched": 2,
		"faults_injected":    3,
		"faults_delayed":     1,
		"nearmiss_queries":   0,
		"stubs_registered":   5,
		"admin_operations":   2,
	}
}

func (m *testMetrics) Add(name string, delta int64) {}
func (m *testMetrics) WritePrometheus(w io.Writer) error {
	_, err := w.Write([]byte("# HELP test\n# TYPE test gauge\ntest 0\n"))
	return err
}

func setupTest() (*Handler, *stub.Registry, *log.RequestLog) {
	registry := stub.NewRegistry()
	requestLog := log.New(100)
	faultLog := faultlog.NewFaultInjectionLog(100)
	callbackLog := callbacklog.New(100)
	engine := nearmiss.NewEngine()
	h := New(registry, requestLog, faultLog, callbackLog, engine, &testMetrics{})
	return h, registry, requestLog
}

func TestHealthLive(t *testing.T) {
	h, _, _ := setupTest()

	req := httptest.NewRequest("GET", "/__admin/health/live", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if body["status"] != "alive" {
		t.Errorf("expected status=alive, got %v", body["status"])
	}
}

func TestHealthReady(t *testing.T) {
	h, registry, _ := setupTest()
	registry.Add(spec.StubDefinition{
		Request:  spec.RequestPattern{Method: "GET", URLPath: "/test"},
		Response: spec.ResponseDefinition{Status: 200},
	})

	req := httptest.NewRequest("GET", "/__admin/health/ready", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if body["status"] != "ready" {
		t.Errorf("expected status=ready, got %v", body["status"])
	}
	if body["stubCount"] != 1.0 {
		t.Errorf("expected stubCount=1, got %v", body["stubCount"])
	}
}

func TestHealthReadyShuttingDown(t *testing.T) {
	h, _, _ := setupTest()

	h.SetShuttingDown(true)

	req := httptest.NewRequest("GET", "/__admin/health/ready", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}

	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if body["status"] != "not ready — shutting down" {
		t.Errorf("expected status=not ready — shutting down, got %v", body["status"])
	}
}

func TestMetricsEndpoint(t *testing.T) {
	h, _, _ := setupTest()

	req := httptest.NewRequest("GET", "/__admin/metrics", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	expectedKeys := []string{
		"requests_total", "requests_matched", "requests_unmatched",
		"faults_injected", "faults_delayed", "nearmiss_queries",
		"stubs_registered", "admin_operations",
	}
	for _, key := range expectedKeys {
		if _, ok := body[key]; !ok {
			t.Errorf("metrics response missing key: %s", key)
		}
	}
}

func TestListFaultLog(t *testing.T) {
	h, _, _ := setupTest()
	// Record a fault injection event
	h.faultLog.Record(spec.FaultInjectionEntry{
		StubID: "stub-1",
		FaultType: "connection_reset",
		RequestPath: "/api/test",
	})

	req := httptest.NewRequest("GET", "/__admin/fault-log", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var result struct {
		Entries []spec.FaultInjectionEntry `json:"entries"`
		Count   int                            `json:"count"`
	}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if len(result.Entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(result.Entries))
	}
	if result.Count != 1 {
		t.Errorf("expected total=1, got %d", result.Count)
	}
	if result.Entries[0].StubID != "stub-1" {
		t.Errorf("expected stubID=stub-1, got %s", result.Entries[0].StubID)
	}
}

func TestClearFaultLog(t *testing.T) {
	h, _, _ := setupTest()
	h.faultLog.Record(spec.FaultInjectionEntry{
		StubID: "stub-1",
		FaultType: "connection_reset",
		RequestPath: "/api/test",
	})

	req := httptest.NewRequest("DELETE", "/__admin/fault-log", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if result["cleared"] != true {
		t.Errorf("expected cleared=true, got %v", result["cleared"])
	}
	if result["count"] != float64(1) {
		t.Errorf("expected count=1, got %v", result["count"])
	}

	if h.faultLog.Len() != 0 {
		t.Errorf("expected 0 entries after clear, got %d", h.faultLog.Len())
	}
}

func TestListRequestsFilterMatched(t *testing.T) {
	h, _, requestLog := setupTest()
	requestLog.Record(httptest.NewRequest("GET", "/matched", nil), true, "s1")
	requestLog.Record(httptest.NewRequest("GET", "/unmatched", nil), false, "")

	req := httptest.NewRequest("GET", "/__admin/requests?filter=matched", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var result struct {
		Requests []interface{}  `json:"requests"`
		Meta     map[string]int `json:"meta"`
	}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if len(result.Requests) != 1 {
		t.Errorf("expected 1 matched, got %d", len(result.Requests))
	}
}
func TestHealthLegacy(t *testing.T) {
	h, registry, _ := setupTest()
	registry.Add(spec.StubDefinition{
		Request:  spec.RequestPattern{Method: "GET", URLPath: "/test"},
		Response: spec.ResponseDefinition{Status: 200},
	})

	req := httptest.NewRequest("GET", "/__admin/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var body map[string]interface{}
	json.NewDecoder(w.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Errorf("expected status=ok, got %v", body["status"])
	}
	if body["stubCount"] != 1.0 {
		t.Errorf("expected stubCount=1, got %v", body["stubCount"])
	}
}
func TestCreateMapping(t *testing.T) {
	h, _, _ := setupTest()

	stubJSON := `{"request":{"method":"POST","urlPath":"/api/create"},"response":{"status":201,"body":"created"}}`
	req := httptest.NewRequest("POST", "/__admin/mappings", bytes.NewReader([]byte(stubJSON)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}

	var created spec.StubDefinition
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if created.ID == "" {
		t.Error("expected non-empty ID")
	}
	if created.Request.Method != "POST" {
		t.Errorf("expected POST, got %s", created.Request.Method)
	}
}

func TestCreateMappingInvalidJSON(t *testing.T) {
	h, _, _ := setupTest()

	req := httptest.NewRequest("POST", "/__admin/mappings", bytes.NewReader([]byte(`not json`)))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestListMappings(t *testing.T) {
	h, registry, _ := setupTest()
	registry.Add(spec.StubDefinition{
		Request:  spec.RequestPattern{Method: "GET", URLPath: "/a"},
		Response: spec.ResponseDefinition{Status: 200},
	})
	registry.Add(spec.StubDefinition{
		Request:  spec.RequestPattern{Method: "POST", URLPath: "/b"},
		Response: spec.ResponseDefinition{Status: 201},
	})

	req := httptest.NewRequest("GET", "/__admin/mappings", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var result struct {
		Mappings []spec.StubDefinition `json:"mappings"`
		Meta     map[string]int        `json:"meta"`
	}
	json.NewDecoder(w.Body).Decode(&result)
	if len(result.Mappings) != 2 {
		t.Errorf("expected 2 mappings, got %d", len(result.Mappings))
	}
	if result.Meta["total"] != 2 {
		t.Errorf("expected total=2, got %d", result.Meta["total"])
	}
}

func TestGetMapping(t *testing.T) {
	h, registry, _ := setupTest()
	id, _ := registry.Add(spec.StubDefinition{
		Request:  spec.RequestPattern{Method: "GET", URLPath: "/test"},
		Response: spec.ResponseDefinition{Status: 200},
	})

	req := httptest.NewRequest("GET", "/__admin/mappings/"+id, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestGetMappingNotFound(t *testing.T) {
	h, _, _ := setupTest()

	req := httptest.NewRequest("GET", "/__admin/mappings/nonexistent", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestDeleteMapping(t *testing.T) {
	h, registry, _ := setupTest()
	id, _ := registry.Add(spec.StubDefinition{
		Request:  spec.RequestPattern{Method: "GET", URLPath: "/to-delete"},
		Response: spec.ResponseDefinition{Status: 200},
	})

	req := httptest.NewRequest("DELETE", "/__admin/mappings/"+id, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
	if registry.Len() != 0 {
		t.Errorf("expected 0 stubs after delete, got %d", registry.Len())
	}
}

func TestDeleteMappingNotFound(t *testing.T) {
	h, _, _ := setupTest()

	req := httptest.NewRequest("DELETE", "/__admin/mappings/nonexistent", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestDeleteAllMappings(t *testing.T) {
	h, registry, _ := setupTest()
	registry.Add(spec.StubDefinition{
		Request:  spec.RequestPattern{Method: "GET", URLPath: "/a"},
		Response: spec.ResponseDefinition{Status: 200},
	})
	registry.Add(spec.StubDefinition{
		Request:  spec.RequestPattern{Method: "GET", URLPath: "/b"},
		Response: spec.ResponseDefinition{Status: 200},
	})

	req := httptest.NewRequest("DELETE", "/__admin/mappings", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
	if registry.Len() != 0 {
		t.Errorf("expected 0 stubs, got %d", registry.Len())
	}
}

func TestListRequests(t *testing.T) {
	h, _, requestLog := setupTest()
	requestLog.Record(httptest.NewRequest("GET", "/req1", nil), true, "s1")
	requestLog.Record(httptest.NewRequest("POST", "/req2", nil), false, "")

	req := httptest.NewRequest("GET", "/__admin/requests", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var result struct {
		Requests []interface{}  `json:"requests"`
		Meta     map[string]int `json:"meta"`
	}
	json.NewDecoder(w.Body).Decode(&result)
	if len(result.Requests) != 2 {
		t.Errorf("expected 2 requests, got %d", len(result.Requests))
	}
	if result.Meta["total"] != 2 {
		t.Errorf("expected total=2, got %d", result.Meta["total"])
	}
}

func TestListRequestsFilter(t *testing.T) {
	h, _, requestLog := setupTest()
	requestLog.Record(httptest.NewRequest("GET", "/matched", nil), true, "s1")
	requestLog.Record(httptest.NewRequest("GET", "/unmatched", nil), false, "")

	req := httptest.NewRequest("GET", "/__admin/requests?filter=unmatched", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var result struct {
		Requests []interface{} `json:"requests"`
	}
	json.NewDecoder(w.Body).Decode(&result)
	if len(result.Requests) != 1 {
		t.Errorf("expected 1 unmatched, got %d", len(result.Requests))
	}
}

func TestClearRequests(t *testing.T) {
	h, _, requestLog := setupTest()
	requestLog.Record(httptest.NewRequest("GET", "/test", nil), true, "s1")

	req := httptest.NewRequest("DELETE", "/__admin/requests", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
	if requestLog.Len() != 0 {
		t.Errorf("expected 0 after clear, got %d", requestLog.Len())
	}
}

func TestReset(t *testing.T) {
	h, registry, requestLog := setupTest()
	registry.Add(spec.StubDefinition{
		Request:  spec.RequestPattern{Method: "GET", URLPath: "/test"},
		Response: spec.ResponseDefinition{Status: 200},
	})
	requestLog.Record(httptest.NewRequest("GET", "/test", nil), true, "s1")

	var hookCalled bool
	h.RegisterResetHook(func() {
		hookCalled = true
	})

	req := httptest.NewRequest("POST", "/__admin/reset", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if registry.Len() != 0 {
		t.Errorf("expected 0 stubs after reset, got %d", registry.Len())
	}
	if requestLog.Len() != 0 {
		t.Errorf("expected 0 log after reset, got %d", requestLog.Len())
	}
	if !hookCalled {
		t.Error("expected reset hook to be called")
	}
}

func TestUnknownEndpoint(t *testing.T) {
	h, _, _ := setupTest()

	req := httptest.NewRequest("GET", "/__admin/unknown", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown endpoint, got %d", w.Code)
	}
}

func TestIsAdminPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/__admin/mappings", true},
		{"/__admin/health", true},
		{"/__admin/reset", true},
		{"/api/users", false},
		{"/", false},
	}
	for _, tt := range tests {
		if got := IsAdminPath(tt.path); got != tt.want {
			t.Errorf("IsAdminPath(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestCreateMappingInvalidFaultType(t *testing.T) {
	h, _, _ := setupTest()

	stubJSON := `{"request":{"method":"GET","urlPath":"/fault"},"response":{"status":500,"fault":{"type":"INVALID"}}}`
	req := httptest.NewRequest("POST", "/__admin/mappings", bytes.NewReader([]byte(stubJSON)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid fault type, got %d", w.Code)
	}
}

func TestCreateMappingValidFaultType(t *testing.T) {
	h, _, _ := setupTest()

	stubJSON := `{"request":{"method":"GET","urlPath":"/fault"},"response":{"status":500,"fault":{"type":"connection_reset"}}}`
	req := httptest.NewRequest("POST", "/__admin/mappings", bytes.NewReader([]byte(stubJSON)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201 for valid fault type, got %d", w.Code)
	}

	var created spec.StubDefinition
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if created.Response.Fault == nil || created.Response.Fault.Type != "connection_reset" {
		t.Errorf("expected fault type connection_reset, got %+v", created.Response.Fault)
	}
}

func TestListCallbacks(t *testing.T) {
	h, _, _ := setupTest()
	h.callbackLog.Record(spec.CallbackEntry{
		StubID:       "stub-1",
		CallbackURL:  "http://example.com/hook",
		Status:       spec.CallbackDelivered,
		StatusCode:   200,
		RequestPath:  "/api/test",
	})

	req := httptest.NewRequest("GET", "/__admin/callbacks", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var result struct {
		Entries []spec.CallbackEntry `json:"entries"`
		Count   int                  `json:"count"`
	}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if len(result.Entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(result.Entries))
	}
	if result.Count != 1 {
		t.Errorf("expected total=1, got %d", result.Count)
	}
	if result.Entries[0].StubID != "stub-1" {
		t.Errorf("expected stubID=stub-1, got %s", result.Entries[0].StubID)
	}
}

func TestClearCallbacks(t *testing.T) {
	h, _, _ := setupTest()
	h.callbackLog.Record(spec.CallbackEntry{
		StubID:      "stub-1",
		CallbackURL: "http://example.com/hook",
		Status:      spec.CallbackDelivered,
	})

	req := httptest.NewRequest("DELETE", "/__admin/callbacks", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if result["cleared"] != true {
		t.Errorf("expected cleared=true, got %v", result["cleared"])
	}
	if result["count"] != float64(1) {
		t.Errorf("expected count=1, got %v", result["count"])
	}

	if h.callbackLog.Len() != 0 {
		t.Errorf("expected 0 entries after clear, got %d", h.callbackLog.Len())
	}
}

func TestPrometheusEndpoint(t *testing.T) {
	h, _, _ := setupTest()

	req := httptest.NewRequest("GET", "/__admin/metrics/prometheus", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	got := w.Body.String()
	if got != "# HELP test\n# TYPE test gauge\ntest 0\n" {
		t.Errorf("unexpected prometheus output: %q", got)
	}
	// Content-Type must be text/plain with Prometheus version.
	if ct := w.Header().Get("Content-Type"); ct != "text/plain; version=0.0.4" {
		t.Errorf("Content-Type = %q, want text/plain; version=0.0.4", ct)
	}
}

func TestPrometheusEndpointMethodNotAllowed(t *testing.T) {
	h, _, _ := setupTest()

	req := httptest.NewRequest("POST", "/__admin/metrics/prometheus", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestMetricsEndpointNilMetrics(t *testing.T) {
	// When no MetricsProvider is configured, the metrics endpoint should
	// return a 200 with a message rather than panicking on a nil interface.
	registry := stub.NewRegistry()
	requestLog := log.New(100)
	faultLog := faultlog.NewFaultInjectionLog(100)
	callbackLog := callbacklog.New(100)
	engine := nearmiss.NewEngine()
	h := New(registry, requestLog, faultLog, callbackLog, engine, nil) // nil metrics

	req := httptest.NewRequest("GET", "/__admin/metrics", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if body["message"] != "metrics not initialized" {
		t.Errorf("expected 'metrics not initialized', got %q", body["message"])
	}
}

func TestPrometheusEndpointNilMetrics(t *testing.T) {
	// When no MetricsProvider is configured, the prometheus endpoint should
	// return an empty 200 with the correct Content-Type rather than panicking.
	registry := stub.NewRegistry()
	requestLog := log.New(100)
	faultLog := faultlog.NewFaultInjectionLog(100)
	callbackLog := callbacklog.New(100)
	engine := nearmiss.NewEngine()
	h := New(registry, requestLog, faultLog, callbackLog, engine, nil) // nil metrics

	req := httptest.NewRequest("GET", "/__admin/metrics/prometheus", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/plain; version=0.0.4" {
		t.Errorf("Content-Type = %q, want text/plain; version=0.0.4", ct)
	}
	if w.Body.Len() != 0 {
		t.Errorf("expected empty body, got %q", w.Body.String())
	}
}

func TestCallbacksMethodNotAllowed(t *testing.T) {
	h, _, _ := setupTest()

	req := httptest.NewRequest("POST", "/__admin/callbacks", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for POST on callbacks, got %d", w.Code)
	}
}
