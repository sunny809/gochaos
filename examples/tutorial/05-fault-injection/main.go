// Fault injection tutorial
//
// This example demonstrates how to use gochaos fault injection to simulate
// network-level failures in your mock server. Fault injection is essential for
// testing how your application handles:
//
//   - Internal server errors (500)
//   - Empty responses (no body, no headers)
//   - Connection resets (TCP RST)
//
// Three fault types are supported:
//
//	"error"            — Returns HTTP 500 with a JSON error body
//	"empty"            — Returns an empty response (no body, no headers)
//	"connection_reset" — Closes the TCP connection abruptly (simulates network failure)
//
// Run:
//
//	cd examples/tutorial/05-fault-injection && go run .
package main

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/sunny809/gochaos/pkg/gmock"
)

func main() {
	// ----------------------------------------------------------------
	// Step 1: Start a mock server
	// ----------------------------------------------------------------
	server := gmock.NewServer(gmock.WithPort(0))
	if err := server.Start(); err != nil {
		panic(err)
	}
	defer server.Stop()

	fmt.Printf("Fault injection server running at %s\n\n", server.URL())

	// ----------------------------------------------------------------
	// Step 2: Register stubs with different fault types
	// ----------------------------------------------------------------

	// 2a. "error" fault — returns HTTP 500 with a JSON error body
	server.Stub(gmock.StubDefinition{
		Name: "error-fault",
		Request: gmock.RequestPattern{
			Method:  http.MethodGet,
			URLPath: "/fault/error",
		},
		Response: gmock.ResponseDefinition{
			Status: http.StatusOK, // ignored when fault is applied
			Body:   "this will not be returned",
			Fault: &gmock.FaultDefinition{
				Type: "error",
			},
		},
	})

	// 2b. "empty" fault — returns an empty response with no body
	server.Stub(gmock.StubDefinition{
		Name: "empty-fault",
		Request: gmock.RequestPattern{
			Method:  http.MethodGet,
			URLPath: "/fault/empty",
		},
		Response: gmock.ResponseDefinition{
			Status: http.StatusOK,
			Body:   "this will not be returned",
			Fault: &gmock.FaultDefinition{
				Type: "empty",
			},
		},
	})

	// 2c. "connection_reset" fault — closes the TCP connection abruptly
	server.Stub(gmock.StubDefinition{
		Name: "connection-reset-fault",
		Request: gmock.RequestPattern{
			Method:  http.MethodGet,
			URLPath: "/fault/connection-reset",
		},
		Response: gmock.ResponseDefinition{
			Status: http.StatusOK,
			Body:   "this will not be returned",
			Fault: &gmock.FaultDefinition{
				Type: "connection_reset",
			},
		},
	})

	// 2d. Normal stub (no fault) — for comparison
	server.Stub(gmock.StubDefinition{
		Name: "normal-endpoint",
		Request: gmock.RequestPattern{
			Method:  http.MethodGet,
			URLPath: "/normal",
		},
		Response: gmock.ResponseDefinition{
			Status: http.StatusOK,
			Body:   `{"status":"ok"}`,
		},
	})

	// ----------------------------------------------------------------
	// Step 3: Test each fault type
	// ----------------------------------------------------------------

	// 3a. Error fault — expect HTTP 500 with JSON error body
	fmt.Println("=== Test 1: Error Fault (500) ===")
	resp, err := http.Get(server.URL() + "/fault/error")
	if err != nil {
		fmt.Printf("Unexpected error: %v\n\n", err)
	} else {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("  Status: %d\n", resp.StatusCode)
		fmt.Printf("  Body:   %s\n", string(body))
		fmt.Printf("  Content-Type: %s\n\n", resp.Header.Get("Content-Type"))
	}

	// 3b. Empty fault — expect empty response body
	fmt.Println("=== Test 2: Empty Fault (no body) ===")
	resp2, err := http.Get(server.URL() + "/fault/empty")
	if err != nil {
		fmt.Printf("Unexpected error: %v\n\n", err)
	} else {
		defer resp2.Body.Close()
		body2, _ := io.ReadAll(resp2.Body)
		fmt.Printf("  Status: %d\n", resp2.StatusCode)
		fmt.Printf("  Body length: %d bytes\n", len(body2))
		fmt.Printf("  Body content: %q\n\n", string(body2))
	}

	// 3c. Connection reset fault — expect a connection error
	fmt.Println("=== Test 3: Connection Reset Fault ===")
	// Use a short timeout so we don't hang if the connection drops
	client := &http.Client{Timeout: 3 * time.Second}
	resp3, err := client.Get(server.URL() + "/fault/connection-reset")
	if err != nil {
		// Expected: connection reset by peer / broken pipe
		fmt.Printf("  Expected error: %v\n", err)
		fmt.Printf("  (Connection reset simulates a network failure)\n\n")
	} else {
		defer resp3.Body.Close()
		body3, _ := io.ReadAll(resp3.Body)
		fmt.Printf("  Status: %d\n", resp3.StatusCode)
		fmt.Printf("  Body:   %s\n\n", string(body3))
	}

	// 3d. Normal request — for comparison
	fmt.Println("=== Test 4: Normal Request (no fault, for comparison) ===")
	resp4, err := http.Get(server.URL() + "/normal")
	if err != nil {
		panic(err)
	}
	defer resp4.Body.Close()
	body4, _ := io.ReadAll(resp4.Body)
	fmt.Printf("  Status: %d\n", resp4.StatusCode)
	fmt.Printf("  Body:   %s\n\n", string(body4))

	// ----------------------------------------------------------------
	// Step 4: Combine faults with delays for realistic chaos testing
	// ----------------------------------------------------------------
	fmt.Println("=== Bonus: Delay + Fault Combination ===")
	server.Stub(gmock.StubDefinition{
		Name: "delayed-error",
		Request: gmock.RequestPattern{
			Method:  http.MethodGet,
			URLPath: "/chaos/delayed-error",
		},
		Response: gmock.ResponseDefinition{
			Fault: &gmock.FaultDefinition{Type: "error"},
			Delay: &gmock.DelayDefinition{
				Type:  "fixed",
				Value: 500, // 500ms delay before the error
			},
		},
	})

	start := time.Now()
	resp5, err := http.Get(server.URL() + "/chaos/delayed-error")
	elapsed := time.Since(start)
	if err != nil {
		fmt.Printf("  Error: %v\n\n", err)
	} else {
		defer resp5.Body.Close()
		body5, _ := io.ReadAll(resp5.Body)
		fmt.Printf("  Status:  %d\n", resp5.StatusCode)
		fmt.Printf("  Body:    %s\n", string(body5))
		fmt.Printf("  Elapsed: %v (500ms delay + error)\n\n", elapsed)
	}

	fmt.Println("\nFault injection is useful for:")
	fmt.Println("  - Testing retry logic (error fault)")
	fmt.Println("  - Testing client timeout handling (delay + empty)")
	fmt.Println("  - Testing connection pool resilience (connection_reset)")
	fmt.Println("  - Validating circuit breaker behavior (fault combinations)")
}
