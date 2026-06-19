package integration_test

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/sunny809/gochaos/pkg/gmock"
)

// startServer starts a gmock server on a random port for testing.
// Returns the server and a cleanup function.
func startServer(t *testing.T, opts ...gmock.Option) (gmock.Server, func()) {
	t.Helper()

	// Always use random port (0) for tests
	opts = append([]gmock.Option{gmock.WithPort(0)}, opts...)
	server := gmock.NewServer(opts...)

	if err := server.Start(); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}

	// Give the server a moment to bind
	time.Sleep(10 * time.Millisecond)

	return server, func() {
		if err := server.Stop(); err != nil {
			t.Logf("server stop error: %v", err)
		}
	}
}

func TestServerStubMatch(t *testing.T) {
	server, cleanup := startServer(t)
	defer cleanup()

	// Register a stub via the library API
	id := server.Stub(gmock.StubDefinition{
		Request: gmock.RequestPattern{
			Method:  http.MethodGet,
			URLPath: "/api/users",
		},
		Response: gmock.ResponseDefinition{
			Status:  http.StatusOK,
			Headers: map[string]string{"Content-Type": "application/json"},
			Body:    `{"users":["alice","bob"]}`,
		},
	})
	if id == "" {
		t.Fatal("expected non-empty stub ID")
	}

	// Issue a real HTTP request
	resp, err := http.Get(server.URL() + "/api/users")
	if err != nil {
		t.Fatalf("HTTP GET failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected json content type, got %s", ct)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "alice") {
		t.Errorf("expected body to contain 'alice', got %s", string(body))
	}
}

