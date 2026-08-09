package main

import (
	"net/http"
	"testing"
	"time"

	"github.com/sunny809/gochaos/pkg/gmock"
)

// TestRetryOnIntermittentFailures demonstrates how gmock can be used to test
// that a service handles intermittent failures from a downstream dependency.
//
// It registers a stub with a 30% error fault probability, makes 50 requests
// (simulating the SUT), and verifies that at least 1 fault was injected.
func TestRetryOnIntermittentFailures(t *testing.T) {
	// Start a gmock server on a random port.
	server := gmock.NewServer(gmock.WithPort(0))
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	defer server.Stop()

	// Register a stub that returns 200 OK with a 30% probability of
	// injecting an "error" fault (returns 500 Internal Server Error).
	server.Stub(gmock.StubDefinition{
		Request: gmock.RequestPattern{
			Method:  http.MethodGet,
			URLPath: "/api/inventory",
		},
		Response: gmock.ResponseDefinition{
			Status: 200,
			Body:   `{"stock":100}`,
			Fault: &gmock.FaultDefinition{
				Type: "error",
				Activation: &gmock.Activation{
					Probability: 0.3,
				},
			},
		},
	})

	// Simulate the SUT making requests to the downstream dependency.
	// The SUT should handle both 200 (success) and 500 (fault) responses.
	client := &http.Client{Timeout: 5 * time.Second}
	url := server.URL() + "/api/inventory"

	successCount := 0
	faultCount := 0
	requestCount := 50

	for range requestCount {
		resp, err := client.Get(url)
		if err != nil {
			// Connection reset or other transport error.
			faultCount++
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			successCount++
		} else {
			faultCount++
		}
	}

	t.Logf("Results: %d success, %d faults (%d total)",
		successCount, faultCount, requestCount)

	// Verify that at least 1 fault was injected by gmock.
	result := server.VerifyFaultsInjected(
		gmock.FaultPattern{FaultType: "error"},
		1, // At least 1 fault injected
	)
	if !result.Matched {
		t.Errorf("expected at least 1 fault injection, got %d: %v",
			result.ActualCount, result.Errors)
	}
}
