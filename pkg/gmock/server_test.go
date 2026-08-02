package gmock_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/sunny809/gochaos/pkg/gmock"
	"github.com/sunny809/gochaos/test/testutil"
)

// noMatchResponse mirrors the on-the-wire 404 body emitted by writeNoMatch in
// pkg/gmock/server.go. It is duplicated here (rather than imported) because the
// type is unexported — tests assert against the public JSON contract only.
type noMatchResponse struct {
	Error      string `json:"error"`
	Method     string `json:"method"`
	Path       string `json:"path"`
	NearMisses []struct {
		StubID        string `json:"stubId"`
		StubName      string `json:"stubName,omitempty"`
		Score         int    `json:"score"`
		MaxScore      int    `json:"maxScore"`
		TopMissReason string `json:"topMissReason"`
	} `json:"nearMisses"`
}

// readNoMatchBody decodes the 404 body and asserts the basic contract:
// status code, content-type, and that nearMisses is present (possibly empty).
func readNoMatchBody(t *testing.T, resp *http.Response) noMatchResponse {
	t.Helper()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", ct)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var body noMatchResponse
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode body %q: %v", string(raw), err)
	}
	return body
}

func TestStubDeleteNotFound(t *testing.T) {
	srv := testutil.StartServer(t)
	deleted := srv.DeleteStub("nonexistent-id")
	if deleted {
		t.Error("expected DeleteStub to return false for unknown ID")
	}
}

func TestClearStubs(t *testing.T) {
	srv := testutil.StartServer(t)
	srv.Stub(gmock.StubDefinition{
		Request:  gmock.RequestPattern{Method: http.MethodGet, URLPath: "/test"},
		Response: gmock.ResponseDefinition{Status: http.StatusOK},
	})

	srv.ClearStubs()

	resp, err := http.Get(srv.URL() + "/test")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 after ClearStubs, got %d", resp.StatusCode)
	}
}

func TestReset(t *testing.T) {
	srv := testutil.StartServer(t)
	srv.Stub(gmock.StubDefinition{
		Request:  gmock.RequestPattern{Method: http.MethodGet, URLPath: "/test"},
		Response: gmock.ResponseDefinition{Status: http.StatusOK},
	})

	// Make a request to populate the request log
	resp, err := http.Get(srv.URL() + "/test")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()

	srv.Reset()

	// After reset, the stub should be gone
	resp, err = http.Get(srv.URL() + "/test")
	if err != nil {
		t.Fatalf("GET after reset: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 after Reset, got %d", resp.StatusCode)
	}
}

func TestStubJSON(t *testing.T) {
	srv := testutil.StartServer(t)
	jsonData := []byte(`{"request":{"method":"GET","urlPath":"/json-stub"},"response":{"status":200,"body":"from-json"}}`)

	id, err := srv.StubJSON(jsonData)
	if err != nil {
		t.Fatalf("StubJSON: %v", err)
	}
	if id == "" {
		t.Error("expected non-empty ID from StubJSON")
	}

	resp, err := http.Get(srv.URL() + "/json-stub")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != "from-json" {
		t.Errorf("expected body 'from-json', got %q", string(body))
	}
}

func TestStubJSONInvalid(t *testing.T) {
	srv := testutil.StartServer(t)
	jsonData := []byte(`not valid json`)

	id, err := srv.StubJSON(jsonData)
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
	if id != "" {
		t.Errorf("expected empty ID for invalid JSON, got %q", id)
	}
}

func TestRequestLog(t *testing.T) {
	srv := testutil.StartServer(t)
	srv.Stub(gmock.StubDefinition{
		Request:  gmock.RequestPattern{Method: http.MethodGet, URLPath: "/test"},
		Response: gmock.ResponseDefinition{Status: http.StatusOK},
	})

	resp, err := http.Get(srv.URL() + "/test")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()

	entries := srv.RequestLog()
	if len(entries) != 1 {
		t.Errorf("expected 1 log entry, got %d", len(entries))
	}
}

func TestUnmatchedRequests(t *testing.T) {
	srv := testutil.StartServer(t)

	resp, err := http.Get(srv.URL() + "/no-match")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()

	unmatched := srv.UnmatchedRequests()
	if len(unmatched) != 1 {
		t.Errorf("expected 1 unmatched request, got %d", len(unmatched))
	}
	if unmatched[0].Path != "/no-match" {
		t.Errorf("expected path /no-match, got %q", unmatched[0].Path)
	}
}
func TestWriteNoMatch_IncludesNearMiss(t *testing.T) {
	srv := testutil.StartServer(t)

	// Register a single stub that the test request will not match.
	srv.Stub(gmock.StubDefinition{
		Name: "create-user",
		Request: gmock.RequestPattern{
			Method:  http.MethodPost,
			URLPath: "/api/users",
		},
		Response: gmock.ResponseDefinition{Status: http.StatusCreated},
	})

	// Send an unmatched request: wrong method AND wrong path.
	resp, err := http.Get(srv.URL() + "/api/orders")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	if resp == nil {
		t.Fatal("nil response")
	}
	defer resp.Body.Close()

	body := readNoMatchBody(t, resp)

	if body.Error != "no matching stub" {
		t.Errorf("expected error 'no matching stub', got %q", body.Error)
	}
	if body.Method != http.MethodGet {
		t.Errorf("expected method GET, got %q", body.Method)
	}
	if body.Path != "/api/orders" {
		t.Errorf("expected path /api/orders, got %q", body.Path)
	}
	if len(body.NearMisses) == 0 {
		t.Fatalf("expected at least one nearMiss entry, got 0; body=%+v", body)
	}

	first := body.NearMisses[0]
	if first.StubID == "" {
		t.Errorf("expected non-empty stubId in first nearMiss, got %+v", first)
	}
	if first.MaxScore <= 0 {
		t.Errorf("expected positive maxScore in first nearMiss, got %d", first.MaxScore)
	}
	if first.TopMissReason == "" {
		t.Errorf("expected non-empty topMissReason for unmatched stub, got %+v", first)
	}
}