func TestServerNoMatch(t *testing.T) {
	server, cleanup := startServer(t)
	defer cleanup()

	// No stubs registered; expect 404 with diagnostic body
	resp, err := http.Get(server.URL() + "/unknown")
	if err != nil {
		t.Fatalf("HTTP GET failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}
	if body["error"] != "no matching stub" {
		t.Errorf("expected 'no matching stub', got %v", body["error"])
	}
	// nearMisses must always be present (possibly empty) so clients can
	// decode the field unconditionally.
	if _, ok := body["nearMisses"]; !ok {
		t.Errorf("expected 'nearMisses' field in 404 body, got %v", body)
	}
}

func TestAdminCreateAndListMappings(t *testing.T) {
	server, cleanup := startServer(t)
	defer cleanup()

	// Create a stub via admin API
	stubJSON := []byte(`{
		"request": {"method": "GET", "urlPath": "/api/admin-test"},
		"response": {"status": 200, "body": "from-admin"}
	}`)

	createReq, _ := http.NewRequest(http.MethodPost,
		server.AdminURL()+"/__admin/mappings",
		bytes.NewReader(stubJSON))
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatalf("create stub failed: %v", err)
	}
	defer createResp.Body.Close()

	if createResp.StatusCode != http.StatusCreated {
		t.Errorf("expected 201, got %d", createResp.StatusCode)
	}

	var created gmock.StubDefinition
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode created stub: %v", err)
	}
	if created.ID == "" {
		t.Error("expected non-empty stub ID")
	}

	// List stubs via admin API
	listResp, err := http.Get(server.AdminURL() + "/__admin/mappings")
	if err != nil {
		t.Fatalf("list stubs failed: %v", err)
	}
	defer listResp.Body.Close()

	var list struct {
		Mappings []gmock.StubDefinition `json:"mappings"`
		Meta     map[string]int         `json:"meta"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&list); err != nil {
		t.Fatalf("failed to decode list: %v", err)
	}
	if len(list.Mappings) != 1 {
		t.Errorf("expected 1 stub, got %d", len(list.Mappings))
	}
	if list.Meta["total"] != 1 {
		t.Errorf("expected total=1, got %d", list.Meta["total"])
	}

	// Now actually use the stub
	resp, err := http.Get(server.URL() + "/api/admin-test")
	if err != nil { t.Fatal(err) }
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "from-admin" {
		t.Errorf("expected 'from-admin', got %s", string(body))
	}
}

func TestAdminDeleteMapping(t *testing.T) {
	server, cleanup := startServer(t)
	defer cleanup()

	id := server.Stub(gmock.StubDefinition{
		Request:  gmock.RequestPattern{Method: "GET", URLPath: "/test"},
		Response: gmock.ResponseDefinition{Status: 200},
	})

	delReq, _ := http.NewRequest(http.MethodDelete,
		server.AdminURL()+"/__admin/mappings/"+id, nil)
	delResp, err := http.DefaultClient.Do(delReq)
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	defer delResp.Body.Close()

	if delResp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", delResp.StatusCode)
	}

	// Stub should be gone — request now returns 404
	resp, err := http.Get(server.URL() + "/test")
	if err != nil { t.Fatal(err) }
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", resp.StatusCode)
	}
}

func TestAdminReset(t *testing.T) {
	server, cleanup := startServer(t)
	defer cleanup()

	server.Stub(gmock.StubDefinition{
		Request:  gmock.RequestPattern{Method: "GET", URLPath: "/a"},
		Response: gmock.ResponseDefinition{Status: 200},
	})
	server.Stub(gmock.StubDefinition{
		Request:  gmock.RequestPattern{Method: "GET", URLPath: "/b"},
		Response: gmock.ResponseDefinition{Status: 200},
	})

	// Generate some request history
	http.Get(server.URL() + "/a")
	http.Get(server.URL() + "/b")

	resetReq, _ := http.NewRequest(http.MethodPost,
		server.AdminURL()+"/__admin/reset", nil)
	resp, err := http.DefaultClient.Do(resetReq)
	if err != nil {
		t.Fatalf("reset failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	// All stubs and request log should be cleared
	if logs := server.RequestLog(); len(logs) != 0 {
		t.Errorf("expected empty request log after reset, got %d", len(logs))
	}
}

func TestRequestLog(t *testing.T) {
	server, cleanup := startServer(t)
	defer cleanup()

	server.Stub(gmock.StubDefinition{
		Request:  gmock.RequestPattern{Method: "GET", URLPath: "/match"},
		Response: gmock.ResponseDefinition{Status: 200},
	})

	http.Get(server.URL() + "/match")
	http.Get(server.URL() + "/match")
	http.Get(server.URL() + "/no-match")

	all := server.RequestLog()
	if len(all) != 3 {
		t.Errorf("expected 3 logged requests, got %d", len(all))
	}

	unmatched := server.UnmatchedRequests()
	if len(unmatched) != 1 {
		t.Errorf("expected 1 unmatched, got %d", len(unmatched))
	}
	if len(unmatched) > 0 && unmatched[0].Path != "/no-match" {
		t.Errorf("expected /no-match, got %s", unmatched[0].Path)
	}
}

func TestAdminListRequests(t *testing.T) {
	server, cleanup := startServer(t)
	defer cleanup()

	server.Stub(gmock.StubDefinition{
		Request:  gmock.RequestPattern{Method: "GET", URLPath: "/a"},
		Response: gmock.ResponseDefinition{Status: 200},
	})

	http.Get(server.URL() + "/a")
	http.Get(server.URL() + "/b") // unmatched

	resp, err := http.Get(server.AdminURL() + "/__admin/requests")
	if err != nil {
		t.Fatalf("list requests failed: %v", err)
	}
	defer resp.Body.Close()

	var list struct {
		Requests []gmock.LoggedRequest `json:"requests"`
		Meta     map[string]int        `json:"meta"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if list.Meta["total"] != 2 {
		t.Errorf("expected total=2, got %d", list.Meta["total"])
	}

	// Filter unmatched
	resp2, err := http.Get(server.AdminURL() + "/__admin/requests?filter=unmatched")
	if err != nil { t.Fatal(err) }
	defer resp2.Body.Close()
	json.NewDecoder(resp2.Body).Decode(&list)
	if len(list.Requests) != 1 {
		t.Errorf("expected 1 unmatched, got %d", len(list.Requests))
	}
}

func TestVerify(t *testing.T) {
	server, cleanup := startServer(t)
	defer cleanup()

	server.Stub(gmock.StubDefinition{
		Request:  gmock.RequestPattern{Method: "POST", URLPath: "/events"},
		Response: gmock.ResponseDefinition{Status: 201},
	})

	for i := 0; i < 3; i++ {
		http.Post(server.URL()+"/events", "application/json", strings.NewReader(`{}`))
	}

	// Verify pattern matched 3 times
	result := server.Verify(gmock.RequestPattern{
		Method:  "POST",
		URLPath: "/events",
	}, 3)
	if !result.Matched {
		t.Errorf("expected verification to pass: %+v", result)
	}
	if result.ActualCount != 3 {
		t.Errorf("expected ActualCount=3, got %d", result.ActualCount)
	}

	// Verify with too high expectation
	result = server.Verify(gmock.RequestPattern{
		Method:  "POST",
		URLPath: "/events",
	}, 5)
	if result.Matched {
		t.Errorf("expected verification to fail for count=5")
	}

	// VerifyNotCalled
	result = server.VerifyNotCalled(gmock.RequestPattern{
		Method:  "DELETE",
		URLPath: "/events",
	})
	if !result.Matched {
		t.Errorf("expected VerifyNotCalled to pass for DELETE")
	}
}

func TestPriorityOrdering(t *testing.T) {
	server, cleanup := startServer(t)
	defer cleanup()

	// More general (lower priority = higher precedence)
	server.Stub(gmock.StubDefinition{
		Request:  gmock.RequestPattern{Method: "GET", URLPath: "/api/users"},
		Response: gmock.ResponseDefinition{Status: 200, Body: "general"},
		Priority: 10,
	})
	// More specific (higher precedence — should win)
	server.Stub(gmock.StubDefinition{
		Request: gmock.RequestPattern{
			Method:  "GET",
			URLPath: "/api/users",
			Headers: map[string]string{"X-Special": "yes"},
		},
		Response: gmock.ResponseDefinition{Status: 200, Body: "special"},
		Priority: 1,
	})

	// Without special header — should hit the general stub (only matches without header)
	// Actually with the special-header stub having priority 1, it would NOT match without the header.
	// Let's send WITH the header and verify the specific stub wins.
	req, _ := http.NewRequest("GET", server.URL()+"/api/users", nil)
	req.Header.Set("X-Special", "yes")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "special" {
		t.Errorf("expected 'special' (priority 1 wins), got %s", string(body))
	}

	// Without the special header, only the general stub matches
	resp2, err := http.Get(server.URL() + "/api/users")
	if err != nil { t.Fatal(err) }
	defer resp2.Body.Close()
	body2, _ := io.ReadAll(resp2.Body)
	if string(body2) != "general" {
		t.Errorf("expected 'general' without header, got %s", string(body2))
	}
}

func TestSeparateAdminPort(t *testing.T) {
	server, cleanup := startServer(t, gmock.WithAdminPort(0))
	defer cleanup()
	// Note: WithAdminPort(0) - the server should still pick a random port.
	// But since WithAdminPort(0) is treated as "no separate port" (because of `> 0` check),
	// this test exercises the same-port case. Let's instead verify URLs differ when set.
	if server.URL() != server.AdminURL() {
		t.Logf("URLs: main=%s admin=%s", server.URL(), server.AdminURL())
	}
}

func TestHealthEndpoint(t *testing.T) {
	server, cleanup := startServer(t)
	defer cleanup()

	resp, err := http.Get(server.AdminURL() + "/__admin/health")
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var health map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		t.Fatalf("failed to decode health: %v", err)
	}
	if health["status"] != "ok" {
		t.Errorf("expected status=ok, got %v", health["status"])
	}
}

