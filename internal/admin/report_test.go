package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sunny809/gochaos/internal/spec"
)

// TestReportEndpointJUnit serves GET /__admin/report?format=junit and checks
// the serialized suite contains a failing testcase per injected fault.
func TestReportEndpointJUnit(t *testing.T) {
	h, _, _ := setupTest()
	h.faultLog.Record(spec.FaultInjectionEntry{
		StubID:        "stub-1",
		FaultType:     "connection_reset",
		RequestMethod: "GET",
		RequestPath:   "/api/test",
	})

	req := httptest.NewRequest("GET", "/__admin/report?format=junit", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/xml") {
		t.Errorf("expected xml content type, got %q", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, `<testsuite name="gmock-chaos" tests="1" failures="1"`) {
		t.Errorf("expected suite header with 1 test and 1 failure, got: %s", body)
	}
	if !strings.Contains(body, `name="connection_reset"`) {
		t.Errorf("expected testcase named after fault type, got: %s", body)
	}
}

// TestReportEndpointJSON serves the report with no format param (defaults to
// json) and checks the structured envelope.
func TestReportEndpointJSON(t *testing.T) {
	h, _, _ := setupTest()
	h.faultLog.Record(spec.FaultInjectionEntry{
		StubID:        "stub-1",
		FaultType:     "delay",
		RequestMethod: "POST",
		RequestPath:   "/api/order",
	})

	req := httptest.NewRequest("GET", "/__admin/report", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("expected json content type, got %q", ct)
	}

	var result struct {
		Suite   string                     `json:"suite"`
		Entries []spec.FaultInjectionEntry `json:"entries"`
	}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if result.Suite != "gmock-chaos" {
		t.Errorf("expected suite=gmock-chaos, got %q", result.Suite)
	}
	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result.Entries))
	}
	if result.Entries[0].StubID != "stub-1" || result.Entries[0].FaultType != "delay" {
		t.Errorf("unexpected entry: %+v", result.Entries[0])
	}
}

// TestReportEndpointEmpty checks the deferred minor: with nothing fired the
// JSON envelope still renders "entries": [] (array, not null) and the JUnit
// suite renders with tests="0".
func TestReportEndpointEmpty(t *testing.T) {
	h, _, _ := setupTest()

	req := httptest.NewRequest("GET", "/__admin/report?format=json", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"entries": []`) {
		t.Errorf("expected empty entries array, got: %s", body)
	}

	req = httptest.NewRequest("GET", "/__admin/report?format=junit", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for junit, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `tests="0"`) {
		t.Errorf("expected tests=\"0\" for empty junit suite, got: %s", w.Body.String())
	}
}

// TestReportEndpointBadFormat surfaces the 400 for an unknown format.
func TestReportEndpointBadFormat(t *testing.T) {
	h, _, _ := setupTest()

	req := httptest.NewRequest("GET", "/__admin/report?format=bogus", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "invalid format: must be junit or json") {
		t.Errorf("expected format error message, got: %s", w.Body.String())
	}
}

// TestReportEndpointMethodNotAllowed rejects non-GET methods on the endpoint.
func TestReportEndpointMethodNotAllowed(t *testing.T) {
	h, _, _ := setupTest()

	req := httptest.NewRequest("POST", "/__admin/report", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}
