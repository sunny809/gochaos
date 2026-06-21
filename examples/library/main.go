// Embedded library example
//
// This example shows the simplest way to use gmock as an embedded
// HTTP mock server in a Go test or main program.
//
// Run:
//
//	go run ./examples/library
package main

import (
	"fmt"
	"io"
	"net/http"

	"github.com/sunny809/gochaos/pkg/gmock"
)

func main() {
	// Create and start a server on a random port
	server := gmock.NewServer(gmock.WithPort(0))
	if err := server.Start(); err != nil {
		panic(err)
	}
	defer server.Stop()

	// Register a stub
	id := server.Stub(gmock.StubDefinition{
		Name: "list-users",
		Request: gmock.RequestPattern{
			Method:  http.MethodGet,
			URLPath: "/api/users",
		},
		Response: gmock.ResponseDefinition{
			Status:  http.StatusOK,
			Headers: map[string]string{"Content-Type": "application/json"},
			Body:    `{"users":[{"id":1,"name":"Alice"}]}`,
		},
	})
	fmt.Printf("Registered stub: %s\n", id)

	// Make a request
	resp, err := http.Get(server.URL() + "/api/users")
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("Status: %d\n", resp.StatusCode)
	fmt.Printf("Body:   %s\n", string(body))

	// Verify the request was made
	result := server.Verify(gmock.RequestPattern{
		Method:  "GET",
		URLPath: "/api/users",
	}, 1)
	fmt.Printf("Verified: %v (expected %d, got %d)\n",
		result.Matched, result.ExpectedCount, result.ActualCount)

	// Try an unmatched path — returns 404 with diagnostic info
	resp2, err := http.Get(server.URL() + "/api/unknown")
	if err != nil {
		panic(err)
	}
	defer resp2.Body.Close()
	body2, _ := io.ReadAll(resp2.Body)
	fmt.Printf("\nUnmatched path returned %d: %s\n", resp2.StatusCode, string(body2))
}