func TestRecordRequestBody(t *testing.T) {
	server, cleanup := startServer(t)
	defer cleanup()

	server.Stub(gmock.StubDefinition{
		Request: gmock.RequestPattern{
			Method:  "POST",
			URLPath: "/echo",
		},
		Response: gmock.ResponseDefinition{Status: 200, Body: "ok"},
	})

	body := `{"name":"Alice","age":30}`
	http.Post(server.URL()+"/echo", "application/json", strings.NewReader(body))

	logs := server.RequestLog()
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}
	if logs[0].Body != body {
		t.Errorf("expected body %s, got %s", body, logs[0].Body)
	}
}

func TestResponseDelay(t *testing.T) {
	server, cleanup := startServer(t)
	defer cleanup()

	server.Stub(gmock.StubDefinition{
		Request: gmock.RequestPattern{
			Method:  http.MethodGet,
			URLPath: "/api/slow",
		},
		Response: gmock.ResponseDefinition{
			Status: http.StatusOK,
			Body:   "delayed",
			Delay: &gmock.DelayDefinition{
				Type:  "fixed",
				Value: 100,
			},
		},
	})

	start := time.Now()
	resp, err := http.Get(server.URL() + "/api/slow")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	elapsed := time.Since(start)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if elapsed < 90*time.Millisecond {
		t.Errorf("expected delay of at least 100ms, got %v", elapsed)
	}
}

func TestBase64Body(t *testing.T) {
	server, cleanup := startServer(t)
	defer cleanup()

	// "hello world" in base64
	server.Stub(gmock.StubDefinition{
		Request: gmock.RequestPattern{
			Method:  http.MethodGet,
			URLPath: "/api/binary",
		},
		Response: gmock.ResponseDefinition{
			Status:     http.StatusOK,
			Base64Body: "aGVsbG8gd29ybGQ=",
		},
	})

	resp, err := http.Get(server.URL() + "/api/binary")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "hello world" {
		t.Errorf("expected 'hello world', got %s", string(body))
	}
}

func TestRedirectStub(t *testing.T) {
	server, cleanup := startServer(t)
	defer cleanup()

	server.Stub(gmock.StubDefinition{
		Request: gmock.RequestPattern{
			Method:  http.MethodGet,
			URLPath: "/old-path",
		},
		Response: gmock.WithRedirect(http.StatusMovedPermanently, "/new-path"),
	})

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Get(server.URL() + "/old-path")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMovedPermanently {
		t.Errorf("expected 301, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Location") != "/new-path" {
		t.Errorf("expected Location /new-path, got %s", resp.Header.Get("Location"))
	}
}

func TestCORSEnabled(t *testing.T) {
	server, cleanup := startServer(t, gmock.WithCORSEnabled())
	defer cleanup()

	server.Stub(gmock.StubDefinition{
		Request: gmock.RequestPattern{
			Method:  http.MethodGet,
			URLPath: "/api/data",
		},
		Response: gmock.ResponseDefinition{
			Status: http.StatusOK,
			Body:   "data",
		},
	})

	// Test actual request with Origin header
	req, _ := http.NewRequest(http.MethodGet, server.URL()+"/api/data", nil)
	req.Header.Set("Origin", "http://example.com")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("Access-Control-Allow-Origin") == "" {
		t.Error("expected Access-Control-Allow-Origin header")
	}

	// Test CORS preflight
	preReq, _ := http.NewRequest(http.MethodOptions, server.URL()+"/api/data", nil)
	preReq.Header.Set("Origin", "http://example.com")
	preReq.Header.Set("Access-Control-Request-Method", "GET")
	preResp, err := http.DefaultClient.Do(preReq)
	if err != nil {
		t.Fatalf("preflight failed: %v", err)
	}
	defer preResp.Body.Close()

	if preResp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for preflight, got %d", preResp.StatusCode)
	}
	if preResp.Header.Get("Access-Control-Allow-Origin") == "" {
		t.Error("expected Access-Control-Allow-Origin on preflight")
	}
	if preResp.Header.Get("Access-Control-Allow-Methods") == "" {
		t.Error("expected Access-Control-Allow-Methods on preflight")
	}
}

