package steps

import (
	"bytes"
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
	response *http.Response
	body     []byte
	stubIDs  []string // track registered stubs for cleanup
}

// BeforeScenario initializes a fresh gmock server on a random port.
func (tc *TestContext) BeforeScenario() error {
	srv := gmock.NewServer(gmock.WithPort(0))
	if err := srv.Start(); err != nil {
		return fmt.Errorf("failed to start mock server: %w", err)
	}
	tc.mu.Lock()
	tc.server = srv
	tc.baseURL = srv.URL()
	tc.stubIDs = nil
	tc.mu.Unlock()
	return nil
}

// AfterScenario stops the gmock server and cleans up any open responses.
func (tc *TestContext) AfterScenario() {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if tc.response != nil {
		tc.response.Body.Close()
		tc.response = nil
	}
	tc.body = nil
	tc.stubIDs = nil
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
		tc.response.Body.Close()
		tc.response = nil
	}
	tc.body = nil

	req, err := http.NewRequest(method, tc.baseURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		resp.Body.Close()
		return err
	}
	resp.Body.Close()

	tc.response = resp
	// Re-wrap so Body can be read again if needed
	tc.response.Body = io.NopCloser(bytes.NewReader(body))
	tc.body = body
	return nil
}
