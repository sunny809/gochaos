package steps

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/sunny809/gochaos/pkg/gmock"
)

// TestContext holds the shared state for a single BDD scenario.
// A fresh instance is created before each scenario and cleaned up after.
type TestContext struct {
	mu       sync.Mutex
	server   gmock.Server
	baseURL  string
	adminURL string
	response *http.Response
	body     []byte
	stubIDs  []string // track registered stubs for cleanup

	// For chaos / multi-request scenarios
	seed      int64            // for WithRandSeed
	responses []*http.Response // collected responses from SendNRequests

	// For building up stub definitions across multiple Given steps
	pendingFault      *gmock.FaultDefinition
	pendingDelay      *gmock.DelayDefinition
	pendingActivation *gmock.Activation
}

// BeforeScenario initializes a fresh gmock server on a random port.
func (tc *TestContext) BeforeScenario() error {
	opts := []gmock.Option{gmock.WithPort(0)}
	if tc.seed != 0 {
		opts = append(opts, gmock.WithRandSeed(tc.seed))
	}
	srv := gmock.NewServer(opts...)
	if err := srv.Start(); err != nil {
		return fmt.Errorf("failed to start mock server: %w", err)
	}
	tc.mu.Lock()
	tc.server = srv
	tc.baseURL = srv.URL()
	tc.adminURL = srv.AdminURL()
	tc.stubIDs = nil
	tc.responses = nil
	tc.pendingFault = nil
	tc.pendingDelay = nil
	tc.pendingActivation = nil
	tc.mu.Unlock()
	return nil
}

// AfterScenario stops the gmock server and cleans up any open responses.
func (tc *TestContext) AfterScenario() {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if tc.response != nil {
		_ = tc.response.Body.Close()
		tc.response = nil
	}
	tc.closeResponses()
	tc.body = nil
	tc.stubIDs = nil
	tc.responses = nil
	tc.pendingFault = nil
	tc.pendingDelay = nil
	tc.pendingActivation = nil
	if tc.server != nil {
		_ = tc.server.Stop()
		tc.server = nil
	}
}

// AddStubID records a stub ID for potential cleanup. Thread-safe.
func (tc *TestContext) AddStubID(id string) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.stubIDs = append(tc.stubIDs, id)
}

// SendRequest sends an HTTP request to the mock server at the given path.
func (tc *TestContext) SendRequest(method, path string) error {
	if tc.response != nil {
		_ = tc.response.Body.Close()
		tc.response = nil
	}
	tc.body = nil

	req, err := http.NewRequestWithContext(context.Background(), method, tc.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		_ = resp.Body.Close()
		return fmt.Errorf("failed to read response body: %w", err)
	}
	_ = resp.Body.Close()

	tc.response = resp
	// Re-wrap so Body can be read again if needed
	tc.response.Body = io.NopCloser(bytes.NewReader(body))
	tc.body = body
	return nil
}

// SendAdminRequest sends a request to the admin API and returns the response, body, and error.
// The response body is read and closed internally; the returned Response has a
// closed Body. Callers should treat respBody as the authoritative content.
func (tc *TestContext) SendAdminRequest(method, path string, body []byte) (*http.Response, []byte, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, tc.adminURL+path, bodyReader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create admin request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("admin request failed: %w", err)
	}
	respBody, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read admin response body: %w", err)
	}
	return resp, respBody, nil
}

// SendNRequests sends n sequential requests to the given path and collects
// all responses (including nil entries for requests that failed with an error).
func (tc *TestContext) SendNRequests(n int, method, path string) error {
	tc.mu.Lock()
	tc.responses = nil
	tc.mu.Unlock()

	for i := 0; i < n; i++ {
		req, err := http.NewRequestWithContext(context.Background(), method, tc.baseURL+path, nil)
		if err != nil {
			return fmt.Errorf("failed to create request %d: %w", i+1, err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			// For TCP-level faults, the request may fail.
			// Append nil to the responses list for counting.
			tc.mu.Lock()
			tc.responses = append(tc.responses, nil) //nolint:bodyclose // nil response, nothing to close
			tc.mu.Unlock()
			continue
		}
		// Read body to enable connection reuse
		_, _ = io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		tc.mu.Lock()
		tc.responses = append(tc.responses, resp) //nolint:bodyclose // body already closed above
		tc.mu.Unlock()
	}
	return nil
}

// closeResponses closes all response bodies in the responses slice.
func (tc *TestContext) closeResponses() {
	for _, resp := range tc.responses {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	}
}

// buildStubWithFault constructs a StubDefinition with the given fault type,
// combining any pending delay and activation that were set by prior steps.
func (tc *TestContext) buildStubWithFault(faultType string) gmock.StubDefinition {
	fault := &gmock.FaultDefinition{
		Type: faultType,
	}

	// Apply activation from pending state
	if tc.pendingActivation != nil {
		fault.Activation = tc.pendingActivation
	}

	// Apply delay from pending state
	var delay *gmock.DelayDefinition
	if tc.pendingDelay != nil {
		delay = tc.pendingDelay
	}

	return gmock.StubDefinition{
		Request: gmock.RequestPattern{
			Method:  "GET",
			URLPath: "/test",
		},
		Response: gmock.ResponseDefinition{
			Status: 200,
			Fault:  fault,
			Delay:  delay,
		},
	}
}

// ParseJSON parses a JSON byte slice into the given value.
// Returns an error message suitable for Gherkin step failures.
func ParseJSON(data []byte, v interface{}) error {
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("failed to parse JSON: %w", err)
	}
	return nil
}