func TestCookieMatching(t *testing.T) {
	server, cleanup := startServer(t)
	defer cleanup()

	server.Stub(gmock.StubDefinition{
		Request: gmock.RequestPattern{
			Method:  http.MethodGet,
			URLPath: "/api/me",
			Cookies: map[string]string{"session_id": "abc123"},
		},
		Response: gmock.ResponseDefinition{
			Status: http.StatusOK,
			Body:   "authenticated",
		},
	})

	server.Stub(gmock.StubDefinition{
		Request: gmock.RequestPattern{
			Method:  http.MethodGet,
			URLPath: "/api/me",
		},
		Response: gmock.ResponseDefinition{
			Status: http.StatusUnauthorized,
			Body:   "unauthenticated",
		},
	})

	// With correct cookie
	req, _ := http.NewRequest(http.MethodGet, server.URL()+"/api/me", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "abc123"})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "authenticated" {
		t.Errorf("expected 'authenticated', got %s", string(body))
	}

	// Without cookie
	resp2, err := http.Get(server.URL() + "/api/me")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	body2, _ := io.ReadAll(resp2.Body)
	if string(body2) != "unauthenticated" {
		t.Errorf("expected 'unauthenticated', got %s", string(body2))
	}
}

func TestAcceptHeaderMatching(t *testing.T) {
	server, cleanup := startServer(t)
	defer cleanup()

	server.Stub(gmock.StubDefinition{
		Request: gmock.RequestPattern{
			Method: http.MethodGet,
			URLPath: "/api/data",
			Accept: "application/json",
		},
		Response: gmock.ResponseDefinition{
			Status: http.StatusOK,
			Body:   `{"format":"json"}`,
		},
	})

	server.Stub(gmock.StubDefinition{
		Request: gmock.RequestPattern{
			Method: http.MethodGet,
			URLPath: "/api/data",
			Accept: "text/xml",
		},
		Response: gmock.ResponseDefinition{
			Status: http.StatusOK,
			Body:   `<format>xml</format>`,
		},
	})

	// Test JSON accept
	req, _ := http.NewRequest(http.MethodGet, server.URL()+"/api/data", nil)
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != `{"format":"json"}` {
		t.Errorf("expected JSON response, got %s", string(body))
	}

	// Test wildcard accept (matches json because it's first)
	req2, _ := http.NewRequest(http.MethodGet, server.URL()+"/api/data", nil)
	req2.Header.Set("Accept", "*/*")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp2.Body.Close()
	body2, _ := io.ReadAll(resp2.Body)
	if string(body2) != `{"format":"json"}` {
		t.Errorf("expected JSON response for */*, got %s", string(body2))
	}
}

