package gmock_test

import (
	"fmt"
	"net/http"

	"github.com/sunny809/gochaos/pkg/gmock"
)

// ExampleServer demonstrates the basic usage of gmock as an embedded library.
func ExampleServer() {
	// Create and start a server on a random port
	server := gmock.NewServer(gmock.WithPort(0))
	if err := server.Start(); err != nil {
		fmt.Println("failed to start:", err)
		return
	}
	defer server.Stop()

	// Register a stub
	server.Stub(gmock.StubDefinition{
		Request: gmock.RequestPattern{
			Method:  http.MethodGet,
			URLPath: "/api/users",
		},
		Response: gmock.ResponseDefinition{
			Status:  http.StatusOK,
			Headers: map[string]string{"Content-Type": "application/json"},
			Body:    `{"users":["alice"]}`,
		},
	})

	// Make a request to the mock server
	resp, err := http.Get(server.URL() + "/api/users")
	if err != nil {
		fmt.Println("request failed:", err)
		return
	}
	defer resp.Body.Close()

	fmt.Println("Status:", resp.StatusCode)
	fmt.Println("Content-Type:", resp.Header.Get("Content-Type"))

	// Output:
	// Status: 200
	// Content-Type: application/json
}

// ExampleServer_verify demonstrates the verification API.
func ExampleServer_verify() {
	server := gmock.NewServer(gmock.WithPort(0))
	if err := server.Start(); err != nil {
		fmt.Println("failed to start:", err)
		return
	}
	defer server.Stop()

	server.Stub(gmock.StubDefinition{
		Request: gmock.RequestPattern{
			Method:  http.MethodPost,
			URLPath: "/api/events",
		},
		Response: gmock.ResponseDefinition{Status: http.StatusCreated},
	})

	// Send two requests
	http.Post(server.URL()+"/api/events", "application/json", nil)
	http.Post(server.URL()+"/api/events", "application/json", nil)

	// Verify exactly 2 requests were received
	result := server.Verify(gmock.RequestPattern{
		Method:  "POST",
		URLPath: "/api/events",
	}, 2)
	fmt.Println("Verified:", result.Matched)
	fmt.Println("Count:", result.ActualCount)

	// Verify no unmatched method
	result = server.VerifyNotCalled(gmock.RequestPattern{
		Method:  "DELETE",
		URLPath: "/api/events",
	})
	fmt.Println("Not called:", result.Matched)

	// Output:
	// Verified: true
	// Count: 2
	// Not called: true
}

// ExampleWithRedirect demonstrates creating a redirect stub.
func ExampleWithRedirect() {
	server := gmock.NewServer(gmock.WithPort(0))
	if err := server.Start(); err != nil {
		fmt.Println("failed to start:", err)
		return
	}
	defer server.Stop()

	// Register a redirect stub
	server.Stub(gmock.StubDefinition{
		Request: gmock.RequestPattern{
			Method:  http.MethodGet,
			URLPath: "/old-path",
		},
		Response: gmock.WithRedirect(http.StatusMovedPermanently, "/new-path"),
	})

	// Make a request without following redirects
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Get(server.URL() + "/old-path")
	if err != nil {
		fmt.Println("request failed:", err)
		return
	}
	defer resp.Body.Close()

	fmt.Println("Status:", resp.StatusCode)
	fmt.Println("Location:", resp.Header.Get("Location"))

	// Output:
	// Status: 301
	// Location: /new-path
}

// ExampleWithCORSEnabled demonstrates enabling CORS on the server.
func ExampleWithCORSEnabled() {
	server := gmock.NewServer(gmock.WithPort(0), gmock.WithCORSEnabled())
	if err := server.Start(); err != nil {
		fmt.Println("failed to start:", err)
		return
	}
	defer server.Stop()

	server.Stub(gmock.StubDefinition{
		Request: gmock.RequestPattern{
			Method:  http.MethodGet,
			URLPath: "/api/data",
		},
		Response: gmock.ResponseDefinition{
			Status: http.StatusOK,
			Body:   `{"data":"ok"}`,
		},
	})

	// Send a request with Origin header to see CORS headers
	req, _ := http.NewRequest(http.MethodGet, server.URL()+"/api/data", nil)
	req.Header.Set("Origin", "http://example.com")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Println("request failed:", err)
		return
	}
	defer resp.Body.Close()

	fmt.Println("Access-Control-Allow-Origin:", resp.Header.Get("Access-Control-Allow-Origin"))
	fmt.Println("Access-Control-Allow-Credentials:", resp.Header.Get("Access-Control-Allow-Credentials"))

	// Output:
	// Access-Control-Allow-Origin: *
	// Access-Control-Allow-Credentials:
}
