package main

import (
	"net/http"
	"testing"
	"time"

	"github.com/sunny809/gochaos/pkg/gmock"
)

// TestMultiServiceChaos simulates a real-world scenario where multiple downstream
// dependencies fail during a deploy window. This test is deterministic when
// run with WithRandSeed.
func TestMultiServiceChaos(t *testing.T) {
	server := gmock.NewServer(
		gmock.WithPort(0),
		gmock.WithRandSeed(42),
	)
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	defer server.Stop()

	// Payment stub: 50% error rate during the first 10 seconds
	server.Stub(gmock.StubDefinition{
		Request: gmock.RequestPattern{
			Method:  http.MethodPost,
			URLPath: "/api/payments",
		},
		Response: gmock.ResponseDefinition{
			Status: http.StatusOK,
			Body:   `{"status":"ok"}`,
			Fault: &gmock.FaultDefinition{
				Type: "error",
				Activation: &gmock.Activation{
					Probability: 0.5,
					ActiveBetween: []gmock.TimeWindow{
						{StartMs: 0, EndMs: 10000},
					},
				},
			},
		},
	})

	// Inventory stub: 2-second delay during the first 10 seconds
	server.Stub(gmock.StubDefinition{
		Request: gmock.RequestPattern{
			Method:  http.MethodGet,
			URLPath: "/api/inventory",
		},
		Response: gmock.ResponseDefinition{
			Status: http.StatusOK,
			Body:   `{"stock":100}`,
			Delay: &gmock.DelayDefinition{
				Type:  "fixed",
				Value: 2000,
			},
		},
	})

	// Shipping stub: healthy (no fault, no delay) — control group
	server.Stub(gmock.StubDefinition{
		Request: gmock.RequestPattern{
			Method:  http.MethodGet,
			URLPath: "/api/shipping",
		},
		Response: gmock.ResponseDefinition{
			Status: http.StatusOK,
			Body:   `{"status":"shipped"}`,
		},
	})

	baseURL := server.URL()

	// Make requests during the chaos window (all within 10 seconds)
	for i := 0; i < 20; i++ {
		// Payment: should see ~50% errors
		resp, err := http.Post(baseURL+"/api/payments", "application/json", nil)
		if err == nil {
			resp.Body.Close()
		}

		// Inventory: should be slow (2s delay per request)
		// We only make a few calls to avoid long test runtime
		if i < 5 {
			start := time.Now()
			resp, err := http.Get(baseURL + "/api/inventory")
			if err == nil {
				resp.Body.Close()
			}
			elapsed := time.Since(start)
			if elapsed < 1500*time.Millisecond {
				t.Logf("inventory request %d: %v (expected ~2s delay)", i, elapsed)
			}
		}

		// Shipping: should always be fast (no chaos)
		resp, err = http.Get(baseURL + "/api/shipping")
		if err == nil {
			resp.Body.Close()
		}
	}

	// Verify payment had faults injected
	paymentResult := server.VerifyFaultsInjected(
		gmock.FaultPattern{
			StubID:    "",
			FaultType: "error",
		},
		1,
	)
	if !paymentResult.Matched {
		t.Errorf("expected payment faults to be injected: %v", paymentResult.Errors)
	}

	t.Logf("Payment faults injected: %d", paymentResult.ActualCount)
	t.Log("Multi-service chaos test complete")
}