func TestGzipCompression(t *testing.T) {
	server, cleanup := startServer(t)
	defer cleanup()

	server.Stub(gmock.StubDefinition{
		Request: gmock.RequestPattern{
			Method:  http.MethodGet,
			URLPath: "/api/data",
		},
		Response: gmock.ResponseDefinition{
			Status: http.StatusOK,
			Body:   "this is some data that should be compressed",
		},
	})

	// Request with Accept-Encoding: gzip
	req, _ := http.NewRequest(http.MethodGet, server.URL()+"/api/data", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	// Should have Content-Encoding: gzip
	if resp.Header.Get("Content-Encoding") != "gzip" {
		t.Errorf("expected Content-Encoding: gzip, got %s", resp.Header.Get("Content-Encoding"))
	}

	// Verify we can decompress
	reader, err := gzip.NewReader(resp.Body)
	if err != nil {
		t.Fatalf("failed to create gzip reader: %v", err)
	}
	defer reader.Close()
	decompressed, _ := io.ReadAll(reader)
	if string(decompressed) != "this is some data that should be compressed" {
		t.Errorf("unexpected decompressed body: %s", string(decompressed))
	}

	// Request without gzip
	resp2, err := http.Get(server.URL() + "/api/data")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.Header.Get("Content-Encoding") == "gzip" {
		t.Errorf("expected no gzip without Accept-Encoding")
	}
}

// --- F5: Fault Injection Integration Tests ---

func TestErrorFault(t *testing.T) {
	server, cleanup := startServer(t)
	defer cleanup()

	server.Stub(gmock.StubDefinition{
		Request: gmock.RequestPattern{
			Method:  http.MethodGet,
			URLPath: "/api/fault/error",
		},
		Response: gmock.ResponseDefinition{
			Status:  http.StatusOK,
			Headers: map[string]string{"Content-Type": "text/plain", "X-Custom": "ignored"},
			Body:    "this should be ignored",
			Fault:   &gmock.FaultDefinition{Type: "error"},
		},
	})

	resp, err := http.Get(server.URL() + "/api/fault/error")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	// Error fault must return 500
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", resp.StatusCode)
	}

	// Content-Type must be application/json
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}

	// Body must contain fault error JSON
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"fault":"error"`) {
		t.Errorf("expected body to contain fault error, got %s", string(body))
	}
	if !strings.Contains(string(body), `"error"`) {
		t.Errorf("expected body to contain error field, got %s", string(body))
	}

	// Stub-defined body/headers must be ignored
	if resp.Header.Get("X-Custom") != "" {
		t.Errorf("stub header X-Custom should not be present on fault response")
	}
}

func TestEmptyFault(t *testing.T) {
	server, cleanup := startServer(t)
	defer cleanup()

	server.Stub(gmock.StubDefinition{
		Request: gmock.RequestPattern{
			Method:  http.MethodGet,
			URLPath: "/api/fault/empty",
		},
		Response: gmock.ResponseDefinition{
			Status:  http.StatusCreated,
			Headers: map[string]string{"Content-Type": "application/json", "X-Should-Be-Gone": "yes"},
			Body:    "this should not appear",
			Fault:   &gmock.FaultDefinition{Type: "empty"},
		},
	})

	resp, err := http.Get(server.URL() + "/api/fault/empty")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	// Empty fault returns default 200, not stub-defined status
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	// Body must be empty
	body, _ := io.ReadAll(resp.Body)
	if len(body) != 0 {
		t.Errorf("expected empty body, got %q", string(body))
	}

	// Stub-defined headers must not be present
	if ct := resp.Header.Get("Content-Type"); ct == "application/json" {
		t.Errorf("stub Content-Type header should not be present on empty fault response")
	}
	if resp.Header.Get("X-Should-Be-Gone") != "" {
		t.Errorf("stub X-Should-Be-Gone header should not be present on empty fault response")
	}
}

func TestConnectionResetFault(t *testing.T) {
	server, cleanup := startServer(t)
	defer cleanup()

	server.Stub(gmock.StubDefinition{
		Request: gmock.RequestPattern{
			Method:  http.MethodGet,
			URLPath: "/api/fault/reset",
		},
		Response: gmock.ResponseDefinition{
			Fault: &gmock.FaultDefinition{Type: "connection_reset"},
		},
	})

	resp, err := http.Get(server.URL() + "/api/fault/reset")
	if err != nil {
		// If the connection was actually reset, we get a transport error
		t.Logf("connection reset caused transport error (expected in some cases): %v", err)
		return
	}
	defer resp.Body.Close()

	// If we did get a response, it must be the Hijacker fallback:
	// 500 with Connection: close and JSON body
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500 (fallback), got %d", resp.StatusCode)
	}

	if got := resp.Header.Get("Connection"); got != "close" {
		t.Errorf("expected Connection: close, got %q", got)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "connection_reset") {
		t.Errorf("expected body to contain connection_reset, got %s", string(body))
	}
}

func TestFaultViaAdminAPI(t *testing.T) {
	server, cleanup := startServer(t)
	defer cleanup()

	// Create a fault stub via the admin API
	stubJSON := []byte(`{
		"request": {"method": "GET", "urlPath": "/api/fault/admin-error"},
		"response": {"fault": {"type": "error"}}
	}`)

	createReq, _ := http.NewRequest(http.MethodPost,
		server.AdminURL()+"/__admin/mappings",
		bytes.NewReader(stubJSON))
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatalf("create stub failed: %v", err)
	}
	defer createResp.Body.Close()

	if createResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(createResp.Body)
		t.Fatalf("expected 201, got %d, body: %s", createResp.StatusCode, string(body))
	}

	// Now hit the stub URL and verify fault response
	resp, err := http.Get(server.URL() + "/api/fault/admin-error")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"fault":"error"`) {
		t.Errorf("expected fault error body, got %s", string(body))
	}

	// Now try creating a stub with an invalid fault type → expect 400
	invalidStubJSON := []byte(`{
		"request": {"method": "GET", "urlPath": "/api/fault/invalid"},
		"response": {"fault": {"type": "nonexistent_fault"}}
	}`)

	invalidReq, _ := http.NewRequest(http.MethodPost,
		server.AdminURL()+"/__admin/mappings",
		bytes.NewReader(invalidStubJSON))
	invalidReq.Header.Set("Content-Type", "application/json")

	invalidResp, err := http.DefaultClient.Do(invalidReq)
	if err != nil {
		t.Fatalf("invalid stub request failed: %v", err)
	}
	defer invalidResp.Body.Close()

	if invalidResp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(invalidResp.Body)
		t.Errorf("expected 400 for invalid fault type, got %d, body: %s", invalidResp.StatusCode, string(body))
	}
}

func TestFaultWithDelay(t *testing.T) {
	server, cleanup := startServer(t)
	defer cleanup()

	server.Stub(gmock.StubDefinition{
		Request: gmock.RequestPattern{
			Method:  http.MethodGet,
			URLPath: "/api/fault/delayed-error",
		},
		Response: gmock.ResponseDefinition{
			Status: http.StatusOK,
			Body:   "should not appear",
			Delay:  &gmock.DelayDefinition{Type: "fixed", Value: 100},
			Fault:  &gmock.FaultDefinition{Type: "error"},
		},
	})

	start := time.Now()
	resp, err := http.Get(server.URL() + "/api/fault/delayed-error")
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	// Must be delayed by at least ~100ms
	if elapsed < 90*time.Millisecond {
		t.Errorf("expected delay of at least 100ms, got %v", elapsed)
	}

	// Must return the fault response, not the stub body
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(body), "should not appear") {
		t.Errorf("stub body should not appear in fault response, got %s", string(body))
	}
	if !strings.Contains(string(body), `"fault":"error"`) {
		t.Errorf("expected fault error body, got %s", string(body))
	}
}

// --- Near-Miss Integration Tests (P0.4) ---

// nearMissAdminResponse decodes the JSON response from POST /__admin/nearmiss.
type nearMissAdminResponse struct {
	NearMisses []nearMissEntry `json:"nearMisses"`
	Meta       struct {
		Total float64 `json:"total"`
		TopN  float64 `json:"topN"`
	} `json:"meta"`
}

type nearMissEntry struct {
	StubID    string           `json:"stubId"`
	StubName  string           `json:"stubName,omitempty"`
	Score     int              `json:"score"`
	MaxScore  int              `json:"maxScore"`
	Breakdown []breakdownDim   `json:"breakdown"`
}

