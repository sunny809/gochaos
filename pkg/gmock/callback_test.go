package gmock

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// waitUntil polls fn every 10ms until it returns true or the timeout elapses.
// Local copy of test/testutil.Wait — this file is package gmock (internal)
// and cannot import test/testutil due to the import cycle.
func waitUntil(t testing.TB, desc string, timeout time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if fn() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s (%v)", desc, timeout)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestCallbackDispatch(t *testing.T) {
	// Start a target server to receive callbacks
	var mu sync.Mutex
	var receivedRequests []*http.Request
	var receivedBodies []string
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		body, _ := io.ReadAll(r.Body)
		receivedRequests = append(receivedRequests, r)
		receivedBodies = append(receivedBodies, string(body))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()

	// Start gmock server
	srv := NewServer(WithPort(0), WithCallbackSSRFBypass())
	srv.Start()
	defer srv.Stop()

	// Register stub with callback
	srv.Stub(StubDefinition{
		Request: RequestPattern{
			Method:  http.MethodPost,
			URLPath: "/api/orders",
		},
		Response: ResponseDefinition{
			Status: http.StatusCreated,
			Body:   `{"id":"123"}`,
			Callback: &CallbackDefinition{
				URL:    target.URL + "/webhook",
				Method: http.MethodPost,
				Headers: map[string]string{
					"X-Webhook-Type": "order-created",
				},
				Body: `{"event":"order_created","path":"{{.Path}}","method":"{{.Method}}"}`,
			},
		},
	})

	// Make request to trigger callback
	resp, err := http.Post(srv.URL()+"/api/orders", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected 201, got %d", resp.StatusCode)
	}

	// Wait for async callback
	waitUntil(t, "callback dispatch", 2*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(receivedRequests) >= 1
	})

	// Verify callback was received
	mu.Lock()
	defer mu.Unlock()
	if len(receivedRequests) != 1 {
		t.Fatalf("expected 1 callback, got %d", len(receivedRequests))
	}
	if receivedRequests[0].Header.Get("X-Webhook-Type") != "order-created" {
		t.Errorf("expected X-Webhook-Type=order-created, got %s", receivedRequests[0].Header.Get("X-Webhook-Type"))
	}

	// Verify template rendering
	var body map[string]string
	if err := json.Unmarshal([]byte(receivedBodies[0]), &body); err != nil {
		t.Fatalf("failed to parse callback body: %v", err)
	}
	if body["event"] != "order_created" {
		t.Errorf("expected event=order_created, got %s", body["event"])
	}
	if body["path"] != "/api/orders" {
		t.Errorf("expected path=/api/orders, got %s", body["path"])
	}
	if body["method"] != "POST" {
		t.Errorf("expected method=POST, got %s", body["method"])
	}
}

func TestCallbackVerify(t *testing.T) {
	// Start a target server
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	srv := NewServer(WithPort(0), WithCallbackSSRFBypass())
	srv.Start()
	defer srv.Stop()

	// Register stub with callback
	stubID := srv.Stub(StubDefinition{
		Request: RequestPattern{
			Method:  http.MethodGet,
			URLPath: "/trigger",
		},
		Response: ResponseDefinition{
			Status: http.StatusOK,
			Callback: &CallbackDefinition{
				URL: target.URL + "/cb",
			},
		},
	})

	// Make two requests
	for i := 0; i < 2; i++ {
		resp, err := http.Get(srv.URL() + "/trigger")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		resp.Body.Close()
	}

	// Wait for callbacks
	waitUntil(t, "callback dispatch", 2*time.Second, func() bool {
		return srv.VerifyCallbacks(CallbackPattern{StubID: stubID}, 2).Matched
	})

	// Verify callbacks
	result := srv.VerifyCallbacks(CallbackPattern{StubID: stubID}, 2)
	if !result.Matched {
		t.Errorf("expected callbacks to match, got: %+v", result)
	}
	if result.ActualCount != 2 {
		t.Errorf("expected ActualCount=2, got %d", result.ActualCount)
	}

	// Verify with zero count (VerifyNoCallbacksDispatched)
	noResult := srv.VerifyCallbacks(CallbackPattern{StubID: "nonexistent"}, 0)
	if !noResult.Matched {
		t.Errorf("expected no callbacks for nonexistent stub, got: %+v", noResult)
	}
}

func TestCallbackSSRFBlocked(t *testing.T) {
	srv := NewServer(WithPort(0))
	srv.Start()
	defer srv.Stop()

	// Register stub with callback to localhost (should be SSRF-blocked)
	srv.Stub(StubDefinition{
		Request: RequestPattern{
			Method:  http.MethodGet,
			URLPath: "/ssrf-test",
		},
		Response: ResponseDefinition{
			Status: http.StatusOK,
			Callback: &CallbackDefinition{
				URL: "http://127.0.0.1:9999/internal",
			},
		},
	})

	resp, err := http.Get(srv.URL() + "/ssrf-test")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	// Wait for callback dispatch
	waitUntil(t, "SSRF-blocked callback", 2*time.Second, func() bool {
		return srv.VerifyCallbacks(CallbackPattern{Status: "ssrf_blocked"}, 1).Matched
	})

	// Verify the callback was blocked
	result := srv.VerifyCallbacks(CallbackPattern{Status: "ssrf_blocked"}, 1)
	if !result.Matched {
		t.Errorf("expected SSRF-blocked callback, got: %+v", result)
	}
}

