package gmock_test

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/sunny809/gochaos/pkg/gmock"
)

// ExampleServer demonstrates starting a gmock server in a test.
func ExampleServer() {
	server := gmock.NewServer(gmock.WithPort(0))
	if err := server.Start(); err != nil {
		panic(err)
	}
	defer server.Stop()

	server.Stub(gmock.StubDefinition{
		Request: gmock.RequestPattern{
			Method:  http.MethodGet,
			URLPath: "/api/users",
		},
		Response: gmock.ResponseDefinition{
			Status:  http.StatusOK,
			Headers: map[string]string{"Content-Type": "application/json"},
			Body:    `{"users":[]}`,
		},
	})

	url := server.URL()
	if strings.HasPrefix(url, "http://") {
		fmt.Println("Server is running")
	}
	// Output: Server is running
}