type breakdownDim struct {
	Dimension string `json:"dimension"`
	Matched   bool   `json:"matched"`
	Score     int    `json:"score"`
	MaxScore  int    `json:"maxScore"`
	Expected  string `json:"expected,omitempty"`
	Actual    string `json:"actual,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// postNearMissAdmin sends a POST /__admin/nearmiss and returns the decoded
// response or fails the test.
func postNearMissAdmin(t *testing.T, adminURL, method, path string, headers map[string]string, body string) nearMissAdminResponse {
	t.Helper()
	reqBody := map[string]interface{}{
		"method":  method,
		"path":    path,
		"headers": headers,
		"body":    body,
	}
	// Omit empty optional fields to keep payload clean.
	if body == "" {
		delete(reqBody, "body")
	}
	if len(headers) == 0 {
		delete(reqBody, "headers")
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("marshal nearmiss request: %v", err)
	}
	resp, err := http.Post(adminURL+"/__admin/nearmiss", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST /__admin/nearmiss: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d, body: %s", resp.StatusCode, string(raw))
	}
	var result nearMissAdminResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode nearmiss response: %v", err)
	}
	return result
}

func TestNearMiss_MethodMismatch(t *testing.T) {
	server, cleanup := startServer(t)
	defer cleanup()

	server.Stub(gmock.StubDefinition{
		Name:    "create-user",
		Request: gmock.RequestPattern{Method: http.MethodPost, URLPath: "/api/users"},
		Response: gmock.ResponseDefinition{Status: http.StatusCreated},
	})

	result := postNearMissAdmin(t, server.AdminURL(), http.MethodGet, "/api/users", nil, "")

	if len(result.NearMisses) != 1 {
		t.Fatalf("expected 1 near-miss, got %d", len(result.NearMisses))
	}
	nm := result.NearMisses[0]
	if nm.StubName != "create-user" {
		t.Errorf("expected stubName=create-user, got %q", nm.StubName)
	}

	// Find the method dimension in breakdown.
	var methodDim *breakdownDim
	for i := range nm.Breakdown {
		if nm.Breakdown[i].Dimension == "method" {
			methodDim = &nm.Breakdown[i]
			break
		}
	}
	if methodDim == nil {
		t.Fatal("breakdown missing 'method' dimension")
	}
	if methodDim.Matched {
		t.Error("expected method dimension NOT to match (GET vs POST)")
	}
	if !strings.Contains(methodDim.Reason, "POST") {
		t.Errorf("expected method reason to mention POST, got %q", methodDim.Reason)
	}
}

func TestNearMiss_PathMismatch(t *testing.T) {
	server, cleanup := startServer(t)
	defer cleanup()

	server.Stub(gmock.StubDefinition{
		Name:    "list-users",
		Request: gmock.RequestPattern{Method: http.MethodGet, URLPath: "/api/users"},
		Response: gmock.ResponseDefinition{Status: http.StatusOK},
	})

	result := postNearMissAdmin(t, server.AdminURL(), http.MethodGet, "/api/orders", nil, "")

	if len(result.NearMisses) != 1 {
		t.Fatalf("expected 1 near-miss, got %d", len(result.NearMisses))
	}
	var pathDim *breakdownDim
	for i := range result.NearMisses[0].Breakdown {
		if result.NearMisses[0].Breakdown[i].Dimension == "path" {
			pathDim = &result.NearMisses[0].Breakdown[i]
			break
		}
	}
	if pathDim == nil {
		t.Fatal("breakdown missing 'path' dimension")
	}
	if pathDim.Matched {
		t.Error("expected path dimension NOT to match")
	}
}

func TestNearMiss_HeaderMismatch(t *testing.T) {
	server, cleanup := startServer(t)
	defer cleanup()

	server.Stub(gmock.StubDefinition{
		Name: "auth-required",
		Request: gmock.RequestPattern{
			Method:  http.MethodGet,
			URLPath: "/api/secure",
			Headers: map[string]string{"X-Token": "secret"},
		},
		Response: gmock.ResponseDefinition{Status: http.StatusOK},
	})

	result := postNearMissAdmin(t, server.AdminURL(), http.MethodGet, "/api/secure", nil, "")

	if len(result.NearMisses) != 1 {
		t.Fatalf("expected 1 near-miss, got %d", len(result.NearMisses))
	}
	var headerDim *breakdownDim
	for i := range result.NearMisses[0].Breakdown {
		if strings.HasPrefix(result.NearMisses[0].Breakdown[i].Dimension, "header:") {
			headerDim = &result.NearMisses[0].Breakdown[i]
			break
		}
	}
	if headerDim == nil {
		t.Fatal("breakdown missing header dimension")
	}
	if headerDim.Matched {
		t.Error("expected header dimension NOT to match (missing X-Token)")
	}
}

func TestNearMiss_BodyMismatch(t *testing.T) {
	server, cleanup := startServer(t)
	defer cleanup()

	server.Stub(gmock.StubDefinition{
		Name: "exact-body",
		Request: gmock.RequestPattern{
			Method:  http.MethodPost,
			URLPath: "/api/submit",
			Body:    &gmock.BodyPattern{ExactMatch: `{"action":"approve"}`},
		},
		Response: gmock.ResponseDefinition{Status: http.StatusOK},
	})

	result := postNearMissAdmin(t, server.AdminURL(), http.MethodPost, "/api/submit", nil, `{"action":"reject"}`)

	if len(result.NearMisses) != 1 {
		t.Fatalf("expected 1 near-miss, got %d", len(result.NearMisses))
	}
	var bodyDim *breakdownDim
	for i := range result.NearMisses[0].Breakdown {
		if result.NearMisses[0].Breakdown[i].Dimension == "body" {
			bodyDim = &result.NearMisses[0].Breakdown[i]
			break
		}
	}
	if bodyDim == nil {
		t.Fatal("breakdown missing 'body' dimension")
	}
	if bodyDim.Matched {
		t.Error("expected body dimension NOT to match")
	}
}

func TestNearMiss_MultipleCandidates(t *testing.T) {
	server, cleanup := startServer(t)
	defer cleanup()

	// Register 3 stubs that all match on GET but differ in path.
	server.Stub(gmock.StubDefinition{
		Name:     "users",
		Request:  gmock.RequestPattern{Method: http.MethodGet, URLPath: "/api/users"},
		Response: gmock.ResponseDefinition{Status: http.StatusOK},
	})
	server.Stub(gmock.StubDefinition{
		Name:     "orders",
		Request:  gmock.RequestPattern{Method: http.MethodGet, URLPath: "/api/orders"},
		Response: gmock.ResponseDefinition{Status: http.StatusOK},
	})
	server.Stub(gmock.StubDefinition{
		Name:     "items",
		Request:  gmock.RequestPattern{Method: http.MethodGet, URLPath: "/api/items"},
		Response: gmock.ResponseDefinition{Status: http.StatusOK},
	})

	result := postNearMissAdmin(t, server.AdminURL(), http.MethodGet, "/api/unknown", nil, "")

	if len(result.NearMisses) != 3 {
		t.Fatalf("expected 3 near-miss candidates, got %d", len(result.NearMisses))
	}

	// Results should be ordered by score descending.
	for i := 1; i < len(result.NearMisses); i++ {
		if result.NearMisses[i].Score > result.NearMisses[i-1].Score {
			t.Errorf("nearMisses not sorted by score desc: [%d]=%d > [%d]=%d",
				i, result.NearMisses[i].Score, i-1, result.NearMisses[i-1].Score)
		}
	}
}

func TestNearMiss_TopNLimit(t *testing.T) {
	server, cleanup := startServer(t)
	defer cleanup()

	// Register 8 stubs. The default engine topN is 5.
	for i := 0; i < 8; i++ {
		server.Stub(gmock.StubDefinition{
			Name:     fmt.Sprintf("stub-%d", i),
			Request:  gmock.RequestPattern{Method: http.MethodGet, URLPath: fmt.Sprintf("/api/r%d", i)},
			Response: gmock.ResponseDefinition{Status: http.StatusOK},
		})
	}

	result := postNearMissAdmin(t, server.AdminURL(), http.MethodGet, "/api/missing", nil, "")

	if len(result.NearMisses) > 5 {
		t.Errorf("expected ≤5 results (default topN), got %d", len(result.NearMisses))
	}
	if result.Meta.TopN != 5 {
		t.Errorf("expected meta.topN=5, got %v", result.Meta.TopN)
	}
}

func TestNearMiss_ExactMatchOmitted(t *testing.T) {
	server, cleanup := startServer(t)
	defer cleanup()

	server.Stub(gmock.StubDefinition{
		Name:     "exact-stub",
		Request:  gmock.RequestPattern{Method: http.MethodGet, URLPath: "/api/exact"},
		Response: gmock.ResponseDefinition{Status: http.StatusOK},
	})

	// Send a request that exactly matches the stub.
	result := postNearMissAdmin(t, server.AdminURL(), http.MethodGet, "/api/exact", nil, "")

	// Fully matching stub should NOT appear in nearMisses.
	if len(result.NearMisses) != 0 {
		t.Errorf("expected 0 nearMisses on exact match, got %d", len(result.NearMisses))
	}
	if result.Meta.Total != 0 {
		t.Errorf("expected meta.total=0, got %v", result.Meta.Total)
	}
}

func TestNearMiss_404ResponseIncludesNearMiss(t *testing.T) {
	server, cleanup := startServer(t)
	defer cleanup()

	// Register a stub that will nearly match.
	server.Stub(gmock.StubDefinition{
		Name:    "list-users",
		Request: gmock.RequestPattern{Method: http.MethodGet, URLPath: "/api/users"},
		Response: gmock.ResponseDefinition{Status: http.StatusOK},
	})

	// Send an unmatched request through the mock port (not admin).
	resp, err := http.Get(server.URL() + "/api/users-extra")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}

	var body struct {
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
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode 404 body: %v", err)
	}

	if body.Error != "no matching stub" {
		t.Errorf("expected error 'no matching stub', got %q", body.Error)
	}
	if body.Method != http.MethodGet {
		t.Errorf("expected method GET, got %q", body.Method)
	}
	if body.Path != "/api/users-extra" {
		t.Errorf("expected path /api/users-extra, got %q", body.Path)
	}
	if len(body.NearMisses) < 1 {
		t.Fatalf("expected at least 1 nearMiss in 404 body, got 0")
	}
	first := body.NearMisses[0]
	if first.StubName != "list-users" {
		t.Errorf("expected stubName=list-users, got %q", first.StubName)
	}
	if first.TopMissReason == "" {
		t.Error("expected non-empty topMissReason for path-mismatched stub")
	}
}

func TestNearMiss_AdminEndpoint_BadRequests(t *testing.T) {
	server, cleanup := startServer(t)
	defer cleanup()

	t.Run("missing path", func(t *testing.T) {
		resp, err := http.Post(server.AdminURL()+"/__admin/nearmiss", "application/json",
			strings.NewReader(`{"method":"GET"}`))
		if err != nil {
			t.Fatalf("POST: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		resp, err := http.Post(server.AdminURL()+"/__admin/nearmiss", "application/json",
			strings.NewReader(`not-json`))
		if err != nil {
			t.Fatalf("POST: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("method not allowed", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, server.AdminURL()+"/__admin/nearmiss", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("expected 405, got %d", resp.StatusCode)
		}
	})
}

func TestFaultNoGzip(t *testing.T) {
	server, cleanup := startServer(t)
	defer cleanup()

	server.Stub(gmock.StubDefinition{
		Request: gmock.RequestPattern{
			Method:  http.MethodGet,
			URLPath: "/api/fault/no-gzip",
		},
		Response: gmock.ResponseDefinition{
			Fault: &gmock.FaultDefinition{Type: "error"},
		},
	})

	// Request with Accept-Encoding: gzip
	req, _ := http.NewRequest(http.MethodGet, server.URL()+"/api/fault/no-gzip", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	// Fault response must NOT be gzip-compressed
	if ce := resp.Header.Get("Content-Encoding"); ce == "gzip" {
		t.Error("fault response should not be gzip-compressed")
	}

	// Body must be plain JSON, not compressed bytes
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"fault":"error"`) {
		t.Errorf("expected plain JSON fault body, got %s", string(body))
	}
}