func TestCallbackDisabled(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("callback should not be dispatched when disabled")
	}))
	defer target.Close()

	srv := NewServer(WithPort(0), WithCallbackEnabled(false), WithCallbackSSRFBypass())
	srv.Start()
	defer srv.Stop()

	srv.Stub(StubDefinition{
		Request: RequestPattern{
			Method:  http.MethodGet,
			URLPath: "/disabled-cb",
		},
		Response: ResponseDefinition{
			Status: http.StatusOK,
			Callback: &CallbackDefinition{
				URL: target.URL + "/cb",
			},
		},
	})

	resp, err := http.Get(srv.URL() + "/disabled-cb")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	waitUntil(t, "disabled callback", 2*time.Second, func() bool {
		return srv.VerifyCallbacks(CallbackPattern{Status: "disabled"}, 1).Matched
	})
	result := srv.VerifyCallbacks(CallbackPattern{Status: "disabled"}, 1)
	if !result.Matched {
		t.Errorf("expected disabled callback, got: %+v", result)
	}
}

func TestCallbackNoCallback(t *testing.T) {
	srv := NewServer(WithPort(0))
	srv.Start()
	defer srv.Stop()

	// Stub without callback — should not panic or dispatch anything
	srv.Stub(StubDefinition{
		Request: RequestPattern{
			Method:  http.MethodGet,
			URLPath: "/no-callback",
		},
		Response: ResponseDefinition{
			Status: http.StatusOK,
			Body:   `ok`,
		},
	})

	resp, err := http.Get(srv.URL() + "/no-callback")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestCallbackAdminEndpoints(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	srv := NewServer(WithPort(0), WithCallbackSSRFBypass())
	srv.Start()
	defer srv.Stop()

	srv.Stub(StubDefinition{
		Request: RequestPattern{
			Method:  http.MethodGet,
			URLPath: "/cb-test",
		},
		Response: ResponseDefinition{
			Status: http.StatusOK,
			Callback: &CallbackDefinition{
				URL: target.URL + "/hook",
			},
		},
	})

	resp, err := http.Get(srv.URL() + "/cb-test")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	waitUntil(t, "callback dispatch", 2*time.Second, func() bool {
		return srv.VerifyCallbacks(CallbackPattern{}, 1).Matched
	})

	// GET /__admin/callbacks
	listResp, err := http.Get(srv.AdminURL() + "/__admin/callbacks")
	if err != nil {
		t.Fatalf("list callbacks failed: %v", err)
	}
	defer listResp.Body.Close()

	if listResp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", listResp.StatusCode)
	}

	var listResult struct {
		Entries []CallbackEntry `json:"entries"`
		Count   int             `json:"count"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&listResult); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if listResult.Count != 1 {
		t.Errorf("expected 1 callback entry, got %d", listResult.Count)
	}

	// DELETE /__admin/callbacks
	delReq, _ := http.NewRequest(http.MethodDelete, srv.AdminURL()+"/__admin/callbacks", nil)
	delResp, err := http.DefaultClient.Do(delReq)
	if err != nil {
		t.Fatalf("delete callbacks failed: %v", err)
	}
	defer delResp.Body.Close()

	if delResp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", delResp.StatusCode)
	}

	// Verify callback log is now empty
	emptyResp, err := http.Get(srv.AdminURL() + "/__admin/callbacks")
	if err != nil {
		t.Fatalf("list callbacks failed: %v", err)
	}
	defer emptyResp.Body.Close()

	var emptyResult struct {
		Count int `json:"count"`
	}
	if err := json.NewDecoder(emptyResp.Body).Decode(&emptyResult); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if emptyResult.Count != 0 {
		t.Errorf("expected 0 after clear, got %d", emptyResult.Count)
	}
}

func TestCallbackTimeout(t *testing.T) {
	// Target server that responds slowly (longer than the callback timeout)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	srv := NewServer(WithPort(0), WithCallbackSSRFBypass(), WithCallbackTimeout(50*time.Millisecond))
	srv.Start()
	defer srv.Stop()

	srv.Stub(StubDefinition{
		Request: RequestPattern{
			Method:  http.MethodGet,
			URLPath: "/timeout-cb",
		},
		Response: ResponseDefinition{
			Status: http.StatusOK,
			Callback: &CallbackDefinition{
				URL: target.URL + "/slow",
			},
		},
	})

	resp, err := http.Get(srv.URL() + "/timeout-cb")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	// Wait for callback to time out
	waitUntil(t, "timeout callback", 2*time.Second, func() bool {
		return srv.VerifyCallbacks(CallbackPattern{Status: "timeout"}, 1).Matched
	})

	// Verify the callback was logged as timeout
	result := srv.VerifyCallbacks(CallbackPattern{Status: "timeout"}, 1)
	if !result.Matched {
		t.Errorf("expected timeout callback, got: %+v", result)
	}
}

func TestCallbackReset(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	srv := NewServer(WithPort(0), WithCallbackSSRFBypass())
	srv.Start()
	defer srv.Stop()

	srv.Stub(StubDefinition{
		Request: RequestPattern{
			Method:  http.MethodGet,
			URLPath: "/reset-cb",
		},
		Response: ResponseDefinition{
			Status: http.StatusOK,
			Callback: &CallbackDefinition{
				URL: target.URL + "/hook",
			},
		},
	})

	resp, err := http.Get(srv.URL() + "/reset-cb")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	waitUntil(t, "callback dispatch", 2*time.Second, func() bool {
		return srv.VerifyCallbacks(CallbackPattern{}, 1).Matched
	})

	// Verify callback was logged
	before := srv.VerifyCallbacks(CallbackPattern{}, 1)
	if !before.Matched {
		t.Errorf("expected 1 callback before reset, got: %+v", before)
	}

	// Reset
	srv.Reset()

	// Verify callback log is cleared
	after := srv.VerifyCallbacks(CallbackPattern{}, 0)
	if !after.Matched {
		t.Errorf("expected 0 callbacks after reset, got: %+v", after)
	}
}
