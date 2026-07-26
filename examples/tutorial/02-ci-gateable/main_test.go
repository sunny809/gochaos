// Tutorial 2: CI-Gateable Chaos Experiment
//
// This example demonstrates how to run a deterministic chaos experiment
// that can be gated in CI. It uses:
//
//   - gmock.WithRandSeed(42)  — deterministic fault injection
//   - Time-window activation  — burst-shaped failure patterns
//   - VerifyFaultsInjected    — CI-friendly assertions
//
// Run:
//
//	cd examples/tutorial/02-ci-gateable && go test -v
package main

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/sunny809/gochaos/pkg/gmock"
)

// paymentClient is the System Under Test (SUT) — it calls a downstream
// payment API and retries on 5xx responses.
type paymentClient struct {
	baseURL    string
	httpClient *http.Client
	maxRetries int
}

// Charge attempts to process a payment. On 5xx responses (including the
// "error" fault which returns 500) it retries up to maxRetries times.
// On 4xx responses it fails immediately. Returns nil on success.
func (c *paymentClient) Charge() error {
	var lastErr error
	for range c.maxRetries + 1 {
		resp, err := c.httpClient.Get(c.baseURL + "/api/payments")
		if err != nil {
			lastErr = err
			continue // retry on transport error
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return nil // success
		}
		lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
		if resp.StatusCode < 500 {
			return lastErr // fail fast on 4xx
		}
		// 5xx: retry (loop continues)
	}
	return fmt.Errorf("failed after %d retries (last: %v)", c.maxRetries, lastErr)
}

// TestCIGateableChaosExperiment demonstrates a CI-gateable chaos experiment.
//
// Key elements:
//  1. WithRandSeed(42) makes all chaos behavior reproducible across runs.
//  2. Time-window activation creates a burst of failures in the [0, 10s) window.
//  3. VerifyFaultsInjected asserts the exact number of injected faults.
func TestCIGateableChaosExperiment(t *testing.T) {
	// ----------------------------------------------------------------
	// Step 1: Start server with deterministic seed
	// ----------------------------------------------------------------
	// WithRandSeed(42) ensures that every run produces the same fault
	// injection sequence. This is what makes the test CI-gateable —
	// no flakiness from randomness.
	server := gmock.NewServer(gmock.WithPort(0), gmock.WithRandSeed(42))
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	defer server.Stop()

	// ----------------------------------------------------------------
	// Step 2: Register stub with time-window activation
	// ----------------------------------------------------------------
	// The fault is always-on within the [0, 10000)ms window measured from
	// server start. This creates a "burst phase": all requests in the first
	// 10 seconds trigger an "error" fault (returns 500).
	//
	// After the window closes (10s+), the stub returns normal 200 responses
	// with no fault injection — simulating a downstream that recovers.
	server.Stub(gmock.StubDefinition{
		Request: gmock.RequestPattern{
			Method:  http.MethodGet,
			URLPath: "/api/payments",
		},
		Response: gmock.ResponseDefinition{
			Status: http.StatusOK,
			Body:   `{"status":"processed"}`,
			Fault: &gmock.FaultDefinition{
				Type: "error",
				Activation: &gmock.Activation{
					ActiveBetween: []gmock.TimeWindow{
						{StartMs: 0, EndMs: 10000}, // 0–10s burst window
					},
				},
			},
		},
	})

	// ----------------------------------------------------------------
	// Step 3: SUT makes requests during the burst window
	// ----------------------------------------------------------------
	// The SUT (paymentClient) retries up to 2 times on 5xx errors.
	// Since the "error" fault returns 500, every request hits the retry
	// loop. Each Charge() call makes 3 requests (1 initial + 2 retries).
	//
	// 5 Charge() calls × 3 requests each = 15 total requests to the mock.
	// All 15 land inside the 10s burst window, so all trigger faults.
	client := paymentClient{
		baseURL:    server.URL(),
		httpClient: &http.Client{Timeout: 5 * time.Second},
		maxRetries: 2,
	}

	var succeeded, failed int
	for i := range 5 {
		if err := client.Charge(); err != nil {
			failed++
			t.Logf("Payment %d: FAILED (expected during burst) — %v", i+1, err)
		} else {
			succeeded++
		}
	}

	t.Logf("SUT results: %d succeeded, %d failed (expected: 0 succeeded, 5 failed)",
		succeeded, failed)

	// ----------------------------------------------------------------
	// Step 4: Assert exactly 15 faults were injected
	// ----------------------------------------------------------------
	// With seed 42 and the time window always-on within [0, 10s), every
	// request deterministically triggers the "error" fault. This assertion
	// is stable across CI runs — no flakiness.
	result := server.VerifyFaultsInjected(
		gmock.FaultPattern{FaultType: "error"},
		15, // 5 attempts × 3 requests each
	)
	if !result.Matched {
		t.Errorf("expected 15 fault injections, got %d: %v",
			result.ActualCount, result.Errors)
	}

	// Also verify that the SUT actually experienced failures
	if failed != 5 {
		t.Errorf("expected 5 failed payments, got %d", failed)
	}
}