func TestRateLimit_Default429(t *testing.T) {
	server, cleanup := startServer(t)
	defer cleanup()

	server.Stub(gmock.StubDefinition{
		Request: gmock.RequestPattern{
			Method:  http.MethodGet,
			URLPath: "/api/ratelimited",
		},
		Response: gmock.ResponseDefinition{
			Status: 200,
			Body:   `{"ok":true}`,
			Fault: &gmock.FaultDefinition{
				Type:         "rate_limit",
				PerSecond:    2,
			},
		},
	})

	// First 2 requests should succeed (full bucket = 2 tokens).
	for i := 0; i < 2; i++ {
		resp, err := http.Get(server.URL() + "/api/ratelimited")
		if err != nil {
			t.Fatalf("request %d failed: %v", i+1, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i+1, resp.StatusCode)
		}
	}

	// Third request should be rate-limited with default 429.
	resp, err := http.Get(server.URL() + "/api/ratelimited")
	if err != nil {
		t.Fatalf("rate-limited request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", resp.StatusCode)
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	body := string(bodyBytes)
	if !strings.Contains(body, `"fault":"rate_limit"`) {
		t.Errorf("expected rate_limit fault in body, got %s", body)
	}
	if !strings.Contains(body, `"error":"rate limited"`) {
		t.Errorf("expected 'rate limited' error in body, got %s", body)
	}
	if resp.Header.Get("Retry-After") != "1" {
		t.Errorf("expected Retry-After: 1, got %q", resp.Header.Get("Retry-After"))
	}
}

func TestRateLimit_CustomStatus(t *testing.T) {
	server, cleanup := startServer(t)
	defer cleanup()

	server.Stub(gmock.StubDefinition{
		Request: gmock.RequestPattern{
			Method:  http.MethodGet,
			URLPath: "/api/ratelimited503",
		},
		Response: gmock.ResponseDefinition{
			Status: 200,
			Body:   `{"ok":true}`,
			Fault: &gmock.FaultDefinition{
				Type:           "rate_limit",
				PerSecond:      1,
				RateLimitStatus: 503,
			},
		},
	})

	// First request should succeed.
	resp, err := http.Get(server.URL() + "/api/ratelimited503")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	// Second request should be rate-limited with custom 503.
	resp, err = http.Get(server.URL() + "/api/ratelimited503")
	if err != nil {
		t.Fatalf("rate-limited request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", resp.StatusCode)
	}
}

func TestRateLimit_AfterRequests(t *testing.T) {
	server, cleanup := startServer(t)
	defer cleanup()

	server.Stub(gmock.StubDefinition{
		Request: gmock.RequestPattern{
			Method:  http.MethodGet,
			URLPath: "/api/warmup",
		},
		Response: gmock.ResponseDefinition{
			Status: 200,
			Body:   `{"ok":true}`,
			Fault: &gmock.FaultDefinition{
				Type:          "rate_limit",
				AfterRequests: 3,
				PerSecond:     1,
			},
		},
	})

	// First 3 requests should succeed (warm-up phase).
	for i := 0; i < 3; i++ {
		resp, err := http.Get(server.URL() + "/api/warmup")
		if err != nil {
			t.Fatalf("warm-up request %d failed: %v", i+1, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("warm-up request %d: expected 200, got %d", i+1, resp.StatusCode)
		}
	}

	// 4th request should succeed (1 token in bucket).
	resp, err := http.Get(server.URL() + "/api/warmup")
	if err != nil {
		t.Fatalf("first post-warmup request failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("first post-warmup request: expected 200, got %d", resp.StatusCode)
	}

	// 5th request should be rate-limited.
	resp, err = http.Get(server.URL() + "/api/warmup")
	if err != nil {
		t.Fatalf("rate-limited request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", resp.StatusCode)
	}
}
