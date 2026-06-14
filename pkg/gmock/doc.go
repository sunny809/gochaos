// Package gmock provides a Go-native HTTP mock server with built-in chaos engineering.
//
// Use gmock as an embeddable library in your Go tests, or run it as a standalone
// CLI for CI/CD integration testing, resilience testing, and API mocking across
// any programming language.
//
// Basic usage:
//
//	server := gmock.NewServer(gmock.WithPort(0))
//	server.Start()
//	defer server.Stop()
//
//	server.Stub(gmock.StubDefinition{
//	    Request: gmock.RequestPattern{
//	        Method:  "GET",
//	        URLPath: "/api/users",
//	    },
//	    Response: gmock.ResponseDefinition{
//	        Status: 200,
//	        Body:   `{"users":[]}`,
//	    },
//	})
//
//	resp, _ := http.Get(server.URL() + "/api/users")
package gmock
