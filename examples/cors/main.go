// CORS example
//
// This example demonstrates how to configure a gmock server with
// Cross-Origin Resource Sharing (CORS) support.
//
// Run:
//   cd examples/cors && go run .
package main

import (
	"fmt"
	"net/http"

	"github.com/sunny809/gochaos/pkg/gmock"
)

func main() {
	// Create a server with CORS enabled
	server := gmock.NewServer(
		gmock.WithPort(0),
		gmock.WithCORS(gmock.CORSOptions{
			AllowedOrigins:   []string{"https://myapp.com", "https://app.example.com"},
			AllowedMethods: []string{"GET", "POST", "PUT", "DELETE"},
			AllowedHeaders: []string{"Content-Type", "Authorization", "X-Request-ID"},
			AllowCredentials: true,
			MaxAge:           3600,
		}),
	)

	if err := server.Start(); err != nil {
		panic(err)
	}
	defer server.Stop()

	// Register a stub
	server.Stub(gmock.StubDefinition{
		Name: "api-data",
		Request: gmock.RequestPattern{
			Method:  http.MethodGet,
			URLPath: "/api/data",
		},
		Response: gmock.ResponseDefinition{
			Status: http.StatusOK,
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			Body: `{"data":"ok"}`,
		},
	})

	fmt.Printf("CORS-enabled server running at %s\n", server.URL())

	// Simulate a preflight OPTIONS request
	req, _ := http.NewRequest(http.MethodOptions, server.URL()+"/api/data", nil)
	req.Header.Set("Origin", "https://myapp.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	fmt.Printf("\nPreflight response:\n")
	fmt.Printf("  Status: %d\n", resp.StatusCode)
	fmt.Printf("  Access-Control-Allow-Origin:  %s\n", resp.Header.Get("Access-Control-Allow-Origin"))
	fmt.Printf("  Access-Control-Allow-Methods: %s\n", resp.Header.Get("Access-Control-Allow-Methods"))
	fmt.Printf("  Access-Control-Allow-Headers: %s\n", resp.Header.Get("Access-Control-Allow-Headers"))
	fmt.Printf("  Access-Control-Max-Age:       %s\n", resp.Header.Get("Access-Control-Max-Age"))

	// Simulate an actual cross-origin GET request
	req2, _ := http.NewRequest(http.MethodGet, server.URL()+"/api/data", nil)
	req2.Header.Set("Origin", "https://myapp.com")

	resp2, err := client.Do(req2)
	if err != nil {
		panic(err)
	}
	defer resp2.Body.Close()

	fmt.Printf("\nActual request response:\n")
	fmt.Printf("  Status: %d\n", resp2.StatusCode)
	fmt.Printf("  Access-Control-Allow-Origin: %s\n", resp2.Header.Get("Access-Control-Allow-Origin"))
	fmt.Printf("  Access-Control-Allow-Credentials: %s\n", resp2.Header.Get("Access-Control-Allow-Credentials"))
}
