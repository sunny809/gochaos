// Delay example
//
// This example demonstrates how to simulate response delays using gmock.
// Both fixed and random delays are supported.
//
// Run:
//   cd examples/delay && go run .
package main

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/sunny809/gochaos/pkg/gmock"
)

func main() {
	// Create and start a server
	server := gmock.NewServer(gmock.WithPort(0))
	if err := server.Start(); err != nil {
		panic(err)
	}
	defer server.Stop()

	// Register a stub with a fixed delay of 500ms
	server.Stub(gmock.StubDefinition{
		Name: "slow-endpoint-fixed",
		Request: gmock.RequestPattern{
			Method:  http.MethodGet,
			URLPath: "/api/slow-fixed",
		},
		Response: gmock.ResponseDefinition{
			Status: http.StatusOK,
			Body:   `{"message":"fixed delay response"}`,
			Delay: &gmock.DelayDefinition{
				Type:  "fixed",
				Value: 500, // milliseconds
			},
		},
	})

	// Register a stub with a random delay between 100ms and 300ms
	server.Stub(gmock.StubDefinition{
		Name: "slow-endpoint-random",
		Request: gmock.RequestPattern{
			Method:  http.MethodGet,
			URLPath: "/api/slow-random",
		},
		Response: gmock.ResponseDefinition{
			Status: http.StatusOK,
			Body:   `{"message":"random delay response"}`,
			Delay: &gmock.DelayDefinition{
				Type: "random",
				Min:  100,
				Max:  300,
			},
		},
	})

	fmt.Printf("Delay server running at %s\n\n", server.URL())

	// Test fixed delay
	fmt.Println("Testing fixed delay (500ms)...")
	start := time.Now()
	resp, err := http.Get(server.URL() + "/api/slow-fixed")
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	elapsed := time.Since(start)
	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("  Status: %d\n", resp.StatusCode)
	fmt.Printf("  Body:   %s\n", string(body))
	fmt.Printf("  Elapsed: %v (expected ~500ms)\n\n", elapsed)

	// Test random delay
	fmt.Println("Testing random delay (100-300ms)...")
	start = time.Now()
	resp2, err := http.Get(server.URL() + "/api/slow-random")
	if err != nil {
		panic(err)
	}
	defer resp2.Body.Close()
	elapsed = time.Since(start)
	body2, _ := io.ReadAll(resp2.Body)
	fmt.Printf("  Status: %d\n", resp2.StatusCode)
	fmt.Printf("  Body:   %s\n", string(body2))
	fmt.Printf("  Elapsed: %v (expected 100-300ms)\n", elapsed)
}
