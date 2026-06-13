package gmock_test

import (
	"fmt"
	"net/http"

	"github.com/sunny809/gochaos/pkg/gmock"
)

// ExampleStubDefinition demonstrates creating a stub with multiple matchers.
func ExampleStubDefinition() {
	stub := gmock.StubDefinition{
		Name: "create-user",
		Request: gmock.RequestPattern{
			Method:  http.MethodPost,
			URLPath: "/api/users",
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
		},
		Response: gmock.ResponseDefinition{
			Status:  http.StatusCreated,
			Headers: map[string]string{"Content-Type": "application/json"},
			Body:    `{"id":1,"name":"Alice"}`,
		},
	}

	fmt.Println("Stub name:", stub.Name)
	// Output: Stub name: create-user
}

// ExampleWithCORS demonstrates CORS configuration.
func ExampleWithCORS() {
	server := gmock.NewServer(
		gmock.WithPort(0),
		gmock.WithCORS(gmock.CORSOptions{
			AllowedOrigins: []string{"https://myapp.com"},
			AllowedMethods: []string{"GET", "POST"},
			AllowedHeaders: []string{"Content-Type", "Authorization"},
		}),
	)

	if err := server.Start(); err != nil {
		panic(err)
	}
	defer server.Stop()

	fmt.Println("CORS-enabled server created")
	// Output:
	// CORS-enabled server created
}
