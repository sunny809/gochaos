// Verification example
//
// This example demonstrates how to use the gmock verification API
// in tests to assert that expected requests were made.
//
// Run:
//
//	cd examples/verification && go test -v
package verification_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/sunny809/gochaos/pkg/gmock"
)

// userClient is the code under test — it calls a remote API.
type userClient struct {
	baseURL string
}

func (c *userClient) GetUser(id string) (string, error) {
	resp, err := http.Get(c.baseURL + "/api/users/" + id)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// In a real client, you'd unmarshal JSON here
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	buf := make([]byte, 1024)
	n, _ := resp.Body.Read(buf)
	return string(buf[:n]), nil
}

func (c *userClient) DeleteUser(id string) error {
	req, _ := http.NewRequest(http.MethodDelete, c.baseURL+"/api/users/"+id, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	return nil
}

func TestUserClient_GetUser(t *testing.T) {
	server := gmock.NewServer(gmock.WithPort(0))
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	defer server.Stop()

	// Register a stub for GET /api/users/{id}
	server.Stub(gmock.StubDefinition{
		Request: gmock.RequestPattern{
			Method:       http.MethodGet,
			URLPathRegex: `^/api/users/[a-z0-9]+$`,
		},
		Response: gmock.ResponseDefinition{
			Status: http.StatusOK,
			Body:   `{"id":"alice","name":"Alice"}`,
		},
	})

	client := &userClient{baseURL: server.URL()}
	body, err := client.GetUser("alice")
	if err != nil {
		t.Fatal(err)
	}
	if body != `{"id":"alice","name":"Alice"}` {
		t.Errorf("unexpected body: %s", body)
	}

	// Verify the GET request was made exactly once
	result := server.Verify(gmock.RequestPattern{
		Method:  "GET",
		URLPath: "/api/users/alice",
	}, 1)
	if !result.Matched {
		t.Errorf("expected 1 GET /api/users/alice, got %d", result.ActualCount)
	}
}

func TestUserClient_MultipleRequests(t *testing.T) {
	server := gmock.NewServer(gmock.WithPort(0))
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	defer server.Stop()

	// Register a stub that matches any GET /api/users/{id}
	server.Stub(gmock.StubDefinition{
		Request: gmock.RequestPattern{
			Method:       http.MethodGet,
			URLPathRegex: `^/api/users/\w+$`,
		},
		Response: gmock.ResponseDefinition{
			Status: http.StatusOK,
			Body:   `{"id":"user","name":"User"}`,
		},
	})

	client := &userClient{baseURL: server.URL()}

	// Make multiple requests
	client.GetUser("alice")
	client.GetUser("bob")
	client.GetUser("charlie")

	// Verify at least 3 GET requests to /api/users/* were made
	result := server.Verify(gmock.RequestPattern{
		Method:       "GET",
		URLPathRegex: `^/api/users/`,
	}, 3)
	if !result.Matched {
		t.Errorf("expected at least 3 GET /api/users/* requests, got %d", result.ActualCount)
	}
	if result.ActualCount != 3 {
		t.Errorf("expected exactly 3 requests, got %d", result.ActualCount)
	}
}

func TestUserClient_VerifyNotCalled(t *testing.T) {
	server := gmock.NewServer(gmock.WithPort(0))
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	defer server.Stop()

	// Register stubs for GET and DELETE
	server.Stub(gmock.StubDefinition{
		Request:  gmock.RequestPattern{Method: http.MethodGet, URLPath: "/api/users/alice"},
		Response: gmock.ResponseDefinition{Status: http.StatusOK, Body: `{"id":"alice"}`},
	})

	client := &userClient{baseURL: server.URL()}
	client.GetUser("alice")

	// Verify DELETE was NOT called
	result := server.VerifyNotCalled(gmock.RequestPattern{
		Method:  "DELETE",
		URLPath: "/api/users/alice",
	})
	if !result.Matched {
		t.Errorf("expected DELETE /api/users/alice to not be called, but it was")
	}
}

func TestUserClient_RequestLogInspection(t *testing.T) {
	server := gmock.NewServer(gmock.WithPort(0))
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	defer server.Stop()

	server.Stub(gmock.StubDefinition{
		Request:  gmock.RequestPattern{Method: http.MethodGet, URLPath: "/api/users"},
		Response: gmock.ResponseDefinition{Status: http.StatusOK, Body: `[]`},
	})

	// Make a request with a custom header
	req, _ := http.NewRequest(http.MethodGet, server.URL()+"/api/users", nil)
	req.Header.Set("X-Request-ID", "abc-123")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	// Inspect the request log
	log := server.RequestLog()
	if len(log) != 1 {
		t.Fatalf("expected 1 logged request, got %d", len(log))
	}

	entry := log[0]
	if entry.Method != "GET" {
		t.Errorf("expected method GET, got %s", entry.Method)
	}
	if entry.Path != "/api/users" {
		t.Errorf("expected path /api/users, got %s", entry.Path)
	}

	// Verify the request was made
	result := server.Verify(gmock.RequestPattern{
		Method:  "GET",
		URLPath: "/api/users",
	}, 1)
	if !result.Matched {
		t.Error("expected request to be verified")
	}
}