// TestWriteNoMatch_EmptyRegistry verifies the empty-registry contract: the
// nearMisses field is an empty (non-nil) array, NOT null and NOT missing.
// Clients should be able to decode and iterate over it unconditionally.
func TestWriteNoMatch_EmptyRegistry(t *testing.T) {
	srv := testutil.StartServer(t)

	resp, err := http.Get(srv.URL() + "/no/such/path")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	if resp == nil {
		t.Fatal("nil response")
	}
	defer resp.Body.Close()

	body := readNoMatchBody(t, resp)

	if body.Error != "no matching stub" {
		t.Errorf("expected error 'no matching stub', got %q", body.Error)
	}
	if body.NearMisses == nil {
		t.Fatalf("expected non-nil empty nearMisses array, got nil")
	}
	if len(body.NearMisses) != 0 {
		t.Errorf("expected empty nearMisses for empty registry, got %d entries", len(body.NearMisses))
	}

	// Re-decode raw bytes to confirm the JSON contains "[]" rather than
	// "null" — the json.Unmarshal above happily decodes both into a nil
	// slice, so a string-level check is the only way to nail this down.
	resp2, err := http.Get(srv.URL() + "/no/such/path")
	if err != nil {
		t.Fatalf("GET (2): %v", err)
	}
	if resp2 == nil {
		t.Fatal("nil response")
	}
	defer resp2.Body.Close()
	raw, err := io.ReadAll(resp2.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var generic map[string]any
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatalf("decode generic: %v", err)
	}
	nm, ok := generic["nearMisses"]
	if !ok {
		t.Fatalf("nearMisses field missing from body: %s", string(raw))
	}
	arr, ok := nm.([]any)
	if !ok {
		t.Fatalf("expected nearMisses to be a JSON array, got %T (%v)", nm, nm)
	}
	if len(arr) != 0 {
		t.Errorf("expected empty JSON array, got %d entries: %v", len(arr), arr)
	}
}

func TestPrometheusEndpoint(t *testing.T) {
	srv := gmock.NewServer(
		gmock.WithPort(0),
		gmock.WithPrometheusEndpoint("/metrics"),
	)
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Stop()

	// Test the custom Prometheus endpoint
	resp, err := http.Get(srv.URL() + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	// Verify Prometheus text format
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "# HELP gochaos_") {
		t.Errorf("expected Prometheus HELP lines, got:\n%s", bodyStr)
	}
	if !strings.Contains(bodyStr, "# TYPE gochaos_") {
		t.Errorf("expected Prometheus TYPE lines, got:\n%s", bodyStr)
	}
	if !strings.Contains(bodyStr, "gochaos_requests_total") {
		t.Errorf("expected gochaos_requests_total metric, got:\n%s", bodyStr)
	}

	// Verify Content-Type header
	contentType := resp.Header.Get("Content-Type")
	if contentType != "text/plain; version=0.0.4" {
		t.Errorf("expected Content-Type 'text/plain; version=0.0.4', got %q", contentType)
	}
}

func TestPrometheusEndpointAdminRoute(t *testing.T) {
	srv := gmock.NewServer(gmock.WithPort(0))
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Stop()

	// Test the admin /__admin/metrics/prometheus route
	resp, err := http.Get(srv.URL() + "/__admin/metrics/prometheus")
	if err != nil {
		t.Fatalf("GET /__admin/metrics/prometheus: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	bodyStr := string(body)
	if !strings.Contains(bodyStr, "gochaos_") {
		t.Errorf("expected gochaos_ metrics, got:\n%s", bodyStr)
	}
}

func TestPrometheusEndpointMethodNotAllowed(t *testing.T) {
	srv := gmock.NewServer(
		gmock.WithPort(0),
		gmock.WithPrometheusEndpoint("/metrics"),
	)
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Stop()

	// POST should return 405
	resp, err := http.Post(srv.URL()+"/metrics", "text/plain", nil)
	if err != nil {
		t.Fatalf("POST /metrics: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for POST, got %d", resp.StatusCode)
	}
}
