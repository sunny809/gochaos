package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sunny809/gochaos/internal/faultlog"
	"github.com/sunny809/gochaos/internal/log"
	"github.com/sunny809/gochaos/internal/nearmiss"
	"github.com/sunny809/gochaos/internal/spec"
	"github.com/sunny809/gochaos/internal/stub"
)

func setupTest() (*Handler, *stub.Registry, *log.RequestLog) {
	registry := stub.NewRegistry()
	requestLog := log.New(100)
	faultLog := faultlog.NewFaultInjectionLog(100)
	engine := nearmiss.NewEngine()
	h := New(registry, requestLog, faultLog, engine)
	return h, registry, requestLog
}

func TestHealth(t *testing.T) {
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
