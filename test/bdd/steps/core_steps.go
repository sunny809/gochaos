package steps

import (
	"fmt"
	"strings"

	"github.com/cucumber/godog"
	"github.com/sunny809/gochaos/pkg/gmock"
)

// registerCoreSteps registers all P1 core API step definitions.
func registerCoreSteps(ctx *godog.ScenarioContext, tc *TestContext) {
	ctx.Step(`^a clean mock server on a random port$`, tc.aCleanMockServerOnARandomPort)
	ctx.Step(`^a stub for (GET|POST|PUT|DELETE|PATCH) ([^\s]+) returns (\d+)$`, tc.aStubForMethodPathReturnsStatus)
	ctx.Step(`^a stub for (GET|POST|PUT|DELETE|PATCH) ([^\s]+) returns (\d+) with body "([^"]*)"$`, tc.aStubForMethodPathReturnsStatusWithBody)
	ctx.Step(`^I send a (GET|POST|PUT|DELETE|PATCH) request to "([^"]*)"$`, tc.iSendARequestTo)
	ctx.Step(`^the response status is (\d+)$`, tc.theResponseStatusIs)
	ctx.Step(`^the response body is "([^"]*)"$`, tc.theResponseBodyIs)
	ctx.Step(`^the stub "([^"]*)" matched (\d+) requests$`, tc.theStubMatchedNRequests)
	ctx.Step(`^I delete the stub$`, tc.iDeleteTheStub)
	ctx.Step(`^I reset the server$`, tc.iResetTheServer)
}

// aCleanMockServerOnARandomPort is the Background step.
// TestContext.BeforeScenario already started a server; this is a guard.
func (tc *TestContext) aCleanMockServerOnARandomPort() error {
	if tc.server == nil {
		return fmt.Errorf("mock server not initialized")
	}
	if tc.baseURL == "" {
		return fmt.Errorf("mock server URL is empty")
	}
	return nil
}

// aStubForMethodPathReturnsStatus registers a stub for the given method + path.
func (tc *TestContext) aStubForMethodPathReturnsStatus(method, path, statusStr string) error {
	status := parseInt(statusStr)
	def := gmock.StubDefinition{
		Request: gmock.RequestPattern{
			Method:  method,
			URLPath: path,
		},
		Response: gmock.ResponseDefinition{
			Status: status,
		},
	}
	id := tc.server.Stub(def)
	if id == "" {
		return fmt.Errorf("failed to register stub for %s %s", method, path)
	}
	tc.AddStubID(id)
	return nil
}

// aStubForMethodPathReturnsStatusWithBody registers a stub with a response body.
func (tc *TestContext) aStubForMethodPathReturnsStatusWithBody(method, path, statusStr, body string) error {
	status := parseInt(statusStr)
	def := gmock.StubDefinition{
		Request: gmock.RequestPattern{
			Method:  method,
			URLPath: path,
		},
		Response: gmock.ResponseDefinition{
			Status: status,
			Body:   body,
		},
	}
	id := tc.server.Stub(def)
	if id == "" {
		return fmt.Errorf("failed to register stub for %s %s", method, path)
	}
	tc.AddStubID(id)
	return nil
}

// iSendARequestTo sends an HTTP request to the given path.
func (tc *TestContext) iSendARequestTo(method, path string) error {
	return tc.SendRequest(method, path)
}

// theResponseStatusIs asserts the last response status code.
func (tc *TestContext) theResponseStatusIs(statusStr string) error {
	expected := parseInt(statusStr)
	if tc.response == nil {
		return fmt.Errorf("no response received")
	}
	if tc.response.StatusCode != expected {
		return fmt.Errorf("expected status %d, got %d", expected, tc.response.StatusCode)
	}
	return nil
}

// theResponseBodyIs asserts the last response body.
func (tc *TestContext) theResponseBodyIs(expected string) error {
	if tc.body == nil {
		return fmt.Errorf("no response body captured")
	}
	if string(tc.body) != expected {
		return fmt.Errorf("expected body %q, got %q", expected, string(tc.body))
	}
	return nil
}

// theStubMatchedNRequests verifies via the Verify API.
func (tc *TestContext) theStubMatchedNRequests(stubName, countStr string) error {
	expectedCount := parseInt(countStr)
	pattern := gmock.RequestPattern{
		URLPath: stubName,
	}
	result := tc.server.Verify(pattern, expectedCount)
	if !result.Matched {
		return fmt.Errorf("verification failed: %s", strings.Join(result.Errors, "; "))
	}
	return nil
}

// iDeleteTheStub removes the last registered stub.
func (tc *TestContext) iDeleteTheStub() error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if len(tc.stubIDs) == 0 {
		return fmt.Errorf("no stub registered to delete")
	}
	lastID := tc.stubIDs[len(tc.stubIDs)-1]
	tc.stubIDs = tc.stubIDs[:len(tc.stubIDs)-1]

	if !tc.server.DeleteStub(lastID) {
		return fmt.Errorf("failed to delete stub %q", lastID)
	}
	return nil
}

// iResetTheServer clears all stubs and request logs.
func (tc *TestContext) iResetTheServer() error {
	tc.server.Reset()
	tc.mu.Lock()
	tc.stubIDs = nil
	tc.mu.Unlock()
	return nil
}

// parseInt converts a string to int for step argument parsing.
func parseInt(s string) int {
	var n int
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}
