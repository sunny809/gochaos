// Example: write a unit test using gmock as the mock backend.
//
// Pattern: each test creates its own server on a random port, registers
// stubs, exercises the code under test, then verifies expected requests
// happened. The server is torn down at test end via defer.
//
// Run:
//   go test -v ./examples/test
package mocktest

import (
	"io"
	"net/http"
	"testing"

	"github.com/sunny809/gochaos/pkg/gmock"
)

// fakeUserClient is the kind of code you'd be testing — it calls a remote API.
type fakeUserClient struct {
	baseURL string
}

func (c *fakeUserClient) GetUser(id string) (string, error) {
	resp, err := http.Get(c.baseURL + "/api/users/" + id)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return string(body), nil
}

func TestUserClient_GetUser(t *testing.T) {
	server := gmock.NewServer(gmock.WithPort(0))
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	defer server.Stop()

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

	client := &fakeUserClient{baseURL: server.URL()}
	body, err := client.GetUser("alice")
	if err != nil {
		t.Fatal(err)
	}
	if body != `{"id":"alice","name":"Alice"}` {
		t.Errorf("unexpected body: %s", body)
	}

	// Verify the request was made
	result := server.Verify(gmock.RequestPattern{
		Method:  "GET",
		URLPath: "/api/users/alice",
	}, 1)
	if !result.Matched {
		t.Errorf("expected 1 GET /api/users/alice, got %d", result.ActualCount)
	}
}

func TestUserClient_HandlesErrors(t *testing.T) {
	server := gmock.NewServer(gmock.WithPort(0))
	server.Start()
	defer server.Stop()

	// Stub a 500 response
	server.Stub(gmock.StubDefinition{
		Request: gmock.RequestPattern{
			Method:  http.MethodGet,
			URLPath: "/api/users/error",
		},
		Response: gmock.ResponseDefinition{
			Status: http.StatusInternalServerError,
			Body:   `{"error":"server failure"}`,
		},
	})

	client := &fakeUserClient{baseURL: server.URL()}
	body, _ := client.GetUser("error")
	if body != `{"error":"server failure"}` {
		t.Errorf("expected error body, got %s", body)
	}
}
